import type { AxiosInstance } from 'axios'
import type { Envelope } from '@/types/api'
import type {
  DockerHost, DockerHostCreateRequest, DockerHostUpdateRequest,
  DockerHostListQuery, DockerHostListResponse, DockerHostTestResult,
} from '@/types/docker'

/** Docker host CRUD client. Maps 1:1 to /api/v1/docker/hosts. */
export function makeDockerHostApi(client: AxiosInstance) {
  const base = '/docker/hosts'
  return {
    list: async (params: DockerHostListQuery = {}) => {
      const r = await client.get<Envelope<DockerHostListResponse>>(base, { params })
      return r.data.data
    },
    get: async (id: number) => {
      const r = await client.get<Envelope<DockerHost>>(`${base}/${id}`)
      return r.data.data
    },
    create: async (body: DockerHostCreateRequest) => {
      const r = await client.post<Envelope<DockerHost>>(base, body)
      return r.data.data
    },
    update: async (id: number, body: DockerHostUpdateRequest) => {
      const r = await client.put<Envelope<DockerHost>>(`${base}/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`${base}/${id}`)
    },
    test: async (id: number) => {
      const r = await client.post<Envelope<DockerHostTestResult>>(`${base}/${id}/test`)
      return r.data.data
    },
  }
}

export type DockerHostApi = ReturnType<typeof makeDockerHostApi>
