# optimus

Optimus infrastructure console: authentication/RBAC, credential vault,
Kubernetes inspection, Helm applications, and AWS assets. The Go backend and the Vue 3 web UI ship as one
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
Kubernetes/Helm are required for application smoke tests. Run commands from the
repository root.

```bash
colima start --runtime docker --kubernetes
docker context use colima
docker compose version
docker compose up -d postgres
make deps
make migrate-up
make seed
make run        # API on http://localhost:8080 via go run
make web-dev    # Vite on http://localhost:5173, proxies /api/v1 to :8080
```

Seed prints the initial administrator password once. Use the Vite server for UI
development; `GET /api/v1/health` returns the raw health probe.

Defaults live in `configs/config.yaml`, overridden by `OPTIMUS_*` environment
variables. The checked-in JWT secret and vault master key match the Compose
development defaults, so host `make run` and `make seed` start without extra
exports. Replace both before any shared or production database, and keep the
vault key stable for that database. Compose reads `.env` when present. The
Compose DSN uses `postgres`, while host development uses `localhost`.

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
CI also runs integration tests.

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
UI paths fall back to `index.html`. Under `/assets/`, known Assets menu pages
and legal detail URLs receive the same uncached index shell; missing built,
hashed or dotted resources and unknown paths return 404. Unknown `/api/*` and
`/swagger*` paths return the JSON error envelope. Set `server.web_dir`
(`OPTIMUS_SERVER_WEB_DIR`) to serve a web build from disk instead of the
embedded copy. A plain `go build` without
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

New CI image tags include `linux/amd64` and `linux/arm64` under the same tag in
both registries. Docker automatically selects the platform matching its daemon;
Apple Silicon Macs using an ARM64 Colima VM use `linux/arm64`. Older tags
published before dual-platform support remain `linux/amd64` only.
CI builds both platforms before publishing. Bun builds the shared web assets
on the builder's native platform, Go cross-compiles with CGO disabled, and QEMU
handles foreign-platform runtime package installation. Build provenance may
appear as additional `unknown/unknown` entries; these are metadata, not runnable
platforms.
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

The supported production feature scope is P0–P4: platform administration,
credential vault, Kubernetes inspection, Helm application management, and AWS
assets. On 2026-10-28 the operator confirmed that the production database was
rebuilt, the latest image was deployed, and all P0–P4 functions passed production
acceptance. Release tagging remains outstanding.

Observability Center retains one Coming Soon navigation entry. Its business
functions are unavailable and excluded from production functional acceptance.
Reconsider implementation after the other Optimus functions mature.

Runtime checklists (Dev / disposable resources only):
[P3 applications](scripts/p3-smoke.md), [P4 AWS assets](scripts/p4-smoke.md)
and [P4 multi-cloud assets](scripts/p4-multicloud-smoke.md).
Do not point those checklists at production credentials or clusters.

### Database baseline

The operator confirmed rebuilding, deploying and accepting `d0016d0` on
2026-10-28. This accepted production baseline ends at `00022`, including the
kubeconfig lifecycle fix in `00015_create_clusters.sql`, cloud-key deletion in
`00022_cloud_key_deletion.sql`, and the Aliyun/AWS/GCP provider constraint in
`00014_create_credentials_cloud_keys.sql`. Asset discovery in the accepted production baseline is AWS-only.

The unshipped `feat/multicloud-assets` source now uses the operator-approved
fresh migration baseline: exactly 19 sequential SQL files, ending at
`00019_create_assets_sync_runs.sql`; the next migration is `00020_xxx.sql`.
It retains all 31 business tables, final multi-cloud schemas and runtime behavior.
Typed credentials, account scopes, AWS/Aliyun/GCP inventory and sync-run metadata
are defined directly in their owning CREATE migrations. Historical migration
numbers above describe the last accepted production revision, not current files.
Production rollout and real-provider acceptance remain pending.

This baseline requires a fresh PostgreSQL database. Existing schema 00022,
00023 or 00028 databases cannot upgrade in place with these renumbered files.
For a separately authorized Dev/Production rollout, back up required data and
retain the vault master key, stop application writers, and actually recreate the
target database (or remove all its objects, including `goose_db_version`) before
running `make migrate-up` and `make seed`, or the image's one-shot services.
The resulting fresh database has 31 business tables plus `goose_db_version` at
version 19. Recreating containers alone retains the PostgreSQL `pgdata` volume
and its old migration history. This source change does not perform a database
recreation, deployment or data restoration.

