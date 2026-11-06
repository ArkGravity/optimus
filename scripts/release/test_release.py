"""Behavioral release guard and artifact tests (no network or registry writes)."""
import importlib.util
import io
import json
import os
import signal
from pathlib import Path
import struct
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest
from unittest.mock import patch

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE.resolve()))

def module(name):
    spec = importlib.util.spec_from_file_location(name, HERE / (name + ".py"))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

class PackageTests(unittest.TestCase):
    def test_linux_architecture_required(self):
        package = module("package")
        for arch, machine in (("amd64", 62), ("arm64", 183)):
            binary = bytearray(64)
            binary[:6] = b"\x7fELF\x02\x01"
            struct.pack_into("<H", binary, 18, machine)
            package.check_elf(bytes(binary), arch)
            with self.assertRaises(ValueError):
                package.check_elf(bytes(binary), "arm64" if arch == "amd64" else "amd64")
        with self.assertRaises(ValueError):
            package.check_elf(b"Mach-O", "arm64")

    def test_compose_no_source_and_required_secrets(self):
        package = module("package")
        compose = package.release_compose((HERE.parents[1] / "docker-compose.yml").read_text())
        self.assertNotIn("  build:", compose)
        self.assertNotIn("dockerfile:", compose)
        self.assertIn("service_completed_successfully", compose)
        self.assertIn("postgres:17-alpine", compose)
        for key in ("POSTGRES_PASSWORD", "OPTIMUS_DATABASE_DSN", "OPTIMUS_JWT_SECRET", "OPTIMUS_VAULT_MASTER_KEY"):
            self.assertIn("${" + key + ":?", compose)
        self.assertNotIn("dev-only", compose)
        self.assertNotIn("MDEyMz", compose)

    def test_checksum_tampering_and_path_traversal_rejected(self):
        package = module("package")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "payload").write_text("first")
            package.write_sums(root, ["payload"], "checksums")
            package.verify_sums(root, "checksums", {"payload"})
            (root / "payload").write_text("changed")
            with self.assertRaises(ValueError):
                package.verify_sums(root, "checksums", {"payload"})
            (root / "checksums").write_text("0" * 64 + "  ../escape\n")
            with self.assertRaises(ValueError):
                package.verify_sums(root, "checksums", {"payload"})

class AssemblyTests(unittest.TestCase):
    def make_inputs(self, root):
        package = module("package")
        inputs = root / "inputs"
        for arch, machine in (("amd64", 62), ("arm64", 183)):
            target = inputs / arch
            target.mkdir(parents=True)
            binary = root / ("binary-" + arch)
            data = bytearray(64)
            data[:6] = b"\x7fELF\x02\x01"
            struct.pack_into("<H", data, 18, machine)
            binary.write_bytes(data)
            binary.chmod(0o700)
            package.native("v1.2.3", arch, binary, target, 12345)
            (target / f"image-{arch}.tar").write_bytes(b"owned saved-image fixture")
            package.write_sums(target, [f"optimus_v1.2.3_linux_{arch}.tar.gz", f"image-{arch}.tar"], "checksums.txt")
        return inputs

    def test_native_deploy_roundtrip_and_reproducible_assembly(self):
        package = module("package")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = self.make_inputs(root)
            first, second = root / "first", root / "second"
            package.assemble("v1.2.3", inputs, first, 12345)
            package.assemble("v1.2.3", inputs, second, 12345)
            self.assertEqual({p.name for p in first.iterdir()}, {"optimus_v1.2.3_linux_amd64.tar.gz", "optimus_v1.2.3_linux_arm64.tar.gz", "optimus_v1.2.3_deploy.tar.gz", "INSTALL.md"})
            for path in first.iterdir():
                self.assertEqual(path.read_bytes(), (second / path.name).read_bytes())
            for arch in package.ARCHES:
                top = f"optimus_v1.2.3_linux_{arch}"
                files = package.read_archive(first / (top + ".tar.gz"), {top + "/" + name for name in ("optimus", "configs/config.yaml", "LICENSE", "INSTALL.md")})
                self.assertEqual(files[top + "/optimus"][0], (root / ("binary-" + arch)).read_bytes())
                self.assertEqual(files[top + "/optimus"][1], 0o755)
                self.assertNotIn(b"@VERSION@", files[top + "/INSTALL.md"][0])
            top = "optimus_v1.2.3_deploy"
            files = package.read_archive(first / (top + ".tar.gz"), {top + "/" + name for name in ("docker-compose.yml", ".env.example", "LICENSE", "INSTALL.md")})
            compose = files[top + "/docker-compose.yml"][0].decode()
            env = files[top + "/.env.example"][0].decode()
            self.assertNotIn("build:", compose)
            self.assertIn("COMPOSE_PROJECT_NAME=optimus-prod", env)
            self.assertIn("HTTP_BIND=127.0.0.1", env)
            self.assertIn("OPTIMUS_VERSION=v1.2.3", env)
            for key in ("POSTGRES_PASSWORD", "OPTIMUS_DATABASE_DSN", "OPTIMUS_JWT_SECRET", "OPTIMUS_VAULT_MASTER_KEY"):
                self.assertIn("\n" + key + "=\n", env)
            self.assertNotIn("dev-only", env)
            self.assertNotIn("MDEyMz", env)

    def test_corrupt_missing_unexpected_and_wrong_arch_inputs_fail(self):
        package = module("package")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = self.make_inputs(root)
            arm = inputs / "arm64"
            payload = arm / "optimus_v1.2.3_linux_arm64.tar.gz"
            original = payload.read_bytes()
            payload.write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                package.assemble("v1.2.3", inputs, root / "corrupt", 12345)
            payload.write_bytes(original)
            extra = arm / "unexpected"
            extra.write_text("do not package")
            with self.assertRaises(ValueError):
                package.assemble("v1.2.3", inputs, root / "unexpected", 12345)
            extra.unlink()
            payload.unlink()
            with self.assertRaises(ValueError):
                package.assemble("v1.2.3", inputs, root / "missing", 12345)
            # Even an internally consistent checksum cannot excuse wrong ELF arch.
            top = "optimus_v1.2.3_linux_arm64"
            original_files = package.read_archive(inputs / "amd64/optimus_v1.2.3_linux_amd64.tar.gz", {"optimus_v1.2.3_linux_amd64/" + name for name in ("optimus", "configs/config.yaml", "LICENSE", "INSTALL.md")})
            package.archive(payload, top, {name.split("/", 1)[1]: value for name, value in original_files.items()}, 12345)
            package.write_sums(arm, [payload.name, "image-arm64.tar"], "checksums.txt")
            with self.assertRaises(ValueError):
                package.assemble("v1.2.3", inputs, root / "wrongarch", 12345)

    def test_tar_links_duplicate_and_traversal_members_fail(self):
        package = module("package")
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "unsafe.tar.gz"
            for names, link in ((["../escape"], False), (["/absolute"], False), (["allowed", "allowed"], False), (["allowed"], True)):
                with tarfile.open(archive, "w:gz") as tar:
                    for name in names:
                        entry = tarfile.TarInfo(name)
                        if link:
                            entry.type, entry.linkname = tarfile.SYMTYPE, "../escape"
                        tar.addfile(entry, None)
                with self.assertRaises(ValueError):
                    package.read_archive(archive, {"allowed"})


