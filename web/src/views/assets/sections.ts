export const inventorySections = [
  { key: 'accounts', label: 'menu.assets.cloud_accounts', permissions: ['assets:account:read'] },
  { key: 'vpcs', label: 'menu.assets.vpcs', permissions: ['assets:resource:read'] },
  { key: 'firewalls', label: 'menu.assets.firewalls', permissions: ['assets:resource:read'] },
  { key: 'instances', label: 'menu.assets.instances', permissions: ['assets:resource:read'] },
  { key: 'databases', label: 'menu.assets.databases', permissions: ['assets:resource:read'] },
  { key: 'sync-runs', label: 'menu.assets.sync_runs', permissions: ['assets:sync:read'] },
] as const

export const inventoryPermissions = [...new Set(inventorySections.flatMap(section => [...section.permissions]))]

const detailFamilies = ['vpcs', 'instances', 'databases', 'firewalls'] as const

export function highlightsInventory(path: string): boolean {
  if (path === '/assets/inventory' || path.startsWith('/assets/inventory/')) return true
  return detailFamilies.some(family => path.startsWith(`/assets/${family}/`))
}
