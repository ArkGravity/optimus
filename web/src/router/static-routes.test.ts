import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import { staticRoutes } from './static-routes'
import { registerDynamicRoutes } from './dynamic-routes'
import menuContract from '@/test/fixtures/menu-contract.json'

describe('static asset routes', () => {
  it('keeps legacy list compatibility as a guarded navigation instead of an early redirect', () => {
    const router = createRouter({ history: createMemoryHistory(), routes: staticRoutes })
    const resolved = router.resolve('/assets/security-groups')
    expect(resolved.name).toBe('assets.security-groups.compatibility')
    expect(resolved.meta.permission).toBe('assets:resource:read')
    expect(resolved.matched.at(-1)?.redirect).toBeUndefined()
    expect(resolved.matched.at(-1)?.beforeEnter).toBeTypeOf('function')
  })
  it('resolves encoded server keys and legacy numeric IDs under each resource menu', () => {
    const router = createRouter({ history: createMemoryHistory(), routes: staticRoutes })
    for (const [path, family] of [['instances', 'instance'], ['vpcs', 'vpc'], ['databases', 'database'], ['firewalls', 'firewall']]) {
      for (const key of [`gcp:${family}:42`, `aws:${family}:9223372036854775807`, '42']) {
        const route = router.resolve(`/assets/${path}/${encodeURIComponent(key)}`)
        expect(route.name).toBe(`assets.${path}.detail`)
        expect(route.params.id).toBe(key)
        expect(route.meta.permission).toBe('assets:resource:read')
        expect(route.meta.menuName).toBeUndefined()
        const props = route.matched.at(-1)?.props.default
        if (path !== 'vpcs') {
          expect(typeof props).toBe('function')
          if (typeof props === 'function') expect(props({ ...route, name: route.name ?? undefined })).toEqual({ family, assetKey: key })
        }
      }
    }
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
