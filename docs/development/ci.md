# Continuous integration

The [CI workflow](../../.github/workflows/ci.yml) runs on main/dev pushes, pull requests and manual dispatch. Its gates are web lint/typecheck/locale parity/tests/build, backend lint/generated-artifact checks, unit races, PostgreSQL database races and a dual-platform Docker build.

## Images and registry setup

After main's gates pass, the image publishing job builds and publishes `main-<short-sha>` to both:

- `ghcr.io/arkgravity/optimus`
- `docker.io/logic3579/optimus`

Pull requests and dev builds do not publish. This main CI image flow is distinct from [versioned release publication](releases.md). Versioned publication must retain the exact tested image bytes and digests.

Both image jobs target `linux/amd64` and `linux/arm64`. QEMU is set up before Buildx for runtime package installation; Bun runs on the builder's native platform and Go cross-compiles on BUILDPLATFORM with TARGETOS/TARGETARCH and CGO disabled. Provenance entries shown as `unknown/unknown` are metadata. Earlier tags from before dual-platform delivery remain amd64-only.

| Actions setting | Name | Purpose |
| --- | --- | --- |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub token with write permission |
| Variable | `DOCKERHUB_USERNAME` | Login, defaults to `logic3579` |
| Variable | `DOCKERHUB_NAMESPACE` | Namespace, defaults to `logic3579` |

GHCR uses `GITHUB_TOKEN` with `packages: write`. Confirm package visibility for anonymous pulls separately. Missing Docker Hub configuration fails main publication explicitly.

## Test execution

CI stores Go caches under `tmp/go-cache` with job-specific cache keys. The classifier in [ci-changes.py](../../scripts/ci-changes.py) skips database execution and image work only for its recognized prose-only paths: `README.md`, `README_zh.md`, `AGENTS.md`, and Markdown under `docs/` or `scripts/`, except `docs/releases/`. Release notes and installation templates require full checks because they are packaged release inputs. Other new top-level documentation is also conservatively treated as requiring full checks. Web, quality and unit checks still run. Manual dispatch, unknown history and non-prose changes run the full suite.

The database job starts one disposable PostgreSQL 17 server and sets `OPTIMUS_TEST_POSTGRES_DSN`. Every test creates a unique database, applies migrations and drops only that database afterward. The DSN must point to a dedicated test server with CREATE DATABASE permission, never a shared application server. Without it, local integration tests use dockertest on Colima. `make test-int` retains `-race -count=1` while compiled artifacts are cached.

A green source CI run does not establish real-provider acceptance or release-archive/image smoke results. Record those separately in the release notes.
