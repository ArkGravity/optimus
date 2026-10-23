# Feature scope reset

Status: Implemented and locally verified on 2026-10-23. Production rollout and
database recreation remain outstanding. Approved by the operator on 2026-10-23.

## Scope

Ship P0–P4. Remove the observability and application-delivery business
implementations, their exclusive HTTP credential extension, APIs, workers,
permissions, configuration, dependencies, tests, and documentation. Remove
cross-module integrations while preserving the standalone Helm lifecycle and
its necessary concurrency protection.

Keep Observability Center and Delivery Management in their existing navigation
positions. Each group has one permission-free authenticated Coming Soon child
at `/observability/overview` or `/delivery/overview`. Pages use existing theme,
locale, and workspace behavior and make no business API calls. Old business
routes return 404; removed API paths use the existing JSON 404 envelope.

## Database and deployment

Production has been deployed but has never been used; the operator permits
rebuilding its application database. Delete SQL migrations 00022 and 00023
without a retirement migration or an in-place upgrade path. Retain existing
migration numbers through 00021. The next new migration starts at 00022,
followed by 00023 and subsequent numbers; no numbers are reserved.

Existing development and production application databases must be recreated,
including the Goose version table, then initialized with the remaining
migrations and updated seed. Recreating containers alone retains the named
PostgreSQL volume and is insufficient. Do not delete unrelated databases or
Kubernetes releases. Production reset is a deployment operation, not part of
the source-code implementation.

## Acceptance

Production functional acceptance covers P0–P4. Check only navigation and the
static unavailable state for the reserved groups. Reconsider these features
after the other Optimus functions mature.

Validate fresh schema and seed, menu contract and route casing, deleted routes,
permission generation, Swagger generation, Go lint/race/database tests,
frontend lint/typecheck/locale parity/tests/build, and the embedded binary.

## Verification results

- Go 1.25.0: golangci-lint and the full `go test ./... -tags=dbtest -race
  -count=1 -timeout=20m` suite passed using disposable PostgreSQL 17 on Colima.
  The suite covers the exact fresh-schema table set, migration head 00021,
  idempotent seed, reserved menus, independent application-service contention,
  deletion protection, and the standalone Helm lifecycle.
- Frontend lint, typecheck, locale parity, all 158 tests, and production build
  passed. The embedded `bin/optimus` build passed. Route regression tests cover
  both reserved entries and removed business URLs.
- Swagger and permission generation consistency checks passed. Comparing the
  generated API specifications confirmed that only the 30 retired API paths
  were removed; all P0–P4 documented paths remain. Existing Kubernetes/Helm
  dependency versions and the Go directive remain unchanged.
- Sequential migration creation against a disposable copy of the SQL files
  produced `00022_sequence_probe.sql`; no probe migration was retained.
- A fresh instance of the embedded binary passed migration/seed, administrator
  login, menu/permission checks, embedded UI/Swagger serving, and JSON 404
  checks for all 30 retired API paths and their former HTTP methods.
- Live Colima Kubernetes checks passed kubeconfig consumption, cluster ping,
  node/pod reads, and real Helm install, upgrade, rollback, history, uninstall,
  installed-application deletion denial, and referenced-cluster deletion denial.
  The temporary server, database, chart server, and namespace were cleaned.
- CI change-classifier tests and `git diff --check` passed. No image was
  published and no production database or deployment was changed.

## Additional existing issue observed

An extra cleanup check found that deleting a kubeconfig after its cluster has
been soft-deleted returns the generic error envelope (10001). The live-cluster
usage check permits deletion, but the retained cluster row still has a database
foreign key to the physically deleted credential. The unchanged P1 kubeconfig
deletion path needs a separate lifecycle fix. This issue is outside this scope
reset; disposable smoke credentials were removed with the test database.
