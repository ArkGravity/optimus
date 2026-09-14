# Backend documentation

This repository owns the generated [API specification](api/swagger.json),
[permission catalog](permissions.md), backend and shared P0-P6
[specifications](superpowers/specs/) and [plans](superpowers/plans/).
`make swag` and `make dump-perms` update files inside this repository only.

Historical designs retain monorepo paths and commit IDs. `optimus-be/` means
this root; `optimus-fe/` refers to the separate frontend repository. Old `deploy/`
paths map to this root's Compose/Dockerfile or the frontend's Dockerfile/nginx.conf.
Use current README/AGENTS commands; old steps are implementation history.

Pure frontend P0 plans/addenda moved to
[frontend docs](https://github.com/ArkGravity/optimus-fe/tree/main/docs/superpowers).
Shared designs stay here as a single source, linked from frontend documentation.

Runtime checklists: [P4](../scripts/p4-smoke.md), [P5](../scripts/p5-smoke.md),
[P6](../scripts/p6-smoke.md). Local acceptance passed before the 2026-09-14 split.
Production still requires persistent-data upgrade validation from original
revision `4e2d08b` through migration `00023_p6_delivery.sql`, production acceptance
and release tagging. Repository migration is not production sign-off.
