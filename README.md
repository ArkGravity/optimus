# optimus-be

Optimus Go backend: authentication/RBAC, credential vault, Kubernetes inspection,
Helm applications, AWS assets, Prometheus metrics and immutable application delivery.

- Repository: https://github.com/ArkGravity/optimus-be
- Frontend: https://github.com/ArkGravity/optimus-fe
- [Conventions](AGENTS.md) · [Documentation](#documentation)

## Development

Requirements: Go 1.25, Make, Colima, Docker CLI/Compose. Kubernetes/Helm are
required for delivery smoke tests. Run commands from this repository root.

```bash
colima start --runtime docker --kubernetes
docker context use colima
docker compose version
docker compose up -d postgres
make tools
export OPTIMUS_JWT_SECRET="$(openssl rand -base64 48)"
export OPTIMUS_VAULT_MASTER_KEY="$(go run ./cmd/vault-keygen)"
make migrate-up
make seed
make run
```

Save the vault key securely and reuse it for the same database. Seed prints the
initial administrator password once. API: http://localhost:8080.
`GET /api/v1/health` returns the raw health probe. Start the frontend separately;
no frontend checkout is required to build or test this repository.

Defaults live in `configs/config.yaml`, overridden by `OPTIMUS_*` environment
variables. JWT needs 32+ bytes; a vault master key or key file is required.
Compose reads `.env`; host Go commands use exported variables. The Compose DSN
uses `postgres`, while host development uses `localhost`.

```bash
make build           # bin/optimus-be
make test            # unit/race
make test-int        # disposable PostgreSQL on Colima
make lint
make swag            # api/docs/* and docs/api/swagger.json
make swagger-diff
make dump-perms       # docs/permissions.md
make perm-check
```

Temporary files and build/lint caches stay under ignored `tmp/`. Keep Go 1.25,
Kubernetes v0.30.14 and Helm v3.15.4 pinned. Permission codes originate in
`internal/infra/permissions/codes.go`. CI also runs gosec and integration tests.

## Containers and deployment

```bash
docker compose up -d --build
docker compose ps -a
```

The image includes server, migrate, seed and vault-keygen binaries. Compose runs
PostgreSQL, migrations, seed, the backend and the frontend by default; no profile
flag is needed. The backend is internal to the Compose network. Compose builds
the frontend from `../optimus-fe` and tags it `local/optimus-fe:dev` by default.

No separate frontend build command is needed. Set `FRONTEND_BUILD_CONTEXT` in
`.env` or the shell if the frontend checkout is elsewhere; relative paths resolve
from this Compose file's directory. Backend-only builds and tests remain independent
of the frontend checkout. Web: http://127.0.0.1:8080. PostgreSQL is loopback-only.
Dev and Production are supported; UAT is skipped for this release.

For host development, start only the database with `docker compose up -d postgres`,
then run the backend and frontend development servers separately.

For Production copy `.env.example` to `.env` and replace development credentials.
Set `COMPOSE_PROJECT_NAME=optimus-prod`, `IMAGE_REPOSITORY=ghcr.io/arkgravity`,
`BACKEND_VERSION=main-<backend-short-sha>`,
`FRONTEND_IMAGE=ghcr.io/arkgravity/optimus-fe`, and
`FRONTEND_VERSION=main-<frontend-short-sha>`. Versions are independent. Docker Hub
alternatives use `docker.io/logic3579`. Set the HTTPS origin, external TLS proxy,
capacities and retention values.

Published-image deployments use `pull` and `up --no-build` below and do not
require a frontend checkout. Existing `.env` image overrides take precedence
over the local image defaults.

```bash
docker compose pull
docker compose up -d --no-build
docker compose ps -a
docker compose logs seed
```

PostgreSQL/backend/frontend should be healthy; migrate/seed exit 0. Preserve the
Compose project name and `pgdata` volume when upgrading an existing stack.
Moving Compose does not require replacing the volume. Back up the database and
vault key; never use `docker compose down -v` on persistent data.

### Linux Docker MTU troubleshooting

Verified on a remote Linux server: image builds stalled at `apk add` while
downloading Alpine indexes. Host and host-network downloads worked, but bridge
containers timed out; lowering container MTU from 1500 to 1460 restored downloads.
Persisting `mtu: 1460` and
`default-network-opts.bridge.com.docker.network.driver.mtu: "1460"` in Docker's
`/etc/docker/daemon.json`, then restarting Docker, resolved the issue. Merge these
settings into existing configuration; use an MTU verified for the server's network.
Existing Compose networks need recreation to adopt new defaults: preserve the
Compose project name and data volumes, and never use `down -v` for this repair.

### Release acceptance

Runtime checklists: [P4 assets](scripts/p4-smoke.md),
[P5 observability](scripts/p5-smoke.md), and [P6 delivery](scripts/p6-smoke.md).
Local acceptance passed before the 2026-09-14 repository split. Production still
requires persistent-data upgrade validation from original revision `4e2d08b`
through migration `00023_p6_delivery.sql`, production acceptance and release
tagging. Repository migration is not production sign-off.

## Documentation

This repository owns the generated [API specification](docs/api/swagger.json),
[permission catalog](docs/permissions.md), and backend and shared P0-P6
[specifications](docs/superpowers/specs/) and [plans](docs/superpowers/plans/).
`make swag` and `make dump-perms` update files inside this repository only.

Pure frontend P0 plans/addenda live in the
[frontend docs](https://github.com/ArkGravity/optimus-fe/tree/main/docs/superpowers).
Shared designs stay here as a single source, linked from frontend documentation.

Historical designs retain monorepo paths and commit IDs. `optimus-be/` means
this root; `optimus-fe/` refers to the separate frontend repository. Old `deploy/`
paths map to this root's Compose/Dockerfile or the frontend's Dockerfile/nginx.conf.
Use current README/AGENTS commands; old steps are implementation history.

## CI and image publishing

[ci.yaml](.github/workflows/ci.yaml) runs quality and compilation checks before
Docker build. Pull requests and dev pushes build without publishing. Main pushes
and manual main runs publish the same build to both registries:

- `ghcr.io/arkgravity/optimus-be:main-<short-sha>`
- `docker.io/logic3579/optimus-be:main-<short-sha>`

Configure Actions settings at repository or accessible organization scope:

| Type | Name | Value |
| --- | --- | --- |
| Variable | `DOCKERHUB_USERNAME` | Docker Hub login with write access to logic3579 |
| Variable | `DOCKERHUB_NAMESPACE` | `logic3579` |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub access token with write permission |

GHCR uses `GITHUB_TOKEN` with `packages: write`; permit Actions package creation
in the organization. Check package visibility separately for anonymous pulls.
GitHub does not allow reading back old Actions secrets: configure the token for
this repository, then rerun Actions → ci → Run workflow on main. Missing
credentials fail the main publishing job with an explicit configuration error.

## Codex

`.codex/config.toml` contains project MCP configuration. Export `MEM0_API_KEY`
and `CONTEXT7_API_KEY`, install `serena` on PATH, and open this repository as
the project. Credentials and local agent caches are excluded from Git and Docker.
Configuration follows the [official reference](https://learn.chatgpt.com/docs/config-file/config-reference).

### CI execution and database tests

CI caches Go build artifacts under `tmp/go-cache`, with separate keys per job.
Docker validation runs alongside quality, unit, and database checks; publishing
on main requires all four jobs to succeed. Changes limited to README.md,
AGENTS.md, or Markdown under docs/ and scripts/ skip database execution and image
build/publication. Quality and unit checks still run. Manual dispatch and unknown
change history always run the full checks.

The database job starts one disposable PostgreSQL 16 instance and sets
`OPTIMUS_TEST_POSTGRES_DSN`. Each test creates a randomly named database, runs all
migrations, and drops only its own database during cleanup. This connection must
point to a dedicated test server with CREATE DATABASE privileges, never a shared
application database server. Without the variable, local tests retain dockertest.
`make test-int` retains race detection and disables test-result caching with
`-count=1`; compiled artifacts remain cached.
