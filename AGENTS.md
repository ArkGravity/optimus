# Optimus Frontend Project Guide

## First Read

Read this file and the active design/plan before substantial changes. See
`docs/README.md` for document ownership and historical path conventions.
This is the sole project operating-contract file. Keep code comments in English.
Use mem0 with `user_id = "logic"`, `metadata.project = "optimus-fe"` and
`metadata.app_id = "optimus-fe"` on every write. Filter project reads by all
three fields and prefix memory text with
`[optimus-fe]`; keep backend checkpoints under `optimus-be`. Cross-check
checkpoints with Git status/history.
Never require a sibling checkout for builds, tests, or generated artifacts.

## Status

P0-P6 are implemented. Local P3/P4/P5/P6 acceptance passed before the 2026-09-14
repository split from `logic3579/optimus` (`17a3862`, tree identical to `cdfb4f3`).
Production acceptance, the persistent-data upgrade smoke from `4e2d08b` through
`00023_p6_delivery.sql`, and release tagging remain outstanding. Dev and Production
are the selected environments; UAT is skipped. Pre-split commit IDs refer to the
original repository. Split histories have new commit IDs.

Verified on 2026-09-15 against local and remote main `b43f605`:
GitHub Actions run `34901419162` passed frontend checks, Docker build and image
publishing to GHCR and Docker Hub. This does not establish production acceptance
or the backend persistent-data upgrade smoke. The general `/dashboard` page is
still a coming-soon placeholder; P5 observability dashboards are implemented.

## Commands

Run from this repository root:

- `bun install`
- `bun run dev`
- `bun run lint`
- `bun run typecheck`
- `bun run i18n:check`
- `bun run test`
- `bun run test:watch`
- `bun run build`

Use `bun` only for frontend dependency work. Do not use npm, pnpm, or yarn.
Run a focused frontend test with
`bun x vitest run path/to/file.test.ts -t "name pattern"`.


## Frontend Architecture

- Bootstrap order is Pinia, Ant Design Vue, i18n, API client, provided module
  APIs, router guards, then mount.
- Static routes contain login/error/profile pages and application, asset and
  delivery detail/action sub-routes. On the first authenticated
  navigation, fetch `/me`, menus, and permissions in parallel, register dynamic
  routes, then replace-navigate to the original destination.
- Permission enforcement has two synchronized layers: route
  `meta.permission` and the `v-permission` directive. Both read the Pinia auth
  permission state; components must not re-fetch permissions.
- The API client validates the fixed envelope and converts nonzero codes to
  `BizError`. Concurrent HTTP and SSE 401 responses share the store's
  single-flight refresh promise; each original request is replayed at most once.
- Locale files are `src/locales/zh-CN.json` and `src/locales/en-US.json`.
  `bun run i18n:check` enforces parity. The Vite alias `@/*` maps to `src/*` and
  the local `/api/v1` proxy targets `http://localhost:8080`.

## UI and API Contracts

- Vue 3, Ant Design Vue, Pinia, vue-router, vue-i18n; use Bun only.
- All application requests use `/api/v1/*` and the fixed backend envelope.
  API and permission references are owned by the backend repository; see docs.
- Keep zh-CN/en-US i18n parity and Linux-compatible component paths.
- Use `ClusterPicker` and the Kubernetes store for cluster selection. An absent
  selection shows a prompt; never auto-select or redirect.
- Kubernetes inspection is read-only. Secret data reveal requires
  `k8s:secret:reveal`. Pod logs use fetch/ReadableStream SSE with JWT headers.
- P4 paths are lowercase/kebab-case. Manual asset sync is asynchronous.
- P5 is metrics only: no alerts, CloudWatch, logs, traces, or APM. Metric-only
  operators use the minimal query-source endpoint. Data-source, dashboard and
  metric permissions are independent. Preserve abort generations against stale
  definition/query responses; never log secrets, headers or full PromQL.
- P6 promotes immutable charts through ordered environments. Do not offer raw
  commands, manifests, values or image editors. Show frozen artifacts, exact
  action permissions and safe errors. Initiators cannot approve their own runs.
  Project, pipeline, run and approval permissions remain independent. Direct P3
  upgrade/uninstall of delivery-managed applications stays denied.
- SSE projections must not expose values, kubeconfigs, manifests, Helm notes,
  authorization headers or raw executor errors. Logout resets active streams.
- `src/test/fixtures/menu-contract.json` records the supported backend menu
  contract. Update it with reviewed backend menu changes and run route/casing
  tests. Unit tests must not import files from a backend checkout.

## Build, Runtime and Release

- GitHub: https://github.com/ArkGravity/optimus-fe
- `Dockerfile` and `nginx.conf` build an independent SPA image. `BACKEND_URL`
  configures nginx's upstream at container startup (default http://optimus-be:8080).
- Host development uses Vite on port 5173 and backend localhost:8080.
- Integrated Compose deployment is owned by optimus-be. Each image has its own
  `main-<short-sha>` version; do not assume matching frontend/backend Git SHAs.
- CI runs lint, typecheck, i18n, tests, build and Docker build. Only main publishes
  to both GHCR and Docker Hub. See README for required registry settings.
- Use Colima for local Docker/Kubernetes, select `colima` contexts and check
  `colima status`, `docker info`, `docker compose version`, `kubectl cluster-info`
  before container smoke work. Never stop/delete the shared cluster for cleanup.
- Run P3-P6 smoke checklists from the backend repo with disposable resources.

## Codex Environment

`.codex/config.toml` configures mem0 and Context7 over HTTP and local Serena.
Export `MEM0_API_KEY` and `CONTEXT7_API_KEY` before starting Codex; install
`serena` on PATH. Start Codex from this repository root so Serena selects it.
Preserve `user_id = "logic"`, `metadata.project = "optimus-fe"` and
`metadata.app_id = "optimus-fe"` for mem0 reads/writes. The `app_id` field belongs
inside the memory metadata. When updating metadata, read and preserve existing
fields, send the complete merged object, and verify the saved result.
Update this file and the current checkpoint after milestones.
Local `.serena/`, `.omo/`, `.worktrees/` and agent caches are ignored.
