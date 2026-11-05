import { computed, onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { ResourceListParams } from '@/api/assets/resource'
import type { AssetFamily } from '@/types/assets'
import { usePageState } from './usePageState'

export interface AssetFilterState extends Omit<ResourceListParams, 'page' | 'size'> {
  state?: string
  vpc_id?: string
  network_ref?: string
  ip?: string
  tag_key?: string
  tag_value?: string
  engine?: string
  status?: string
}
export interface AssetFiltersOptions {
  family: AssetFamily
  canReadAccounts: MaybeRefOrGetter<boolean>
  accountApi?: Pick<AssetsAccountApi, 'list'>
  defaultFilters?: AssetFilterState
  defaultPageSize?: number
}

// Only controls are restored. Responses, options and request generations are ephemeral.
export function useAssetFilters(options: AssetFiltersOptions) {
  const filters = ref<AssetFilterState>({ ...options.defaultFilters })
  const page = ref(1)
  const pageSize = ref(options.defaultPageSize ?? 20)
  usePageState({ filters, page, pageSize }, 'asset-filters')
  const canReadAccounts = computed(() => toValue(options.canReadAccounts))
  const accountOptions = ref<Array<{ label: string; value: number }>>([])
  const loadingAccounts = ref(false)
  const accountError = ref<unknown>()
  const loading = ref(false)
  const requestError = ref<unknown>()
  let alive = true
  let generation = 0
  let accountGeneration = 0

  function invalidateRequests() { generation += 1; loading.value = false; requestError.value = undefined }
  function captureRequest() { generation += 1; return generation }
  function isCurrentRequest(token: number) { return alive && token === generation }
  async function runRequest<T>(request: () => Promise<T>, apply: (value: T) => void): Promise<boolean> {
    const token = captureRequest()
    loading.value = true
    requestError.value = undefined
    try {
      const result = await request()
      if (!isCurrentRequest(token)) return false
      apply(result)
      return true
    } catch (error) {
      if (!isCurrentRequest(token)) return false
      requestError.value = error
      throw error
    } finally {
      if (isCurrentRequest(token)) loading.value = false
    }
  }

  function setFilters(patch: Partial<AssetFilterState>) {
    const next = { ...filters.value }
    const changed = (key: keyof AssetFilterState) => Object.hasOwn(patch, key) && patch[key] !== next[key]
    if (changed('provider') || changed('account_id')) {
      for (const key of ['account_id', 'project_id', 'region', 'zone', 'vpc_id', 'network_ref', 'state', 'engine', 'status'] as const) {
        if (key !== 'account_id' || changed('provider')) delete next[key]
      }
    } else if (changed('project_id')) {
      for (const key of ['region', 'zone', 'vpc_id', 'network_ref'] as const) delete next[key]
    } else if (changed('region')) delete next.zone
    Object.assign(next, patch)
    for (const key of Object.keys(next) as Array<keyof AssetFilterState>) {
      if (next[key] === undefined || next[key] === '') delete next[key]
    }
    filters.value = next
    page.value = 1
  }

  watch(filters, () => { page.value = 1; invalidateRequests() }, { deep: true, flush: 'sync' })
  watch([page, pageSize], invalidateRequests, { flush: 'sync' })

  async function loadAccounts() {
    const token = ++accountGeneration
    accountOptions.value = []
    accountError.value = undefined
    loadingAccounts.value = false
    if (!canReadAccounts.value || !options.accountApi) return
    loadingAccounts.value = true
    const current = () => alive && canReadAccounts.value && token === accountGeneration
    const provider = filters.value.provider
    try {
      const choices: Array<{ label: string; value: number }> = []
      let currentPage = 1
      let total = 0
      do {
        const result = await options.accountApi.list({ provider, page: currentPage, size: 100 })
        if (!current()) return
        choices.push(...result.items.map(account => ({ label: account.name, value: account.id })))
        total = result.total
        if (result.items.length === 0) break
        currentPage += 1
      } while (choices.length < total)
      if (current()) accountOptions.value = choices
    } catch (error) {
      if (current()) accountError.value = error
    } finally {
      if (current()) loadingAccounts.value = false
    }
  }
  watch([canReadAccounts, () => filters.value.provider], () => { void loadAccounts() }, { immediate: true, flush: 'sync' })
  onScopeDispose(() => {
    alive = false
    invalidateRequests()
    accountGeneration += 1
    loadingAccounts.value = false
    accountOptions.value = []
    accountError.value = undefined
  })
  return { family: options.family, filters, page, pageSize, canReadAccounts, accountOptions, loadingAccounts, accountError, loading, requestError, setFilters, loadAccounts, captureRequest, isCurrentRequest, invalidateRequests, runRequest }
}

export type AssetFiltersController = ReturnType<typeof useAssetFilters>
