"""Fail-closed eligibility checks for an existing annotated stable release tag."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

JOBS = {"Web checks", "Backend quality", "Backend unit tests", "Backend database tests", "Docker build", "Publish image to GHCR and Docker Hub"}

def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL, timeout=90).strip()

def api(path, key=None):
    # Slurped pagination keeps missing jobs/releases on later pages visible.
    pages = json.loads(run("gh", "api", "--paginate", "--slurp", path))
    if not isinstance(pages, list):
        raise ValueError("malformed API pagination")
    result = []
    for page in pages:
        rows = page[key] if key else page
        if not isinstance(rows, list) or not all(isinstance(row, dict) for row in rows):
            raise ValueError("malformed API data")
        result.extend(rows)
    return result

def validate():
    version = os.environ["GITHUB_REF_NAME"]
    if os.environ.get("GITHUB_REF_TYPE") != "tag" or not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("select an existing stable vX.Y.Z tag")
    if run("git", "cat-file", "-t", "refs/tags/" + version) != "tag":
        raise ValueError("release tag must be annotated")
    commit = run("git", "rev-parse", "refs/tags/" + version + "^{commit}")
    if commit != os.environ["GITHUB_SHA"] or run("git", "rev-parse", "HEAD") != commit:
        raise ValueError("HEAD, peeled tag, and workflow SHA must match")
    run("git", "fetch", "origin", "+refs/heads/main:refs/remotes/origin/main")
    run("git", "merge-base", "--is-ancestor", commit, "refs/remotes/origin/main")
    if not Path("docs/releases", version + ".md").is_file():
        raise ValueError("missing versioned release notes")
    repo = os.environ["GITHUB_REPOSITORY"]
    releases = api(f"repos/{repo}/releases?per_page=100")
    for release in releases:
        if not isinstance(release.get("tag_name"), str) or not isinstance(release.get("draft"), bool):
            raise ValueError("malformed release data")
        if release.get("tag_name") == version and release.get("draft") is not True:
            raise ValueError("release is already published")
    runs = api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=push&branch=main&head_sha={commit}&per_page=100", "workflow_runs")
    for candidate in runs:
        if not all(candidate.get(k) == v for k, v in {"head_sha": commit, "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success"}.items()):
            continue
        if not isinstance(candidate.get("id"), int):
            raise ValueError("malformed run id")
        jobs = api(f"repos/{repo}/actions/runs/{candidate['id']}/jobs?filter=latest&per_page=100", "jobs")
        selected = [j for j in jobs if j.get("name") in JOBS]
        if len(selected) == len(JOBS) and {j["name"] for j in selected} == JOBS and all(j.get("status") == "completed" and j.get("conclusion") == "success" for j in selected):
            result = {"version": version, "commit": commit, "build_time": run("git", "show", "-s", "--format=%cI", commit)}
            if os.environ.get("GITHUB_OUTPUT"):
                with open(os.environ["GITHUB_OUTPUT"], "a") as output:
                    output.writelines(f"{k}={v}\n" for k, v in result.items())
            print(f"Eligible release: {version} ({commit})")
            return result
    raise ValueError("exact main push requires all six successful CI jobs")

if __name__ == "__main__":
    try:
        validate()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired, TypeError) as error:
        print(f"Release validation failed: {type(error).__name__}: {error if not isinstance(error, subprocess.CalledProcessError) else 'command/API failure'}", file=sys.stderr)
        sys.exit(1)