class SmokeIsolationTests(unittest.TestCase):
    def test_polluted_parent_never_reaches_compose(self):
        smoke = module("smoke")
        polluted = {"OPTIMUS_DATABASE_DSN": "host=production.example dbname=real", "POSTGRES_PASSWORD": "real-password", "POSTGRES_USER": "real-user", "OPTIMUS_TEST_POSTGRES_DSN": "real-test-dsn", "COMPOSE_FILE": "/private/real-compose.yml", "COMPOSE_PROJECT_NAME": "real", "HTTP_PORT": "80", "IMAGE_REPOSITORY": "real/registry", "TEMPLATE_KNOB": "parent", "DOCKER_HOST": "unix:///example/colima.sock"}
        generated = {"OPTIMUS_DATABASE_DSN": "host=postgres dbname=disposable", "POSTGRES_PASSWORD": "random-test-password", "COMPOSE_PROJECT_NAME": "optimus-release-owned", "HTTP_PORT": "49123"}
        with patch.dict(os.environ, polluted):
            environment = smoke.compose_environment("TEMPLATE_KNOB=default\nOPTIMUS_DATABASE_DSN=\n", generated)
            output = smoke.run(sys.executable, "-c", "import json, os; print(json.dumps(dict(os.environ)))", env=environment)
        child = json.loads(output)
        for key, value in generated.items():
            self.assertEqual(child[key], value)
        for key in ("OPTIMUS_TEST_POSTGRES_DSN", "COMPOSE_FILE", "POSTGRES_USER", "IMAGE_REPOSITORY", "TEMPLATE_KNOB"):
            self.assertNotIn(key, child)
        self.assertEqual(child["DOCKER_HOST"], "unix:///example/colima.sock")
        self.assertNotIn("production.example", output)
        self.assertNotIn("real-password", output)

    def test_subprocess_timeout_is_bounded_without_raw_args(self):
        smoke = module("smoke")
        with self.assertRaisesRegex(RuntimeError, "timed out"):
            smoke.run(sys.executable, "-c", "import time; time.sleep(10)", timeout=0.01)

    def test_subprocess_timeout_also_stops_owned_descendants(self):
        smoke = module("smoke")
        with tempfile.TemporaryDirectory() as directory:
            pid_file = Path(directory) / "child-pid"
            child = "import os, sys, time; from pathlib import Path; Path(sys.argv[1]).write_text(str(os.getpid())); time.sleep(30)"
            parent = "import subprocess, sys, time; subprocess.Popen([sys.executable, '-B', '-c', sys.argv[1], sys.argv[2]]); time.sleep(30)"
            try:
                with self.assertRaisesRegex(RuntimeError, "timed out"):
                    smoke.run(sys.executable, "-B", "-c", parent, child, str(pid_file), timeout=0.5)
                pid = int(pid_file.read_text())
                status = subprocess.run(["ps", "-p", str(pid), "-o", "stat="], capture_output=True, text=True)
                # An orphaned zombie can await the host's reaper, but no command
                # or descendant may remain running after the timeout returns.
                self.assertTrue(not status.stdout.strip() or status.stdout.strip().startswith("Z"), status.stdout)
            finally:
                if pid_file.exists():
                    try:
                        os.kill(int(pid_file.read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass


class SignalCleanupTests(unittest.TestCase):
    def wait_file(self, path, process):
        deadline = time.monotonic() + 10
        while not path.exists():
            if process.poll() is not None or time.monotonic() >= deadline:
                output, error = process.communicate(timeout=5)
                self.fail("fake Docker did not reach checkpoint: " + output + error)
            time.sleep(0.01)

    def check_signals(self, cases, process_group=False):
        package = module("package")
        for wrapper, region in cases:
            for termination in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
                with self.subTest(wrapper=wrapper, region=region, termination=termination), tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    (root / "scripts").symlink_to(HERE.resolve().parent, target_is_directory=True)
                    tools, artifacts = root / "tools", root / "artifacts"
                    tools.mkdir()
                    artifacts.mkdir()
                    binary = bytearray(64)
                    binary[:6] = b"\x7fELF\x02\x01"
                    struct.pack_into("<H", binary, 18, 62)
                    (root / "binary").write_bytes(binary)
                    (root / "binary").chmod(0o700)
                    package.native("v1.2.3", "amd64", root / "binary", artifacts, 12345)
                    package.deployment("v1.2.3", artifacts, 12345)
                    fake = r'''#!/usr/bin/env python3
import json, os, signal, sys, time
from pathlib import Path
# Like a CLI with its own signal runtime, do not rely on inherited SIG_IGN.
for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
    signal.signal(number, signal.SIG_DFL)
args = sys.argv[1:]
root = Path(os.environ["FAKE_ROOT"])
with (root / "commands.jsonl").open("a") as output:
    output.write(json.dumps(args) + "\n")
if args[0] == "create":
    (root / "extraction-name").write_text(args[args.index("--name") + 1])
    print("owned-extraction-container")
elif args[:2] == ["container", "ls"]:
    if not (root / "extraction-cleaned").exists(): print("a" * 64)
elif args[:2] == ["container", "inspect"]:
    print(json.dumps({"io.optimus.release.extraction": (root / "extraction-name").read_text()}))
elif args[0] == "cp": Path(args[-1]).write_bytes((root / "binary").read_bytes())
elif args[0] == "run": print("v1.2.3")
elif args[0] == "rm":
    if os.environ["SIGNAL_REGION"] == "extraction-cleanup":
        (root / "started").touch()
        time.sleep(0.5)
    (root / "extraction-cleaned").touch()
elif args[0] == "save": Path(args[args.index("--output") + 1]).write_bytes(b"saved image fixture")
elif args[0] == "compose":
    if "config" in args: print(json.dumps({"services":{"optimus":{"image":"fake"}}}))
    elif "up" in args:
        if os.environ["SIGNAL_REGION"] == "compose-start": (root / "started").touch()
    elif "logs" in args:
        if os.environ["SIGNAL_REGION"] == "compose-start": time.sleep(10)
        else: print(json.dumps({"password":"synthetic-test-password"}))
    elif "exec" in args:
        query = args[-1]
        if "information_schema" in query: print("32")
        elif "max(version_id)" in query: print("19")
        else: print("1|35|35")
    elif "run" in args:
        if args[-1] == "version": print("v1.2.3")
        else: print("already initialized")
    elif "down" in args:
        if os.environ["SIGNAL_REGION"] == "normal-down": (root / "started").touch()
        (root / "cleanup-started").touch()
        time.sleep(0.5)
        (root / "cleanup-complete").touch()
    else: sys.exit(2)
else: sys.exit(2)
'''
                    (tools / "docker").write_text(fake)
                    (tools / "docker").chmod(0o755)
                    (tools / "git").write_text("#!/usr/bin/env python3\nprint('12345')\n")
                    (tools / "git").chmod(0o755)
                    launcher = "#!" + sys.executable + " -B\n" + r'''import os, signal, sys, time
from contextlib import contextmanager
from pathlib import Path
args = sys.argv[1:]
script = next((i for i,a in enumerate(args) if a.endswith("/smoke.py")), None)
if script is None:
    os.execv(sys.executable, [sys.executable, "-B"] + args)
sys.path.insert(0, str(Path(args[script]).resolve().parent))
import smoke
smoke.port = lambda: 49123
original_signals = smoke.termination_signals
@contextmanager
def observed_signals():
    with original_signals() as protect:
        for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
            handler = signal.getsignal(number)
            def observed(number, frame, handler=handler):
                try:
                    handler(number, frame)
                finally:
                    (Path(os.environ["FAKE_ROOT"]) / "signal-accepted").touch()
            signal.signal(number, observed)
        yield protect
smoke.termination_signals = observed_signals
if os.environ["SIGNAL_REGION"] == "readiness":
    def pending_health(*args, **kwargs):
        (Path(os.environ["FAKE_ROOT"]) / "started").touch()
        time.sleep(10)
        raise RuntimeError("not ready yet")
    smoke.request = pending_health
else:
    smoke.http_smoke = lambda *args: None
if os.environ["SIGNAL_REGION"] == "private-cleanup":
    original_cleanup = smoke.tempfile.TemporaryDirectory.cleanup
    def pending_cleanup(directory):
        root = Path(os.environ["FAKE_ROOT"])
        (root / "started").touch()
        time.sleep(0.5)
        original_cleanup(directory)
        (root / "private-cleanup-complete").touch()
    smoke.tempfile.TemporaryDirectory.cleanup = pending_cleanup
sys.argv = args[script:]
smoke.main()
'''
                    (tools / "python3").write_text(launcher)
                    (tools / "python3").chmod(0o755)
                    environment = {**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"], "FAKE_ROOT": str(root), "SIGNAL_REGION": region}
                    if wrapper == "smoke":
                        command = [str(tools / "python3"), "-B", str(HERE.resolve() / "smoke.py"), "--image", "fake:release", "--version", "v1.2.3", "--arch", "amd64", "--artifacts", str(artifacts)]
                    else:
                        command = ["bash", str(HERE.resolve() / "prepare.sh"), "fake:release", "v1.2.3", "amd64", str(artifacts)]
                    process = subprocess.Popen(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=process_group)
                    send_signal = (lambda number: os.killpg(process.pid, number)) if process_group else process.send_signal
                    try:
                        self.wait_file(root / "started", process)
                        send_signal(termination)
                        if region != "extraction-cleanup":
                            self.wait_file(root / "signal-accepted", process)
                        self.wait_file(root / "cleanup-started", process)
                        # Repeated signals must not interrupt bounded teardown.
                        for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
                            send_signal(number)
                        output, error = process.communicate(timeout=10)
                        if region == "extraction-cleanup":
                            # prepare's EXIT cleanup already ignores termination.
                            self.assertEqual(process.returncode, 0, error)
                        else:
                            self.assertEqual(process.returncode, 128 + termination, error)
                            self.assertNotIn("Release smoke passed", output)
                        self.assertTrue((root / "cleanup-complete").exists(), error)
                        self.assertEqual(list((root / "tmp/release").iterdir()), [])
                        calls = [json.loads(line) for line in (root / "commands.jsonl").read_text().splitlines()]
                        down = [args for args in calls if args[0] == "compose" and "down" in args]
                        self.assertEqual(len(down), 2 if region in ("private-cleanup", "extraction-cleanup") else 1)
                        self.assertIn("--volumes", down[0])
                        self.assertTrue(down[0][down[0].index("--project-name") + 1].startswith("optimus-release-"))
                        if region == "private-cleanup":
                            self.assertTrue((root / "private-cleanup-complete").exists())
                        if wrapper == "prepare":
                            self.assertTrue((root / "extraction-cleaned").exists())
                            self.assertFalse((artifacts / "optimus").exists())
                    finally:
                        if process.poll() is None:
                            if process_group:
                                os.killpg(process.pid, signal.SIGKILL)
                            else:
                                process.kill()
                            process.communicate(timeout=5)

    def test_term_and_hup_unwind_compose_and_prepare_private_resources(self):
        self.check_signals((("smoke", "readiness"), ("prepare", "compose-start"), ("smoke", "normal-down"), ("smoke", "private-cleanup")))

    def test_process_group_signals_preserve_all_cleanup_phases(self):
        self.check_signals((("smoke", "readiness"), ("smoke", "normal-down"), ("smoke", "private-cleanup"), ("prepare", "compose-start"), ("prepare", "normal-down"), ("prepare", "private-cleanup"), ("prepare", "extraction-cleanup")), process_group=True)


class CreationCleanupTests(unittest.TestCase):
    wait_file = SignalCleanupTests.wait_file

    def check_creation(self, termination=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").symlink_to(HERE.resolve().parent, target_is_directory=True)
            tools = root / "tools"
            tools.mkdir()
            unrelated = root / "unrelated-container"
            unrelated.write_text("must remain untouched")
            fake = "#!" + sys.executable + " -B\n" + r'''import json, os, signal, sys, time
from pathlib import Path
for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
    signal.signal(number, signal.SIG_DFL)
args = sys.argv[1:]
root = Path(os.environ["FAKE_ROOT"])
with (root / "commands.jsonl").open("a") as output:
    output.write(json.dumps(args) + "\n")
if args[0] == "create":
    name = args[args.index("--name") + 1] if "--name" in args else "daemon-assigned-extraction"
    (root / "created-name").write_text(name)
    (root / "create-pid").write_text(str(os.getpid()))
    (root / name).touch()
    (root / "started").touch()
    if os.environ["CREATE_RESULT"] == "failure": sys.exit(7)
    time.sleep(0.5)
    # A delayed create must settle before removal, not recreate after cleanup.
    (root / name).touch()
    (root / "create-settled").touch()
    print("returned-container-id")
elif args[:2] == ["container", "ls"]:
    if (root / (root / "created-name").read_text()).exists(): print("a" * 64)
elif args[:2] == ["container", "inspect"]:
    print(json.dumps({"io.optimus.release.extraction": (root / "created-name").read_text()}))
elif args[0] == "rm":
    try:
        os.kill(int((root / "create-pid").read_text()), 0)
    except ProcessLookupError:
        pass
    else:
        (root / "cleanup-raced-create").touch()
        sys.exit(8)
    name = (root / "created-name").read_text()
    (root / name).unlink()
    (root / "removed-name").write_text(name)
else: sys.exit(2)
'''
            (tools / "docker").write_text(fake)
            (tools / "docker").chmod(0o755)
            environment = {**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"], "FAKE_ROOT": str(root), "CREATE_RESULT": "failure" if termination is None else "delayed"}
            command = ["bash", str(HERE.resolve() / "prepare.sh"), "fake:release", "v1.2.3", "amd64", str(root / "artifacts")]
            process = subprocess.Popen(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
            try:
                self.wait_file(root / "started", process)
                if termination is not None:
                    os.killpg(process.pid, termination)
                output, error = process.communicate(timeout=10)
                name = (root / "created-name").read_text()
                self.assertFalse((root / name).exists(), output + error)
                self.assertFalse((root / "cleanup-raced-create").exists())
                self.assertEqual((root / "removed-name").read_text(), name)
                self.assertRegex(name, r"^optimus-release-extract-[0-9a-f]{24,}$")
                self.assertEqual(unrelated.read_text(), "must remain untouched")
                calls = [json.loads(line) for line in (root / "commands.jsonl").read_text().splitlines()]
                mutations = [args for args in calls if args[0] in ("create", "rm")]
                self.assertEqual(mutations, [["create", "--name", name, "--label", "io.optimus.release.extraction=" + name, "fake:release"], ["rm", "-f", "a" * 64]])
                if termination is not None:
                    self.assertEqual(process.returncode, 128 + termination, error)
                    self.assertTrue((root / "create-settled").exists())
                else:
                    self.assertEqual(process.returncode, 7, error)
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate(timeout=5)

    def test_group_signals_after_create_before_id_return_remove_owned_name(self):
        for termination in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
            with self.subTest(termination=termination):
                self.check_creation(termination)

    def test_create_failure_after_recorded_creation_removes_owned_name(self):
        self.check_creation()


class DetachedCreationTests(unittest.TestCase):
    wait_file = SignalCleanupTests.wait_file

    def check_detached_creation(self, result, termination=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").symlink_to(HERE.resolve().parent, target_is_directory=True)
            tools = root / "tools"
            tools.mkdir()
            (root / "server.py").write_text(r'''import json, sys, time
from pathlib import Path
root, name = Path(sys.argv[1]), sys.argv[2]
deadline = time.monotonic() + 5
while not (root / "initial-absence").exists():
    if time.monotonic() > deadline: sys.exit(2)
    time.sleep(0.01)
if sys.argv[3] == "never":
    (root / "server-done").touch()
    sys.exit(0)
time.sleep(0.1)
(root / "container").write_text(json.dumps({"id": "a" * 64, "labels": {"io.optimus.release.extraction": name}}))
(root / "server-done").touch()
''')
            fake = "#!" + sys.executable + " -B\n" + r'''import json, os, signal, subprocess, sys, time
from pathlib import Path
for number in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT): signal.signal(number, signal.SIG_DFL)
root = Path(os.environ["FAKE_ROOT"])
args = sys.argv[1:]
with (root / "commands.jsonl").open("a") as output: output.write(json.dumps(args) + "\n")
if args[0] == "create":
    name = args[args.index("--name") + 1]
    (root / "name").write_text(name)
    server = subprocess.Popen([sys.executable, "-B", str(root / "server.py"), str(root), name, os.environ["CREATE_RESULT"]], start_new_session=True, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (root / "server-pid").write_text(str(server.pid))
    if os.environ["CREATE_RESULT"] == "timeout": time.sleep(30)
    sys.exit(7)
elif args[:2] == ["container", "ls"]:
    if (root / "container").exists(): print("a" * 64)
    else: (root / "initial-absence").touch()
elif args[:2] == ["container", "inspect"]:
    print(json.dumps(json.loads((root / "container").read_text())["labels"]))
elif args[0] == "rm":
    if not (root / "container").exists():
        (root / "initial-absence").touch()
        sys.exit(1)
    (root / "rm-started").touch()
    time.sleep(0.2)
    (root / "container").unlink()
else: sys.exit(2)
'''
            (tools / "docker").write_text(fake)
            (tools / "docker").chmod(0o755)
            launcher = "#!" + sys.executable + " -B\n" + r'''import functools, os, runpy, subprocess, sys
from pathlib import Path
args = sys.argv[1:]
script = next((i for i, arg in enumerate(args) if arg.endswith(("/process.py", "/extraction.py"))), None)
if script is None: os.execv(sys.executable, [sys.executable, "-B"] + args)
sys.path.insert(0, str(Path(args[script]).resolve().parent))
sys.argv = args[script:]
if args[script].endswith("/extraction.py"):
    import extraction
    extraction.reconcile = functools.partial(extraction.reconcile, timeout=1, command_timeout=0.3, poll_interval=0.02)
    extraction.main()
else:
    original = subprocess.Popen.communicate
    def bounded(process, input=None, timeout=None):
        return original(process, input=input, timeout=0.3 if timeout == 180 else timeout)
    subprocess.Popen.communicate = bounded
    runpy.run_path(args[script], run_name="__main__")
'''
            (tools / "python3").write_text(launcher)
            (tools / "python3").chmod(0o755)
            environment = {**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"], "FAKE_ROOT": str(root), "CREATE_RESULT": result}
            process = subprocess.Popen(["bash", str(HERE.resolve() / "prepare.sh"), "fake:release", "v1.2.3", "amd64", str(root / "artifacts")], cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
            try:
                if termination is not None:
                    self.wait_file(root / "rm-started", process)
                    for number in (termination, signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
                        os.killpg(process.pid, number)
                output, error = process.communicate(timeout=10)
                deadline = time.monotonic() + 5
                while not (root / "server-done").exists() and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue((root / "server-done").exists(), output + error)
                self.assertFalse((root / "container").exists(), "detached daemon created after initial cleanup: " + output + error)
                self.assertEqual(process.returncode, 1 if result == "timeout" else 7)
                calls = [json.loads(line) for line in (root / "commands.jsonl").read_text().splitlines()]
                removals = [args for args in calls if args[0] == "rm"]
                if result == "never":
                    self.assertEqual(removals, [])
                    self.assertIn("Uncertain extraction cleanup", error)
                    self.assertIn((root / "name").read_text(), error)
                else:
                    self.assertEqual(removals, [["rm", "-f", "a" * 64]])
                    self.assertNotIn("uncertain", error.lower())
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate(timeout=5)
                if (root / "server-pid").exists():
                    try:
                        os.kill(int((root / "server-pid").read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass

    def test_detached_daemon_creation_after_timeout_or_transport_failure_is_reconciled(self):
        for result in ("timeout", "transport"):
            with self.subTest(result=result):
                self.check_detached_creation(result)

    def test_reconciliation_children_survive_repeated_process_group_signals(self):
        for termination in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
            with self.subTest(termination=termination):
                self.check_detached_creation("transport", termination)


    def test_create_never_appearing_preserves_failure_and_diagnoses_uncertainty(self):
        self.check_detached_creation("never")


class ReconciliationTests(unittest.TestCase):
    name = "optimus-release-extract-" + "b" * 24

    def reconcile(self, extraction, known_created=False):
        now = [0.0]
        def advance(delay):
            now[0] += delay
        extraction.reconcile(self.name, known_created, timeout=1, command_timeout=0.2,
                             poll_interval=0.25, clock=lambda: now[0], sleep=advance)

    def test_absence_or_transport_failure_cannot_prove_uncertain_creation_clean(self):
        extraction = module("extraction")
        for failure in (False, True):
            calls = []
            def command(*args, **kwargs):
                calls.append(args)
                if failure:
                    raise RuntimeError("synthetic transport failure")
                return ""
            with self.subTest(failure=failure), patch.object(extraction, "run", command):
                with self.assertRaisesRegex(RuntimeError, "Uncertain extraction cleanup"):
                    self.reconcile(extraction)
            self.assertEqual(len(calls), 4)
            self.assertTrue(all(args[1:3] == ("container", "ls") for args in calls))

    def test_label_mismatch_refuses_removal_of_name_collision(self):
        extraction = module("extraction")
        calls = []
        def command(*args, **kwargs):
            calls.append(args)
            return "a" * 64 if args[2] == "ls" else json.dumps({extraction.OWNER_LABEL: "another-owner"})
        with patch.object(extraction, "run", command):
            with self.assertRaisesRegex(ValueError, "ownership mismatch"):
                self.reconcile(extraction)
        self.assertFalse(any(args[1] == "rm" for args in calls))

    def test_known_creation_or_successful_owned_removal_needs_no_grace_wait(self):
        extraction = module("extraction")
        for known_created in (False, True):
            calls = []
            def command(*args, **kwargs):
                calls.append(args)
                if args[1] == "rm":
                    self.assertEqual(args, ("docker", "rm", "-f", "a" * 64))
                    return "a" * 64
                if args[2] == "inspect":
                    return json.dumps({extraction.OWNER_LABEL: self.name})
                return "a" * 64 if len(calls) == 1 and not known_created else ""
            with self.subTest(known_created=known_created), patch.object(extraction, "run", command):
                extraction.reconcile(self.name, known_created, sleep=lambda _: self.fail("unnecessary grace wait"))
            self.assertEqual(len(calls), 1 if known_created else 4)

    def test_failed_removal_then_absence_still_does_not_prove_uncertain_request_settled(self):
        extraction = module("extraction")
        calls = []
        def command(*args, **kwargs):
            calls.append(args)
            if args[1] == "rm":
                raise RuntimeError("lost removal response")
            if args[2] == "inspect":
                return json.dumps({extraction.OWNER_LABEL: self.name})
            return "a" * 64 if len(calls) == 1 else ""
        with patch.object(extraction, "run", command):
            with self.assertRaisesRegex(RuntimeError, "Uncertain extraction cleanup"):
                self.reconcile(extraction)

    def test_timed_out_reconciliation_commands_stop_their_owned_descendants(self):
        extraction = module("extraction")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "child-pids"
            fake = "#!" + sys.executable + " -B\n" + "import subprocess, sys, time\nfrom pathlib import Path\nchild = subprocess.Popen([sys.executable, '-B', '-c', 'import time; time.sleep(30)'])\nwith Path(" + repr(str(log)) + ").open('a') as output: output.write(str(child.pid) + '\\n')\ntime.sleep(30)\n"
            (root / "docker").write_text(fake)
            (root / "docker").chmod(0o755)
            try:
                with patch.dict(os.environ, {"PATH": str(root) + os.pathsep + os.environ["PATH"]}):
                    with self.assertRaisesRegex(RuntimeError, "Uncertain extraction cleanup"):
                        extraction.reconcile(self.name, False, timeout=0.6, command_timeout=0.2, poll_interval=0.01)
                pids = log.read_text().splitlines()
                self.assertTrue(pids)
                for pid in pids:
                    status = subprocess.run(["ps", "-p", pid, "-o", "stat="], capture_output=True, text=True)
                    self.assertTrue(not status.stdout.strip() or status.stdout.strip().startswith("Z"), status.stdout)
            finally:
                if log.exists():
                    for pid in log.read_text().splitlines():
                        try:
                            os.kill(int(pid), signal.SIGKILL)
                        except ProcessLookupError:
                            pass


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.git("init", "-b", "main")
        self.git("config", "user.email", "release@example.invalid")
        self.git("config", "user.name", "Release test")
        (self.repo / "docs/releases").mkdir(parents=True)
        (self.repo / "docs/releases/v1.2.3.md").write_text("Release notes")
        self.git("add", ".")
        self.git("commit", "-m", "fixture")
        self.sha = self.git("rev-parse", "HEAD")
        self.git("tag", "-a", "v1.2.3", "-m", "stable")
        self.git("remote", "add", "origin", str(self.repo))
        self.fake = self.root / "bin"
        self.fake.mkdir()
        (self.fake / "gh").write_text("#!/usr/bin/env python3\nimport json, os, sys\na=sys.argv\nif os.environ.get('FAIL_API'): sys.exit(1)\nif '/releases?' in a[-1]: print(os.environ.get('RELEASES', '[[]]'))\nelif '/jobs?' in a[-1]: print(json.dumps([{'jobs': json.loads(os.environ['JOBS'])}]))\nelse: print(json.dumps([{'workflow_runs': json.loads(os.environ['RUNS'])}]))\n")
        (self.fake / "gh").chmod(0o755)
        names = ["Web checks", "Backend quality", "Backend unit tests", "Backend database tests", "Docker build", "Publish image to GHCR and Docker Hub"]
        self.env = {**os.environ, "PATH": str(self.fake) + os.pathsep + os.environ["PATH"], "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v1.2.3", "GITHUB_SHA": self.sha, "GITHUB_REPOSITORY": "example/optimus", "RUNS": json.dumps([{"id": 1, "head_sha": self.sha, "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success"}]), "JOBS": json.dumps([{"name": n, "status": "completed", "conclusion": "success"} for n in names])}

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.repo), *args], stderr=subprocess.DEVNULL, text=True).strip()

    def validate(self, **env):
        return subprocess.run(["python3", "-B", str(HERE.resolve() / "validate.py")], cwd=self.repo, env={**self.env, **env}, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def test_success_and_api_fail_closed(self):
        self.assertEqual(self.validate().returncode, 0)
        self.assertNotEqual(self.validate(FAIL_API="1").returncode, 0)

    def test_missing_queued_failed_malformed_and_nonmain_ci_fail(self):
        for change in (lambda jobs: jobs[:-1], lambda jobs: [{**j, "status": "queued"} for j in jobs], lambda jobs: [{**j, "conclusion": "failure"} for j in jobs]):
            self.assertNotEqual(self.validate(JOBS=json.dumps(change(json.loads(self.env["JOBS"])))).returncode, 0)
        for runs in ("null", "{}", "[]", json.dumps([{**json.loads(self.env["RUNS"])[0], "head_branch": "dev"}])):
            self.assertNotEqual(self.validate(RUNS=runs).returncode, 0)
        self.assertNotEqual(self.validate(GITHUB_REF_TYPE="branch").returncode, 0)
        self.assertNotEqual(self.validate(RELEASES='[[{}]]').returncode, 0)

    def test_published_rejected_draft_retry_allowed(self):
        self.assertNotEqual(self.validate(RELEASES='[[{"tag_name":"v1.2.3","draft":false}]]').returncode, 0)
        self.assertEqual(self.validate(RELEASES='[[{"tag_name":"v1.2.3","draft":true}]]').returncode, 0)

    def test_unmerged_tag_rejected(self):
        self.git("checkout", "-b", "unmerged")
        (self.repo / "another").write_text("new code")
        self.git("add", ".")
        self.git("commit", "-m", "not main")
        self.git("tag", "-d", "v1.2.3")
        self.git("tag", "-a", "v1.2.3", "-m", "unmerged")
        self.assertNotEqual(self.validate(GITHUB_SHA=self.git("rev-parse", "HEAD")).returncode, 0)

    def test_skipped_job_wrong_sha_and_lightweight_tag_fail(self):
        jobs = json.loads(self.env["JOBS"])
        jobs[-1]["conclusion"] = "skipped"
        self.assertNotEqual(self.validate(JOBS=json.dumps(jobs)).returncode, 0)
        self.assertNotEqual(self.validate(GITHUB_SHA="0" * 40).returncode, 0)
        self.git("tag", "-d", "v1.2.3")
        self.git("tag", "v1.2.3")
        self.assertNotEqual(self.validate().returncode, 0)

class DraftTests(unittest.TestCase):
    git = ValidationTests.git

    def setUp(self):
        ValidationTests.setUp(self)
        self.artifacts = self.root / "artifacts"
        self.artifacts.mkdir()
        self.names = [f"optimus_v1.2.3_linux_{arch}.tar.gz" for arch in ("amd64", "arm64")] + ["optimus_v1.2.3_deploy.tar.gz", "INSTALL.md", "IMAGES.txt"]
        for name in self.names:
            (self.artifacts / name).write_text("owned fixture " + name)
        module("package").write_sums(self.artifacts, self.names)
        self.state = self.root / "releases.json"
        self.state.write_text("[]")
        self.commands = self.root / "gh-commands.jsonl"
        self.env.update({"FAKE_RELEASE_STATE": str(self.state), "FAKE_GH_COMMANDS": str(self.commands)})
        fake = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
state = Path(os.environ["FAKE_RELEASE_STATE"])
with open(os.environ["FAKE_GH_COMMANDS"], "a") as output:
    output.write(json.dumps(args) + "\n")
if args[0] == "api":
    if "/releases?" in args[-1]: print(json.dumps([json.loads(state.read_text())]))
    elif "/jobs?" in args[-1]: print(json.dumps([{"jobs":json.loads(os.environ["JOBS"])}]))
    else: print(json.dumps([{"workflow_runs":json.loads(os.environ["RUNS"])}]))
elif args[:2] == ["release", "create"]:
    state.write_text(json.dumps([{"tag_name":args[2],"draft":True,"assets":[]}]))
elif args[:2] == ["release", "upload"]:
    releases = json.loads(state.read_text())
    names = list(dict.fromkeys([asset["name"] for asset in releases[0]["assets"]] + [Path(name).name for name in args[4:]]))
    if os.environ.get("UPLOAD_MISSING"): names.pop()
    if os.environ.get("UPLOAD_EXTRA"): names.append("unexpected.tar.gz")
    if os.environ.get("UPLOAD_DUPLICATE"): names.append(names[0])
    releases[0]["assets"] = [{"name": name} for name in names]
    state.write_text(json.dumps(releases))
elif args[:2] == ["release", "edit"]: pass
else: sys.exit(2)
'''
        (self.fake / "gh").write_text(fake)

    def draft(self):
        return subprocess.run([sys.executable, "-B", str(HERE.resolve() / "draft.py"), "--version", "v1.2.3", "--artifacts", str(self.artifacts)], cwd=self.repo, env=self.env, capture_output=True, text=True)

    def mutations(self):
        return [json.loads(line) for line in self.commands.read_text().splitlines() if json.loads(line)[0] == "release"]

    def test_published_release_and_tampered_checksums_block_every_mutation(self):
        self.state.write_text('[{"tag_name":"v1.2.3","draft":false}]')
        self.assertNotEqual(self.draft().returncode, 0)
        self.assertEqual(self.mutations(), [])
        self.state.write_text("[]")
        (self.artifacts / "INSTALL.md").write_text("tampered")
        self.assertNotEqual(self.draft().returncode, 0)
        self.assertEqual(self.mutations(), [])

    def test_only_draft_creation_and_six_fixed_attachments(self):
        result = self.draft()
        self.assertEqual(result.returncode, 0, result.stderr)
        mutations = self.mutations()
        self.assertEqual([args[1] for args in mutations], ["create", "upload"])
        self.assertIn("--verify-tag", mutations[0])
        self.assertIn("--draft", mutations[0])
        upload = mutations[1]
        self.assertEqual([Path(name).name for name in upload[4:]], self.names + ["SHA256SUMS"])
        self.assertEqual(upload[3], "--clobber")
        self.commands.write_text("")
        self.assertEqual(self.draft().returncode, 0)
        self.assertEqual([args[1] for args in self.mutations()], ["edit", "upload"])
        self.assertTrue(json.loads(self.state.read_text())[0]["draft"])

    def test_stale_draft_assets_block_every_mutation(self):
        release = {"tag_name": "v1.2.3", "draft": True, "assets": [{"name": "obsolete.tar.gz"}]}
        self.state.write_text(json.dumps([release]))
        result = self.draft()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.mutations(), [])
        self.assertEqual(json.loads(self.state.read_text()), [release])

    def test_partial_unpublished_draft_retry_allowed(self):
        self.state.write_text(json.dumps([{"tag_name": "v1.2.3", "draft": True, "assets": [{"name": self.names[0]}]}]))
        result = self.draft()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([args[1] for args in self.mutations()], ["edit", "upload"])
        self.assertEqual({asset["name"] for asset in json.loads(self.state.read_text())[0]["assets"]}, set(self.names + ["SHA256SUMS"]))

    def test_malformed_or_duplicate_existing_assets_block_every_mutation(self):
        for assets in (None, [{"name": 123}], [{"name": "INSTALL.md"}] * 2):
            with self.subTest(assets=assets):
                self.commands.write_text("")
                self.state.write_text(json.dumps([{"tag_name": "v1.2.3", "draft": True, "assets": assets}]))
                self.assertNotEqual(self.draft().returncode, 0)
                self.assertEqual(self.mutations(), [])

    def test_post_upload_missing_or_extra_assets_fail_closed(self):
        failures = ("UPLOAD_MISSING", "UPLOAD_EXTRA", "UPLOAD_DUPLICATE")
        for failure in failures:
            with self.subTest(failure=failure):
                self.state.write_text("[]")
                self.commands.write_text("")
                for key in failures:
                    self.env.pop(key, None)
                self.env[failure] = "1"
                result = self.draft()
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual([args[1] for args in self.mutations()], ["create", "upload"])
                self.assertTrue(json.loads(self.state.read_text())[0]["draft"])


class PublisherTests(unittest.TestCase):
    def test_wrong_image_architecture_prevents_all_registry_writes(self):
        package, publisher = module("package"), module("publish")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for arch in package.ARCHES:
                target = root / arch
                target.mkdir()
                config = {"os": "linux", "architecture": "amd64", "config": {"Labels": {"org.opencontainers.image.version": "v1.2.3", "org.opencontainers.image.revision": "a" * 40, "org.opencontainers.image.licenses": "MIT", "org.opencontainers.image.source": "https://github.com/ArkGravity/optimus"}}}
                raw = json.dumps(config).encode()
                with tarfile.open(target / f"image-{arch}.tar", "w") as tar:
                    for name, value in (("manifest.json", json.dumps([{"Config": "config.json"}]).encode()), ("config.json", raw)):
                        entry = tarfile.TarInfo(name)
                        entry.size = len(value)
                        tar.addfile(entry, io.BytesIO(value))
                (target / f"optimus_v1.2.3_linux_{arch}.tar.gz").write_bytes(b"fixture")
                package.write_sums(target, [f"image-{arch}.tar", f"optimus_v1.2.3_linux_{arch}.tar.gz"], "checksums.txt")
            commands = []
            original = publisher.command
            def record(*args, **kwargs):
                commands.append(args)
                return "sha256:" + "b" * 64
            publisher.command = record
            with self.assertRaises(ValueError):
                publisher.publish("v1.2.3", "a" * 40, root, root / "output", ["example/first", "example/second"])
            self.assertFalse(any(args[1] in ("push", "ls", "buildx") for args in commands))
            publisher.command = original

    def test_fake_registry_publish_exact_saved_images_and_retry_without_writes(self):
        package = module("package")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs, tools = root / "inputs", root / "tools"
            tools.mkdir()
            configs = {}
            for arch in package.ARCHES:
                target = inputs / arch
                target.mkdir(parents=True)
                config = {"os": "linux", "architecture": arch, "config": {"Labels": {"org.opencontainers.image.version": "v1.2.3", "org.opencontainers.image.revision": "a" * 40, "org.opencontainers.image.licenses": "MIT", "org.opencontainers.image.source": "https://github.com/ArkGravity/optimus"}}}
                raw = json.dumps(config).encode()
                configs[arch] = "sha256:" + package.digest(raw)
                with tarfile.open(target / f"image-{arch}.tar", "w") as tar:
                    for name, value in (("manifest.json", json.dumps([{"Config": "config.json"}]).encode()), ("config.json", raw)):
                        entry = tarfile.TarInfo(name)
                        entry.size = len(value)
                        tar.addfile(entry, io.BytesIO(value))
                (target / f"optimus_v1.2.3_linux_{arch}.tar.gz").write_bytes(b"fixture")
                package.write_sums(target, [f"image-{arch}.tar", f"optimus_v1.2.3_linux_{arch}.tar.gz"], "checksums.txt")
            fake = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ["COMMAND_LOG"], "a") as output:
    output.write(json.dumps([Path(sys.argv[0]).name] + args) + "\n")
if Path(sys.argv[0]).name == "docker": sys.exit(0)
digests = {"amd64": "sha256:" + "a" * 64, "arm64": "sha256:" + "b" * 64}
arch = "arm64" if "arm64" in args[-1] or digests["arm64"] in args[-1] else "amd64"
if args[0] == "ls": print("v1.2.3" if os.environ.get("EXISTING") else "")
elif args[0] == "push": pass
elif args[0] == "digest": print(digests[arch] if "--tarball" in args or "@" in args[-1] else "sha256:" + "c" * 64)
elif args[0] == "config": print(json.dumps({"os":"linux", "architecture":arch}))
elif args[0] == "manifest":
    if "@" in args[-1]: print(json.dumps({"config":{"digest":json.loads(os.environ["CONFIGS"])[arch]}}))
    else:
        rows = [{"platform":{"os":"linux","architecture":a}, "digest":d} for a,d in digests.items()]
        if os.environ.get("BAD_INDEX"): rows.append(rows[0])
        print(json.dumps({"manifests":rows}))
else: sys.exit(2)
'''
            for name in ("crane", "docker"):
                (tools / name).write_text(fake)
                (tools / name).chmod(0o755)
            log = root / "commands.jsonl"
            environment = {**os.environ, "PATH": str(tools) + os.pathsep + os.environ["PATH"], "COMMAND_LOG": str(log), "CONFIGS": json.dumps(configs)}
            command = ["python3", "-B", str(HERE.resolve() / "publish.py"), "--version", "v1.2.3", "--commit", "a" * 40, "--input", str(inputs), "--output", str(root / "output"), "--ghcr", "example/first", "--dockerhub", "example/second"]
            result = subprocess.run(command, env=environment, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            pushes = [call for call in calls if call[:2] == ["crane", "push"]]
            self.assertEqual(len(pushes), 4)
            self.assertTrue(all("@sha256:" in call[-1] for call in pushes))
            self.assertEqual(sum(call[0] == "docker" for call in calls), 2)
            self.assertEqual(len((root / "output/IMAGES.txt").read_text().splitlines()), 6)
            log.write_text("")
            result = subprocess.run(command, env={**environment, "EXISTING": "1"}, capture_output=True)
            self.assertEqual(result.returncode, 0)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertFalse(any(call[0] == "docker" or call[:2] == ["crane", "push"] for call in calls))
            log.write_text("")
            result = subprocess.run(command, env={**environment, "EXISTING": "1", "BAD_INDEX": "1"}, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertFalse(any(call[0] == "docker" or call[:2] == ["crane", "push"] for call in calls))

    def test_remote_indexes_reject_extra_platform_and_different_payload(self):
        publisher = module("publish")
        expected = {"amd64": "sha256:" + "a" * 64, "arm64": "sha256:" + "b" * 64}
        rows = [{"digest": value, "platform": {"os": "linux", "architecture": arch}} for arch, value in expected.items()]
        for manifests in (rows + [rows[0]], [{**rows[0], "digest": "wrong"}, rows[1]], [{**rows[0], "platform": {"os": "windows", "architecture": "amd64"}}, rows[1]]):
            publisher.command = lambda *a, **k: json.dumps({"manifests": manifests})
            with self.assertRaises(ValueError):
                publisher.remote_index("repo:v1.2.3", expected)

if __name__ == "__main__":
    unittest.main()
