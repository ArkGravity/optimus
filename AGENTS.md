# Optimus Project Guide

## First Read

Read this file and the active design/plan before substantial changes. See
[docs/README.md](docs/README.md) for document ownership and the mapping from
historical paths to the current layout. This is the sole project
operating-contract file. Keep code comments in English.
Use mem0 with `user_id = "logic"`. Write with top-level `app_id = "optimus"`
and `metadata = {"project": "optimus", "app_id": "optimus"}`; filter reads by
`user_id` and `metadata.project = "optimus"`. Prefix memory text with
`[optimus]`. Cross-check checkpoints with Git status/history.

## Status

P0-P6 are implemented. The project is in Dev acceptance and has not been used
in production. Dev and Production are the selected environments; UAT is
skipped. Production acceptance and release tagging remain outstanding.

2026-09-24 monorepo merge: the `optimus-be` and `optimus-fe` repositories (split
on 2026-09-14 from the original `logic3579/optimus` monorepo) were merged back
into this repository with both split histories preserved. The frontend lives in
`web/` and is embedded into the single `optimus` binary; PostgreSQL is 17. The
split GitHub repositories and the original monorepo were deleted, so pre-split
commit IDs do not resolve. Design:
`docs/superpowers/specs/2026-09-24-monorepo-single-binary-design.md`.

Merge verification (2026-09-24): locally, Go lint (Go 1.25 toolchain),
`swagger-diff`, `perm-check`, Go unit tests with race, the `dbtest` suite on
PostgreSQL 17, web lint/typecheck/i18n/298 tests/build and `make build` passed.
A host-binary smoke passed per-IP login rate limiting with forged
`X-Forwarded-For` and trusted-proxy client IPs. The Docker image built on
Colima (4 CPU / 8 GiB), and an isolated Compose smoke of that image passed
PostgreSQL 17, migrate, seed, embedded UI serving (SPA fallback, immutable gzip
assets, stale-chunk 404), JSON 404 for unknown API paths, Swagger, admin login,
authenticated API calls and pod-log SSE streaming from Colima k3s. CI run
`36048083419` on `ArkGravity/optimus` (`e0d7c58`) passed every job and
published `ghcr.io/arkgravity/optimus` and `docker.io/logic3579/optimus` as
`main-e0d7c58` (linux/amd64 only). The GHCR package is private until its
visibility is changed. Browser visual acceptance remains unverified.

Latest frontend UI work (2026-09-17: Ant Design theme tokens, dark sidebar,
compact workspace and menu tabs) passed lint, typecheck, i18n, unit tests and
build; browser visual and backend-integration acceptance remain unverified.
The general `/dashboard` page is still a coming-soon placeholder; P5
observability dashboards are implemented.

## Repository Layout

- Repository root: Go module `github.com/ArkGravity/optimus` — `cmd/optimus`
  (single binary), `cmd/dump-permissions` (dev tool), `internal/`,
  `migrations/`, `configs/`, `api/docs/`, `tests/`.
- `web/`: Vue 3 SPA built with Bun and Vite. `web/embed.go` embeds `web/dist`.
- `docs/`: generated API spec and permission catalog, UI notes, and
  `superpowers/specs|plans`.
- `scripts/`: CI change classifier and P3-P6 smoke checklists;
  `web/scripts/`: i18n key parity checker.

## Commands

Run from the repository root:

- `make tools`
- `make run` (air, API on :8080) and `make web-dev` (Vite on :5173)
- `make build` (web build, then `bin/optimus` with the UI embedded)
- `make test` / `make test-int`
- `make lint`
- `make web-install` / `make web` / `make web-check`
- `make swag` / `make swagger-diff`
- `make dump-perms` / `make perm-check`
- `make migrate-up` / `make migrate-down`
- `make migrate-new name=<snake_case>`
- `make seed`

Frontend commands run from `web/`: `bun install`, `bun run dev`,
`bun run lint`, `bun run typecheck`, `bun run i18n:check`, `bun run test`,
`bun run test:watch`, `bun run build`. Use `bun` only for web dependency work;
do not use npm, pnpm, or yarn.

