"""Standard-library release packaging and untrusted-input verification."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import struct
import tarfile

ROOT = Path(__file__).resolve().parents[2]
ARCHES = ("amd64", "arm64")

def check_version(version):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("expected stable vX.Y.Z")

def check_elf(data, arch):
    if len(data) < 64 or data[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", data, 18)[0] != {"amd64": 62, "arm64": 183}[arch]:
        raise ValueError("binary must be a Linux 64-bit ELF for " + arch)

def digest(data):
    return hashlib.sha256(data).hexdigest()

def write_sums(directory, names, filename="SHA256SUMS"):
    (directory / filename).write_text("".join(f"{digest((directory / name).read_bytes())}  {name}\n" for name in names))

def verify_sums(directory, filename, expected):
    found = set()
    for line in (directory / filename).read_text().splitlines():
        match = re.fullmatch(r"([a-f0-9]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in found or match[2] not in expected:
            raise ValueError("unexpected, duplicate or unsafe checksum entry")
        found.add(match[2])
        path = directory / match[2]
        if path.is_symlink() or not path.is_file() or digest(path.read_bytes()) != match[1]:
            raise ValueError("artifact checksum mismatch")
    if found != set(expected):
        raise ValueError("missing checksum entries")

def release_compose(source):
    # Fail on layout drift rather than silently shipping a source-dependent file.
    block = "  build:\n    context: .\n    dockerfile: Dockerfile\n    args:\n      VERSION: ${OPTIMUS_VERSION:-dev}\n"
    if source.count(block) != 1:
        raise ValueError("canonical Compose build block changed")
    source = source.replace(block, "")
    for key in ("POSTGRES_PASSWORD", "OPTIMUS_DATABASE_DSN", "OPTIMUS_JWT_SECRET", "OPTIMUS_VAULT_MASTER_KEY"):
        source, count = re.subn(r"\$\{" + key + r":-[^}]*\}", "${" + key + ":?Set " + key + " in .env}", source)
        if count < 1:
            raise ValueError("missing required Compose setting " + key)
    source = source.replace("${COMPOSE_PROJECT_NAME:-optimus-dev}", "${COMPOSE_PROJECT_NAME:-optimus-prod}")
    source = source.replace("${IMAGE_REPOSITORY:-local}", "${IMAGE_REPOSITORY:-ghcr.io/arkgravity}")
    source = source.replace("${OPTIMUS_VERSION:-dev}", "${OPTIMUS_VERSION:?Set OPTIMUS_VERSION in .env}")
    source = source.replace("${OPTIMUS_LOG_LEVEL:-debug}", "${OPTIMUS_LOG_LEVEL:-info}")
    source = source.replace("${OPTIMUS_CORS_ALLOWED_ORIGINS:-http://localhost:5173}", "${OPTIMUS_CORS_ALLOWED_ORIGINS:-}")
    return source

def environment(version):
    # Retain the canonical documented knobs; strip Dev comments with their values.
    values = {"COMPOSE_PROJECT_NAME": "optimus-prod", "IMAGE_REPOSITORY": "ghcr.io/arkgravity", "OPTIMUS_VERSION": version, "HTTP_BIND": "127.0.0.1", "POSTGRES_PASSWORD": "", "OPTIMUS_DATABASE_DSN": "", "OPTIMUS_JWT_SECRET": "", "OPTIMUS_VAULT_MASTER_KEY": "", "OPTIMUS_LOG_LEVEL": "info", "OPTIMUS_CORS_ALLOWED_ORIGINS": ""}
    lines = ["# Generate unique secrets and DSN as described in INSTALL.md before startup."]
    for line in (ROOT / ".env.example").read_text().splitlines():
        if re.match(r"^[A-Z][A-Z0-9_]*=", line):
            key, value = line.split("=", 1)
            lines.append(key + "=" + values.get(key, value))
    return "\n".join(lines) + "\n"

def install(version):
    text = (ROOT / "docs/releases/INSTALL.md").read_text().replace("@VERSION@", version)
    if re.search(r"@[A-Z_]+@", text):
        raise ValueError("unresolved install placeholder")
    return text.encode()

def archive(path, top, files, epoch):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("wb") as output, gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=epoch) as compressed, tarfile.open(fileobj=compressed, mode="w") as tar:
        for name, (data, mode) in sorted(files.items()):
            member = tarfile.TarInfo(top + "/" + name)
            member.size, member.mode, member.mtime = len(data), mode, epoch
            tar.addfile(member, io.BytesIO(data))

def read_archive(path, expected):
    files = {}
    with tarfile.open(path, "r:gz") as tar:
        for entry in tar:
            name = PurePosixPath(entry.name)
            if not entry.isfile() or name.is_absolute() or ".." in name.parts or entry.name in files:
                raise ValueError("unsafe archive member")
            files[entry.name] = (tar.extractfile(entry).read(), entry.mode)
    if set(files) != set(expected):
        raise ValueError("unexpected or missing archive files")
    return files

def native(version, arch, binary, output, epoch):
    check_version(version)
    data = binary.read_bytes()
    check_elf(data, arch)
    if not binary.stat().st_mode & 0o111:
        raise ValueError("binary is not executable")
    top = f"optimus_{version}_linux_{arch}"
    config = (ROOT / "configs/config.yaml").read_bytes()
    archive(output / (top + ".tar.gz"), top, {"optimus": (data, 0o755), "configs/config.yaml": (config, 0o600), "LICENSE": ((ROOT / "LICENSE").read_bytes(), 0o644), "INSTALL.md": (install(version), 0o644)}, epoch)

def deployment(version, output, epoch):
    check_version(version)
    top = f"optimus_{version}_deploy"
    archive(output / (top + ".tar.gz"), top, {"docker-compose.yml": (release_compose((ROOT / "docker-compose.yml").read_text()).encode(), 0o644), ".env.example": (environment(version).encode(), 0o600), "LICENSE": ((ROOT / "LICENSE").read_bytes(), 0o644), "INSTALL.md": (install(version), 0o644)}, epoch)

def image_config(path, arch, version, commit):
    with tarfile.open(path) as tar:
        # Only read explicit regular members. Never extract a Docker archive.
        manifest_member = tar.getmember("manifest.json")
        if not manifest_member.isfile():
            raise ValueError("invalid Docker manifest")
        manifest = json.load(tar.extractfile(manifest_member))
        if not isinstance(manifest, list) or len(manifest) != 1:
            raise ValueError("require exactly one saved image")
        name = manifest[0]["Config"]
        if not isinstance(name, str) or PurePosixPath(name).is_absolute() or ".." in PurePosixPath(name).parts:
            raise ValueError("unsafe config path")
        member = tar.getmember(name)
        if not member.isfile():
            raise ValueError("invalid image config member")
        raw = tar.extractfile(member).read()
        config = json.loads(raw)
    labels = config.get("config", {}).get("Labels", {})
    if config.get("os") != "linux" or config.get("architecture") != arch or any(labels.get(k) != v for k, v in {"org.opencontainers.image.version": version, "org.opencontainers.image.revision": commit, "org.opencontainers.image.licenses": "MIT", "org.opencontainers.image.source": "https://github.com/ArkGravity/optimus"}.items()):
        raise ValueError("saved image platform or OCI labels do not match release")
    return "sha256:" + digest(raw)

def assemble(version, inputs, output, epoch):
    check_version(version)
    output.mkdir(parents=True, exist_ok=True)
    for arch in ARCHES:
        directory = inputs / arch
        filename = f"optimus_{version}_linux_{arch}.tar.gz"
        expected = {filename, f"image-{arch}.tar"}
        if {p.name for p in directory.iterdir()} != expected | {"checksums.txt"}:
            raise ValueError("unexpected architecture input")
        verify_sums(directory, "checksums.txt", expected)
        top = filename.removesuffix(".tar.gz")
        files = read_archive(directory / filename, {top + "/" + n for n in ("optimus", "configs/config.yaml", "LICENSE", "INSTALL.md")})
        check_elf(files[top + "/optimus"][0], arch)
        if files[top + "/optimus"][1] & 0o111 == 0 or files[top + "/INSTALL.md"][0] != install(version) or files[top + "/LICENSE"][0] != (ROOT / "LICENSE").read_bytes():
            raise ValueError("invalid native archive metadata")
        (output / filename).write_bytes((directory / filename).read_bytes())
    deployment(version, output, epoch)
    (output / "INSTALL.md").write_bytes(install(version))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("native", "deploy", "assemble", "sums", "verify", "arch-sums"))
    parser.add_argument("--version", required=True)
    parser.add_argument("--arch", choices=ARCHES)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--input", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--epoch", type=int, default=0)
    args = parser.parse_args()
    check_version(args.version)
    args.output.mkdir(parents=True, exist_ok=True)
    if args.command == "native":
        native(args.version, args.arch, args.binary, args.output, args.epoch)
    elif args.command == "deploy":
        deployment(args.version, args.output, args.epoch)
    elif args.command == "assemble":
        assemble(args.version, args.input, args.output, args.epoch)
    elif args.command == "arch-sums":
        write_sums(args.output, [f"optimus_{args.version}_linux_{args.arch}.tar.gz", f"image-{args.arch}.tar"], "checksums.txt")
    else:
        names = [f"optimus_{args.version}_linux_{arch}.tar.gz" for arch in ARCHES] + [f"optimus_{args.version}_deploy.tar.gz", "INSTALL.md", "IMAGES.txt"]
        if args.command == "sums":
            write_sums(args.output, names)
        else:
            verify_sums(args.output, "SHA256SUMS", set(names))

if __name__ == "__main__":
    main()
