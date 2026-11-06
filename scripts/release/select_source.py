"""Select an existing release source, then run that source's strict validator."""
import os
import re
import subprocess
import sys

from package import check_version


def git(*args):
    return subprocess.check_output(["git", *args], text=True, stderr=subprocess.DEVNULL, timeout=90).strip()


def select_source():
    event = os.environ["GITHUB_EVENT_NAME"]
    if event == "workflow_dispatch":
        if (os.environ.get("GITHUB_REF"), os.environ.get("GITHUB_REF_TYPE"), os.environ.get("GITHUB_REF_NAME")) != ("refs/heads/main", "branch", "main"):
            raise ValueError("manual release selection is allowed only from refs/heads/main")
        version = os.environ.get("RELEASE_VERSION_INPUT", "")
    elif event == "push":
        if os.environ.get("GITHUB_REF_TYPE") != "tag":
            raise ValueError("automatic releases require a tag push")
        version = os.environ["GITHUB_REF_NAME"]
        if os.environ.get("GITHUB_REF") != "refs/tags/" + version:
            raise ValueError("tag push ref does not match event tag")
        # Dispatch inputs can never replace a push event's tag or exact SHA.
    else:
        raise ValueError("unsupported release event")

    # Validate all selectors before using any of them as a Git ref or argument.
    check_version(version)
    event_commit = os.environ["GITHUB_SHA"]
    if not re.fullmatch(r"[0-9a-f]{40}", event_commit):
        raise ValueError("workflow SHA must be a full commit SHA")
    if git("rev-parse", "HEAD") != event_commit:
        raise ValueError("initial checkout must match the workflow event SHA")
    tag = "refs/tags/" + version
    # No forced ref updates: a local/remote disagreement must fail, not replace
    # an existing tag. Raw-commit Actions checkouts preserve annotated tag refs.
    git("fetch", "--no-tags", "origin", tag + ":" + tag)
    if git("cat-file", "-t", tag) != "tag":
        raise ValueError("release tag must be annotated")
    commit = git("rev-parse", tag + "^{commit}")
    if event == "push" and commit != event_commit:
        raise ValueError("tagged source must match the exact push event SHA")
    git("checkout", "--detach", commit)
    # Keep the trusted workflow identity intact. Only the source validator's
    # child environment represents the selected tag; preserve GITHUB_OUTPUT.
    environment = {**os.environ, "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": version,
                   "GITHUB_SHA": commit, "GITHUB_REF": tag}
    subprocess.run(["bash", "scripts/release/validate.sh"], env=environment, check=True, timeout=240)


if __name__ == "__main__":
    try:
        select_source()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as error:
        message = "command failed" if isinstance(error, subprocess.CalledProcessError) else str(error)
        print("Release source selection failed: " + message, file=sys.stderr)
        sys.exit(1)
