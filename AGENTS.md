# Optimus Project Guide

## First Read

Read this file and the active design/plan before substantial changes. See
[docs/README.md](docs/README.md) for document ownership and the mapping from
historical paths to the current layout. This is the sole project
operating-contract file. Keep code comments in English.
Unless the operator explicitly instructs otherwise, implement code changes on
a new branch or worktree; never edit code directly on main.
Use mem0 with `user_id = "logic"` and native `app_id = "optimus"`. Pass both
as top-level fields when writing; when searching or listing, include both in
`filters`, combined with AND. Do not duplicate project scope in metadata or add
project-name prefixes to memory text. Omit metadata by default; add only
essential information not already represented by native fields or memory text.
Cross-check checkpoints with Git status/history.

## Status

Current GKE delivery (2026-11-13): native GKE authentication on
`feat/gke-authentication`, based on `e9d7a4c`, using existing GCP Service Account
Cloud Keys. Active design/plan: `docs/superpowers/specs/2026-11-13-gke-authentication-design.md`
and `docs/superpowers/plans/2026-11-13-gke-authentication.md`. New source migration
00020 appends GKE credential bindings to published head19 without replacing any
published migration; next new migration is 00021. Implementation commit
`709db86e80935ded60e1215db63b84ecfa6c3031` was pushed to the feature branch.
The operator now confirms successful Prod verification of this branch and
explicitly authorizes PR creation and merge to main. Release publication and
agent-operated deployment are outside this request. Earlier v0.1.2 Prod
acceptance steps 1–6 remain confirmed; observation/backup sign-off (step 8)
has not been reported.
Local GKE implementation is complete: full Go race/database suite, frontend
321 tests/checks/build, focused normal/tagged lint, generated checks, 44 release
helper tests and actual embedded binary fixture smoke passed. All 19 published
migration bytes were preserved; main remained unchanged during implementation.
The dedicated test container,
anonymous volume and smoke database/server were removed; all 12 original shared
Kubernetes containers and the existing PostgreSQL image were retained. Operator
Prod verification is separate from these local fixture results. No release or
agent-operated Prod action was performed.

