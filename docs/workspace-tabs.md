# Compact workspace and menu tabs

Implementation scope approved on 2026-09-17: compact content spacing and
application tabs for menu navigation. This uses existing backend routes and
permissions; no API or menu-contract change is required.

- The viewport contains the header, horizontally scrollable menu tabs and one
  scrolling content surface. Outer spacing is 8px; first-level cards use 16px
  padding. Filters wrap on narrow screens; tables scroll horizontally as needed.
- A menu path identifies one tab. Reopening it activates the same tab. Titles
  are translation keys, so language changes update existing tabs immediately.
- Detail and action routes remain normal routes under their nearest menu tab;
  clicking that tab returns to its list. They do not create multiple detail tabs.
- Closing the active tab selects its right neighbor, then its left neighbor.
  Closing all opens an empty workspace. Navigation cancellation leaves tabs intact.
- Tabs and approved filter/pagination/scroll state live in memory only. A browser
  reload starts a new workspace. Logout clears everything. Revoked/removed menu
  entries are removed from the tab list and their saved state is discarded.
- Pages unmount on navigation. No KeepAlive, API result caching, form/credential
  storage or hidden polling is introduced. Returning to a list restores controls
  and pagination, then fetches current data. Kubernetes resource tabs restore
  the resource kind, but always use the shared current cluster/namespace.
- Installation, upgrade and profile forms warn before navigating away with
  unsaved changes. Existing modal save/cancel flows remain unchanged.

`usePageState` must receive only explicitly approved filter/navigation refs.
Do not pass passwords, Secret data, values YAML, form drafts or API responses.
`useTable` restores only pagination and filters, never rows. New list-specific
controls can opt in via `usePageState` without retaining their page component.
