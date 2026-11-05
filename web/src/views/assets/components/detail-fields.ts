import type { AssetKey, DatabaseSummary, FirewallDetail, InstanceSummary, SubnetSummary, VPCSummary } from '@/types/assets'

export type DetailFamily = 'instance' | 'vpc' | 'database' | 'firewall'
export type DetailRecord = { family: 'instance'; data: InstanceSummary } | { family: 'vpc'; data: VPCSummary } | { family: 'database'; data: DatabaseSummary } | { family: 'firewall'; data: FirewallDetail }
type FieldValue = string | number | boolean | null | undefined
export interface DetailField { label: string; value: FieldValue | string[] }
export interface DetailSection { key: string; title: string; fields: DetailField[] }
const field = (label: string, value: DetailField['value']): DetailField => ({ label, value })
const section = (key: string, title: string, fields: DetailField[]): DetailSection => ({ key, title, fields })
const pairs = (value: Record<string, string> | undefined) => Object.entries(value ?? {}).map(([key, value]) => `${key}=${value}`)

// Canonical keys remain strings, including IDs beyond JavaScript's safe integer range.
export function parseDetailKey(raw: string | number | undefined, family: DetailFamily): AssetKey | number | undefined {
  if (typeof raw === 'string') {
    const canonical = raw.match(new RegExp(`^(aws|aliyun|gcp):${family}:([1-9]\\d*)$`))
    const digits = canonical?.[2] ?? raw
    if (!/^[1-9]\d*$/.test(digits) || digits.length > 19 || (digits.length === 19 && digits > '9223372036854775807')) return undefined
    if (canonical) return raw as AssetKey
    if (Number(digits) > Number.MAX_SAFE_INTEGER) return `aws:${family}:${digits}` as AssetKey
  }
  const numeric = typeof raw === 'string' ? Number(raw) : raw
  return numeric !== undefined && Number.isSafeInteger(numeric) && numeric > 0 ? numeric : undefined
}

