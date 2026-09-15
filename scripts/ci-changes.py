"""Conservatively skip expensive checks only for prose-only changes."""
import os
import subprocess


def prose_only(paths):
    return bool(paths) and all(
        path in ("README.md", "AGENTS.md")
        or (path.startswith(("docs/", "scripts/")) and path.endswith(".md"))
        for path in paths
    )


def main():
    run = True
    base = os.environ.get("BASE_SHA", "")
    head = os.environ.get("HEAD_SHA", "HEAD")
    if os.environ.get("EVENT_NAME") != "workflow_dispatch" and base.strip("0"):
        try:
            # Include both sides of renames; deleted code must trigger checks.
            diff = subprocess.check_output([
                "git", "diff", "--no-renames", "--name-only", "-z", base, head
            ])
            paths = diff.decode().rstrip("\0").split("\0") if diff else []
            run = not prose_only(paths)
        except (subprocess.CalledProcessError, UnicodeDecodeError):
            pass  # Unknown history must run the full checks.
    with open(os.environ["GITHUB_OUTPUT"], "a") as output:
        output.write(f"run={str(run).lower()}\n")


if __name__ == "__main__":
    main()
