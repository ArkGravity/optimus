import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AssetDetail from './AssetDetail.vue'
import { database, firewall, instance, rule, vpc } from './__tests__/fixtures'
import type { AssetsResourceApi } from '@/api/assets/resource'
import type { InstanceSummary } from '@/types/assets'
import { pageStateKey } from '@/hooks/usePageState'
import { BizError } from '@/utils/http-error'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const permission = reactive({ allowed: true })
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => permission.allowed }) }))
const stubs = { 'a-card': { template: '<section><slot/></section>' }, 'a-spin': { template: '<div><slot/></div>' }, 'a-alert': { props: ['message'], template: '<p role="alert">{{ message }}</p>' }, 'a-button': { template: '<button><slot/></button>' }, PageHeader: { props: ['title'], template: '<header>{{ title }}<slot/></header>' } }
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (reason: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup(props: { family: 'instance' | 'vpc' | 'database' | 'firewall'; assetKey: string | number; open?: boolean }, api: Partial<AssetsResourceApi> = {}) {
  const resourceApi = { getInstance: vi.fn().mockResolvedValue(instance), getVPC: vi.fn().mockResolvedValue(vpc), getDatabase: vi.fn().mockResolvedValue(database), getFirewall: vi.fn().mockResolvedValue(firewall), listSubnets: vi.fn().mockResolvedValue({ items: [], total: 0 }), ...api }
  const state = { values: {}, scrollTop: 0, scrollLeft: 0 }
  const wrapper = mount(AssetDetail, { props, global: { provide: { assetsResourceApi: resourceApi, [pageStateKey as symbol]: state }, stubs } })
  return { wrapper, api: resourceApi, state }
}
beforeEach(() => { permission.allowed = true })
describe('shared asset details', () => {
  it('renders all NIC addresses and external references with indexed alias facts and distinct tag families', async () => {
    const { wrapper, api, state } = setup({ family: 'instance', assetKey: 'gcp:instance:42' })
    await flushPromises()
    expect(api.getInstance).toHaveBeenCalledWith('gcp:instance:42', { include_deleted: true })
    for (const value of ['9223372036854775807', 'target-project', 'inventory-account', 'Linux', 'image-1', 'network-tag', 'tagKeys/1=tagValues/2', 'label=value']) expect(wrapper.text()).toContain(value)
    for (const nic of instance.network_interfaces) for (const value of [...nic.private_ipv4, ...nic.public_ipv4, ...nic.private_ipv6, ...nic.public_ipv6, nic.network_ref, nic.subnet_ref]) expect(wrapper.text()).toContain(value)
    expect(wrapper.find('[data-section="nic-0"]').text()).not.toContain('10.1.1.0/24')
    expect(wrapper.find('[data-section="nic-1"]').text()).toContain('10.1.1.0/24 (pods)')
    expect(wrapper.find('[data-section="nic-1"]').text()).toContain('fd21:1::/64 (services)')
    expect(wrapper.find('[data-section="nic-1"]').text()).toContain('2001:db8:1::/96 (external-v6)')
    expect(wrapper.findAll('a')).toHaveLength(0)
    expect(state.values).toEqual({})
    wrapper.unmount()
  })
  it('keeps nullable facts unknown while preserving real zero and false values', async () => {
    const { wrapper } = setup({ family: 'database', assetKey: 'gcp:database:42' })
    await flushPromises()
    expect(wrapper.find('[data-field="ha_mode"]').text()).toContain('assets.detail.not_applicable')
    expect(wrapper.find('[data-field="multi_az"]').text()).toContain('assets.detail.not_applicable')
    expect(wrapper.find('[data-field="publicly_accessible"] dd').text()).toBe('assets.detail.no')
    expect(wrapper.find('[data-field="deleted"] dd').text()).toBe('assets.detail.no')
    expect(wrapper.find('[data-field="storage_gb"]').text()).toContain('0')
    expect(wrapper.find('[data-section="endpoint-0"]').text()).toContain('0')
    expect(wrapper.find('[data-section="endpoint-1"]').text()).toContain('assets.detail.not_applicable')
    wrapper.unmount()
  })
  it('shows all database provenance separately from connection endpoints', async () => {
    const { wrapper } = setup({ family: 'database', assetKey: 'gcp:database:42' })
    await flushPromises()
    for (const value of ['primary.example', 'writer.example', '2001:db8::99', 'europe-west4-b', 'native.example', 'projects/target/serviceAttachments/db', 'NEW_NETWORK_ARCHITECTURE', 'POSTGRES_17_1', 'CLOUD_SQL_INSTANCE', 'PD_SSD', 'target:europe:db', 'REGIONAL', 'HighAvailability']) expect(wrapper.find('[data-section="database-native"]').text()).toContain(value)
    const dns = wrapper.find('[data-section="database-provenance-dns-0"]')
    expect(dns.find('[data-field="name"]').text()).toContain('psc.example')
    expect(dns.find('[data-field="connection_type"]').text()).toContain('PRIVATE_SERVICE_CONNECT')
    expect(dns.find('[data-field="dns_scope"]').text()).toContain('INSTANCE')
    expect(dns.find('[data-field="record_manager"]').text()).toContain('CLOUD_SQL')
    expect(wrapper.find('[data-section="database-provenance-ip-0"]').text()).toContain('OUTGOING')
    expect(wrapper.find('[data-section="database-provenance-ip-0"]').text()).toContain('203.0.113.22')
    expect(wrapper.find('[data-section="endpoint-0"]').text()).not.toContain('203.0.113.22')
    expect(wrapper.text()).toContain('assets.detail.provenance_notice')
    wrapper.unmount()
  })
  it('renders configured firewall rules with zero priority, disabled state, selectors and no invented GCP ICMP codes', async () => {
    const { wrapper } = setup({ family: 'firewall', assetKey: 'gcp:firewall:42' }, { getFirewall: vi.fn().mockResolvedValue({ ...firewall, inbound_rules: [rule, { ...rule, protocol: 'icmp', from_port: 8, to_port: 8, priority: null, enabled: null }] }) })
    await flushPromises()
    const first = wrapper.find('[data-rule="inbound-0"]')
    for (const value of ['assets.firewall_rules.deny', 'assets.firewall_rules.disabled', 'source-tag', 'target-tag', 'source@example', 'target@example', '10.0.0.0/8', '10.1.0.0/16']) expect(first.text()).toContain(value)
    expect(first.find('[data-rule-field="priority"]').text()).toBe('0')
    expect(first.find('[data-rule-field="ports"]').text()).toBe('0')
    expect(wrapper.find('[data-rule="inbound-1"] [data-rule-field="ports"]').text()).toBe('assets.detail.not_applicable')
    expect(wrapper.text()).toContain('assets.detail.firewall_inventory_notice')
    wrapper.unmount()
  })
  it('preserves direction-local indexed AWS targets and Aliyun managed port-list references', async () => {
    const data = { ...firewall, provider: 'aliyun', asset_key: 'aliyun:firewall:42', inbound_rules: [rule, { ...rule, from_port: undefined, to_port: undefined }], outbound_rules: [{ ...rule, direction: 'outbound' }], native_details: { security_group_type: 'enterprise', inner_access_policy: 'Drop', aliyun_inbound_rule_facts: [{ rule_index: 1, port_range_list_id: 'pl-123', port_range_list_name: 'managed-ports', source_port_range: '1024/65535', source_group_id: 'sg-source', source_group_name: 'source-group', source_group_owner_account: 'owner-a', destination_group_id: 'sg-dest', destination_group_name: 'dest-group', destination_group_owner_account: 'owner-b', source_prefix_list_id: 'prefix-source', source_prefix_list_name: 'source-prefix', destination_prefix_list_id: 'prefix-dest', destination_prefix_list_name: 'dest-prefix', rule_id: 'sgr-1', nic_type: 'intranet' }], aws_outbound_rule_facts: [{ rule_index: 0, owner_id: 'aws-owner', vpc_id: 'vpc-external', peering_id: 'pcx-1' }] } }
    const { wrapper } = setup({ family: 'firewall', assetKey: 'aliyun:firewall:42' }, { getFirewall: vi.fn().mockResolvedValue(data) })
    await flushPromises()
    expect(wrapper.find('[data-rule="inbound-0"]').text()).not.toContain('managed-ports')
    const indexed = wrapper.find('[data-rule="inbound-1"]')
    expect(indexed.find('[data-rule-field="ports"]').text()).toBe('assets.firewall_rules.port_list: managed-ports (pl-123)')
    for (const value of ['1024/65535', 'sg-source', 'source-group', 'owner-a', 'sg-dest', 'dest-group', 'owner-b', 'prefix-source', 'source-prefix', 'prefix-dest', 'dest-prefix', 'sgr-1', 'intranet']) expect(indexed.text()).toContain(value)
    for (const value of ['aws-owner', 'vpc-external', 'pcx-1']) expect(wrapper.find('[data-rule="outbound-0"]').text()).toContain(value)
    wrapper.unmount()
  })
  it('discards out-of-order loads and clears data as soon as the key changes', async () => {
    const old = deferred<InstanceSummary>(); const current = deferred<InstanceSummary>(); const next = deferred<InstanceSummary>()
    const { wrapper } = setup({ family: 'instance', assetKey: 'aws:instance:42' }, { getInstance: vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise).mockReturnValueOnce(next.promise) })
    await wrapper.setProps({ assetKey: 'gcp:instance:42' })
    current.resolve(instance); await flushPromises()
    old.resolve({ ...instance, name: 'stale-aws' }); await flushPromises()
    expect(wrapper.text()).toContain('observed-resource'); expect(wrapper.text()).not.toContain('stale-aws')
    await wrapper.setProps({ assetKey: 'gcp:instance:43' })
    expect(wrapper.text()).not.toContain('observed-resource')
    wrapper.unmount()
  })
  it('discards responses across close/reopen and unmount without retaining data', async () => {
    const old = deferred<InstanceSummary>(); const current = deferred<InstanceSummary>()
    const getInstance = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const { wrapper, state } = setup({ family: 'instance', assetKey: 'gcp:instance:42', open: true }, { getInstance })
    await wrapper.setProps({ open: false })
    expect(wrapper.find('[data-testid="asset-detail"]').exists()).toBe(false)
    await wrapper.setProps({ open: true })
    old.resolve({ ...instance, name: 'closed-response' }); await flushPromises()
    expect(wrapper.text()).not.toContain('closed-response')
    current.resolve(instance); await flushPromises()
    expect(wrapper.text()).toContain('observed-resource')
    await wrapper.setProps({ open: false }); expect(wrapper.text()).not.toContain('observed-resource')
    expect(state.values).toEqual({})
    wrapper.unmount()
    const unmounted = deferred<InstanceSummary>()
    const other = setup({ family: 'instance', assetKey: 'gcp:instance:42' }, { getInstance: vi.fn().mockReturnValue(unmounted.promise) })
    other.wrapper.unmount(); unmounted.reject(new Error('late failure')); await flushPromises()
    expect(other.state.values).toEqual({})
  })
  it('handles current read failures locally and ignores obsolete failures', async () => {
    const old = deferred<InstanceSummary>()
    const { wrapper } = setup({ family: 'instance', assetKey: 'aws:instance:42' }, { getInstance: vi.fn().mockReturnValueOnce(old.promise).mockResolvedValueOnce(instance).mockRejectedValueOnce(new Error('403 raw transport')) })
    await wrapper.setProps({ assetKey: 'gcp:instance:42' }); await flushPromises()
    old.reject(new Error('stale failure')); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    await wrapper.setProps({ assetKey: 'gcp:instance:43' }); await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('network.error')
    expect(wrapper.text()).not.toContain('observed-resource'); expect(wrapper.text()).not.toContain('raw transport')
    wrapper.unmount()
  })
  it('clears reads on reactive resource permission revocation and refuses invalid identities', async () => {
    const pending = deferred<InstanceSummary>()
    const { wrapper, api } = setup({ family: 'instance', assetKey: 'gcp:instance:42' }, { getInstance: vi.fn().mockReturnValue(pending.promise) })
    permission.allowed = false; await flushPromises()
    pending.resolve(instance); await flushPromises()
    expect(wrapper.text()).not.toContain('observed-resource'); expect(wrapper.text()).toContain('assets.detail.forbidden')
    expect(api.getInstance).toHaveBeenCalledOnce()
    wrapper.unmount()
    permission.allowed = true
    for (const key of ['gcp:vpc:42', '42oops', Number.MAX_SAFE_INTEGER + 2]) {
      const invalid = setup({ family: 'instance', assetKey: key }); await flushPromises()
      expect(invalid.api.getInstance).not.toHaveBeenCalled(); expect(invalid.wrapper.text()).toContain('assets.detail.invalid_key')
      invalid.wrapper.unmount()
    }
  })
  it('shows envelope read failures without exposing prior details or requesting subnets', async () => {
    const { wrapper, api } = setup({ family: 'vpc', assetKey: 'gcp:vpc:42' }, { getVPC: vi.fn().mockRejectedValue(new BizError(40301, 'Read forbidden')) })
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toBe('Read forbidden')
    expect(api.listSubnets).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('observed-resource')
    wrapper.unmount()
  })
  it('preserves AWS zero ICMP type/code, wildcards and native group/prefix references', async () => {
    const rules = [{ ...rule, protocol: 'icmp', from_port: undefined, to_port: undefined, icmp_type: 0, icmp_code: 0, target_type: 'security_group', target: 'sg-1' }, { ...rule, protocol: '-1', target_type: 'prefix_list', target: 'pl-1' }]
    const { wrapper } = setup({ family: 'firewall', assetKey: 'aws:firewall:42' }, { getFirewall: vi.fn().mockResolvedValue({ ...firewall, provider: 'aws', inbound_rules: rules }) })
    await flushPromises()
    expect(wrapper.find('[data-rule="inbound-0"] [data-rule-field="ports"]').text()).toBe('0 / 0')
    expect(wrapper.find('[data-rule="inbound-0"]').text()).toContain('sg-1')
    expect(wrapper.find('[data-rule="inbound-1"] [data-rule-field="ports"]').text()).toBe('assets.firewall_rules.all')
    expect(wrapper.find('[data-rule="inbound-1"]').text()).toContain('pl-1')
    wrapper.unmount()
  })
})
