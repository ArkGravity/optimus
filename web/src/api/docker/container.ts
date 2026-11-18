import type { AxiosInstance } from 'axios'
import type { Envelope } from '@/types/api'
import type { DockerContainer, DockerContainerAction, DockerContainerDetail, DockerList } from '@/types/docker'

/** Live container client for /api/v1/docker/hosts/:id/containers. */
export function makeDockerContainerApi(client: AxiosInstance) {
  const base = (hostId: number) => `/docker/hosts/${hostId}/containers`
  const item = (hostId: number, containerId: string) => `${base(hostId)}/${encodeURIComponent(containerId)}`
  return {
    list: async (hostId: number) => {
      const r = await client.get<Envelope<DockerList<DockerContainer>>>(base(hostId))
      return r.data.data
    },
    get: async (hostId: number, containerId: string) => {
      const r = await client.get<Envelope<DockerContainerDetail>>(item(hostId, containerId))
      return r.data.data
    },
    action: async (hostId: number, containerId: string, action: DockerContainerAction) => {
      await client.post<Envelope<null>>(`${item(hostId, containerId)}/${action}`)
    },
    remove: async (hostId: number, containerId: string, force = false) => {
      await client.delete<Envelope<null>>(item(hostId, containerId), { params: force ? { force: true } : undefined })
    },
  }
}

export type DockerContainerApi = ReturnType<typeof makeDockerContainerApi>