export function detailSections(detail: DetailRecord): DetailSection[] {
  const data = detail.data
  const sections = [section('common', 'common', [
    field('asset_key', data.asset_key), field('provider', data.provider.toUpperCase()), field('account', data.cloud_account_name || data.cloud_account_id), field('project_id', data.project_id),
    field('name', data.name), field('native_id', data.native_id), field('native_status', data.native_status), field('location_type', data.location_type), field('location', data.location), field('region', data.region), field('zone', data.zone),
    field('last_seen_at', data.last_seen_at), field('deleted', data.deleted), field(data.provider === 'gcp' ? 'labels' : 'tags', pairs(data.tags)),
  ])]
  if (detail.family === 'instance') {
    const data = detail.data; const native = data.native_details
    sections.push(section('instance', 'instance', [field('instance_type', data.instance_type), field('state', data.state), field('launch_time', data.launch_time), field('cpu_count', native.cpu_count), field('memory_mb', native.memory_mb), field('image_id', native.image_id), field('os', native.os), field('network_tags', native.network_tags), field('resource_manager_tags', native.resource_manager_tags ? pairs(native.resource_manager_tags) : undefined)]))
    data.network_interfaces.forEach((nic, index) => {
      const nativeNIC = native.gcp_network_interfaces?.find(value => value.interface_index === index)
      const fields = [field('interface_index', index), field('native_id', nic.id), field('name', nic.name), field('network_ref', nic.network_ref), field('subnet_ref', nic.subnet_ref), field('private_ipv4', nic.private_ipv4), field('public_ipv4', nic.public_ipv4), field('private_ipv6', nic.private_ipv6), field('public_ipv6', nic.public_ipv6)]
      if (nativeNIC) fields.push(
        field('native_name', nativeNIC.name), field('stack_type', nativeNIC.stack_type), field('ipv6_access_type', nativeNIC.ipv6_access_type), field('internal_ipv6_prefix_length', nativeNIC.internal_ipv6_prefix_length),
        field('alias_ipv4_ranges', nativeNIC.alias_ipv4_ranges?.map(range => range.subnetwork_range_name ? `${range.cidr} (${range.subnetwork_range_name})` : range.cidr)),
        field('alias_ipv6_ranges', nativeNIC.alias_ipv6_ranges?.map(range => range.subnetwork_range_name ? `${range.cidr} (${range.subnetwork_range_name})` : range.cidr)),
        field('external_ipv6', nativeNIC.external_ipv6?.map(value => `${value.address}${value.prefix_length == null ? '' : `/${value.prefix_length}`}${value.name ? ` (${value.name})` : ''}`)),
      )
      sections.push(section(`nic-${index}`, 'network_interface', fields))
    })
  } else if (detail.family === 'vpc') {
    const data = detail.data
    sections.push(section('vpc', 'vpc', [field('network_ref', data.network_ref), field('cidr_blocks', data.cidr_blocks), field('is_default', data.is_default), field('state', data.state), field('mtu', data.native_details.mtu), field('routing_mode', data.native_details.routing_mode), field('auto_create_subnetworks', data.native_details.auto_create_subnetworks)]))
  } else if (detail.family === 'database') {
    const data = detail.data; const native = data.native_details
    sections.push(section('database', 'database', [field('product', data.product), field('engine', data.engine), field('engine_version', data.engine_version), field('instance_class', data.instance_class), field('status', data.status), field('ha_mode', data.ha_mode), field('multi_az', data.multi_az), field('publicly_accessible', data.publicly_accessible), field('storage_gb', data.storage_gb)]))
    data.endpoints.forEach((endpoint, index) => sections.push(section(`endpoint-${index}`, 'connection_endpoint', [field('address', endpoint.address), field('port', endpoint.port), field('type', endpoint.type)])))
    sections.push(section('database-native', 'database_native', [
      field('primary_dns_name', native.primary_dns_name), field('write_endpoint', native.write_endpoint), field('ipv6_address', native.ipv6_address), field('secondary_gce_zone', native.secondary_gce_zone), field('dns_name', native.dns_name), field('psc_service_attachment_link', native.psc_service_attachment_link), field('sql_network_architecture', native.sql_network_architecture), field('database_installed_version', native.database_installed_version), field('cpu_count', native.cpu_count), field('memory_mb', native.memory_mb), field('instance_type', native.instance_type), field('storage_type', native.storage_type), field('connection_name', native.connection_name), field('availability_type', native.availability_type), field('category', native.category),
    ]))
    native.gcp_dns_names?.forEach((dns, index) => sections.push(section(`database-provenance-dns-${index}`, 'database_provenance_dns', [field('name', dns.name), field('connection_type', dns.connection_type), field('dns_scope', dns.dns_scope), field('record_manager', dns.record_manager)])))
    native.gcp_ip_addresses?.forEach((address, index) => sections.push(section(`database-provenance-ip-${index}`, 'database_provenance_ip', [field('address', address.address), field('type', address.type)])))
  } else {
    const data = detail.data
    sections.push(section('firewall', 'firewall', [field('network_ref', data.network_ref), field('description', data.description), field('group_id', data.group_id), field('group_name', data.group_name), field('vpc_id', data.vpc_id), field('owner_id', data.owner_id), field('inbound_rule_count', data.inbound_rule_count), field('outbound_rule_count', data.outbound_rule_count), field('priority', data.native_details.priority), field('enabled', data.native_details.disabled == null ? undefined : !data.native_details.disabled), field('direction', data.native_details.direction), field('security_group_type', data.native_details.security_group_type), field('inner_access_policy', data.native_details.inner_access_policy)]))
  }
  return sections
}

export function subnetSection(data: SubnetSummary): DetailSection {
  return section(data.asset_key, 'subnet', [field('asset_key', data.asset_key), field('native_id', data.native_id), field('name', data.name), field('provider', data.provider.toUpperCase()), field('account', data.cloud_account_name || data.cloud_account_id), field('project_id', data.project_id), field('location_type', data.location_type), field('location', data.location), field('region', data.region), field('zone', data.zone), field('network_ref', data.network_ref), field('cidr_blocks', data.cidr_blocks), field('available_ip_count', data.native_details.available_ip_count), field('purpose', data.native_details.purpose), field('private_ip_google_access', data.native_details.private_ip_google_access), field(data.provider === 'gcp' ? 'labels' : 'tags', pairs(data.tags)), field('last_seen_at', data.last_seen_at), field('deleted', data.deleted)])
}
