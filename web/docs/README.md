# Frontend documentation

- [Workspace layout and menu tabs](workspace-tabs.md): navigation, state retention
  and lifecycle rules for the compact workspace.

Frontend-only P0 [design addenda](superpowers/specs/) and
[implementation plans](superpowers/plans/) live here. Backend-owned references:

- [Shared specifications](https://github.com/ArkGravity/optimus-be/tree/main/docs/superpowers/specs)
- [Shared plans](https://github.com/ArkGravity/optimus-be/tree/main/docs/superpowers/plans)
- [Swagger API](https://github.com/ArkGravity/optimus-be/blob/main/docs/api/swagger.json)
- [Permissions](https://github.com/ArkGravity/optimus-be/blob/main/docs/permissions.md)
- [Smoke checklists](https://github.com/ArkGravity/optimus-be/tree/main/scripts)

Historical plans retain monorepo paths and commit IDs. `optimus-fe/` means this
root; backend paths refer to the separate backend repository. Use current
README/AGENTS commands.

## Menu contract fixture

[`menu-contract.json`](../src/test/fixtures/menu-contract.json) records supported
backend menu components, paths and permissions. Its source field identifies the
backend revision. This is test data, not the runtime menu source.
When backend menus change, review `/api/v1/me/menus` and backend seed changes,
update the fixture and source revision, and run frontend tests. These tests
validate the supported contract; they do not detect an unreviewed change in an
unpinned backend revision. Coordinate both repositories for API/menu changes.