Focused tests: `go test ./internal/modules/user/... -run TestService_Create -race`
and, from `web/`, `bun x vitest run path/to/file.test.ts -t "name pattern"`.
Integration variants require Colima Docker and the `dbtest` build tag.
`OPTIMUS_JWT_SECRET` must be at least 32 bytes or the server refuses to start.

## Local Runtime Policy

- Use Colima for project-local Docker and Kubernetes on both macOS and Linux.
- Start the default profile with
  `colima start --runtime docker --kubernetes`, then select the `colima` Docker
  and kube contexts.
- Verify readiness with `colima status`, `docker info`, and
  `kubectl cluster-info` before Docker-backed integration or smoke tests.
- Run Docker Compose, dockertest, `make test-int`, and P5 containers on Colima's
  Docker runtime. Do not silently use Docker Desktop or a host system Docker
  daemon instead.
- Run the full stack with `docker compose up -d --build`; it builds the single
  image from this repository. Published-image deployments use `pull` followed by
  `up -d --no-build` and need no source checkout. Start only `postgres` when
  running the server and Vite on the host. Connect with `psql` through the
  loopback-only PostgreSQL port; no Adminer service is maintained.
- Invoke Docker Compose exclusively through the Docker CLI as
  `docker compose ...`. The Homebrew Compose plugin is exposed through
  `~/.docker/cli-plugins/docker-compose`; verify it with
  `docker compose version` before local development or smoke tests.
- For tools that ignore Docker contexts, use the socket reported by
  `colima status` through `DOCKER_HOST`; never hardcode a macOS `/Users/...`
  path or assume `/var/run/docker.sock`.
- Use Colima's built-in Kubernetes for routine P2/P3 local development and P6
  release smoke. P6 must use the `colima` kube context and isolate resources in
  its disposable namespace; never stop or delete the shared Colima cluster
  during teardown.
- Linux/WSL2 without `/dev/kvm` may use slower QEMU software virtualization.
  This local-runtime policy does not apply to CI runners.
