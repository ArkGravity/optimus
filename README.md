# optimus

Optimus infrastructure console: authentication/RBAC, credential vault,
Kubernetes inspection, Helm applications, AWS assets, Prometheus metrics and
immutable application delivery. The Go backend and the Vue 3 web UI ship as one
`optimus` binary with the UI embedded.

- Repository: https://github.com/ArkGravity/optimus
- [Conventions](AGENTS.md) · [Documentation](docs/README.md)

## Layout

- Repository root: Go module `github.com/ArkGravity/optimus` (`cmd/`,
  `internal/`, `migrations/`, `configs/`, `tests/`).
- `web/`: Vue 3, TypeScript, Ant Design Vue, Pinia, vue-router and vue-i18n,
  built with Bun. `web/embed.go` embeds the Vite output `web/dist`.
- `docs/`: API specification, permission catalog, designs and plans.

## Development

Requirements: Go 1.25, Bun 1.3.x, Make, Colima, Docker CLI/Compose.
Kubernetes/Helm are required for delivery smoke tests. Run commands from the
repository root.

```bash
colima start --runtime docker --kubernetes
docker context use colima
docker compose version
docker compose up -d postgres
make tools
make web-install
export OPTIMUS_JWT_SECRET="$(openssl rand -base64 48)"
export OPTIMUS_VAULT_MASTER_KEY="$(go run ./cmd/optimus vault-keygen)"
make migrate-up
make seed
make run        # API on http://localhost:8080 with air hot reload
make web-dev    # Vite on http://localhost:5173, proxies /api/v1 to :8080
```

Save the vault key securely and reuse it for the same database. Seed prints the
initial administrator password once. Use the Vite server for UI development;
`GET /api/v1/health` returns the raw health probe.

Defaults live in `configs/config.yaml`, overridden by `OPTIMUS_*` environment
variables. JWT needs 32+ bytes; a vault master key or key file is required.
Compose reads `.env`; host commands use exported variables. The Compose DSN
uses `postgres`, while host development uses `localhost`.

```bash
make build        # web build, then bin/optimus with the UI embedded
make test         # Go unit/race
make test-int     # PostgreSQL 17 via dockertest on Colima
make lint
make web-check    # web lint, typecheck, i18n parity and unit tests
make swag         # api/docs/* and docs/api/swagger.json
make swagger-diff
make dump-perms   # docs/permissions.md
make perm-check
```

Go temporary files and build/lint caches stay under ignored `tmp/`. Keep Go
1.25, Kubernetes v0.30.14 and Helm v3.15.4 pinned. Permission codes originate in
`internal/infra/permissions/codes.go`. Use Bun only for web dependencies.
CI also runs gosec and integration tests.

## Single binary

```bash
make build
./bin/optimus migrate -config configs/config.yaml
./bin/optimus seed -config configs/config.yaml
./bin/optimus -config configs/config.yaml    # same as `optimus server`
./bin/optimus vault-keygen
```

The server exposes the web UI at `/`, the API at `/api/v1` and Swagger at
`/swagger/index.html` on one port. Hashed `/assets/*` are cached as immutable,
`index.html` is never cached, text assets are served gzip-compressed, unknown
UI paths fall back to `index.html` and unknown `/api/*` paths return the JSON
error envelope. Set `server.web_dir` (`OPTIMUS_SERVER_WEB_DIR`) to serve a web
build from disk instead of the embedded copy. A plain `go build` without
`make web` embeds only a placeholder and the server reports that the UI is not
built; the API is unaffected.

The server trusts no `X-Forwarded-For` by default. Behind a TLS reverse proxy,
set `server.trusted_proxies` (`OPTIMUS_SERVER_TRUSTED_PROXIES`, comma-separated
IPs/CIDRs) so login rate limiting and audit logs see real client addresses.

## Containers and deployment

```bash
docker compose up -d --build
docker compose ps -a
```

