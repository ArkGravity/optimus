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
- Docker: `/containers/docker`, with buttons for Hosts, Images and Containers.
  Each section has a direct URL under `/containers/docker/<section>` and every
  section requires `docker:environment:read`. Mutation controls (host
  create/edit/delete, image delete/prune, container start/stop/restart/delete)
  additionally require `docker:environment:write`; connection tests are reads.
  Images and Containers share the selected host from HostPicker; an absent
  selection shows a prompt and never auto-selects. Deleting the selected host,
  or a persisted host that no longer exists, clears the selection. Image and
  container lists are live Docker API reads; late responses are discarded after
  host or permission changes. Host passwords are write-only and never shown.

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

Assets Management / 资产管理 has one built-in sidebar child:

- Inventory / 资产清单: `/assets/inventory`, with permission-filtered buttons for
  Cloud Accounts, VPCs, Firewalls, Instances, Databases and Sync Runs. Each
  section has a direct URL under `/assets/inventory/<section>`. Lists stay
  cross-account and keep the existing provider and account filters. The menu
  row is visible when the user has any of `assets:account:read`,
  `assets:resource:read` or `assets:sync:read`. Those permission codes and the
  assets HTTP APIs are unchanged.

Detail URLs stay at `/assets/vpcs/:id`, `/assets/instances/:id`,
`/assets/databases/:id` and `/assets/firewalls/:id`. Opening one keeps the
Inventory menu selected. Legacy list URLs `/assets/cloud-accounts`,
`/assets/vpcs`, `/assets/firewalls`, `/assets/instances`, `/assets/databases`,
`/assets/sync-runs` and `/assets/security-groups` check their existing
permission, then replace the address with the matching inventory section and
keep the query and hash.

For existing installations, run `optimus seed` with the updated binary. Seed
renames `assets.cloud_accounts` to `assets.inventory` in place, moves custom
children off the retired VPC, firewall, instance, database and sync-run rows,
then deletes those rows. A reserved-code collision rolls the seed transaction
back. Custom roles keep their existing assets grants. Published migrations are
unchanged.

Implementation and verification: [container navigation plan](superpowers/plans/2026-11-12-container-navigation.md).
