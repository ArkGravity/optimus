// Shapes mirror the backend assets DTOs. Field names intentionally remain
// snake_case so the TypeScript contracts match the JSON wire format directly.

export interface CloudAccountSummary {
  id: number
  name: string
  provider: 'aws'
  cloudkey_id: number
  cloudkey_name: string
  regions_count: number
  enabled: boolean
  last_sync_at?: string
  last_sync_status?: string
  created_at: string
  updated_at: string
}

export interface CloudAccountDetail extends CloudAccountSummary {
  enabled_regions: string[]
  description: string
}

export interface CreateCloudAccountRequest {
  name: string
  provider: 'aws'
  cloudkey_id: number
  enabled_regions: string[]
  enabled?: boolean
  description?: string
}

export interface UpdateCloudAccountRequest {
  name?: string
  enabled_regions?: string[]
  enabled?: boolean
  description?: string
}

export interface InstanceSummary {
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  instance_id: string
  name: string
  instance_type: string
  state: string
  private_ip?: string
  public_ip?: string
  vpc_id?: string
  subnet_id?: string
  availability_zone?: string
  launch_time?: string
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export interface VPCSummary {
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  vpc_id: string
  name: string
  cidr_block?: string
  is_default: boolean
  state: string
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export interface SubnetSummary {
  id: number
  cloud_account_id: number
  region: string
  subnet_id: string
  vpc_id: string
  name: string
  cidr_block?: string
  availability_zone: string
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export interface DatabaseSummary {
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  db_instance_id: string
  engine: string
  engine_version: string
  instance_class: string
  status: string
  endpoint: string
  port?: number
  multi_az: boolean
  publicly_accessible: boolean
  storage_gb?: number
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export type SyncRunStatus = 'running' | 'success' | 'failed' | 'skipped'
export type SyncRunResourceType = 'instance' | 'network' | 'database' | 'security_group'
export type SyncRunTrigger = 'cron' | 'manual' | 'test'

export interface SyncRunSummary {
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  resource_type: SyncRunResourceType
  started_at: string
  finished_at?: string
  status: SyncRunStatus
  items_seen: number
  items_softdeleted: number
  error?: string
  error_code?: number
  trigger: SyncRunTrigger
  triggered_by_user_id?: number
}

export interface ListResponse<T> {
  items: T[]
  total: number
}

export interface SecurityGroupSummary {
  id: number
  cloud_account_id: number
  cloud_account_name: string
  region: string
  group_id: string
  group_name: string
  name: string
  description: string
  vpc_id: string
  owner_id: string
  inbound_rule_count: number
  outbound_rule_count: number
  last_seen_at: string
  deleted: boolean
}
export interface SecurityGroupRule {
  protocol: string
  from_port?: number
  to_port?: number
  target_type: 'ipv4' | 'ipv6' | 'prefix_list' | 'security_group'
  target: string
  description: string
  owner_id?: string
  vpc_id?: string
  peering_id?: string
}
export interface SecurityGroupDetail extends SecurityGroupSummary {
  tags: Record<string, string>
  inbound_rules: SecurityGroupRule[]
  outbound_rules: SecurityGroupRule[]
}
