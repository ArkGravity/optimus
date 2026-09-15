# Optimus Backend Project Guide

## First Read

Read this file and the active design/plan before substantial changes. See
[README.md — Documentation](README.md#documentation) for document ownership and
historical path conventions.
This is the sole project operating-contract file. Keep code comments in English.
Use mem0 with `user_id = "logic"`; cross-check checkpoints with Git status/history.
Scope backend memory reads with `app_id = "optimus-be"` and write with that
app ID plus `metadata.project = "optimus-be"`. Prefix memory text with
`[optimus-be]`; keep frontend (`optimus-fe`) memories in their own scope.
Backend builds, tests, and generated artifacts must not require a sibling checkout.
Compose starts the full stack, including the frontend, without a profile flag.
It builds `../optimus-fe` by default for local development;
`FRONTEND_BUILD_CONTEXT` overrides that path.

## Status

P0-P6 are implemented. Local P3/P4/P5/P6 acceptance passed before the 2026-09-14
repository split from `logic3579/optimus` (`17a3862`, tree identical to `cdfb4f3`).
Production acceptance, the persistent-data upgrade smoke from `4e2d08b` through
`00023_p6_delivery.sql`, and release tagging remain outstanding. Dev and Production
are the selected environments; UAT is skipped. Pre-split commit IDs refer to the
original repository. Split histories have new commit IDs.

Last repository review: 2026-09-15, local and remote `main` at `a2aebef`.
Post-split CI run `34901421269` passed quality, unit, database, Docker build,
and GHCR/Docker Hub publication jobs. These checks do not replace the outstanding
production acceptance or persistent-data upgrade smoke.

## Commands

Run from this repository root:

- `make tools`
- `make run`
- `make build`
- `make test`
- `make test-int`
- `make lint`
- `make swag`
- `make swagger-diff`
- `make dump-perms`
- `make perm-check`
- `make migrate-up` / `make migrate-down`
- `make migrate-new name=<snake_case>`
- `make seed`

Run a focused backend test with
`go test ./internal/modules/user/... -run TestService_Create -race`.
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
- Run the backend stack from this root with `docker compose up -d --build`.
  This includes building and running the sibling frontend checkout, defaulting
  to `local/optimus-fe:dev`. Published-image deployments use `pull` followed by
  `up -d --no-build` and do not require the frontend source.
  Start only `postgres` when running the backend/frontend directly on the host.
  Connect with `psql` through the loopback-only PostgreSQL port; no Adminer
  service is maintained.
- Invoke Docker Compose exclusively through the Docker CLI as
  `docker compose ...`. The Homebrew Compose plugin is exposed through
  `~/.docker/cli-plugins/docker-compose`; verify it with
  `docker compose version` before local development or smoke tests.
- For tools that ignore Docker contexts, use the socket reported by
  `colima status` through `DOCKER_HOST`; never hardcode a macOS `/Users/...`
  path or assume `/var/run/docker.sock`.
- Use Colima's built-in Kubernetes for routine P2/P3 local development and P6
  release smoke. P6 must use the `colima` kube context and isolate resources in
  its disposable namespace; it must not stop or delete the shared Colima
  cluster during teardown.
- Linux/WSL2 without `/dev/kvm` may use slower QEMU software virtualization.
  This local-runtime policy does not apply to CI runners.
- Keep backend `TMPDIR`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` under the ignored
  `tmp/` tree. The backend Makefile owns these defaults. Do not place
  backend build caches under `/tmp`, which may be a size-limited tmpfs.

## Non-Negotiable Invariants

- Keep `go.mod` at `go 1.25`.
- Keep `k8s.io/client-go` and `k8s.io/apimachinery` pinned to `v0.30.14`.
- Keep `helm.sh/helm/v3` pinned to `v3.15.4` unless the compatibility story is
  reopened deliberately.
- Add permissions only in `internal/infra/permissions/codes.go`, then
  run `make dump-perms` and `make perm-check`.
- Regenerate Swagger with `make swag` after handler annotation or API contract
  changes, then run `make swagger-diff`.
- Keep client-facing backend errors inside the envelope and use `apperr.New` or
  `apperr.Wrap`; do not leak raw error text to clients.
- Keep code comments in English.

## Backend Architecture

- Module layering is `dto.go` -> `repo.go` -> `service.go` -> `handler.go`.
  Handlers bind and validate input, services own business logic and audit/cache
  effects, repositories own GORM access, and business JSON handlers return the fixed
  `{code,data,message,message_key?}` envelope. `GET /api/v1/health` returns a raw
  `{db,version}` probe; pod logs and delivery events use SSE stream responses.
- `cmd/server/main.go` is the only composition root. It loads configuration,
  registers all in-code permissions, creates the shared RBAC cache and audit
  recorder, wires credential consumers, and mounts routes.
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
- Goose migrations live in `migrations/` and are embedded. Container
  and local migration commands use the same files. Models live in
  `internal/models/`; database integration tests use dockertest and `dbtest`.

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
- Pod logs use SSE and `http.ResponseController(c.Writer).Flush()`. The frontend
  consumes the stream with `fetch` and `ReadableStream`, not `EventSource`, so
  the JWT remains in the Authorization header.


## Generated Artifacts and Dependency Pins

- `make swag` updates both `api/docs/swagger.json` and
  `docs/api/swagger.json`; run `make swagger-diff` after regeneration.
- Permission codes originate only in
  `internal/infra/permissions/codes.go`, are registered into the DB
  at startup, gate backend routes/frontend controls, and generate
  `docs/permissions.md` through `make dump-perms`.
- The AWS SDK Go v2 modules and `github.com/robfig/cron/v3` must remain versions
  compatible with Go 1.25. Pin an offending transitive module instead of
  raising the Go directive.
- CORS environment values are comma-separated, not JSON arrays, for example
  `OPTIMUS_CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com`.

## P4 Assets Rules

For P4, follow `docs/superpowers/plans/2026-06-11-p4-assets.md` task by task.
The most important rules are:

- `credentials.Consumer` is the only path for cloud keys.
- Fetch cloud keys with a purpose like `assets.sync.<reason>` and wipe them
  after use.
- Build AWS clients per sweep/request; do not cache SDK clients.
- Only authoritative successful full sweeps may soft-delete missing resources.
- VPC and subnet sweeps are one transaction and one `network` sync-run unit.
- Manual sync is asynchronous: handler returns immediately and the worker owns
  the account lock.
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

P4's manual release checklist is `scripts/p4-smoke.md`. Use it
against a disposable read-only AWS credential before production sign-off; do
not add AWS write/manage APIs in P4.

## P5 Observability Rules

- P5 is metrics display only: no alerts, rules, notifications, CloudWatch,
  metric sample storage, logs, traces, or APM.
- Consume P1 HTTP credentials only through `credentials.Consumer`; never
  expose or audit secrets, authorization headers, custom CA PEM, or full
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
  manifests, values, container images, or credentials.
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
- Container health checks use GET; keep `/api/v1/health` registered for GET.
- The initial administrator password is printed exactly once by seed. If it is
  lost, reset it through the database.
- Generate the vault key with `go run ./cmd/vault-keygen`, store it securely,
  and never rotate it casually; an absent or incorrect key prevents startup or
  credential decryption.
- A PostgreSQL named volume retains its original database password even if
  `.env` changes. Never use `docker compose down -v` as a password-rotation
  technique on persistent environments because it erases all data.
- Use multi-character namespaces in YAML round-trip tests; a one-character
  namespace can be decoded unexpectedly by `sigs.k8s.io/yaml`.

## Repository and Delivery

- GitHub: https://github.com/ArkGravity/optimus-be
- `Dockerfile` builds server, migrate, seed, and vault-keygen from this root.
- `docker-compose.yml` and `.env.example` own the integrated deployment stack.
  Frontend starts by default with a configurable sibling build context for local
  development and an independently versioned image for deployment.
- `.github/workflows/ci.yaml` runs quality, unit, database and independent Docker
  build gates; only main publishes to `ghcr.io/arkgravity/optimus-be` and
  `docker.io/logic3579/optimus-be`, tagged `main-<short-sha>`.
  Docker validation runs in parallel with tests; publication requires every gate.
  CI uses one disposable PostgreSQL instance via `OPTIMUS_TEST_POSTGRES_DSN`,
  with a separate migrated database per test. Never point this variable at an
  application server. Local tests default to dockertest. Keep `-race -count=1`.
  Cache `tmp/go-cache` per job. Prose-only changes skip database and image work;
  manual dispatch and unknown history run full checks. See README for details
  and registry setup.
- API/menu/permission changes require coordination with optimus-fe. Its menu
  fixture is a reviewed contract snapshot, not a live backend dependency.

## Codex Environment

`.codex/config.toml` configures mem0 and Context7 over HTTP and local Serena.
Export `MEM0_API_KEY` and `CONTEXT7_API_KEY` before starting Codex; install
`serena` on PATH. Start Codex from this repository root so Serena selects it.
Preserve `user_id = "logic"` and `app_id = "optimus-be"` for mem0 reads/writes;
include `project = "optimus-be"` in checkpoint metadata. Update this file and
the current checkpoint after milestones.
Local `.serena/`, `.omo/`, `.worktrees/` and agent caches are ignored.
