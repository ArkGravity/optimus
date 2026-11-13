# Native GKE authentication implementation

Design: [GKE authentication](../specs/2026-11-13-gke-authentication-design.md).
Authorized: local implementation using existing GCP Service Account credentials.
The operator subsequently authorized committing and pushing this feature branch
for their Prod deployment and verification. They now confirm successful Prod
verification and explicitly authorize PR creation and merge to main. Release
publication and agent-operated Production actions remain outside this request.
Implementation branch: `feat/gke-authentication`, based on `e9d7a4c`. The operator
requires future code changes to use a new branch or worktree unless explicitly
instructed otherwise. Existing uncommitted edits were carried onto this branch;
main's commit was not changed.
Prod connectivity and acceptance steps 1–6 were operator-confirmed before this
feature. The operator now confirms successful verification of the GKE branch.

- [x] Append migration00020 and typed GKE credential bindings; enforce valid keys,
   reference lifecycle and metadata-only editing without secret retrieval.
- [x] Implement shared explicit OAuth/GKE REST configuration with safe endpoint/TLS,
   cancellation, refresh and normalized-error behavior; wire Kubernetes and Helm.
- [x] Add the bilingual GKE credential form and cluster-context defaults.
- [x] Verify focused unit/database races, UI checks, generated artifacts and build;
   document deployment requirements and remaining live acceptance.

Status: implementation commit `709db86e80935ded60e1215db63b84ecfa6c3031` is
committed, pushed and operator-verified in Prod. PR creation and merge are
authorized. No agent-operated deployment or application database operation was
performed; operator acceptance and local fixture results are separate evidence.

## Verification

- Complete Go 1.25 `go test ./... -tags=dbtest -race -count=1` passed using a
  dedicated PostgreSQL 17 Colima container and individually isolated databases.
  Final affected GKE/credential race suites passed again after the final error
  classification and repository-layer cleanup.
- Native OAuth/GKE fixtures verify Google-only metadata/token endpoints,
  cloud-platform/userinfo.email scopes, token renewal, destination pinning,
  public/private/DNS endpoint selection, CA validation, context cancellation,
  redirects and safe authentication/permission error messages. Kubernetes and
  Helm dispatch tests exercise their bound Cloud Key consumer paths.
- GKE credential database races verify immutable target/mode, metadata-only
  updates, key rebinding, key deletion/conversion refusal, concurrent binding
  versus deletion, and unchanged ordinary kubeconfig consumption.
- Incremental migration19-to20 preserves original encrypted kubeconfig bytes;
  the database FK protects bindings and Down refuses existing GKE credentials.
  All 19 published SQL files remain byte-identical to v0.1.2. The source still
  has 31 business tables plus Goose history; no migration was renumbered.
- Frontend lint, typecheck, locale parity, all 321 tests and production build
  passed. The 6 new form regressions cover native/standard creation, immutable
  GKE targets, metadata-only edits, parent prop ordering, draft cleanup and stale
  saves. A reopening/loading ownership issue observed RED was fixed and rechecked.
- Focused normal and dbtest-tagged Go lint passed. Swagger regeneration and
  swagger-diff/permission checks passed, including typed credential responses.
  All 44 release helper tests passed after updating the head20 fixture. The
  initial sandboxed helper run could not inspect process groups; its rerun
  outside the sandbox passed without changing the release gates.
- Actual rebuilt embedded binary smoke passed fresh20, repeat migrate/seed,
  login, synthetic GCP/GKE credential creation, gke context cluster registration,
  guarded deletion, metadata editing, SPA/Swagger and reference cleanup. This
  made no Google or Kubernetes API requests and used only generated fixture keys.
  The smoke server and unique database were removed; a final run used the final
  binary/Swagger output. This remains local fixture evidence; the operator's
  subsequent Prod verification is recorded separately below.
- Removed only `optimus-gke-tests-20261113` and its anonymous volume
  `c1abcd3913f34a5d83564e1cc0c9d494e23d3a97a179ccb00e7ee3ccfb58d69c`.
  Final inventory retained all 12 original shared Kubernetes containers and the
  pre-existing PostgreSQL image; no Docker volumes remained. Built binary/UI and
  normal ignored build caches remain available locally.
- During implementation the branch was `feat/gke-authentication` and main
  remained at `e9d7a4c`. Commit/push completed as `709db86`; the operator's
  subsequent verification and PR/merge authorization supersede the earlier
  delivery boundary. No tag, release or agent-operated Production action is
  included. Normal main-image CI remains unchanged.

## Operator acceptance and PR delivery

On 2026-11-13 the operator reported successfully validating the deployed
`feat/gke-authentication` branch at implementation commit `709db86`. This is
operator-reported Prod evidence, not an agent-run cloud test. The operator then
requested creating a PR and merging this branch to main. Only acceptance and
delivery documentation is updated after that tested implementation; runtime
code and migration00020 remain unchanged. Merge follows successful PR checks.

Operator usage and required Google/resource permissions:
[native GKE connections](../../operations/gke.md).
