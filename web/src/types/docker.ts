// Hand-written DTOs mirroring internal/modules/docker/*/dto.go.

export type DockerAuthType = 'ssh_key' | 'password'

export interface DockerHost {
  id: number
  name: string
  description: string
  host: string
  port: number
  socket_path: string
  auth_type: DockerAuthType
  ssh_key_id?: number
  ssh_key_name?: string
  username: string
  last_checked_at?: string
  last_check_ok?: boolean
  last_check_error?: string
  created_at: string
  updated_at: string
}

export interface DockerHostCreateRequest {
  name: string
  description?: string
  host: string
  port?: number
  socket_path?: string
  auth_type: DockerAuthType
  ssh_key_id?: number
  username?: string
  password?: string
}

export type DockerHostUpdateRequest = Partial<DockerHostCreateRequest>

export interface DockerHostProbeRequest {
  host_id?: number
  host: string
  port?: number
  socket_path?: string
  auth_type: DockerAuthType
  ssh_key_id?: number
  username?: string
  password?: string
}

export interface DockerHostListQuery {
  page?: number
  page_size?: number
  search?: string
}

export interface DockerHostListResponse {
  items: DockerHost[]
  total: number
  page: number
  page_size: number
}

export interface DockerHostTestResult {
  ok: boolean
  server_version?: string
  api_version?: string
  message?: string
  checked_at: string
}

export interface DockerImage {
  id: string
  short_id: string
  repo_tags: string[]
  dangling: boolean
  size: number
  created: string
}

export interface DockerImageDeleteResponse {
  untagged: string[]
  deleted: string[]
}

export interface DockerImagePruneResponse {
  untagged: string[]
  deleted: string[]
  space_reclaimed: number
}

export interface DockerContainer {
  id: string
  short_id: string
  name: string
  image: string
  image_id: string
  command: string
  state: string
  status: string
  ports: string[]
  created: string
}

export interface DockerContainerDetail {
  id: string
  short_id: string
  name: string
  image: string
  image_id: string
  created?: string
  state: {
    status: string
    running: boolean
    paused: boolean
    restarting: boolean
    oom_killed: boolean
    dead: boolean
    exit_code: number
    error?: string
    health?: string
    started_at?: string
    finished_at?: string
  }
  command: string[]
  restart_policy: { name: string; maximum_retry_count: number }
  restart_count: number
  ports: string[]
  networks: Array<{ name: string; ip_address?: string; ipv6_address?: string; gateway?: string; mac_address?: string }>
  mounts: Array<{ type: string; name?: string; source: string; destination: string; mode?: string; rw: boolean }>
  env: string[]
}

export type DockerContainerAction = 'start' | 'stop' | 'restart'

export interface DockerList<T> {
  items: T[]
  total: number
}
