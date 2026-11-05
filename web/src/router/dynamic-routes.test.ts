import { describe, it, expect } from 'vitest'
import { buildViewResolver, flattenMenusToRoutes } from './dynamic-routes'
import type { MeMenuNode } from '@/types/api'
import menuContract from '@/test/fixtures/menu-contract.json'

const tree: MeMenuNode[] = [
  {
    id: 1, code: 'dashboard', name: 'menu.dashboard',
    path: '/dashboard', component: 'dashboard/Index',
    icon: 'dashboard', sort_order: 0, hidden: false
  },
  {
    id: 2, code: 'system', name: 'menu.system_group',
    path: '/system', component: '', icon: 'setting', sort_order: 1, hidden: false,
    children: [
      {
        id: 3, code: 'system.users', name: 'menu.system.users',
        path: '/system/users', component: 'system/users/List',
        icon: '', permission_code: 'system:user:read',
        sort_order: 0, hidden: false
      }
    ]
  }
]

describe('flattenMenusToRoutes', () => {
  it('resolves every canonical assets menu with generic firewall identity and unchanged permissions', () => {
    const assets = menuContract.menus.filter(menu => menu.path.startsWith('/assets/'))
    expect(assets.map(menu => menu.path)).toEqual(['/assets/cloud-accounts', '/assets/vpcs', '/assets/firewalls', '/assets/instances', '/assets/databases', '/assets/sync-runs'])
    const routes = flattenMenusToRoutes(assets.map((menu, i) => ({ id: i + 1, code: menu.path, name: menu.path, ...menu, icon: '', sort_order: i, hidden: false, permission_code: menu.permission })), buildViewResolver())
    expect(routes.map(route => route.path)).toEqual(assets.map(menu => menu.path))
    expect(routes.find(route => route.path === '/assets/firewalls')?.meta?.permission).toBe('assets:resource:read')
  })
  it('skips group nodes (empty component) and flattens leaves', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })],
      ['system/users/List', async () => ({ default: {} })]
    ])
    const routes = flattenMenusToRoutes(tree, p => components.get(p))
    expect(routes.map(r => r.path).sort()).toEqual(['/dashboard', '/system/users'])
    const usersRoute = routes.find(r => r.path === '/system/users')!
    expect(usersRoute.name).toBe('system.users')
    expect(usersRoute.meta?.permission).toBe('system:user:read')
  })

  it('skips nodes whose component path has no loader (with warn)', () => {
    const components = new Map<string, () => Promise<unknown>>([
      ['dashboard/Index', async () => ({ default: {} })]
    ])
    const warns: string[] = []
    const routes = flattenMenusToRoutes(tree, p => components.get(p), msg => warns.push(msg))
    expect(routes.map(r => r.path)).toEqual(['/dashboard'])
    expect(warns.some(w => w.includes('system/users/List'))).toBe(true)
  })

  it('resolves the seeded Linux-cased observability component', () => {
    const component = 'observability/overview/Index'
    const path = '/observability/overview'
    const node = { id: 9, code: component, name: component, path, component, icon: '', sort_order: 0, hidden: false }
    const routes = flattenMenusToRoutes([node], buildViewResolver())
    expect(routes[0]?.path).toBe(path)
    expect(routes[0]?.meta?.permission).toBeUndefined()
  })
})