Current delivery (2026-11-12): **v0.1.2 is published and independently verified**
as [GitHub Release `443492677`](https://github.com/ArkGravity/optimus/releases/tag/v0.1.2)
at `2026-11-12T21:10:00Z`. Selected source is
`c610ee11de8ad3a889f784897d9eaa534ce22f2a`; immutable annotated tag object is
`d6c9f82e01fb577960eae14f52e4e7bdb24deef9`. The user explicitly authorized repair,
formal publication and comprehensive post-publication checks.

The RBAC group expectation, persisted timestamp comparison and timing-sensitive
cache TTL test are fixed. Exact-source main CI `43301709420` passed all six
executed jobs. Release `43303096304` attempt 2 passed all four jobs; both native
build/runtime jobs from attempt 1 were reused after the publisher's initial
draft read-back failure. Independently downloaded six attachments passed every
checksum/archive/config/license/INSTALL check and binary equality with saved
tested images. Anonymous GHCR/Docker Hub reads confirmed identical amd64/arm64
index/platform/configuration digests and exact source/version labels. Shared
index: `sha256:9ca79bf2a2f038270b17e95cdaee0510621cbee05ffb022901049ecb9b7457e9`.

After publication, anonymous latest/public downloads preserved all six original
asset IDs, sizes and hashes, and registry digests remained unchanged. Actual
published arm64 image/native/source-free Compose smoke passed, including the new
container menus, permissions and routes. Owned test resources/downloads were
removed, retaining six shared Kubernetes containers and all nine original image
IDs. The pre-existing development PostgreSQL container was absent at final
inventory; this task did not target it for deletion or modify its database.
Existing published versions were not replaced; no Production operation occurred.
Full evidence: `docs/superpowers/plans/2026-11-12-v0.1.2-release.md`.

Previous source delivery (2026-11-12): **v0.1.1 is published** as
[GitHub Release `443447913`](https://github.com/ArkGravity/optimus/releases/tag/v0.1.1)
at `2026-11-12T20:04:08Z`. The user accepted the container-navigation work and
authorized direct formal publication without waiting for or reviewing CI or
release artifacts. This is a source-only Release; no binary attachments or
versioned-image availability is asserted. Source main commit is
`bdab35a1beef238f0738d66a100e54dbf6481b77`; immutable annotated tag object is
`442c8100757b424364c5495820f13c2b00042c0c`. Existing release workflow gates remain
unchanged. No Production deployment or application database operation occurred.
See `docs/superpowers/plans/2026-11-12-v0.1.1-release.md`.

Container navigation work (2026-11-12): the user requested removal of global
workspace tabs and a Container Management group with Kubernetes/Docker children.
The active plan is `docs/superpowers/plans/2026-11-12-container-navigation.md`;
the workspace contract is `docs/workspace-tabs.md`. Local implementation passed
315 frontend tests, lint/typecheck/locale/build, affected Go race/database suites,
normal/tagged lint, generated checks and fixture-backed Chrome layout checks.
Storage adds read-only PV/PVC/StorageClass inventory; Docker is an empty page.
Existing installations need updated-binary `optimus seed` for built-in menus and
role grants; custom roles require explicit new Storage/Docker grants. The user
accepted the feature and heading-removal follow-up, and explicitly authorized
commit and direct v0.1.1 publication without waiting for or reviewing CI results
or release artifacts. The communicated delivery is a source-only GitHub Release;
the existing automated artifact-publishing gates remain unchanged. Track actual
publication in `docs/superpowers/plans/2026-11-12-v0.1.1-release.md`.
No application database update or deployment was performed.

Previous delivery (2026-11-06): **v0.1.0 is published** as
[GitHub Release `437039266`](https://github.com/ArkGravity/optimus/releases/tag/v0.1.0)
at `2026-11-06T21:35:24Z`, after the user explicitly reviewed the verified draft
and authorized formal publication. No Production deployment or database recreation
was performed. The user approved the delivery sequence and selected MIT.
Read `docs/superpowers/plans/2026-11-05-v0.1.0-release.md` for full evidence.

The immutable annotated tag object is `f8a132fa02ee97aa1733bb055ad0c8ccb8812eb8`,
peeled to selected source `7102a3f60b84ce9dcf978596a49e3552ea907dc5`.
Preparation PR #3 (`c095f36`) merged as that source; exact source main CI
`42259034364` passed all six jobs. Checkout recovery PR #4 (`d3f077b`) merged
as workflow definition `ac605a55252d655c67257a7ea5ea7eaba50eb84f`, whose main
CI `42291597086` also passed all six jobs. Never move the version tag.

Release run `42292847315`, attempt 2, passed all four jobs, including actual
native Linux amd64/arm64 image, extracted binary and source-free Compose smoke.
The original tested outputs were reused for the failed draft-read-back retry;
version images and the tag were not replaced. Both registries serve v0.1.0 images.
Independent downloads verified exactly six attachments, every checksum, archive
contents/configuration/license/INSTALL and equality of each archive binary with
its saved tested image. Anonymous reads verified identical index/platform/config
digests in GHCR and Docker Hub. Shared release index:
`sha256:9ab3c99b89b47dc98b511e3adbb240f74269d0ddad5dd672a91d7c56beb3dd7c`.
The published notes contain concrete source/workflow CI and verification evidence.
Publication preserved all six original attachment IDs, digests and sizes.

English/Chinese documentation, MIT/security guidance and release automation
passed independent specification, quality and documentation/CLI/archive reviews.
All 44 helper tests, five classifier tests, actionlint, shellcheck and Python3.12
grammar checks passed. Local real Docker TERM/HUP/INT and late-daemon creation
checks passed; unresolved creation uncertainty is explicitly bounded/reported.
Owned local test images, all 36 owned BuildKit records and host Go cache were
removed; all eight shared Kubernetes images and 12 pre-existing containers were
preserved and cluster readiness verified. Owned temporary archives, downloads,
fixture clones, tools, logs and caches were removed and absence verified. The
final verification record merged through PR #5 as `ddd1481`; prose CI
`42297015632` passed. Publication changed only Release metadata and notes; the immutable
tag, images and attachments were not replaced.

Preserve main-image CI, published baseline19 and all 31 business tables. Published
migrations are immutable; GKE appends 00020, with future migrations from 00021. Never replace a published version
tag, image or attachment. GitHub private vulnerability reporting is enabled and
verified. The operator's 2026-11-05 real AWS/Aliyun/GCP acceptance of
`main-1bbb8fe` supersedes older pending real-provider statements below but is
separate from release-artifact verification and does not identify a newer
Production rollout. Historical Production acceptance remains `d0016d0` / 00022.

### Historical delivery and verification

P0–P4 are the supported feature scope. On 2026-10-28 the operator confirmed
that the production database was rebuilt, the latest image was deployed, and
all P0–P4 functions passed production acceptance. Production functional
acceptance is complete; release tagging remains outstanding. Dev and
Production are selected; UAT is skipped.

The accepted production code baseline is `d0016d0`. CI run `40677238414` passed
all six jobs, including publication of `main-d0016d0` to GHCR and Docker Hub.
The operator confirmed rebuilding, deploying and accepting this revision on
2026-10-28, including cloud-key deletion and the three-provider baseline.

Observability Center contains one authenticated Coming Soon child. Reconsider
its implementation after other Optimus functions mature.

Local verification (2026-10-27) passed Go lint (Go 1.25.0), all Go race/database
tests on disposable PostgreSQL 17, frontend lint/typecheck/locale parity/157
tests/build, embedded binary build, and generated-artifact checks. Real P2/P3
Colima smoke passed on 2026-10-23. Production rollout, database recreation and
P0–P4 acceptance were confirmed by the operator on 2026-10-28. The kubeconfig
deletion issue discovered during that smoke is fixed by the follow-up in
`docs/superpowers/plans/2026-10-23-kubeconfig-deletion.md`: live references deny
deletion; historical references are detached in the same transaction as the
physical credential deletion and shared audit event.

The accepted production baseline ends at 00022 and includes the kubeconfig
lifecycle fix directly in `00015_create_clusters.sql`. Cloud-key deletion in
`00022_cloud_key_deletion.sql` only detaches deleted-account references;
credential deletion and its shared audit event are atomic. Cloud-key providers
are Aliyun, AWS and GCP, enforced directly in `00014_create_credentials_cloud_keys.sql`.
Asset discovery in the accepted production baseline is AWS-only.

The current unshipped source uses the operator-approved fresh 19-file migration
baseline. It ends at `00019_create_assets_sync_runs.sql`; the next new migration
is 00020. All 31 business tables and final multi-cloud constraints remain.
This supersedes the former 00022/00023 -> 00028 in-place upgrade contract.
Existing databases, including their `goose_db_version` history, must actually
be recreated before migrate + seed with this baseline; recreating containers
alone retains PostgreSQL data. The operator permits Dev/Production recreation
for a later rollout, but this task authorizes source reorganization and dedicated
disposable test databases only. No application database has been rebuilt by
this change; last production acceptance remains `d0016d0` / historical 00022.
Seed preserves the built-in Security Groups menu row ID, permission grants and
custom children when renaming it to Firewalls. The menu order is Cloud Accounts,
VPCs, Firewalls, Instances, Databases, Sync Runs. AWS credentials need
`ec2:DescribeRegions` and `ec2:DescribeSecurityGroups` alongside inventory reads.
AWS GetCallerIdentity does not require an extra IAM permission.

Fresh baseline19 verification (2026-11-05): exact layout/head tests observed
RED then GREEN. Changed command/migration/seed database race packages passed,
including all 31 business tables populated before full Down0, fresh Up19 and
seed twice. Final auth/reference/native/scope/history checks and normal/tagged
focused lint passed. Independent PostgreSQL schema comparison matched old head28
to fresh19 across 31 tables, 396 columns, 118 constraints, 135 indexes and 29
sequences/ownership. Dedicated comparison databases were removed. Independent
specification and quality reviews approved with no findings. The complete
repository database/race suite passed against the new baseline. The embedded
build and actual binary smoke passed fresh head19/32-table initialization,
repeated migrate/seed, authentication, list APIs and UI/Swagger serving checks;
the smoke server and its unique database were removed and absence verified.
No runtime/app behavior or Dev/Production database changed.

These extensions have not been deployed or accepted in Production. Local
verification of the original 00023 extension on 2026-10-28 passed Go 1.25 lint,
race/database tests, frontend checks/165 tests, generated artifacts and embedded
build. Real AWS Security Groups acceptance remains pending. Use
`scripts/p4-smoke.md` and `scripts/p4-multicloud-smoke.md` with separately supplied
disposable read-only credentials. Fresh-baseline Down operations drop credentials and inventory after their
dependants; they are not rollout recovery. Legacy-invalid GCP SQL/auth/UI
compatibility remains supported, while historical backfills and intermediate
SA-auth downgrade guards are absent from the fresh baseline. Retain database backups and the matching vault key. Do not commit work
without explicit user authorization for that specific change.

The active approved requirement is multi-cloud Assets with Aliyun AK/SK and GCP
Service Account JSON authentication, independent provider/resource tables and
shared read projections/UI. The GCP database table is `gcp_databases`. Menus
will be Cloud Accounts, VPCs, Firewalls, Instances, Databases, Sync Runs.
Read the active design and plan before implementation:
`docs/superpowers/specs/2026-10-28-multicloud-assets-design.md` and
`docs/superpowers/plans/2026-10-28-multicloud-assets.md`.
The implementation branch is `feat/multicloud-assets`, based on `3bd0add`; that base's CI
run `40702433368` passed all six jobs and published its images. The design and
17-task plan are written. The operator requested subagent-driven implementation;
Tasks 1–14 (common resource contracts, credential migration 00024, typed
credential service integration and provider-specific Cloud Keys UI) passed
separate spec/code-quality reviews. Go 1.25 lint, the complete credential
database race suite, HTTP secret non-disclosure and regenerated Swagger
consistency passed. The UI passed typecheck/lint/locale parity and all 194
tests, including optimized-parent prop-order regressions. Multi-cloud account
scope migration 00025, metadata binding, catalog seam and identity pinning also
passed account/cloud-key/migration database race suites and affected HTTP tests.
Native provider schemas/models (Task 6a, migrations 00026–00027) passed separate
reviews, full migration/resource race tests, all-ten model roundtrips and tagged
lint. NIC serialization preserves empty address arrays. Persistence/cascade
(Task 6b) passed both reviews and final account/Aliyun/GCP database race suites,
including zone/region consistency, full region-change rollback and Unscoped
history retention. Shared sync engine/run metadata (Task 7, migration 00028) passed both reviews,
final affected database race suites, normal/tagged lint and Swagger checks.
The AWS adapter (Task 8) passed independent spec and quality reviews on
2026-10-29. Production wiring now uses NewProviderEngine with explicit AWS
credentials, authenticated region catalogs and fresh-unit STS identity checks;
staged legacy orchestration is removed. Required unit/database race suites,
affected async HTTP integration, normal/tagged focused lint and SDK graph/
checksum checks passed. Eight real-STS-path cancellation/deadline regressions
cover the review fix. All 554 resolved modules remain compatible with Go 1.25;
the researched Aliyun/GCP SDK versions are pinned. Unified query core (Task 9a)
passed independent spec/quality reviews, PostgreSQL race fixtures, focused
normal/tagged lint and bounded-page/index checks on 2026-10-29. Same-NIC IP
scoping, zoned-IP validation, nullable native facts and typed AWS rule target
metadata are covered. Family DTO/repository/service integration (9b) also
passed independent spec/quality reviews, affected unit/database race suites,
normal/tagged focused lint and regenerated Swagger checks. Native arrays,
nullable facts, int64 capacities, key/scoped-subnet seams and legacy AWS aliases
are covered; the nullable ha_mode schema correction was rechecked. Canonical
HTTP details, Firewalls and scoped Consumer (Task 10) passed both reviews,
all Assets unit/database race suites, real JWT/RBAC route integration, focused
normal/tagged lint and generated Swagger checks. Private-only lookups preserve
ambiguity/not-found identities and return the canonical requested IP plus
matched-NIC context; legacy AWS ID/VPC methods remain intact. Aliyun identity,
compute and network collection (Task 11) passed both reviews, affected unit/
database race suites, HTTP integration and normal/tagged focused lint. Explicit
AK/SK clients, fresh-unit STS checks, strict pagination and attached-NIC
enrichment are fixture-verified; real Aliyun acceptance is pending and requires
ecs:DescribeNetworkInterfaces read permission. The SDK retry policy is explicit
and all 554 modules remain Go 1.25 compatible. Aliyun databases/Firewalls
(Task 12) also passed both reviews, affected unit/database race suites,
measured four-worker cancellation fixtures, normal/tagged lint, Swagger and
locale checks. All four Aliyun units are fixture-verified, including classic
SG scopes, typed port-list references, required detail failure preservation,
stable revival and fixed provider-neutral run errors. Real Aliyun acceptance
remains pending. GCP explicit authentication/scope and compute/network (Task 13)
passed both reviews on 2026-11-04, including explicit Compute/SQL clients,
authoritative zone planning, typed NIC facts and scope-isolated publication.
Affected unit/database race evidence, lint, Swagger and checksum checks passed;
final JWT body-lifetime, warning and constructor security regressions are green.
Google universe/endpoints are pinned; enabled/unknown ambient client-certificate
discovery fails closed before SDK construction, without global environment
mutation or validation bypass. Real GCP acceptance remains pending. GCP
databases/Firewalls (Task 14) passed both reviews on 2026-11-04, affected race/
database suites, five-family SDK integration, lint, Swagger and checksum/tidy
checks. Priority presence and SDK debug-payload logging have permanent security
regressions; supported explicit constructor logging suppresses raw payloads.
Selected dependencies remain Go 1.25 compatible with required pins intact.
SQL listing repeats per selected region and firewall pages over 16 MiB fail
closed. Real GCP acceptance remains pending. Shared UI/menu integration (Task 15)
was executed as reviewed substeps 15a–15c.
Substep 15a passed independent spec/quality reviews on 2026-11-04: exact
TypeScript/API contracts, shared filters/request generations and rule formatting
passed 59 affected tests, typecheck, lint and locale parity. Permission cleanup,
saved-state restoration, loading/error ownership, nullable native facts and
managed port-list references are covered. This preceded resource/detail page integration (15b) and forms/menus (15c).
Substep 15b1 also passed both reviews on 2026-11-04: typed shared details,
canonical routes and scoped subnets passed affected tests, typecheck, lint and
locale parity. Actual Ant Design search/pagination regressions cover the review
fixes; exact Int64 keys, detail lifecycle and factual boolean rendering are
verified. Resource/sync lists (15b2) and forms/menus (15c) followed.
Substep 15b2 passed both reviews on 2026-11-04: resource/Firewalls/sync lists
passed 120 affected tests, typecheck, lint and locale parity. Server-key identity,
real control events, guarded async state, native collection display and localized
unknown facts are covered. Click/keyboard navigation assertions retain provider
and Int64 precision. Substep 15c passed independent spec/quality reviews on
2026-11-05: account forms preserve metadata-only edits without credential reads,
seed retains the legacy menu ID and rejects reserved-code collisions atomically,
and real router guards protect the compatibility URL. Initial full frontend
296 tests/build and seed database races passed. Five real-control red/green
regressions fixed stale submission/list ownership; affected checks and independent
26-test rechecks passed with no findings. Tasks 15.1–15.7 are complete;
Task 16 also passed independent spec/quality reviews on 2026-11-05: general/AWS
timeouts, configured account ceilings and elapsed planning budgets are covered
by actual SDK/HTTP/database regressions. Affected race/lint/generated checks
passed; operating docs and safe catalog error contracts match the implementation.
Task 17 local verification and a fresh independent overall composition review
completed on 2026-11-05 with no remaining findings. Three-provider HTTP/vault/
sync/deletion and combined upgrade/fresh-init fixtures passed. Full Go 1.25
lint/unit races and database package coverage passed; one legacy test fixture
needed migration 00028 metadata, then its entire cloud-key race suite passed
with all assertions retained. Reviewed embedded Assets and Swagger repairs
passed affected race/lint checks. Final web checks passed 302 tests, typecheck,
lint and locale parity; generated checks and the embedded build passed. The
actual rebuilt binary passed 176 valid Assets GET/HEAD, 78 raw invalid-path,
15 API/Swagger JSON404, 39 Swagger resource/query and authentication/cache
checks, using a unique disposable database that was removed after shutdown.
All 17 planned tasks are locally complete. Real AWS/Aliyun/GCP acceptance remains
pending; the operator confirmed basic local functional acceptance on 2026-11-05.
Commit, push, PR creation and merge to main are now explicitly authorized.
Production rollout and full acceptance remain pending.
Current source schema ends at 00019; the next new migration is 00020. The historical baseline
Go unit/database suites and all 165 frontend tests passed before schema changes.
Historical migrations 00024–00028 implemented the approved feature sequence;
the current fresh baseline supersedes their numbering. Track work and delivery in the active plan. The operator will rebuild and deploy
Production for full validation; production acceptance remains `d0016d0`/00022.
The dedicated test PostgreSQL container, its data volume and both test PostgreSQL
images were removed after verification; shared Colima Kubernetes resources remain.

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
visibility is changed.

2026-09-25 Dev close: the operator completed the P3 applications and P4 assets
smoke checklists on Dev, and confirmed the
embedded UI on local host development and a remote Linux Compose stack. The
2026-09-17 frontend UI work (Ant Design theme tokens, dark sidebar, compact
workspace and menu tabs) is included in that Dev close.

2026-09-27 Production: the published Compose stack was first deployed. The
general `/dashboard` page remains a coming-soon placeholder.

2026-10-07 image delivery: CI is configured to build and publish both
`linux/amd64` and `linux/arm64` under the same version tag in both registries.
Go cross-compiles on the builder's native platform; QEMU handles runtime
package installation. Previously published tags remain amd64-only. Validation
and publication status are tracked in
`docs/superpowers/plans/2026-10-07-dual-platform-images.md`.
PR #1 implementation commit `64af14b` passed all five CI gates in run
`37597470933`, including both platform builds. Local Colima smoke was deferred
at the operator's request. PR #1 was merged as `5d63bef`; main CI run
`37601159022` passed all six jobs and published `main-5d63bef` to both
registries. Direct manifest inspection confirmed linux/amd64 and linux/arm64,
two provenance records, and identical image indexes in both registries.
The workflow is now `.github/workflows/ci.yml`.

2026-10-08 first-visit preferences: new browser sessions without saved app
preferences default to dark theme and English (`en-US`). Saved localStorage
preferences continue to take precedence. Frontend lint, typecheck, locale
parity, all 298 tests, and the production build passed locally.

## Repository Layout

- Repository root: Go module `github.com/ArkGravity/optimus` — `cmd/optimus`
  (single binary), `cmd/dump-permissions` (dev tool), `internal/`,
  `migrations/`, `configs/`, `api/docs/`, `tests/`.
- `web/`: Vue 3 SPA built with Bun and Vite. `web/embed.go` embeds `web/dist`.
- `docs/`: generated API spec and permission catalog, UI notes, and
  `superpowers/specs|plans`.
- `scripts/`: CI change classifier and P3/P4 smoke checklists;
  `web/scripts/`: i18n key parity checker.

## Commands

Run from the repository root:

- `make help`
- `make deps` (frontend lockfile install, `go mod download`, pinned goose/swag/golangci-lint)
- `make run` (`go run`, API on :8080) and `make web-dev` (Vite on :5173)
- `make build` (web build, then `bin/optimus` with the UI embedded)
- `make test` / `make test-int`
- `make lint`
- `make web-install` / `make web` / `make web-check`
- `make swag` / `make swagger-diff`
- `make dump-perms` / `make perm-check`
- `make migrate-up` / `make migrate-down`
- `make migrate-new name=<snake_case>` (sequential numbering, next is 00021)
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
- Run Docker Compose, dockertest, `make test-int`, and smoke containers on Colima's
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
- Use Colima's built-in Kubernetes for routine P2/P3 local development and
  application smoke tests. Use the `colima` kube context and a disposable
  namespace; never stop or delete the shared cluster during teardown.
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
  returns a raw `{db,version}` probe; pod logs use SSE.
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
- P3 Helm mutations and application deletion share the transaction-scoped
  application advisory lock. Keep the installed-release deletion check inside
  that lock.
- Goose migrations live in `migrations/` and are embedded; `optimus migrate`
  and `make migrate-*` use the same files. Models live in `internal/models/`;
  database integration tests use dockertest and `dbtest`.

## Web UI Serving

- `internal/infra/webui` is the Gin `NoRoute` handler, so API routes always
  win. Unknown `/api*` and `/swagger*` paths return the JSON envelope (40401),
  never the SPA shell. Only GET/HEAD are served.
- Hashed `/assets/*` are immutable; `index.html` is `no-store`; other files
  revalidate. Known Assets menu pages and legal detail URLs under `/assets/`
  fall back to the `no-store` index shell. Missing built/hashed/dotted resources
  and unknown paths under that prefix return 404; other misses fall back to
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
- Static routes contain login/error/profile pages and application and asset
  detail/action sub-routes. On the first authenticated navigation,
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
  page state retention: only approved filter/pagination/scroll state
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
- Active clusters require a kubeconfig reference. Credential deletion locks
  that credential, refuses live references, detaches only deleted-cluster
  references, and physically deletes the credential with its audit event in
  one transaction. Keep the restrictive FK and live-row check constraint.
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

For P4, preserve the historical AWS baseline in
`docs/superpowers/plans/2026-06-11-p4-assets.md`; active multi-cloud work follows
`docs/superpowers/plans/2026-10-28-multicloud-assets.md`.
The most important rules are:

- `credentials.Consumer` is the only path for cloud keys.
- Fetch cloud keys with a purpose like `assets.sync.<reason>` and wipe them
  after use.
- Build provider clients per unit/request using explicit selected credentials; do
  not cache SDK clients or use ambient authentication.
- Only authoritative successful full sweeps may soft-delete missing resources.
- VPC and subnet sweeps are one transaction and one `network` sync-run unit.
- Manual sync is asynchronous: the handler returns immediately and the worker
  owns the account lock.
- Cron writes `assets_sync_runs`; it does not write audit rows.
- Cloud-key delete must remain nil-safe when the P4 assets in-use counter is not
  wired. When wired, deleting a referenced cloud key fails with code `43001`.
- Cloud-key deletion locks the credential, refuses live cloud-account
  references (including disabled accounts), and detaches only deleted-account
  references in the same transaction as physical deletion and the shared audit
  event. Keep the foreign key and live-account reference check constraint.
- Removing regions from a cloud account must explicitly soft-delete resources in
  those removed regions.
- P4 frontend paths must be lowercase/kebab-case to match Linux production.
- `assets.Consumer` is the non-HTTP lookup seam for downstream phases and
  returns `ErrAssetsInstanceNotFound` for absent matches.
- P4 runtime configuration uses `OPTIMUS_ASSETS_SYNC_CRON`,
  `OPTIMUS_ASSETS_SYNC_STARTUP_DELAY`,
  `OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS`,
  `OPTIMUS_ASSETS_REQUEST_TIMEOUT` (Aliyun/GCP, 30s),
  `OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT` (independent AWS override, 30s) and
  `OPTIMUS_ASSETS_SYNC_TIMEOUT` (account ceiling, 15m). Positive configured account
  ceilings above 15m remain effective; unit budgets remain at most 15m and actual
  global/regional/zonal plans determine the overflow-safe aggregate budget.

P4's manual release checklists are `scripts/p4-smoke.md` (AWS) and
`scripts/p4-multicloud-smoke.md` (all providers). Real-provider acceptance remains
pending; fixtures are separate evidence. Do not add cloud write/manage APIs.
GCP uses fixed official universe/endpoints. Ambient mTLS discovery is unsupported:
`GOOGLE_API_USE_CLIENT_CERTIFICATE` accepts unset/case-insensitive false only;
true/unknown fails Config before SDK discovery. Never mutate global environment
variables or log raw SDK payloads, SA JSON, JWTs or AK/SK.

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
- New CI image tags include linux/amd64 and linux/arm64; Docker selects the
  daemon's architecture automatically. Older amd64-only tags still require
  emulation on arm64 hosts or a local build. Provenance entries marked
  unknown/unknown are metadata, not additional runnable architectures.
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
- `.github/workflows/ci.yml` runs web, backend quality, unit, database and
  Docker build gates; only main publishes to `ghcr.io/arkgravity/optimus` and
  `docker.io/logic3579/optimus`, tagged `main-<short-sha>`, after every gate
  passes. Both Docker jobs build linux/amd64 and linux/arm64. Keep QEMU setup
  before Buildx; compile Go on BUILDPLATFORM with TARGETOS/TARGETARCH and
  CGO_ENABLED=0. CI uses one disposable PostgreSQL 17 instance via
  `OPTIMUS_TEST_POSTGRES_DSN`, with a separate migrated database per test.
  Never point this variable at an application server. Local tests default to
  dockertest. Keep `-race -count=1`. Cache `tmp/go-cache` per job. Prose-only
  changes skip database and image work; manual dispatch and unknown history run
  full checks. See README for details and registry setup.

## Codex Environment

`.codex/config.toml` configures mem0 over HTTP and enables the Context7,
superpowers and build-web-apps plugins. The Context7 plugin is disabled in the
global Codex configuration and enabled for this repository. Authenticate it
through the plugin's OAuth flow, then start a new Codex thread; do not add a
duplicate `[mcp_servers.context7]` entry. Export `MEM0_API_KEY` before starting
Codex; install `serena` on PATH. Start Codex
from the repository root so Serena selects it. Preserve the mem0 scope from
First Read; before updating a memory by ID, verify its user and application
scope. Keep metadata minimal and verify the saved text, scope and metadata after
updates. Update this file and the current checkpoint after milestones.
Local `.serena/`, `.omo/`, `.worktrees/` and agent caches are ignored.