- Keep Go `TMPDIR`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` under the ignored
  `tmp/` tree. The Makefile owns these defaults. Do not place build caches under
  `/tmp`, which may be a size-limited tmpfs.

## Non-Negotiable Invariants

- Keep `go.mod` at `go 1.25` with `ignore ./web/node_modules`.
- Keep `k8s.io/client-go` and `k8s.io/apimachinery` pinned to `v0.30.14`.
- Keep `helm.sh/helm/v3` pinned to `v3.15.4` unless the compatibility story is
  reopened deliberately.
- Add permissions only in `internal/infra/permissions/codes.go`, then
  run `make dump-perms` and `make perm-check`.
- Regenerate Swagger with `make swag` after handler annotation or API contract
  changes, then run `make swagger-diff`.
- Keep client-facing backend errors inside the envelope and use `apperr.New` or
  `apperr.Wrap`; do not leak raw error text to clients.
- Keep zh-CN/en-US locale parity and Linux-compatible (lowercase/kebab-case)
  frontend paths.
- Keep `web/dist/.gitkeep` tracked; `bun run build` recreates it after Vite
  empties `dist/`.
- Keep code comments in English.

## Backend Architecture

- Module layering is `dto.go` -> `repo.go` -> `service.go` -> `handler.go`.
  Handlers bind and validate input, services own business logic and audit/cache
  effects, repositories own GORM access, and business JSON handlers return the
  fixed `{code,data,message,message_key?}` envelope. `GET /api/v1/health`
  returns a raw `{db,version}` probe; pod logs and delivery events use SSE.
- `cmd/optimus/server.go` is the only composition root. It loads configuration,
  registers all in-code permissions, creates the shared RBAC cache and audit
  recorder, wires credential consumers, and mounts routes. `cmd/optimus/main.go`
  only dispatches subcommands (`server` is the default).
- Every mutating service path records through that shared audit recorder; do
  not construct a second recorder.
- Mount protected routes through nested `Group("", middleware)` groups. Passing
  permission middleware as variadic `GET`/`POST` arguments is not equivalent
  when handlers are registered separately.
- Permission resolution joins permissions, roles, and users while excluding
  soft-deleted users and roles. Services that change roles, user roles, or role
  permissions must invalidate the appropriate shared permission cache.
- Access tokens expire after 15 minutes and refresh tokens after 168 hours.
  Refresh tokens are persisted and rotated; replay is rejected. Login is
  rate-limited per IP.
- Goose migrations live in `migrations/` and are embedded; `optimus migrate`
  and `make migrate-*` use the same files. Models live in `internal/models/`;
  database integration tests use dockertest and `dbtest`.

## Web UI Serving

- `internal/infra/webui` is the Gin `NoRoute` handler, so API routes always
  win. Unknown `/api*` and `/swagger*` paths return the JSON envelope (40401),
  never the SPA shell. Only GET/HEAD are served.
- Hashed `/assets/*` are immutable; `index.html` is `no-store`; other files
  revalidate. Missing `/assets/*` return 404; other misses fall back to
  `index.html`.
- The embedded build is preloaded at startup with ETags and gzip variants.
  `server.web_dir` serves a build from disk per request instead.
- `middleware.SecurityHeaders` applies nosniff, `X-Frame-Options: SAMEORIGIN`
  and `Referrer-Policy` to every response.
- `server.trusted_proxies` is empty by default, so `X-Forwarded-For` is ignored
  and `c.ClientIP()` is the TCP peer. Set it to the TLS proxy's IP/CIDR when
  fronted by a proxy; never trust all proxies.

## Frontend Architecture

- Bootstrap order is Pinia, Ant Design Vue, i18n, API client, provided module
  APIs, router guards, then mount.
- Static routes contain login/error/profile pages and application, asset and
  delivery detail/action sub-routes. On the first authenticated navigation,
  fetch `/me`, menus, and permissions in parallel, register dynamic routes,
  then replace-navigate to the original destination.
- Permission enforcement has two synchronized layers: route
  `meta.permission` and the `v-permission` directive. Both read the Pinia auth
  permission state; components must not re-fetch permissions.
- The API client validates the fixed envelope and converts nonzero codes to
  `BizError`. Concurrent HTTP and SSE 401 responses share the store's
  single-flight refresh promise; each original request is replayed at most once.
- Locale files are `web/src/locales/zh-CN.json` and `web/src/locales/en-US.json`;
  `bun run i18n:check` enforces parity. The Vite alias `@/*` maps to `src/*` and
  the dev `/api/v1` proxy targets `http://localhost:8080`.
- Custom surfaces use Ant Design theme tokens via `AppTheme`.
  `web/public/optimus-logo.png` is shared by the sidebar, login and favicon
  (see `docs/branding.md`). See `docs/workspace-tabs.md` before extending
  workspace tab state retention: only approved filter/pagination/scroll state
  is retained in memory; streams, polling and sensitive content are never cached.

## UI and API Contracts

- All application requests use `/api/v1/*` and the fixed backend envelope.
  API, menu and permission changes land in the same change as the UI that uses
  them.
- `web/src/test/fixtures/menu-contract.json` is the reviewed snapshot of
  `internal/seed` menus. Update it with backend menu changes and run the
  route/casing tests; it is test data, not the runtime menu source.
- Use `ClusterPicker` and the Kubernetes store for cluster selection, inside
  Kubernetes pages only. An absent selection shows a prompt; never auto-select
  or redirect.
- Pod logs use SSE and `http.ResponseController(c.Writer).Flush()`. The
  frontend consumes streams with `fetch` and `ReadableStream`, not
  `EventSource`, so the JWT stays in the Authorization header. Logout resets
  active streams.

## Credentials and Kubernetes Architecture

- The P1 vault uses AES-256-GCM. Load `OPTIMUS_VAULT_MASTER_KEY` or
  `OPTIMUS_VAULT_MASTER_KEY_FILE` before opening the database so a missing key
  fails fast.
- `credentials.Consumer` is the sole downstream Go API for credentials. Never
  bypass it with HTTP or a second cipher. Use `credentials.WithActor` for
  non-HTTP actors. Kubeconfigs reject `exec` and `auth-provider` plugins.
- P2 Kubernetes endpoints are read-only: do not add exec, apply, write verbs,
  or watch without reopening the P2 design. Build and discard a fresh client
  per request. Normalize API-server failures through `k8s/apierr`.
- The secret `/data` reveal endpoint is the only path that returns plaintext
  Kubernetes secret values and remains gated by `k8s:secret:reveal`.

## Generated Artifacts and Dependency Pins

- `make swag` updates both `api/docs/swagger.json` and
  `docs/api/swagger.json`; run `make swagger-diff` after regeneration.
- Permission codes originate only in `internal/infra/permissions/codes.go`, are
  registered into the DB at startup, gate backend routes and frontend controls,
  and generate `docs/permissions.md` through `make dump-perms`.
- The AWS SDK Go v2 modules and `github.com/robfig/cron/v3` must remain versions
  compatible with Go 1.25. Pin an offending transitive module instead of
  raising the Go directive.
- CORS environment values are comma-separated, not JSON arrays, for example
  `OPTIMUS_CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com`.
  The embedded UI is same-origin and needs no CORS entry.

## P4 Assets Rules

For P4, follow `docs/superpowers/plans/2026-06-11-p4-assets.md` task by task.
The most important rules are:

- `credentials.Consumer` is the only path for cloud keys.
- Fetch cloud keys with a purpose like `assets.sync.<reason>` and wipe them
  after use.
- Build AWS clients per sweep/request; do not cache SDK clients.
- Only authoritative successful full sweeps may soft-delete missing resources.
- VPC and subnet sweeps are one transaction and one `network` sync-run unit.
- Manual sync is asynchronous: the handler returns immediately and the worker
  owns the account lock.
- Cron writes `assets_sync_runs`; it does not write audit rows.
- Cloud-key delete must remain nil-safe when the P4 assets in-use counter is not
  wired. When wired, deleting a referenced cloud key fails with code `43001`.
- Removing regions from a cloud account must explicitly soft-delete resources in
  those removed regions.
- P4 frontend paths must be lowercase/kebab-case to match Linux production.
- `assets.Consumer` is the non-HTTP lookup seam for downstream phases and
  returns `ErrAssetsInstanceNotFound` for absent matches.
- P4 runtime configuration uses `OPTIMUS_ASSETS_SYNC_CRON`,
  `OPTIMUS_ASSETS_SYNC_STARTUP_DELAY`,
  `OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS`, and
  `OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT`.

P4's manual release checklist is `scripts/p4-smoke.md`. Use it against a
disposable read-only AWS credential before production sign-off; do not add AWS
write/manage APIs in P4.

## P5 Observability Rules

- P5 is metrics display only: no alerts, rules, notifications, CloudWatch,
  metric sample storage, logs, traces, or APM.
- Consume P1 HTTP credentials only through `credentials.Consumer`; never
  expose, log or audit secrets, authorization headers, custom CA PEM, or full
  PromQL.
- Private Prometheus targets require a narrow CIDR. Metadata and mixed DNS
  answers stay denied even under broad ranges. Never follow redirects or
  cache clients.
- Keep query count, concurrency, PromQL bytes, range, step, points, series,
  response, timeout, and enrichment limits enforced.
- Metric-only operators use the minimal query-source endpoint; built-ins must
  not call the administrative data-source list.
- Data-source, dashboard, and metric permissions remain independent; every
  API call and concrete UI control uses its exact gate.
- Preserve abort generations so stale definition/query responses cannot
  commit.
- Run `scripts/p5-smoke.md` with disposable local Prometheus; no
  production credential or Kubernetes cluster is needed.

## P6 Application Delivery Rules

- P6 promotes immutable chart artifacts through ordered environments bound to
  existing P3 applications; it does not accept arbitrary commands, scripts,
  manifests, values, container images, or credentials. The UI offers no raw
  command, manifest, values or image editors and shows frozen artifacts, exact
  action permissions and safe errors.
- Direct P3 upgrade/uninstall of a delivery-managed application stays denied;
  only the closed in-process delivery capability may perform an upgrade.
- Resolve and persist the chart digest before run creation. Every stage must
  use the frozen repository, chart name, version, digest, application, cluster,
  namespace, release name, executor, approval policy, and timeout.
- Initiators cannot approve their own run. Project, pipeline, run, and approval
  permissions remain independent and every UI control uses its exact gate.
- Workers use database leases and stable operation IDs. Ambiguous outcomes go
  through reconciliation; never guess success or blindly replay Helm.
- System kubeconfig consumption must use a `system:` purpose. The production
  Helm loader must preserve `LoadVerifiedChart` digest verification.
- SSE and audit projections must never expose values, kubeconfigs, auth
  headers, manifests, Helm notes, or raw executor errors.
- Run `scripts/p6-smoke.md` only against disposable PostgreSQL, an
  isolated namespace in Colima Kubernetes, and disposable chart-repository
  resources.

## Local Gotchas

- Remote Linux Docker MTU incident: `apk add` hung downloading Alpine indexes;
  host/host-network downloads worked while bridge downloads timed out. MTU 1460
  fixed the test, and persisting Docker daemon `mtu` plus bridge
  `default-network-opts` at 1460 and restarting Docker resolved the issue.
  See README's Linux Docker MTU troubleshooting section. Verify the appropriate
  MTU per server; recreate existing networks without deleting data volumes.
- The image build runs the Vite build and a large Go build. On 2026-09-24 a
  2 GiB Colima VM (with k3s) ran out of memory during the Vite stage; the OOM
  killer also broke Colima's Docker socket forward. 4 CPU / 8 GiB builds fine.
- Published images are linux/amd64 only. On arm64 hosts (Apple silicon Colima)
  pull with `--platform linux/amd64` or build locally.
- Colima's k3s API listens on the VM port shown in the `colima` kubeconfig
  (not 6443). Containers reach it via their network gateway IP with
  `tls-server-name: kubernetes`.
- `golangci-lint` v1.64.8 cannot read Go 1.27+ export data. When the local Go
  is newer than CI's 1.25, run `GOTOOLCHAIN=go1.25.0 make lint`.
- Container health checks use GET; keep `/api/v1/health` registered for GET.
- The initial administrator password is printed exactly once by seed. If it is
  lost, reset it through the database.
- Generate the vault key with `go run ./cmd/optimus vault-keygen`, store it
  securely, and never rotate it casually; an absent or incorrect key prevents
  startup or credential decryption.
- A PostgreSQL named volume retains its original database password even if
  `.env` changes. Never use `docker compose down -v` as a password-rotation
  technique on persistent environments because it erases all data.
- Development volumes initialized by PostgreSQL 16 cannot start under 17;
  recreate disposable dev volumes.
- Use multi-character namespaces in YAML round-trip tests; a one-character
  namespace can be decoded unexpectedly by `sigs.k8s.io/yaml`.

## Repository and Delivery

- GitHub: https://github.com/ArkGravity/optimus
- `Dockerfile` builds `web/` with Bun and the single `optimus` binary with the
  UI embedded. `docker-compose.yml` and `.env.example` own the deployment stack:
  `postgres`, one-shot `migrate` and `seed`, and `optimus`, all from one image
  versioned by `OPTIMUS_VERSION`.
- `.github/workflows/ci.yaml` runs web, backend quality, unit, database and
  Docker build gates; only main publishes to `ghcr.io/arkgravity/optimus` and
  `docker.io/logic3579/optimus`, tagged `main-<short-sha>`, after every gate
  passes. CI uses one disposable PostgreSQL 17 instance via
  `OPTIMUS_TEST_POSTGRES_DSN`, with a separate migrated database per test.
  Never point this variable at an application server. Local tests default to
  dockertest. Keep `-race -count=1`. Cache `tmp/go-cache` per job. Prose-only
  changes skip database and image work; manual dispatch and unknown history run
  full checks. See README for details and registry setup.

## Codex Environment

`.codex/config.toml` configures mem0 and Context7 over HTTP, local Serena, and
the superpowers and build-web-apps plugins. Export `MEM0_API_KEY` and
`CONTEXT7_API_KEY` before starting Codex; install `serena` on PATH. Start Codex
from the repository root so Serena selects it. Preserve the mem0 scope from
First Read; when updating metadata, read and preserve existing fields, send the
complete merged object, and verify the saved result. Update this file and the
current checkpoint after milestones.
Local `.serena/`, `.omo/`, `.worktrees/` and agent caches are ignored.
