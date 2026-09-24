# Monorepo merge and single-binary delivery — implementation plan

Design: [2026-09-24-monorepo-single-binary-design.md](../specs/2026-09-24-monorepo-single-binary-design.md).
Each task is one commit.

- [x] **Bootstrap history.** Clone local `optimus-be` (`7e10391`) as the base
  and subtree-merge local `optimus-fe` (`d0dde73`) under `web/`. Verify the base
  tree equals `optimus-be` HEAD and `web/` equals `optimus-fe` HEAD.
- [x] **Clean split-era tooling.** Remove `web/.github`, `web/.codex`,
  `web/Dockerfile`, `web/nginx.conf`, `web/.dockerignore` and `web/.gitignore`;
  merge ignore/editorconfig rules at the root; union `.codex/config.toml`;
  rename the web package to `optimus-web` (package.json and bun.lock).
- [x] **Rename the Go module** to `github.com/ArkGravity/optimus`, add
  `ignore ./web/node_modules`, regenerate Swagger and the permission catalog.
- [x] **Single binary.** Move the four entrypoints into `cmd/optimus` with
  subcommands; update Makefile, air, Dockerfile and Compose.
- [x] **Embed the web UI.** `web/embed.go`, `internal/infra/webui`,
  `middleware.SecurityHeaders`, `server.trusted_proxies`, `server.web_dir`,
  Makefile `web*` targets, `web/dist/.gitkeep` restored by the web build.
- [x] **PostgreSQL 17** in Compose, CI and dockertest.
- [x] **Container delivery.** Three-stage Dockerfile, single-image Compose,
  `.env.example` and `.dockerignore`.
- [x] **CI.** Add the web job; publish `optimus` to GHCR and Docker Hub after
  every job passes.
- [x] **Docs.** Merge frontend docs and superpowers plans/specs into `docs/`,
  write the docs index with the historical path mapping, merge the README.
- [x] **AGENTS.md.** One operating contract for the monorepo.
- [x] **Verification.** `make lint test swagger-diff perm-check`,
  `make test-int` on PostgreSQL 17, `make web-check`, `make build`, and a
  host-binary smoke against isolated Compose PostgreSQL 17 (health, SPA deep
  link, asset caching and gzip, ETag, Swagger, JSON 404, login, forged
  `X-Forwarded-For`, trusted proxy). Also ignore `./tmp` in `go.mod`, where
  `make swagger-diff` writes generated Go. After resizing Colima to 8 GiB: the
  image built, an isolated Compose smoke of the image passed (including pod-log
  SSE from Colima k3s), and CI run `36048083419` published `main-e0d7c58`.
- [x] **Repository move.** The repository is `github.com/ArkGravity/optimus`:
  rename the module accordingly, publish GHCR as `ghcr.io/arkgravity/optimus`,
  and default `DOCKERHUB_USERNAME` to `logic3579`.
- [ ] **mem0.** Replace the `optimus-be`/`optimus-fe` memories with one
  `optimus` checkpoint.

Out of scope: production acceptance, release tagging and running the image as
a non-root user.
