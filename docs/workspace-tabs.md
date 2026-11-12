# Single-page workspace and container navigation

The 2026-11-12 navigation update replaces the application menu-tab bar. Clicking
a sidebar menu opens that page directly; browser Back/Forward remains available.
Only one page is mounted at a time. There is no close-tab or empty-workspace flow.

Safe filter, pagination and scroll controls remain in memory per route. They
are cleared on logout or changes to permissions/menus; a reload starts a fresh
workspace. Pages fetch current data on each visit. API responses, form drafts,
credentials, Secret data and active streams are never saved in page state.
Installation, upgrade and profile forms retain their unsaved-change guards.

Container Management / 容器管理 has two built-in sidebar children:

- Kubernetes: `/containers/kubernetes`, with permission-filtered buttons for
  Clusters, Nodes, Namespaces, Workloads, Network, Storage and Configs. Each
  section has a direct URL under `/containers/kubernetes/<section>`.
- Docker: `/containers/docker`, an empty page reserved for future work,
  gated by `docker:environment:read`. No Docker API is provided yet.

Resource-type switches appear at the top right of each resource card. Storage
offers PV, PVC and StorageClass with `k8s:storage:read`; only PVC uses a namespace
filter. Configs offers ConfigMap and Secrets, each with its existing permission.
Nodes and Namespaces keep `k8s:cluster_resource:read`; Events remain available
in the Namespaces card. Existing workload, network, cluster and Secret-reveal
permissions remain unchanged. Resource pages use the shared selected cluster and
namespace; an absent cluster selection shows a prompt and never auto-selects.
Late list/detail/reveal responses are discarded after scope changes or unmount.

For existing installations, run `optimus seed` with the updated binary to update
the built-in menus and built-in role grants. Seed preserves the old parent and
Clusters menu IDs, moves custom descendants off retired built-in menu rows and
rejects reserved-code collisions atomically. Custom roles must receive the new
Storage/Docker permissions explicitly. Published migrations are unchanged.
Legacy `/k8s/...` page URLs check their existing permission before forwarding to
the corresponding new page; Kubernetes HTTP APIs remain under `/api/v1/k8s`.

Implementation and verification: [container navigation plan](superpowers/plans/2026-11-12-container-navigation.md).
