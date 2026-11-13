"""Docker-only smoke of the extracted deployment and native release archives.

All logs and credentials stay in memory; only a short success/failure is printed.
Unique Compose projects own every container/network/volume removed by teardown.
"""
import argparse
import base64
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import sys
import tempfile
import time
import urllib.error
import urllib.request

from package import check_elf, read_archive
from process import run

@contextmanager
def termination_signals():
    """Defer termination throughout every owned-resource cleanup phase."""
    signals = (signal.SIGTERM, signal.SIGHUP, signal.SIGINT)
    previous = {number: signal.getsignal(number) for number in signals}
    pending = None
    protecting = 0

    def interrupt():
        # Once termination is pending, later signals cannot interrupt outer
        # cleanup phases before all private files and Compose resources are gone.
        for number in signals:
            signal.signal(number, signal.SIG_IGN)
        raise SystemExit(128 + pending)

    def terminate(number, _frame):
        nonlocal pending
        if pending is None:
            pending = number
        if not protecting:
            interrupt()

    @contextmanager
    def protect_cleanup():
        nonlocal protecting
        protecting += 1
        try:
            yield
        finally:
            protecting -= 1
            if not protecting and pending is not None:
                interrupt()

    try:
        for number in signals:
            signal.signal(number, terminate)
        yield protect_cleanup
    finally:
        for number, handler in previous.items():
            signal.signal(number, handler)


def compose_environment(template, generated):
    """Keep Docker connectivity while isolating Compose from application state."""
    template_keys = {line.split("=", 1)[0] for line in template.splitlines() if re.match(r"^[A-Z][A-Z0-9_]*=", line)}
    forbidden = template_keys | {"HTTP_BIND", "HTTP_PORT", "IMAGE_REPOSITORY", "OPTIMUS_VERSION"}
    environment = {key: value for key, value in os.environ.items() if key not in forbidden and not key.startswith(("OPTIMUS_", "POSTGRES_", "COMPOSE_"))}
    # Every application/Compose value comes exclusively from the extracted
    # template and this smoke's random secrets, ports and unique project.
    environment.update(generated)
    return environment


def port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]

def request(base, path, token=None, body=None, status=200):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(base + path, data=json.dumps(body).encode() if body is not None else None, headers=headers)
    try:
        response = urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    data = response.read()
    if response.status != status:
        raise RuntimeError("unexpected HTTP status for " + path)
    return data

def envelope(data):
    value = json.loads(data)
    if value.get("code") != 0:
        raise RuntimeError("unexpected API business failure")
    return value["data"]

def http_smoke(base, version, password):
    for _ in range(90):
        try:
            health = json.loads(request(base, "/api/v1/health"))
            if health.get("version") == version and health.get("db") == "ok":
                break
        except (OSError, ValueError, RuntimeError):
            pass
        time.sleep(1)
    else:
        raise RuntimeError("health/version did not become ready")
    shell = request(base, "/").decode()
    if "<html" not in shell.lower() or request(base, "/assets/cloud-accounts") != shell.encode():
        raise RuntimeError("embedded SPA fallback failed")
    asset = re.search(r'(?:src|href)="([^" ]*assets/[^" ]+\.js)"', shell)
    if not asset or not request(base, "/" + asset[1].lstrip("/")):
        raise RuntimeError("embedded hashed JS missing")
    request(base, "/assets/release-missing.js", status=404)
    missing = json.loads(request(base, "/api/v1/release-missing", status=404))
    if missing.get("code") != 40401:
        raise RuntimeError("unknown API must return JSON 404")
    request(base, "/api/v1/me", status=401)
    auth = envelope(request(base, "/api/v1/auth/login", body={"username": "admin", "password": password}))
    token = auth["access_token"]
    for path in ["/me", "/credentials/cloud-keys"] + ["/assets/" + name for name in ("cloud-accounts", "vpcs", "firewalls", "instances", "databases", "sync-runs")]:
        envelope(request(base, "/api/v1" + path, token))
    # Invalid local input exercises an authenticated mutating failure with no
    # cloud account, credentials or provider calls possible.
    failed = json.loads(request(base, "/api/v1/assets/cloud-accounts", token, body={}, status=400))
    if not failed.get("code"):
        raise RuntimeError("invalid mutation unexpectedly succeeded")

def extract(path, destination, expected):
    files = read_archive(path, expected)
    for name, (data, mode) in files.items():
        target = destination / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        target.chmod(mode)

