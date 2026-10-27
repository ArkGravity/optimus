import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { staticRoutes } from './static-routes'
import { registerDynamicRoutes } from './dynamic-routes'
import menuContract from '@/test/fixtures/menu-contract.json'

describe('static asset routes', () => {
  it('registers the numeric VPC detail route with resource read permission', () => {
    const root = staticRoutes.find((route) => route.name === 'root')
    const detail = root?.children?.find((route) => route.name === 'assets.vpcs.detail')
    expect(detail?.path).toBe('assets/vpcs/:id(\\d+)')
    expect(detail?.meta?.permission).toBe('assets:resource:read')
  })

  it('keeps the catch-all behind auth bootstrap instead of redirecting early', () => {
    const catchall = staticRoutes.find((route) => route.name === 'catchall')
    expect(catchall?.redirect).toBeUndefined()
    expect(catchall?.component).toBeDefined()
    expect(catchall?.meta?.public).not.toBe(true)
  })

  it('resolves the observability placeholder and sends removed URLs to the catch-all', () => {
    const router = createRouter({ history: createMemoryHistory(), routes: staticRoutes })
    registerDynamicRoutes(router, menuContract.menus.map((menu, index) => ({
      id: index + 1, code: menu.path, name: menu.path, path: menu.path,
      component: menu.component, icon: '', permission_code: menu.permission || undefined,
      sort_order: index, hidden: false
    })))
    const route = router.resolve('/observability/overview')
    expect(route.name).toBe('/observability/overview')
    expect(route.meta.permission).toBeUndefined()
    expect(route.meta.public).not.toBe(true)
    for (const path of [
      '/observability/kubernetes', '/observability/dashboards', '/observability/datasources',
      '/delivery', '/delivery/overview',
      '/delivery/projects', '/delivery/projects/1', '/delivery/approvals', '/delivery/runs/1',
      '/credentials/http-credentials'
    ]) {
      expect(router.resolve(path).name).toBe('catchall')
    }
  })
})
