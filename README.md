# optimus-fe

Optimus Vue 3 / TypeScript frontend with Ant Design Vue, Pinia, vue-router and
vue-i18n. Includes administration, credentials, Kubernetes, applications, AWS
assets, observability and application delivery.

- Repository: https://github.com/ArkGravity/optimus-fe
- Backend: https://github.com/ArkGravity/optimus-be
- [Conventions](AGENTS.md) · [Documentation/contracts](docs/README.md)

## Development

Use Bun 1.3.x. Builds/tests work from a standalone clone. Runtime API requests
require a backend; Vite proxies `/api/v1` to http://localhost:8080.

```bash
bun install --frozen-lockfile
bun run dev              # http://localhost:5173
bun run lint
bun run typecheck
bun run i18n:check
bun run test
bun run build            # dist/
```

Start the backend using its README, save the administrator password printed by
seed, then sign in through the frontend. Use Bun only. Maintain Chinese/English
locale parity and route/directive permission gates. CI runs every check above.

## Container

The Docker build context is this repository root:

```bash
docker build -t local/optimus-fe:dev .
docker run --rm -p 8081:80 \
  -e BACKEND_URL=http://host.docker.internal:8080 \
  local/optimus-fe:dev
```

Use an upstream reachable from the container; the host example depends on the
runtime's host DNS mapping. For a shared Docker network, attach `--network` and
use the backend service name. `BACKEND_URL` defaults to `http://optimus-be:8080`
and must be scheme/host/port without an API path or trailing slash.

nginx renders `nginx.conf` at startup using its official template mechanism and
proxies `/api/v1/` and `/swagger/`. It serves the SPA with gzip, security headers,
immutable hashed assets and an uncached HTML shell. Changing the upstream does
not require rebuilding. Do not put secrets in Vite build inputs.

The integrated Dev/Production Compose stack belongs to
[optimus-be](https://github.com/ArkGravity/optimus-be) and runs this image through
the optional `web` profile with `FRONTEND_VERSION`. Frontend and backend Git SHA
tags are independent. Local container work uses Colima.

## CI and image publishing

[ci.yaml](.github/workflows/ci.yaml) runs quality and compilation checks before
Docker build. Pull requests and dev pushes build without publishing. Main pushes
and manual main runs publish the same build to both registries:

- `ghcr.io/arkgravity/optimus-fe:main-<short-sha>`
- `docker.io/logic379/optimus-fe:main-<short-sha>`

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
