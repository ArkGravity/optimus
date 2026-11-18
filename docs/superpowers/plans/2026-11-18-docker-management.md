# Docker management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Docker placeholder with a three-section workspace that manages SSH-connected Docker hosts and live image/container operations over the remote Unix socket.

**Architecture:** Persist only `docker_hosts` metadata (migration 00021). Each live API call authenticates over SSH (existing SSH key via `credentials.Consumer`, or vault-encrypted password on the host row), dials the configured Docker socket, and uses the official Docker client. Frontend mirrors Kubernetes: section buttons in `Docker.vue`, shared HostPicker for Images/Containers.

**Tech Stack:** Go 1.25, GORM/PostgreSQL 17, Docker Engine API client, `golang.org/x/crypto/ssh`, Vue 3 + Ant Design Vue + Pinia, existing RBAC/audit/vault.

## Global Constraints

- New branch `feat/docker-management` from current `main`; never edit main directly.
- Next migration is **00021**; never rewrite published 00001–00020.
- Permissions originate only in `internal/infra/permissions/codes.go`; then `make dump-perms` and `make perm-check`.
- Client errors via `apperr.New`/`Wrap` in the fixed envelope; no secret or raw dialer text to clients.
- Keep zh-CN/en-US locale parity; English code comments.
- Use `bun` only for web dependency work.
- No Production deploy, release tag, or application DB recreation in this plan.
- Container delete is in scope; exec/logs/create/compose/pull are not.

Design: [docker-management-design](../specs/2026-11-18-docker-management-design.md).

## File map

| Path | Responsibility |
| --- | --- |
| `migrations/00021_create_docker_hosts.sql` | Host table, FK, checks |
| `internal/models/docker_host.go` | GORM model |
| `internal/modules/docker/host/` | Host CRUD, test, password vault |
| `internal/modules/docker/client/` | SSH dialer + Docker client factory |
| `internal/modules/docker/image/` | List/delete/prune |
| `internal/modules/docker/container/` | List/get/start/stop/restart/delete |
| `internal/modules/docker/module.go` | Wire routes |
| `internal/modules/credentials/sshkey/` | Refuse delete when live docker host references |
| `cmd/optimus/server.go` | Mount docker module |
| `web/src/views/containers/Docker.vue` | Section workspace |
| `web/src/views/containers/sections.ts` | Add dockerSections |
| `web/src/views/containers/docker/*` | Hosts/Images/Containers pages + HostPicker |
| `web/src/stores/docker.ts` | Selected host ID |
| `web/src/api/docker/*` | HTTP clients |
| `docs/workspace-tabs.md`, locales, Swagger, seed | Contracts |

---

### Task 1: Migration, model and permissions

**Files:**
- Create: `migrations/00021_create_docker_hosts.sql`
- Create: `internal/models/docker_host.go`
- Modify: `internal/infra/permissions/codes.go`
- Modify: locales for new permission name/description
- Test: `internal/modules/docker/host/repo_test.go` (or migration package race)

**Interfaces:**
- Produces: model `DockerHost`; permission `docker:environment:write`; table ready for FK from hosts to `credentials_ssh_keys`

- [ ] **Step 1: Write failing migration/repo test** expecting table `docker_hosts` and write permission registration after Up21.

- [ ] **Step 2: Add migration 00021** with columns from the design, soft delete, unique live name, check constraints for auth_type, FK `ssh_key_id` → `credentials_ssh_keys(id)`.

- [ ] **Step 3: Add GORM model** matching the migration.

- [ ] **Step 4: Register `docker:environment:write`** and update the read permission description away from “placeholder”. Run `make dump-perms` and `make perm-check`. Update en/zh permission strings.

- [ ] **Step 5: Run database race tests** for Up21/Down and permission registration; commit.

---

### Task 2: Host service, SSH-key in-use guard and dialer

**Files:**
- Create: `internal/modules/docker/host/{dto,repo,service,handler}.go`
- Create: `internal/modules/docker/client/dialer.go` (+ fakes for tests)
- Modify: `internal/modules/credentials/sshkey/service.go` (in-use counter seam)
- Create: docker host in-use counter (mirror kubeconfig/cloud-key pattern)
- Modify: `cmd/optimus/server.go` / credentials module wire
- Test: host CRUD races, dialer unit tests with mocked SSH/Docker, key-delete refusal

