import type { AxiosInstance } from 'axios'
import type { Envelope } from '@/types/api'
import type { DockerImage, DockerImageDeleteResponse, DockerImagePruneResponse, DockerList } from '@/types/docker'

/** Live image client for /api/v1/docker/hosts/:id/images. */
export function makeDockerImageApi(client: AxiosInstance) {
  const base = (hostId: number) => `/docker/hosts/${hostId}/images`
  return {
    list: async (hostId: number) => {
      const r = await client.get<Envelope<DockerList<DockerImage>>>(base(hostId))
      return r.data.data
    },
    remove: async (hostId: number, imageId: string, force = false) => {
      const r = await client.delete<Envelope<DockerImageDeleteResponse>>(
        `${base(hostId)}/${encodeURIComponent(imageId)}`,
        { params: force ? { force: true } : undefined },
      )
      return r.data.data
    },
    prune: async (hostId: number, all = false) => {
      const r = await client.post<Envelope<DockerImagePruneResponse>>(`${base(hostId)}/prune`, { all })
      return r.data.data
    },
  }
}

export type DockerImageApi = ReturnType<typeof makeDockerImageApi>
