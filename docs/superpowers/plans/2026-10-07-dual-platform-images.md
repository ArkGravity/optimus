# Dual-platform image delivery

Date: 2026-10-07. Extends the delivery configuration in
[the monorepo design](../specs/2026-09-24-monorepo-single-binary-design.md).

## Implementation

- [x] Build both `linux/amd64` and `linux/arm64` in the CI image gate and
  publishing job, with QEMU initialized before Buildx.
- [x] Build Bun assets on `BUILDPLATFORM` and cross-compile the Go binary on
  `BUILDPLATFORM` using `TARGETOS`/`TARGETARCH`, with CGO disabled and separate
  Go build caches per target platform. Keep Alpine runtime stages on the target
  platform.
- [x] Preserve the existing main-only publication gates, registries, version
  tags, documentation change filtering, and provenance attestations.
- [x] Update deployment and project instructions, distinguishing new
  multi-platform tags from existing amd64-only tags.
- [x] Validate workflow syntax with actionlint v1.7.7, CI change classification
  with all five existing tests, and patch whitespace with `git diff --check`.
- [x] Validate both platform builds in GitHub CI: run `37597470933` passed
  all five gates for implementation commit `64af14b`. Docker build logs confirm
  `GOARCH=amd64` and `GOARCH=arm64`, with both runtime images assembled.
- Local Colima runtime smoke (version, migrations, seed, health, embedded UI)
  is deferred at the operator's request on 2026-10-07.
- [ ] After merge to main, confirm the new tag in both registries contains
  both runnable platforms. Existing published tags are not rewritten.

## Checkpoint

Implementation started from `main` at `42cf46e`. Workflow syntax, CI change
classification tests, and whitespace checks passed. Local builds resolved both
platforms and completed the amd64 Alpine package installation, but did not
produce a verified image: the build connection closed and the Colima Docker
daemon became unavailable. The cause is unconfirmed. A separate Dockerfile
static check also encountered a Docker Hub TLS handshake timeout.

On 2026-10-07 the operator requested stopping Colima validation and proceeding
to the next step. Continue build verification through a pull request's GitHub
CI. [PR #1](https://github.com/ArkGravity/optimus/pull/1) contains the change;
[CI run 37597470933](https://github.com/ArkGravity/optimus/actions/runs/37597470933)
passed web, backend quality, backend unit, backend database, and dual-platform
Docker build checks. The publishing job was correctly skipped for the PR.
Publication remains pending merge to main; no registry tag has been changed by
this work. The Colima Kubernetes API was unavailable and no Kubernetes resources
were changed.