def smoke(image, version, arch, artifacts, workspace, protect_cleanup):
    if run("docker", "run", "--rm", image, "version") != version:
        raise RuntimeError("image version does not match tag")
    deploy_top = f"optimus_{version}_deploy"
    native_top = f"optimus_{version}_linux_{arch}"
    extract(artifacts / (deploy_top + ".tar.gz"), workspace, {deploy_top + "/" + n for n in ("docker-compose.yml", ".env.example", "INSTALL.md", "LICENSE")})
    extract(artifacts / (native_top + ".tar.gz"), workspace, {native_top + "/" + n for n in ("optimus", "configs/config.yaml", "INSTALL.md", "LICENSE")})
    native = workspace / native_top
    check_elf((native / "optimus").read_bytes(), arch)
    for mode in ("image", "native"):
        project = "optimus-release-" + secrets.token_hex(6)
        directory = workspace / deploy_top
        http_port = port()
        template = (directory / ".env.example").read_text()
        env = template
        values = {"COMPOSE_PROJECT_NAME": project, "OPTIMUS_VERSION": version, "HTTP_PORT": str(http_port), "POSTGRES_PORT": str(port()), "POSTGRES_PASSWORD": secrets.token_hex(24), "OPTIMUS_JWT_SECRET": secrets.token_hex(32), "OPTIMUS_VAULT_MASTER_KEY": base64.b64encode(secrets.token_bytes(32)).decode(), "OPTIMUS_ASSETS_SYNC_STARTUP_DELAY": "24h"}
        values["OPTIMUS_DATABASE_DSN"] = "host=postgres port=5432 user=optimus password=" + values["POSTGRES_PASSWORD"] + " dbname=optimus sslmode=disable"
        for key, value in values.items():
            env = re.sub(r"(?m)^" + key + r"=.*$", lambda _: key + "=" + value, env)
        generated = dict(line.split("=", 1) for line in env.splitlines() if re.match(r"^[A-Z][A-Z0-9_]*=", line))
        command_environment = compose_environment(template, generated)
        (directory / ".env").write_text(env)
        (directory / ".env").chmod(0o600)
        overrides = {"services": {}}
        for service in ("migrate", "seed", "optimus"):
            item = {"image": image, "pull_policy": "never"}
            if mode == "native":
                item.update({"entrypoint": ["/release/optimus"], "volumes": [str(native) + ":/release:ro"], "command": [{"migrate": "migrate", "seed": "seed", "optimus": "server"}[service], "-config", "/release/configs/config.yaml"] + (["-dir", "up"] if service == "migrate" else [])})
            overrides["services"][service] = item
        override = directory / "smoke.override.json"
        override.write_text(json.dumps(overrides))
        prefix = ["docker", "compose", "--project-name", project, "--env-file", str(directory / ".env"), "-f", str(directory / "docker-compose.yml"), "-f", str(override)]
        def compose(*args):
            return run(*prefix, *args, cwd=directory, env=command_environment)
        def sql(query):
            return compose("exec", "-T", "postgres", "psql", "-U", "optimus", "-d", "optimus", "-At", "-c", query)
        try:
            # Config is evaluated from extracted files, without any checkout build.
            config = json.loads(compose("config", "--format", "json"))
            if any("build" in service for service in config["services"].values()):
                raise RuntimeError("deployment archive depends on source")
            if mode == "native":
                if compose("run", "--rm", "--no-deps", "seed", "version") != version:
                    raise RuntimeError("unpacked native binary version mismatch")
            compose("up", "-d", "--no-build", "--pull", "never")
            # Wait through DB initialization/migration/seed while retaining all logs privately.
            password = None
            for _ in range(90):
                logs = compose("logs", "--no-log-prefix", "seed")
                for line in logs.splitlines():
                    try:
                        row = json.loads(line)
                    except ValueError:
                        continue
                    if row.get("password"):
                        if password is not None and password != row["password"]:
                            raise RuntimeError("multiple generated admin passwords")
                        password = row["password"]
                if password:
                    break
                time.sleep(1)
            if not password:
                raise RuntimeError("seed did not print one structured initial password")
            http_smoke(f"http://127.0.0.1:{http_port}", version, password)
            if sql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'") != "32" or sql("SELECT max(version_id) FROM goose_db_version WHERE is_applied") != "20":
                raise RuntimeError("fresh baseline must contain 31 business tables and Goose head20")
            snapshot_query = "SELECT (SELECT count(*) FROM users WHERE username='admin'),(SELECT count(*) FROM menus),(SELECT count(DISTINCT code) FROM menus)"
            snapshot = sql(snapshot_query)
            if not snapshot.startswith("1|") or snapshot.split("|")[1] != snapshot.split("|")[2]:
                raise RuntimeError("duplicate admin/menu seed")
            compose("run", "--rm", "--no-deps", "migrate")
            repeated = compose("run", "--rm", "--no-deps", "seed")
            if any('"password"' in line for line in repeated.splitlines()) or sql(snapshot_query) != snapshot:
                raise RuntimeError("repeated seed changed admin/menu identity")
        finally:
            with protect_cleanup():
                compose("down", "--volumes", "--remove-orphans", "--timeout", "10")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--arch", choices=("amd64", "arm64"), required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()
    root = Path("tmp/release")
    root.mkdir(parents=True, exist_ok=True)
    try:
        with termination_signals() as protect_cleanup:
            directory = tempfile.TemporaryDirectory(prefix="smoke-", dir=root.resolve())
            try:
                smoke(args.image, args.version, args.arch, args.artifacts.resolve(), Path(directory.name), protect_cleanup)
            finally:
                # The first termination signal may arrive on an ordinary,
                # successful exit while Python is removing .env/private files.
                with protect_cleanup():
                    directory.cleanup()
        print("Release smoke passed: image + extracted native + source-free Compose (linux/" + args.arch + ")")
    except (RuntimeError, ValueError, KeyError, OSError) as error:
        print("Release smoke failed: " + str(error), file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
