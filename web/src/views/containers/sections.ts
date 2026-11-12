export const kubernetesSections = [
  { key: 'clusters', label: 'menu.k8s.clusters', permissions: ['k8s:cluster:read'] },
  { key: 'nodes', label: 'containers.nodes', permissions: ['k8s:cluster_resource:read'] },
  { key: 'namespaces', label: 'containers.namespaces', permissions: ['k8s:cluster_resource:read'] },
  { key: 'workloads', label: 'menu.k8s.workloads', permissions: ['k8s:workload:read'] },
  { key: 'network', label: 'menu.k8s.network', permissions: ['k8s:network:read'] },
  { key: 'storage', label: 'containers.storage', permissions: ['k8s:storage:read'] },
  { key: 'configs', label: 'containers.configs', permissions: ['k8s:config:read', 'k8s:secret:read'] },
] as const

export const kubernetesPermissions = [...new Set(kubernetesSections.flatMap(section => [...section.permissions]))]