**Interfaces:**
- Consumes: `credentials.Consumer.GetSSHKey`, vault cipher for password_enc
- Produces: `HostService` CRUD + `Test(ctx,id)`; `Dial(ctx, host) (*client.Client, cleanup, error)`

- [ ] **Step 1: RED tests** for create ssh_key/password hosts, update without echoing password, delete, test-connection persistence of last_check_*, and SSH key delete refused when a live host references it.

- [ ] **Step 2: Implement host repo/service/handler** with audit on mutate; password seal/open; never return password or private key.

- [ ] **Step 3: Implement dialer** — SSH config from key or password; session + unix dial to `socket_path`; Docker client with HTTP over that connection; always cleanup; bounded timeout.

- [ ] **Step 4: Wire SSH key deletion guard** counting live `docker_hosts` by `ssh_key_id`; detach only soft-deleted hosts if that matches existing credential patterns.

- [ ] **Step 5: GREEN tests + focused lint**; commit.

---

### Task 3: Live image and container APIs

**Files:**
- Create: `internal/modules/docker/image/{dto,service,handler}.go`
- Create: `internal/modules/docker/container/{dto,service,handler}.go`
- Create: `internal/modules/docker/module.go`
- Modify: `cmd/optimus/server.go`
- Test: handler/service tests with fake Docker API

**Interfaces:**
- Consumes: host loader + dialer
- Produces: routes under `/api/v1/docker` as listed in the design, including `DELETE .../containers/:containerId`

- [ ] **Step 1: RED tests** for list images, delete image, prune; list/get containers; start/stop/restart/delete; permission middleware; safe errors when dial fails.

- [ ] **Step 2: Implement services** calling Docker API ImageList/Remove/Prune and ContainerList/Inspect/Start/Stop/Restart/Remove. Map to DTOs with short IDs and ports summary.

- [ ] **Step 3: Mount module** with nested permission groups (`read` vs `write`). Regenerate Swagger (`make swag`, `make swagger-diff`).

- [ ] **Step 4: GREEN tests + lint**; commit.

---

### Task 4: Frontend workspace, HostPicker and pages

**Files:**
- Modify: `web/src/views/containers/Docker.vue`, `sections.ts`
- Modify: `web/src/router/static-routes.ts`, `dynamic-routes.ts`, `AppSidebar` if needed for permissionsAny
- Create: `web/src/stores/docker.ts`
- Create: `web/src/views/containers/docker/{Hosts,Images,Containers,HostPicker}.vue` (+ tests)
- Create: `web/src/api/docker/*.ts`
- Modify: locales, `docs/workspace-tabs.md`, menu contract if needed
- Modify: seed only if menu name/component/path metadata must change (keep code `containers.docker`)

**Interfaces:**
- Consumes: `/api/v1/docker/*`
- Produces: three section UI with shared selectedHostId

- [ ] **Step 1: RED frontend tests** for section buttons, HostPicker no auto-select, write-gated buttons, container delete confirm.

- [ ] **Step 2: Implement Docker.vue sections** (hosts/images/containers), static routes, `dockerPermissions = [read]` for menu visibility (write gates controls only), Pinia store.

- [ ] **Step 3: Implement Hosts/Images/Containers pages** and API clients; bilingual strings; remove `docker_empty` placeholder usage.

- [ ] **Step 4: Update workspace-tabs.md** and seed permission grants for write on admin/editor via normal seed rules; run `bun run i18n:check`, vitest, lint, typecheck.

- [ ] **Step 5: Commit.**

---

### Task 5: End-to-end verification

**Files:**
- Possibly: `scripts/` smoke note or ops doc snippet for Docker SSH hosts
- Modify: release helper head fixture only if migration head checks require 00021

- [ ] **Step 1: Run Go** `go test` affected packages with `-race` and dbtest where applicable on Colima PostgreSQL.

- [ ] **Step 2: Run web** lint/typecheck/i18n/test/build.

- [ ] **Step 3: Run** `make swag swagger-diff dump-perms perm-check` and focused lint.

- [ ] **Step 4: Optional Colima smoke** — disposable SSH-able Docker host or documented skip if unavailable; record evidence in the plan status section.

- [ ] **Step 5: Stop** for operator review before commit/push/PR unless already authorized for that step.

---

## Out of scope reminders

- No container exec/logs/create; no compose; no image pull/push/build
- No Docker TCP/TLS without SSH
- No Production migrate/seed by the agent unless separately authorized
