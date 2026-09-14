# optimus-be

Optimus Go backend: authentication/RBAC, credential vault, Kubernetes inspection,
Helm applications, AWS assets, Prometheus metrics and immutable application delivery.

- Repository: https://github.com/ArkGravity/optimus-be
- Frontend: https://github.com/ArkGravity/optimus-fe
- [Conventions](AGENTS.md) · [Documentation](docs/README.md)

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
docker build -t local/optimus-be:dev .
docker compose up -d --build
docker compose ps -a
```

The image includes server, migrate, seed and vault-keygen binaries. Compose runs
PostgreSQL, migrations, seed and the backend, internal to its network. To expose
the UI, enable the optional `web` profile with a published frontend version:

```bash
FRONTEND_VERSION=main-<frontend-short-sha> docker compose --profile web up -d --build
```

For local frontend builds, build `local/optimus-fe:dev` in its own checkout,
then select `FRONTEND_IMAGE=local/optimus-fe FRONTEND_VERSION=dev`. Compose never
builds a sibling checkout. Web: http://127.0.0.1:8080. PostgreSQL is loopback-only.
Dev and Production are supported; UAT is skipped for this release.

For Production copy `.env.example` to `.env` and replace development credentials.
Set `COMPOSE_PROJECT_NAME=optimus-prod`, `IMAGE_REPOSITORY=ghcr.io/arkgravity`,
`BACKEND_VERSION=main-<backend-short-sha>`,
`FRONTEND_IMAGE=ghcr.io/arkgravity/optimus-fe`, and
`FRONTEND_VERSION=main-<frontend-short-sha>`. Versions are independent. Docker Hub
alternatives use `docker.io/logic379`. Set the HTTPS origin, external TLS proxy,
capacities and retention values.

```bash
docker compose --profile web pull
docker compose --profile web up -d --no-build
docker compose ps -a
docker compose logs seed
```

PostgreSQL/backend/frontend should be healthy; migrate/seed exit 0. Preserve the
Compose project name and `pgdata` volume when upgrading an existing stack.
Moving Compose does not require replacing the volume. Back up the database and
vault key; never use `docker compose down -v` on persistent data.

## CI and image publishing

[ci.yaml](.github/workflows/ci.yaml) runs quality and compilation checks before
Docker build. Pull requests and dev pushes build without publishing. Main pushes
and manual main runs publish the same build to both registries:

- `ghcr.io/arkgravity/optimus-be:main-<short-sha>`
- `docker.io/logic379/optimus-be:main-<short-sha>`

Configure Actions settings at repository or accessible organization scope:

| Type | Name | Value |
| --- | --- | --- |
| Variable | `DOCKERHUB_USERNAME` | Docker Hub login with write access to logic379 |
| Variable | `DOCKERHUB_NAMESPACE` | `logic379` |
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
