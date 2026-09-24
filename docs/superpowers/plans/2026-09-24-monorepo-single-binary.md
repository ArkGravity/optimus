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
- [x] **Rename the Go module** to `github.com/logic3579/optimus`, add
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
- [ ] **Verification.** `make lint test swagger-diff perm-check`,
  `make test-int` on PostgreSQL 17, `make web-check` and web build, and an
  isolated Compose smoke (health, SPA deep link, asset caching, Swagger, JSON
  404, login, SSE, forged `X-Forwarded-For`).
- [ ] **mem0.** Replace the `optimus-be`/`optimus-fe` memories with one
  `optimus` checkpoint.

Out of scope: pushing to `logic3579/optimus`, registry secrets, production
acceptance, release tagging and running the image as a non-root user.
