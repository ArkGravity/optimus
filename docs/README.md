# Documentation

Optimus ships its Go API and Vue web UI as one binary with PostgreSQL 17. Start with the user guides; historical designs explain implementation decisions rather than current installation steps.

## Getting started

- [Product overview](../README.md) · [中文概览](../README_zh.md)
- [Install](installation.md) · [中文安装指南](installation_zh.md)
- [v0.1.0 release notes](releases/v0.1.0.md) — published 2026-11-06 (UTC)
- [Published releases](https://github.com/ArkGravity/optimus/releases)

## User guides

The installation guides cover first login and the first Cloud Key → Cloud Account → Sync workflow, plus kubeconfig/cluster setup. Supported inventory includes AWS, Aliyun and GCP, using read-only cloud access. Dashboard and Observability Center remain Coming Soon placeholders.

- [AWS inventory checklist](../scripts/p4-smoke.md)
- [Multi-cloud inventory checklist](../scripts/p4-multicloud-smoke.md)
- [Helm application checklist](../scripts/p3-smoke.md)
- [Security reporting](../SECURITY.md)

Use disposable resources for checklists and never publish credentials or provider payloads.

## Development

- [Developer setup and commands](development/getting-started.md)
- [CI and registry setup](development/ci.md)
- [Versioned release process](development/releases.md)
- [Contributing](../CONTRIBUTING.md)
- [Project operating contract](../AGENTS.md)

## Operations

- [Database transitions, backup and recovery](operations/recovery.md)
- [Installation and configuration](installation.md)

Current main uses fresh head19 with 31 business tables plus Goose history. Historical 00022/00023/00028 databases cannot upgrade in place using renumbered files. Preserve the database backup and matching vault key before a separately authorized recreation. See the [recovery guide](operations/recovery.md), including historical acceptance and current operator reports.

## Reference

- [API specification](api/swagger.json), generated with `make swag`, also served at `/swagger/index.html`.
- [Permission catalog](permissions.md), generated from `internal/infra/permissions/codes.go` by `make dump-perms`.
- [Workspace layout and menu tabs](workspace-tabs.md), including retained-state/lifecycle rules.
- [Brand mark](branding.md), identifying the shared logo.
- [Menu contract fixture](../web/src/test/fixtures/menu-contract.json), reviewed test data for backend paths/components/permissions, not a runtime menu source. Update it with backend menu changes and run web tests.
- [Standalone release installation template](releases/INSTALL.md), rendered with the selected version during packaging.

## Design and implementation history

[Specifications](superpowers/specs/) and [plans](superpowers/plans/) are named `YYYY-MM-DD-<topic>`. The [multi-cloud design](superpowers/specs/2026-10-28-multicloud-assets-design.md) and [implementation plan](superpowers/plans/2026-10-28-multicloud-assets.md) describe the feature now implemented on main. Their chronological checkpoints retain the evidence and constraints applicable at the time.

The operator confirmed real AWS/Aliyun/GCP acceptance of `main-1bbb8fe` on 2026-11-05. This is separate from completed [v0.1.0 archive/image verification](https://github.com/ArkGravity/optimus/actions/runs/42292847315/attempts/2) and [Release publication](https://github.com/ArkGravity/optimus/releases/tag/v0.1.0) on 2026-11-06 (UTC). Publication did not deploy or recreate a Production database. The earlier `d0016d0` / historical 00022 production acceptance remains historical evidence, not the current schema contract.

Backend/frontend documents from former split repositories are preserved here. The monorepo merge is described in the [2026-09-24 design](superpowers/specs/2026-09-24-monorepo-single-binary-design.md). Read old paths and commands using this mapping:

- `optimus-be/` means the repository root; `optimus-fe/` means `web/`.
- `deploy/` and the split-era frontend `Dockerfile`/`nginx.conf` were replaced by the root `Dockerfile` and `docker-compose.yml`; the binary serves the UI.
- `cmd/server`, `cmd/migrate`, `cmd/seed` and `cmd/vault-keygen` are now `cmd/optimus` subcommands (`optimus server|migrate|seed|vault-keygen`).
- The Go module is `github.com/ArkGravity/optimus`, formerly `optimus-be`.
- Pre-split commit IDs refer to the deleted original `logic3579/optimus` repository and do not resolve here. Split-era backend/frontend history is preserved.
- Historical migration numbers through 00028 describe earlier development/production layouts. Current source ends at 00019; append 00020 for new schema work.

Current user and operational instructions belong in the guides above. Preserve designs/plans as chronological history and use [AGENTS.md](../AGENTS.md) as the sole project operating-contract file.
