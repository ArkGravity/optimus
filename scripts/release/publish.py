"""Publish saved, previously smoke-tested images without rebuilding."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from package import ARCHES, check_version, image_config, verify_sums

def command(*args, env=None):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE, env=env, timeout=300).strip()

def remote_index(reference, expected, env=None):
    index = json.loads(command("crane", "manifest", reference, env=env))
    manifests = index.get("manifests")
    if not isinstance(manifests, list) or len(manifests) != 2:
        raise ValueError("release index must contain exactly two runnable manifests")
    actual = {}
    for item in manifests:
        platform = item.get("platform", {})
        arch = platform.get("architecture")
        if platform.get("os") != "linux" or arch not in ARCHES or arch in actual or platform.get("variant", "") not in ("", "v8" if arch == "arm64" else ""):
            raise ValueError("unexpected release index platform")
        actual[arch] = item.get("digest")
    if actual != expected:
        raise ValueError("release index does not match tested image digests")
    return command("crane", "digest", reference, env=env)

def publish(version, commit, inputs, output, repositories):
    check_version(version)
    # Validate every local input before the first registry write.
    digests, configs = {}, {}
    for arch in ARCHES:
        directory = inputs / arch
        verify_sums(directory, "checksums.txt", {f"image-{arch}.tar", f"optimus_{version}_linux_{arch}.tar.gz"})
        archive = directory / f"image-{arch}.tar"
        configs[arch] = image_config(archive, arch, version, commit)
        digests[arch] = command("crane", "digest", "--tarball", str(archive))
        if not __import__("re").fullmatch(r"sha256:[a-f0-9]{64}", digests[arch]):
            raise ValueError("invalid saved-image digest")
    existing = {}
    for repository in repositories:
        # Listing must succeed: authorization, rate limits and transport failures
        # are not interpreted as permission to replace a version.
        tags = command("crane", "ls", repository).splitlines()
        existing[repository] = version in tags
        if existing[repository]:
            remote_index(repository + ":" + version, digests)
    for repository in repositories:
        if existing[repository]:
            continue
        for arch in ARCHES:
            reference = repository + "@" + digests[arch]
            command("crane", "push", str(inputs / arch / f"image-{arch}.tar"), reference)
            raw = command("crane", "config", reference).encode()
            # crane config returns exact config JSON bytes (its CLI adds newline).
            # Use the manifest's content-addressed config, independent of formatting.
            manifest = json.loads(command("crane", "manifest", reference))
            if manifest.get("config", {}).get("digest") != configs[arch] or command("crane", "digest", reference) != digests[arch]:
                raise ValueError("remote platform/config digest differs from tested image")
            config = json.loads(raw)
            if config.get("os") != "linux" or config.get("architecture") != arch:
                raise ValueError("remote platform config mismatch")
        command("docker", "buildx", "imagetools", "create", "--tag", repository + ":" + version, *(repository + "@" + digests[a] for a in ARCHES))
    output.mkdir(parents=True, exist_ok=True)
    # Verify anonymously so IMAGES.txt only names publicly retrievable artifacts.
    with tempfile.TemporaryDirectory(prefix="anonymous-docker-", dir=output.parent) as directory:
        environment = {**os.environ, "DOCKER_CONFIG": directory}
        indexes = [remote_index(repository + ":" + version, digests, environment) for repository in repositories]
        if len(set(indexes)) != 1:
            raise ValueError("registries returned different release index digests")
        for repository in repositories:
            for arch in ARCHES:
                reference = repository + "@" + digests[arch]
                manifest = json.loads(command("crane", "manifest", reference, env=environment))
                if command("crane", "digest", reference, env=environment) != digests[arch] or manifest.get("config", {}).get("digest") != configs[arch]:
                    raise ValueError("anonymous platform/config verification failed")
    lines = [f"{r}:{version}@{indexes[i]}" for i, r in enumerate(repositories)]
    lines.extend(f"{r}@{digests[a]} # linux/{a}" for r in repositories for a in ARCHES)
    (output / "IMAGES.txt").write_text("\n".join(lines) + "\n")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--ghcr", default="ghcr.io/arkgravity/optimus")
    parser.add_argument("--dockerhub", default="docker.io/logic3579/optimus")
    args = parser.parse_args()
    publish(args.version, args.commit, args.input, args.output, [args.ghcr, args.dockerhub])

if __name__ == "__main__":
    main()
