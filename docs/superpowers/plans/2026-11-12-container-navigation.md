# Container navigation and single-page workspace

User-approved scope: use the supplied BytePlus, Storage and Configs screenshots
to remove application workspace tabs and consolidate container navigation.

- Render one route at a time; retain only existing safe in-memory list controls.
- Replace the Kubernetes sidebar group with Container Management / 容器管理,
  containing Kubernetes and Docker. Canonical URLs use `/containers`.
- Kubernetes has seven permission-filtered navigation buttons: Clusters, Nodes,
  Namespaces, Workloads, Network, Storage, Configs. Resource kind controls sit
  at the top right of each resource card. Events remain available with Namespaces.
- Follow-up: remove the redundant Kubernetes heading/description card so the
  section navigation and resource data begin at the top of the content area.
- Preserve existing `k8s:*` permissions and grants. Add `k8s:storage:read` for
  read-only PV/PVC/StorageClass lists and `docker:environment:read` for the empty
  Docker page. No Docker management API or Kubernetes write API is introduced.
- Existing menu IDs and custom descendants survive idempotent seed updates;
  legacy URLs resolve through permission-checked compatibility routes.
- Published migrations remain unchanged; existing installations apply menus and
  new built-in role grants through `optimus seed` from the updated binary.

Validation: navigation/permission/resource-switch and stale-request regressions,
frontend lint/typecheck/locale parity/tests/build, affected Go race tests, seed
database tests on disposable PostgreSQL, permission and Swagger generation checks.
The user subsequently accepted the functionality and explicitly authorized
commit and direct v0.1.1 publication without CI/artifact review. See the
[release plan](2026-11-12-v0.1.1-release.md). Application database operations and
Production deployment remain outside this delivery.

Progress: implementation complete; local verification passed.

- Removed workspace-tab components, close-tab actions and tab registry. One
  route remains mounted, with existing safe control/scroll retention.
- Heading-removal follow-up passed all three existing Kubernetes navigation
  tests, frontend lint, locale parity and the production web build/typecheck.
  The unused heading styles and bilingual description keys were also removed.
- Seed and fixture now expose the Container Management group with Kubernetes
  and Docker. Compatibility URLs retain permission checks. Section URLs and
  buttons enforce existing resource permissions, including Secret-only roles.
- Storage lists use fresh credential-backed clientsets, bounded continuation
  requests and normalized errors. PV/PVC/StorageClass projections preserve
  unknown capacity rather than inventing a zero value. No Kubernetes writes.
- Lists, detail drawers and Secret reveal reject obsolete responses on scope
  changes, overlapping requests and unmount. Kind switches use real controls at
  the top right; search, wrapping filters and table pagination remain available.
- All 315 frontend tests passed (57 files, four workers), including real Ant
  Design switches, sidebar revocation, direct-route authorization, stale list/
  reveal handling and actual Axios envelope parsing. Lint, typecheck, locale
  parity and the production web build passed. The existing Ant Design bundle
  size advisory remains a non-blocking build warning.
- All affected Kubernetes/permission Go race suites and all seed/Kubernetes
  database race packages passed on isolated databases in a dedicated PostgreSQL
  17 Colima container. The final added HTTP test confirms storage permission
  denial precedes credential reads, then checks successful typed API requests
  and the real credential-consumption audit actor. Seed upgrade tests retain
  IDs, custom descendants and custom role grants, and reject collisions with
  complete transaction rollback. Normal and dbtest-tagged focused Go lint
  passed. Swagger and permission generation checks passed.
- Chrome checked production bundles with fixture APIs in dark Storage, Chinese
  dark Configs, 1024px light Configs and Docker empty-page views. These checks
  validate rendering and client integration, not live-cluster acceptance.
  Screenshots are retained under ignored `tmp/navigation-*.png`.
- The final single binary build passed after the final web build. The owned
  PostgreSQL container, anonymous volume, downloaded image and Chrome profile
  were removed; absence of the container was verified. All 12 pre-existing
  containers were retained, and the shared Colima node remained Ready.

No application database, published migration, release or Production deployment
was changed during implementation. The accepted change was committed as
`bdab35a` and published in the v0.1.1 source Release under the linked release plan.
