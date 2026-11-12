import type { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import WorkspaceView from '@/components/layout/WorkspaceView.vue'
import { kubernetesSections } from '@/views/containers/sections'

// The root renders child pages in isolated sessions while App.vue selects
// the surrounding layout. Sessions retain approved controls, not components.
const RouterViewPassthrough = WorkspaceView

export const staticRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/Login.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/403',
    name: 'forbidden',
    component: () => import('@/views/errors/403.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/404',
    name: 'notfound',
    component: () => import('@/views/errors/404.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/500',
    name: 'serverError',
    component: () => import('@/views/errors/500.vue'),
    meta: { public: true, layout: 'blank' }
  },
  {
    path: '/',
    name: 'root',
    component: RouterViewPassthrough,
    redirect: '/dashboard',
    children: [
      {
        path: 'workspace',
        name: 'workspace',
        redirect: '/dashboard'
      },
      ...kubernetesSections.map(section => ({
        path: `containers/kubernetes/${section.key}`,
        name: `containers.kubernetes.${section.key}`,
        component: () => import('@/views/containers/Kubernetes.vue'),
        meta: { permissionsAny: [...section.permissions], kubernetesSection: section.key },
      })),
      ...[
        ['clusters', 'clusters', 'k8s:cluster:read'],
        ['nodes', 'nodes', 'k8s:cluster_resource:read'],
        ['workloads', 'workloads', 'k8s:workload:read'],
        ['network', 'network', 'k8s:network:read'],
        ['config', 'configs', 'k8s:config:read'],
        ['cluster-resources', 'namespaces', 'k8s:cluster_resource:read'],
      ].map(([legacy, section, permission]) => ({
        path: `k8s/${legacy}`,
        name: `k8s.${legacy}.compatibility`,
        component: () => import('@/views/containers/Kubernetes.vue'),
        meta: { permission },
        beforeEnter: (to: RouteLocationNormalized) => ({ path: `/containers/kubernetes/${section}`, query: to.query, hash: to.hash, replace: true }),
      })),
      {
        path: 'profile',
        name: 'profile',
        component: () => import('@/views/profile/Index.vue')
      },
      // P3 application sub-routes — Detail/Install/Upgrade are reachable from
      // the list page but are not menu nodes themselves. Registered here so
      // their perm meta gates the route guard the same way menu routes do.
      {
        path: 'apps/applications/new',
        name: 'apps.applications.new',
        component: () => import('@/views/apps/applications/Install.vue'),
        meta: { permission: 'apps:application:write' }
      },
      {
        path: 'apps/applications/:id(\\d+)',
        name: 'apps.applications.detail',
        component: () => import('@/views/apps/applications/Detail.vue'),
        meta: { permission: 'apps:application:read' }
      },
      {
        path: 'apps/applications/:id(\\d+)/upgrade',
        name: 'apps.applications.upgrade',
        component: () => import('@/views/apps/applications/Upgrade.vue'),
        meta: { permission: 'apps:release:upgrade' }
      },
      {
        path: 'assets/security-groups',
        name: 'assets.security-groups.compatibility',
        component: () => import('@/views/assets/firewalls/List.vue'),
        meta: { permission: 'assets:resource:read' },
        // A navigation guard runs after the global permission check. A route
        // redirect would skip that check on this compatibility record.
        beforeEnter: to => ({ path: '/assets/firewalls', query: to.query, hash: to.hash, replace: true }),
      },
      {
        path: 'assets/vpcs/:id',
        name: 'assets.vpcs.detail',
        component: () => import('@/views/assets/vpcs/Detail.vue'),
        meta: { permission: 'assets:resource:read' }
      },
      ...(['instances', 'databases', 'firewalls'] as const).map(path => ({
        path: `assets/${path}/:id`,
        name: `assets.${path}.detail`,
        component: () => import('@/views/assets/components/AssetDetail.vue'),
        props: (route: RouteLocationNormalized) => ({
          family: { instances: 'instance', databases: 'database', firewalls: 'firewall' }[path],
          assetKey: typeof route.params.id === 'string' ? route.params.id : undefined,
        }),
        meta: { permission: 'assets:resource:read' },
      })),
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'catchall',
    component: () => import('@/views/errors/404.vue'),
    meta: { layout: 'blank' }
  }
]
