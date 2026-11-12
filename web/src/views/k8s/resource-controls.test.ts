import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Antd, { message } from 'ant-design-vue'
import { useK8sStore } from '@/stores/k8s'
import { useAuthStore } from '@/stores/auth'
import Configs from './config/List.vue'
import Storage from './storage/List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addListener: vi.fn(), removeListener: vi.fn() })))
  const getStyle = window.getComputedStyle
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => getStyle(element))
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); document.body.innerHTML = '' })
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const result = (name: string) => ({ items: [{ name, namespace: 'default', data_keys: [], data_count: 0, access_modes: [], age: '2026-11-01T00:00:00Z' }], truncated: false, continue: '' })
async function selectKind(wrapper: VueWrapper, value: string) {
  await wrapper.get(`input[type="radio"][value="${value}"]`).setValue(true)
  await flushPromises()
}
function setup(storage = false, permissions = ['k8s:config:read', 'k8s:secret:read', 'k8s:secret:reveal', 'k8s:storage:read']) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setPermissions(permissions)
  useK8sStore().setCluster(7, 'development')
  const config = { list: vi.fn().mockResolvedValue(result('app-config')), get: vi.fn() }
  const secret = { list: vi.fn().mockResolvedValue(result('app-secret')), get: vi.fn(), data: vi.fn() }
  const storageApi = { list: vi.fn().mockResolvedValue(result('data-volume')) }
  const wrapper = mount(storage ? Storage : Configs, { global: {
    plugins: [pinia, Antd], mocks: { $t: (key: string) => key }, directives: { permission: () => undefined },
    provide: { configMapApi: config, secretApi: secret, k8sStorageApi: storageApi, k8sNsApi: { list: vi.fn().mockResolvedValue({ items: [] }) } },
    stubs: { ClusterPicker: true, ResourceDrawer: true },
  } })
  return { wrapper, config, secret, storageApi, k8s: useK8sStore() }
}

describe('resource lists with actual Ant Design controls', () => {
  it('switches ConfigMap / Secrets at the top right and searches the current list', async () => {
    const { wrapper, config, secret } = setup()
    await flushPromises()
    expect(wrapper.find('.ant-card-extra .resource-kind').exists()).toBe(true)
    expect(wrapper.find('.ant-tabs-nav-wrap').exists()).toBe(false)
    expect(wrapper.text()).toContain('app-config')
    await selectKind(wrapper, 'secrets')
    expect(secret.list).toHaveBeenCalledWith(7, { namespace: undefined })
    expect(config.list).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('app-secret')
    expect(wrapper.text()).not.toContain('app-config')
    await wrapper.get('.resource-toolbar input.ant-input').setValue('absent')
    expect(wrapper.text()).not.toContain('app-secret')
    wrapper.unmount()
  })
  it('loads only Secrets for a Secret-only role without requesting ConfigMaps or namespaces', async () => {
    const { wrapper, config, secret } = setup(false, ['k8s:secret:read'])
    await flushPromises()
    expect(wrapper.find('input[value="configmaps"]').exists()).toBe(false)
    expect(config.list).not.toHaveBeenCalled()
    expect(secret.list).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
  it('discards old resource/cluster successes and old errors without changing current loading', async () => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => (() => undefined) as ReturnType<typeof message.error>)
    const { wrapper, config, secret, k8s } = setup()
    await flushPromises()
    const old = deferred<ReturnType<typeof result>>()
    const current = deferred<ReturnType<typeof result>>()
    secret.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    await selectKind(wrapper, 'secrets')
    k8s.setCluster(9, 'production')
    await flushPromises()
    old.resolve(result('wrong-cluster'))
    await flushPromises()
    expect(wrapper.text()).not.toContain('wrong-cluster')
    expect(wrapper.find('.ant-spin-spinning').exists()).toBe(true)
    current.resolve(result('current-secret'))
    await flushPromises()
    expect(wrapper.text()).toContain('current-secret')
    const stale = deferred<ReturnType<typeof result>>()
    config.list.mockReturnValueOnce(stale.promise)
    await selectKind(wrapper, 'configmaps')
    await selectKind(wrapper, 'secrets')
    stale.reject(new Error('old failure'))
    await flushPromises()
    expect(error).not.toHaveBeenCalled()
    wrapper.unmount()
    error.mockRestore()
  })
  it('does not reopen a Secret reveal after switching resource type', async () => {
    const { wrapper, secret } = setup()
    await flushPromises()
    await selectKind(wrapper, 'secrets')
    const reveal = deferred<{ data: Record<string, string> }>()
    secret.data.mockReturnValueOnce(reveal.promise)
    await wrapper.get('.ant-table-tbody a').trigger('click')
    await selectKind(wrapper, 'configmaps')
    reveal.resolve({ data: { password: 'sensitive-test-value' } })
    await flushPromises()
    expect(document.body.textContent).not.toContain('sensitive-test-value')
    expect(wrapper.text()).not.toContain('sensitive-test-value')
    wrapper.unmount()
  })
  it('switches PV / PVC / StorageClass and sends namespace filters only for PVC', async () => {
    const { wrapper, storageApi, k8s } = setup(true)
    await flushPromises()
    expect(storageApi.list).toHaveBeenLastCalledWith(7, 'persistentvolumes', {})
    k8s.setCurrentNamespace('production')
    await flushPromises()
    await selectKind(wrapper, 'persistentvolumeclaims')
    expect(storageApi.list).toHaveBeenLastCalledWith(7, 'persistentvolumeclaims', { namespace: 'production' })
    expect(wrapper.find('.resource-toolbar .ant-select').exists()).toBe(true)
    await selectKind(wrapper, 'storageclasses')
    expect(storageApi.list).toHaveBeenLastCalledWith(7, 'storageclasses', {})
    expect(wrapper.find('.resource-toolbar .ant-select').exists()).toBe(false)
    wrapper.unmount()
  })
})
