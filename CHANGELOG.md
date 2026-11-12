# Changelog

## v0.1.1 — 2026-11-12

- Replace workspace tabs with single-page sidebar navigation.
- Add the Container Management group with Kubernetes and an empty Docker page.
- Consolidate Kubernetes into Clusters, Nodes, Namespaces, Workloads, Network, Storage and Configs; remove the redundant heading card.
- Add read-only PV/PVC/StorageClass inventory and top-right resource-type controls.
- Update routes, permissions and idempotent menu seeding while preserving legacy URLs, menu IDs and custom grants.
- Publish directly as a source release at the operator's request, without waiting for CI or verifying release artifacts. No binary attachments or versioned-image availability are asserted. See [v0.1.1 notes](docs/releases/v0.1.1.md).

## v0.1.0 — 2026-11-06

The first versioned release was [published](https://github.com/ArkGravity/optimus/releases/tag/v0.1.0) on 2026-11-06 (UTC), after final operator review of the verified draft.

- Self-hosted Go binary with embedded Vue UI, PostgreSQL 17 and Docker Compose deployment.
- Authentication, RBAC, menu administration and audit records.
- Encrypted credential vault for kubeconfigs and typed AWS/Aliyun AK/SK and GCP Service Account JSON.
- Read-only Kubernetes inspection and log streaming, plus Helm application management.
- Read-only AWS, Aliyun and GCP inventory with provider-native resource storage, shared lists/details, Firewalls and asynchronous sync history.
- English and Simplified Chinese UI, light/dark themes and workspace tabs.
- Fresh migration baseline ending at 00019. Historical 00022/00023/00028 databases require recreation; see [recovery](docs/operations/recovery.md).
- MIT License and installation, contribution and release documentation.

The operator confirmed real-cloud acceptance of `main-1bbb8fe` on 2026-11-05. That report is separate from completed verification of the exact versioned artifacts and Release publication; no Production rollout or database recreation was performed for this release. See [v0.1.0 notes](docs/releases/v0.1.0.md).
