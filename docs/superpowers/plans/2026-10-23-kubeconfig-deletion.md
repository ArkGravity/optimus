# Kubeconfig deletion after cluster removal

Status: Implemented and locally verified against the revised fresh-schema
baseline. The operator confirmed production database recreation, deployment of
the latest image, and successful P0–P4 functional acceptance on 2026-10-28.
Requested by the operator on 2026-10-23.

## Behavior

Keep credential deletion denied while any live cluster references it. When
only soft-deleted clusters remain, detach their credential references and
physically delete the credential in one transaction. Preserve cluster history,
application history, and the shared audit trail; retain no deleted secret.

At implementation time on 2026-10-23, both Dev and Production were unused
acceptance environments and the operator permitted rebuilding them. The fix
was included directly in `00015_create_clusters.sql`: a check constraint permits
a NULL `clusters.kubeconfig_id` only on deleted rows, and the restrictive
foreign key remains. No migration was added for this fix. The sequence ends at
00021; future schema changes use new sequential migrations starting at 00022.
The production application database has since been rebuilt and initialized with
migrate + seed as part of the accepted baseline.

Lock the credential row before checking references. Count, detach, delete and
audit share that transaction. PostgreSQL FK locking serializes concurrent
cluster references with credential deletion; map a missing-credential FK error
on cluster create/update to the existing safe not-found business error.

## Validation

Cover live-reference denial, deletion after soft deletion, preserved tombstone
rows, physical credential removal, one successful audit event, rollback on a
failed delete, required live references, and concurrent cluster reference races.
Run the Go race/database suite with temporary Colima PostgreSQL 17, backend
lint, build, and a live API/Helm smoke including final kubeconfig deletion.

## Results

- Go 1.25.0 lint and the complete race/database suite passed on disposable
  Colima PostgreSQL 17 (`-tags=dbtest -race -count=1 -timeout=20m`).
- Regression coverage passed for live-reference denial, tombstone retention,
  physical secret-row deletion, atomic audit, rollback after a forced delete
  or audit failure, a live-row NULL prohibition, and concurrent FK references.
- Fresh-schema initialization passed with the corrected 00015 migration and
  head version 00021; no new migration number is consumed. Sequential migration
  generation on a disposable SQL copy produced 00022 as the next version.
- The Go 1.25.0 embedded binary build and live Colima API/Helm smoke passed.
  After uninstalling and deleting the application, cluster deletion followed
  by kubeconfig deletion succeeded. The credential returned 404 afterward;
  SQL confirmed that the deleted cluster row survived with a NULL reference.
- The smoke also rechecked P2 live reads, P3 lifecycle, and application/cluster
  deletion guards. All disposable containers, servers and Kubernetes namespace
  were cleaned.
- `git diff --check` passed. No production database or deployment was changed
  during local verification.
