export type Provider = 'aws' | 'aliyun' | 'gcp'
export type AssetFamily = 'instance' | 'vpc' | 'subnet' | 'database' | 'firewall'
export type AssetKey = `${Provider}:${AssetFamily}:${number}`
export type LocationType = 'global' | 'region' | 'zone'

// Use the server key for row identity and links; numeric IDs are legacy aliases.
export interface AssetIdentity {
  asset_key: AssetKey
  provider: Provider
  project_id: string
  location_type: LocationType
  location: string
  zone: string
  native_id: string
  native_status: string
}

export interface NetworkInterface {
  id: string
  name: string
  network_ref: string
  subnet_ref: string
  private_ipv4: string[]
  public_ipv4: string[]
  private_ipv6: string[]
  public_ipv6: string[]
}
export interface GCPAliasRange {
  cidr: string
  subnetwork_range_name?: string
}
export interface GCPExternalIPv6 {
  name?: string
  address: string
  prefix_length?: number
}
export interface GCPNetworkInterfaceDetails {
  interface_index: number
  name?: string
  stack_type?: string
  ipv6_access_type?: string
  internal_ipv6_prefix_length?: number
  alias_ipv4_ranges?: GCPAliasRange[]
  alias_ipv6_ranges?: GCPAliasRange[]
  external_ipv6?: GCPExternalIPv6[]
}
export interface InstanceNativeDetails {
  cpu_count?: number
  memory_mb?: number
  image_id?: string
  os?: string
  network_tags?: string[]
  resource_manager_tags?: Record<string, string>
  gcp_network_interfaces?: GCPNetworkInterfaceDetails[]
}
export interface VPCNativeDetails {
  mtu?: number
  routing_mode?: string
  auto_create_subnetworks?: boolean
}
export interface SubnetNativeDetails {
  available_ip_count?: number
  purpose?: string
  private_ip_google_access?: boolean
}
export interface DatabaseEndpoint {
  address: string
  port: number | null
  type: string
}
export interface GCPDatabaseIPAddress {
  address: string
  type: string
}
export interface GCPDatabaseDNSName {
  name: string
  connection_type?: string
  dns_scope?: string
  record_manager?: string
}
export interface DatabaseNativeDetails {
  primary_dns_name?: string
  write_endpoint?: string
  ipv6_address?: string
  secondary_gce_zone?: string
  gcp_ip_addresses?: GCPDatabaseIPAddress[]
  gcp_dns_names?: GCPDatabaseDNSName[]
  dns_name?: string
  psc_service_attachment_link?: string
  sql_network_architecture?: string
  database_installed_version?: string
  cpu_count?: number
  memory_mb?: number
  instance_type?: string
  storage_type?: string
  connection_name?: string
  availability_type?: string
  category?: string
}
export interface RegionOption { id: string; name: string }
export interface RegionOptionsResponse { items: RegionOption[] }

// Shapes mirror the backend assets DTOs. Field names intentionally remain
// snake_case so the TypeScript contracts match the JSON wire format directly.

export interface CloudAccountSummary {
  id: number
  name: string
  provider: Provider
  cloudkey_id: number
  cloudkey_name: string
  project_id: string
  external_account_id: string
  scope_revision: number
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
  project_id?: string
  name: string
  provider: Provider
  cloudkey_id: number
  enabled_regions: string[]
  enabled?: boolean
  description?: string
}

export interface UpdateCloudAccountRequest {
  provider?: never
  project_id?: never
  cloudkey_id?: never
  name?: string
  enabled_regions?: string[]
  enabled?: boolean
  description?: string
}

export interface InstanceSummary extends AssetIdentity {
  network_interfaces: NetworkInterface[]
  native_details: InstanceNativeDetails
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

export interface VPCSummary extends AssetIdentity {
  network_ref: string
  cidr_blocks: string[]
  native_details: VPCNativeDetails
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  vpc_id: string
  name: string
  cidr_block?: string
  is_default: boolean | null
  state: string
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export interface SubnetSummary extends AssetIdentity {
  cloud_account_name?: string
  network_ref: string
  cidr_blocks: string[]
  native_details: SubnetNativeDetails
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

export interface DatabaseSummary extends AssetIdentity {
  name: string
  product: string
  endpoints: DatabaseEndpoint[]
  ha_mode: string | null
  native_details: DatabaseNativeDetails
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
  multi_az: boolean | null
  publicly_accessible: boolean | null
  storage_gb: number | null
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
}

export type SyncRunStatus = 'running' | 'success' | 'failed' | 'skipped'
export type SyncRunResourceType = 'instance' | 'network' | 'database' | 'security_group' | 'planning' | 'subnet' | 'firewall'
export type SyncRunTrigger = 'cron' | 'manual' | 'test'

export interface SyncRunSummary {
  provider: Provider
  location_type: LocationType
  location: string
  scope_revision: number
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

// Native indexed facts refer to the corresponding direction's common rule array.
export interface AWSRuleFacts {
  rule_index: number
  owner_id?: string
  vpc_id?: string
  peering_id?: string
}
export interface AliyunRuleFacts {
  rule_index: number
  source_group_id?: string
  destination_group_id?: string
  source_prefix_list_id?: string
  destination_prefix_list_id?: string
  rule_id?: string
  nic_type?: string
  source_group_name?: string
  source_group_owner_account?: string
  destination_group_name?: string
  destination_group_owner_account?: string
  source_prefix_list_name?: string
  destination_prefix_list_name?: string
  source_port_range?: string
  port_range_list_id?: string
  port_range_list_name?: string
}
export interface FirewallNativeSummary {
  priority?: number
  disabled?: boolean
  direction?: string
}
export interface FirewallNativeDetails extends FirewallNativeSummary {
  aliyun_inbound_rule_facts?: AliyunRuleFacts[]
  aliyun_outbound_rule_facts?: AliyunRuleFacts[]
  aws_inbound_rule_facts?: AWSRuleFacts[]
  aws_outbound_rule_facts?: AWSRuleFacts[]
  security_group_type?: string
  inner_access_policy?: string
}
export interface FirewallRule {
  direction: string
  protocol: string
  from_port?: number
  to_port?: number
  icmp_type?: number
  icmp_code?: number
  target_type: string
  target: string
  description: string
  action: string
  priority: number | null
  enabled: boolean | null
  source_ranges: string[] | null
  destination_ranges: string[] | null
  source_network_tags: string[] | null
  target_network_tags: string[] | null
  source_service_accounts: string[] | null
  target_service_accounts: string[] | null
}
export interface FirewallSummary extends AssetIdentity {
  id: number
  cloud_account_id: number
  cloud_account_name?: string
  region: string
  name: string
  tags: Record<string, string>
  last_seen_at: string
  deleted: boolean
  network_ref: string
  description: string
  group_id?: string
  group_name?: string
  vpc_id?: string
  owner_id?: string
  inbound_rule_count: number
  outbound_rule_count: number
  native_details: FirewallNativeSummary
}
export interface FirewallDetail extends FirewallSummary {
  inbound_rules: FirewallRule[]
  outbound_rules: FirewallRule[]
  native_details: FirewallNativeDetails
}
