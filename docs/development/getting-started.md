# Developer setup

Read [AGENTS.md](../../AGENTS.md) for the project contract. Release users should use [installation](../installation.md).

## Requirements and layout

Use Go 1.25, Bun 1.3.x, Make, Colima and Docker CLI/Compose. Kubernetes and Helm are needed for application smoke tests. The Go module `github.com/ArkGravity/optimus` is at the repository root; the Vue 3/TypeScript/Ant Design Vue/Pinia/router/i18n frontend lives in `web/`. `web/embed.go` embeds Vite output into the single binary.

Use Colima for project-local containers and Kubernetes on macOS and Linux. Verify the selected runtime before integration tests:

```bash
colima start --runtime docker --kubernetes
docker context use colima
kubectl config use-context colima
colima status
docker info
kubectl cluster-info
docker compose version
docker compose up -d postgres
make deps
make migrate-up
make seed
make run
```

Run `make web-dev` in a second terminal: Vite serves `http://localhost:5173` and proxies `/api/v1` to the API on `:8080`. Seed prints the initial admin password once.

The checked-in YAML and source Compose defaults are for local Dev only. They intentionally align database/JWT/vault values so host commands work without additional exports. Host YAML uses `localhost` for PostgreSQL; Compose uses `postgres`. A native process does not load Compose's `.env`; export `OPTIMUS_*` overrides or edit YAML when changing host settings. Use unique secrets for shared databases and keep each vault key stable. Read [database baseline and recovery](../operations/recovery.md) before reusing an old database.

For tools that ignore Docker contexts, set `DOCKER_HOST` from the socket reported by `colima status`; do not assume a socket path. Use the Colima Kubernetes context and a disposable namespace for smoke tests. Leave the shared cluster running. Linux/WSL2 without `/dev/kvm` may use slower QEMU emulation.

## Build and check

```bash
make build
./bin/optimus version
./bin/optimus migrate -config configs/config.yaml -dir status
./bin/optimus server -config configs/config.yaml
make test
make test-int
make lint
make web-check
make swag
make swagger-diff
make dump-perms
make perm-check
```

`make build` builds the UI then `bin/optimus`. `make deps` installs the frozen Bun lockfile and pinned Go tools. A plain Go build without building `web/dist` embeds a placeholder UI; the API can still run. Keep `web/dist/.gitkeep` tracked after building.

Use Bun only for web dependency work. From `web/`, focused tests can use `bun x vitest run path/to/file.test.ts -t "name pattern"`. Focused Go tests can use `go test ./internal/modules/user/... -run TestService_Create -race`. Database variants need the `dbtest` tag and a disposable PostgreSQL server. See [CI](ci.md) for shared test-server isolation. Do not point test DSNs at an application server.

Go temporary files, build caches and lint caches belong under ignored `tmp/`; Make owns these defaults. If invoking Go directly, preserve those locations. With a newer host Go, use `GOTOOLCHAIN=go1.25.0 make lint`: golangci-lint v1.64.8 cannot read Go 1.27+ export data. Keep Go 1.25, Kubernetes client-go/apimachinery v0.30.14 and Helm v3.15.4 pinned.

## Source containers and web serving

`docker compose up -d --build` builds the complete source stack. Release deployments use an archive with no build block and `up -d --no-build`. Building web and Go together needs more memory than runtime; a Colima VM with 4 CPUs/8 GiB was sufficient, whereas 2 GiB failed during Vite build.

The server provides UI `/`, API `/api/v1` and Swagger `/swagger/index.html`. Hashed assets are immutable; index HTML is no-store; text assets have gzip variants and ETags. Known Assets pages and legal detail routes under `/assets/` get the uncached SPA shell. Missing hashed/dotted resources and unknown Assets paths return 404. Unknown `/api*` and `/swagger*` paths return JSON errors. Set `server.web_dir` / `OPTIMUS_SERVER_WEB_DIR` to use a disk build instead.

Trust no forwarded client IPs by default. For a TLS proxy, configure only its actual IP/CIDR through `OPTIMUS_SERVER_TRUSTED_PROXIES`, so rate limiting and audit use the correct client.

## Change contracts

Permission codes originate in `internal/infra/permissions/codes.go`; run permission generation/checks after changes. Handler annotation/API changes require `make swag` and `make swagger-diff`. Keep both locales in parity and frontend paths lowercase/kebab-case. Update the reviewed [menu fixture](../../web/src/test/fixtures/menu-contract.json) with backend menu changes; it is not a runtime source.

The [workspace contract](../workspace-tabs.md) governs retained navigation state. [Branding](../branding.md) identifies the shared logo. Keep credentials behind the existing consumers and record mutations through the shared audit recorder. Current migrations end at 00019; append 00020 for a new schema change, and do not edit published migrations.

Local agent setup is optional: `.codex/config.toml` contains project MCP configuration. Export the required `MEM0_API_KEY` and `CONTEXT7_API_KEY`, install `serena` on PATH and open the repository root. Keep credentials and local agent caches outside Git and container build contexts.
