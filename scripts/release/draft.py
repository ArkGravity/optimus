"""Create or refresh draft attachments; never publish a GitHub Release."""
import argparse
import os
from pathlib import Path
import subprocess

from package import ARCHES, check_version, verify_sums
from validate import api, validate


def check_assets(release, expected, exact=False):
    assets = release.get("assets")
    if not isinstance(assets, list) or not all(isinstance(asset, dict) and isinstance(asset.get("name"), str) for asset in assets):
        raise ValueError("malformed release assets")
    names = [asset["name"] for asset in assets]
    if len(names) != len(set(names)) or not set(names) <= expected:
        raise ValueError("refusing unexpected or duplicate release assets")
    if exact and set(names) != expected:
        raise ValueError("release must contain exactly the six expected attachments")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()
    check_version(args.version)
    eligibility = validate()
    if eligibility["version"] != args.version:
        raise ValueError("draft version differs from selected tag")
    names = [f"optimus_{args.version}_linux_{a}.tar.gz" for a in ARCHES] + [f"optimus_{args.version}_deploy.tar.gz", "INSTALL.md", "IMAGES.txt"]
    expected = set(names + ["SHA256SUMS"])
    verify_sums(args.artifacts, "SHA256SUMS", set(names))
    repo = os.environ["GITHUB_REPOSITORY"]
    selected = [r for r in api(f"repos/{repo}/releases?per_page=100") if r.get("tag_name") == args.version]
    if len(selected) > 1 or any(r.get("draft") is not True for r in selected):
        raise ValueError("refusing to modify published or ambiguous release")
    if selected:
        check_assets(selected[0], expected)
    notes = str(Path("docs/releases", args.version + ".md"))
    if not selected:
        subprocess.run(["gh", "release", "create", args.version, "--verify-tag", "--draft", "--title", args.version, "--notes-file", notes], check=True, timeout=300)
    else:
        subprocess.run(["gh", "release", "edit", args.version, "--notes-file", notes], check=True, timeout=300)
    # Recheck draft status immediately before the only attachment mutation.
    current = [r for r in api(f"repos/{repo}/releases?per_page=100") if r.get("tag_name") == args.version]
    if len(current) != 1 or current[0].get("draft") is not True:
        raise ValueError("release is no longer a draft")
    check_assets(current[0], expected)
    subprocess.run(["gh", "release", "upload", args.version, "--clobber", *(str(args.artifacts / n) for n in names + ["SHA256SUMS"])], check=True, timeout=300)
    final = [r for r in api(f"repos/{repo}/releases?per_page=100") if r.get("tag_name") == args.version]
    if len(final) != 1 or final[0].get("draft") is not True:
        raise ValueError("release is no longer a draft")
    check_assets(final[0], expected, exact=True)

if __name__ == "__main__":
    main()
