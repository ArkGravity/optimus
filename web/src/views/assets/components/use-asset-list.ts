import { computed, inject, onMounted, ref, shallowRef } from 'vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { ResourceListParams } from '@/api/assets/resource'
import { useAssetFilters, type AssetFilterState, type AssetFiltersController } from '@/hooks/use-asset-filters'
import { usePermission } from '@/hooks/usePermission'
import { useI18n } from '@/hooks/useI18n'
import type { AssetFamily, DatabaseEndpoint, ListResponse, LocationType, NetworkInterface } from '@/types/assets'

// The controller owns request generations. Rows and totals are never workspace state.
export function useListData<T>(controller: AssetFiltersController, request: () => Promise<ListResponse<T>>) {
  const items = shallowRef<T[]>([])
  const total = ref(0)
  const { t } = useI18n()
  const errorMessage = computed(() => controller.requestError.value || controller.accountError.value ? t('network.error') : '')
  async function reload() {
    items.value = []
    total.value = 0
    try {
      await controller.runRequest(request, result => { items.value = result.items; total.value = result.total })
    } catch { /* The current error is rendered locally; obsolete failures are discarded by the controller. */ }
  }
  function onPageChange(page: number, size: number) {
    const nextPage = size !== controller.pageSize.value ? 1 : page
    if (nextPage === controller.page.value && size === controller.pageSize.value) return
    controller.pageSize.value = size
    controller.page.value = nextPage
    void reload()
  }
  onMounted(() => { void reload() })
  return { items, total, errorMessage, reload, onPageChange }
}

// Frozen list seam: family-specific pages supply supported extra filters and their typed fetcher.
export function useResourceList<T>(family: Exclude<AssetFamily, 'subnet'>, fetcher: (params: AssetFilterState & ResourceListParams) => Promise<ListResponse<T>>, extraFields: Array<keyof AssetFilterState> = []) {
  const { has } = usePermission()
  const controller = useAssetFilters({ family, canReadAccounts: () => has('assets:account:read'), accountApi: inject<AssetsAccountApi>('assetsAccountApi') })
  const data = useListData(controller, () => {
    const params: AssetFilterState & ResourceListParams = { page: controller.page.value, size: controller.pageSize.value }
    for (const key of ['provider', 'account_id', 'project_id', 'region', 'zone', 'q', 'include_deleted', ...extraFields] as Array<keyof AssetFilterState>) {
      const value = controller.filters.value[key]
      if (value !== undefined && value !== '') Object.assign(params, { [key]: value })
    }
    return fetcher(params)
  })
  return { controller, ...data }
}
export function locationLabel(record: { location_type: LocationType; location: string }, t: (key: string) => string) {
  const type = t(`assets.filters.location.${record.location_type}`)
  return record.location_type === 'global' ? type : `${type}: ${record.location || '—'}`
}
export function formatTags(tags: Record<string, string>) { return Object.entries(tags).map(([key, value]) => `${key}=${value}`).join(', ') || '—' }
export function booleanFact(value: boolean | null | undefined, t: (key: string) => string) { return t(value == null ? 'assets.detail.not_applicable' : value ? 'assets.detail.yes' : 'assets.detail.no') }

export function observedAddresses(nics: NetworkInterface[], visibility: 'private' | 'public') {
  return [...new Set(nics.flatMap(nic => [...nic[`${visibility}_ipv4`], ...nic[`${visibility}_ipv6`]]))].join(', ')
}
export function observedNetworks(nics: NetworkInterface[]) { return [...new Set(nics.map(nic => nic.network_ref).filter(Boolean))].join(', ') }

export function formatEndpoints(endpoints: DatabaseEndpoint[]) { return endpoints.map(endpoint => `${endpoint.type}: ${endpoint.address}${endpoint.port == null ? '' : `:${endpoint.port}`}`).join(', ') }

export function scalarFact(value: unknown, t: (key: string) => string) { return value == null || value === '' ? t('assets.detail.not_applicable') : value }