The image contains the single `optimus` binary. Compose runs PostgreSQL 17,
then `migrate` and `seed` as one-shot services, then the `optimus` server, all
from the same image. Web: http://127.0.0.1:8080 (`HTTP_BIND`/`HTTP_PORT`).
PostgreSQL is loopback-only. Dev and Production are supported; UAT is skipped.

For host development, start only the database with `docker compose up -d postgres`,
then run the server and the Vite dev server separately.

For Production copy `.env.example` to `.env` and replace development credentials.
Set `COMPOSE_PROJECT_NAME=optimus-prod`, `IMAGE_REPOSITORY=ghcr.io/arkgravity`
(or `docker.io/logic3579`) and `OPTIMUS_VERSION=main-<short-sha>`. Set the HTTPS
origin, trusted proxies of the external TLS proxy, capacities and retention
values. Published-image deployments do not need a source checkout:

```bash
docker compose pull
docker compose up -d --no-build
docker compose ps -a
docker compose logs seed
```

Published images are linux/amd64 only; on arm64 hosts add
`--platform linux/amd64` or build locally with `docker compose up -d --build`.
PostgreSQL and optimus should be healthy; migrate/seed exit 0. Preserve the
Compose project name and `pgdata` volume when upgrading an existing stack. Back
up the database and vault key; never use `docker compose down -v` on persistent
data.

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

Runtime checklists: [P3 applications](scripts/p3-smoke.md),
[P4 assets](scripts/p4-smoke.md), [P5 observability](scripts/p5-smoke.md) and
[P6 delivery](scripts/p6-smoke.md). The project is in Dev acceptance and not yet
used in production. Production acceptance and release tagging remain
outstanding.

## Documentation

The repository owns the generated [API specification](docs/api/swagger.json),
[permission catalog](docs/permissions.md), and the P0-P6
[specifications](docs/superpowers/specs/) and [plans](docs/superpowers/plans/).
See [docs/README.md](docs/README.md) for how historical documents map to the
current layout.

## CI and image publishing

[ci.yaml](.github/workflows/ci.yaml) runs web checks, backend quality, unit and
database tests, and a Docker build. Pull requests and dev pushes build without
publishing. Main pushes and manual main runs publish the same build to both
registries once every job passes:

- `ghcr.io/arkgravity/optimus:main-<short-sha>`
- `docker.io/logic3579/optimus:main-<short-sha>`

Configure Actions settings for the repository:

| Type | Name | Value |
| --- | --- | --- |
| Secret | `DOCKERHUB_TOKEN` | Docker Hub access token with write permission (required) |
| Variable | `DOCKERHUB_USERNAME` | Docker Hub login; optional, defaults to `logic3579` |
| Variable | `DOCKERHUB_NAMESPACE` | Docker Hub namespace; optional, defaults to `logic3579` |

GHCR uses `GITHUB_TOKEN` with `packages: write`. Check package visibility
separately for anonymous pulls. Missing credentials fail the main publishing job
with an explicit configuration error.

### CI execution and database tests

CI caches Go build artifacts under `tmp/go-cache`, with separate keys per job.
Changes limited to README.md, AGENTS.md, or Markdown under docs/ and scripts/
skip database execution and image build/publication. Web, quality and unit
checks still run. Manual dispatch and unknown change history always run the
full checks.

The database job starts one disposable PostgreSQL 17 instance and sets
`OPTIMUS_TEST_POSTGRES_DSN`. Each test creates a randomly named database, runs all
migrations, and drops only its own database during cleanup. This connection must
point to a dedicated test server with CREATE DATABASE privileges, never a shared
application database server. Without the variable, local tests retain dockertest.
`make test-int` retains race detection and disables test-result caching with
`-count=1`; compiled artifacts remain cached.

## Codex

`.codex/config.toml` contains project MCP configuration. Export `MEM0_API_KEY`
and `CONTEXT7_API_KEY`, install `serena` on PATH, and open this repository as
the project. Credentials and local agent caches are excluded from Git and Docker.
Configuration follows the [official reference](https://learn.chatgpt.com/docs/config-file/config-reference).
