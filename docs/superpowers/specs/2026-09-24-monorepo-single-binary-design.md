# Monorepo merge and single-binary delivery — design

Date: 2026-09-24. Status: implemented (Dev acceptance pending).

## Goal

Recombine the split `optimus-be` and `optimus-fe` repositories into one
repository, `github.com/logic3579/optimus`, and ship the web UI inside the Go
binary instead of a separate nginx image, following the Casdoor model of one
server process serving both API and frontend.

## Decisions

- **Layout.** The backend stays at the repository root; the frontend moves to
  `web/`. Both split histories are preserved: the backend history is the base
  and the frontend history is subtree-merged under `web/`. The GitHub split
  repositories and the original monorepo were deleted, so pre-split commit IDs
  no longer resolve.
- **Module path.** `optimus-be` becomes `github.com/logic3579/optimus`.
  `go.mod` ignores `./web/node_modules` because npm packages can ship Go files
  (for example `flatted`) that would otherwise join `./...`.
- **One binary.** `cmd/optimus` replaces `cmd/server`, `cmd/migrate`,
  `cmd/seed` and `cmd/vault-keygen` with `server` (default), `migrate`, `seed`,
  `vault-keygen` and `version` subcommands built on stdlib `flag`. Compose keeps
  migrate and seed as one-shot services running the same image.
  `cmd/dump-permissions` remains a development tool.
- **Embedded UI.** Casdoor reads `web/build` from disk at runtime; Optimus embeds
  `web/dist` with `go:embed` so a single file runs anywhere. `server.web_dir`
  optionally serves a build from disk for hot replacement. A tracked
  `web/dist/.gitkeep` keeps backend-only builds compiling; the web build script
  restores it after Vite empties the directory.
- **PostgreSQL 17** replaces 16 in Compose, CI and dockertest. Existing 16 data
  directories are not upgraded in place; the project is still in Dev acceptance.

## Web UI serving (`internal/infra/webui`)

Mounted as Gin's `NoRoute` handler, so API routes always win.

- `/api`, `/api/*` and `/swagger*` misses return the JSON envelope with
  `CodeNotFound` (40401, `common.not_found`), never the SPA shell.
- Only GET/HEAD are served; other methods get 405 with `Allow: GET, HEAD`.
- Existing files are served directly. Hashed `/assets/*` use
  `public, max-age=31536000, immutable`; `index.html` uses
  `no-cache, no-store, must-revalidate`; other files use `no-cache`.
- Missing `/assets/*` return 404 so stale chunks fail loudly. Any other miss
  falls back to `index.html` for client-side routing. Dot-segments are hidden.
- Embedded mode preloads every file at startup with a weak ETag and, for
  compressible types of at least 1 KiB, a best-compression gzip variant. Requests
  do no disk I/O or compression and honor `If-None-Match`.
- Disk mode (`server.web_dir`) requires `index.html` at startup and reads files
  per request through `http.ServeContent`, so rebuilt assets apply without a
  restart.
- Without an embedded `index.html`, UI paths return a "web UI is not built"
  message and the server logs a warning; the API keeps working.

## Replacing nginx responsibilities

- `middleware.SecurityHeaders` sets `X-Content-Type-Options: nosniff`,
  `X-Frame-Options: SAMEORIGIN` and
  `Referrer-Policy: strict-origin-when-cross-origin` on every response, as nginx
  did at server level.
- `server.trusted_proxies` (`OPTIMUS_SERVER_TRUSTED_PROXIES`) configures Gin's
  trusted proxies. The default is empty: Gin previously trusted every proxy, so
  a directly exposed server would let clients spoof `X-Forwarded-For` and evade
  per-IP login rate limiting and audit attribution.
- CORS is unnecessary for the same-origin UI; the default allowlist only keeps
  the Vite dev origin.

## Delivery

- The Dockerfile builds `web/` with Bun, compiles `cmd/optimus` with the build
  output embedded, and ships one binary on Alpine.
- Compose runs `postgres:17-alpine`, `migrate`, `seed` and `optimus`; the
  server publishes `HTTP_BIND:HTTP_PORT` directly. `OPTIMUS_VERSION` replaces
  `BACKEND_VERSION` and the `FRONTEND_*` variables.
- One CI workflow runs web checks, backend quality/unit/database jobs and a
  Docker build, then publishes `ghcr.io/logic3579/optimus` and
  `docker.io/logic3579/optimus` as `main-<short-sha>`.

## Contract changes

Cross-repository rules (sibling checkouts, independent image versions,
coordinated API/menu releases) no longer apply. API, menu fixture and
permission changes land in the same commit; the menu fixture's `source` is now
the in-repository `internal/seed/seed.go`.