Seed renames the built-in Security Groups menu to Firewalls while preserving its
row ID, route ownership and existing permission grants and custom child references; repeated seed does not duplicate it.
The menu order is Cloud Accounts, VPCs, Firewalls, Instances, Databases, Sync Runs.
The legacy `/assets/security-groups` browser URL redirects under the same resource
permission; the legacy AWS API remains available. Existing permissions are reused.

Compatibility for legacy GCP AK/SK records remains: they use `auth_type=legacy_invalid` and show
`needs_configuration`; repair them in Cloud Keys with a valid Service Account
JSON before collection. AWS/Aliyun use explicit AK/SK; GCP uses only the selected
Service Account JSON and an explicit target Project ID, which may differ from the
credential's home project. Provider, bound credential and target project are
immutable on Cloud Accounts; create a new account to change that binding. Rotate
secret material on the existing credential when retaining its account binding.
Deleting a cloud key is blocked (`43001`) while any live cloud account references
it, including disabled accounts. Delete the account first; only historical
references are detached during physical credential deletion and its atomic audit.

Downgrade is not a rollout recovery procedure: the fresh baseline's Down
operations drop tables and their data, including credentials once dependants
have been removed. The former intermediate SA-auth downgrade guard no longer
applies. Use a tested backup with its matching application revision and vault
key for recovery.

### Multi-cloud collection settings

| YAML key | Environment variable | Default |
|---|---|---|
| `assets.request_timeout` | `OPTIMUS_ASSETS_REQUEST_TIMEOUT` | `30s` |
| `assets.aws_request_timeout` | `OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT` | `30s` |
| `assets.sync_timeout` | `OPTIMUS_ASSETS_SYNC_TIMEOUT` | `15m` |

Timeouts accept positive Go durations; malformed, overflowing, zero and negative
values fail configuration loading or startup validation. The independent AWS
override applies to identity planning, authenticated region catalogs and resource
requests: AWS `60s` remains effective with general `30s`. The general request
setting applies to Aliyun/GCP. Each call also respects its parent cancellation and
remaining unit/account deadline. Individual unit budgets stay bounded at 15m;
`sync_timeout` is an account ceiling and may exceed 15m. The worker sums actual
planned unit budgets with overflow protection and subtracts planning elapsed time;
GCP plans global networks/firewalls, regional subnets/databases and discovered
zones, rather than assuming four units per region. Manual sync returns immediately
and performs identity/zone planning in the worker. Shared cron, startup delay and
run-retention settings keep their existing names and behavior.

Google clients use fixed official Google universe/endpoints and explicit selected
credentials. Ambient client-certificate discovery is unsupported:
`GOOGLE_API_USE_CLIENT_CERTIFICATE` must be unset or case-insensitive `false`;
`true` and unknown values fail with Config before SDK discovery. The application
does not mutate global environment variables. Supported explicit SDK logging
suppresses raw debug payloads. Never include SA JSON, JWTs, AK/SK or raw provider
responses in logs, screenshots or bug reports.

Provider failures use fixed sanitized messages and codes: `43201` Unauthorized,
`43202` Forbidden, `43203` Unreachable, `43204` Throttled, `43205` Config,
`43206` Partial, `43207` APIDisabled, `43208` Other and `43209` IdentityMismatch.
AWS `43102`–`43107` remain compatible. Provider Unauthorized is a domain failure,
not an application JWT 401; OAuth acquisition and target API denial remain distinct.
Incomplete units preserve the preceding snapshot and cannot delete missing rows.

Use the [AWS checklist](scripts/p4-smoke.md) and
[multi-cloud checklist](scripts/p4-multicloud-smoke.md) with separately supplied
disposable read-only credentials. Fixture tests establish local behavior; real
AWS Security Groups, Aliyun and GCP acceptance remains pending. The accepted
production baseline remains `d0016d0`/00022.

## Documentation

The repository owns the generated [API specification](docs/api/swagger.json),
[permission catalog](docs/permissions.md), and the P0–P4
[specifications](docs/superpowers/specs/) and [plans](docs/superpowers/plans/).
See [docs/README.md](docs/README.md) for how historical documents map to the
current layout.

## CI and image publishing

[ci.yml](.github/workflows/ci.yml) runs web checks, backend quality, unit and
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
