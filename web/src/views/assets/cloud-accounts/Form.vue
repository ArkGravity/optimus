<template>
  <a-modal :open="open" :title="isEdit ? t('assets.account.actions.edit') : t('assets.account.actions.create')" :confirm-loading="submitting" width="640px" @ok="onOk" @cancel="close">
    <a-alert v-if="!isEdit && !canReadKeys" type="warning" :message="t('assets.account.credential_read_required')" />
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" />
    <a-form ref="formRef" :model="form" :rules="rules" layout="vertical">
      <a-form-item :label="t('assets.account.name')" name="name"><a-input v-model:value="form.name" :maxlength="128" /></a-form-item>
      <a-form-item :label="t('assets.account.provider')"><a-input data-testid="provider" :value="providerLabel" disabled /></a-form-item>
      <a-form-item :label="t('assets.account.cloudkey')" name="cloudkey_id">
        <a-select v-model:value="form.cloudkey_id" data-testid="cloudkey" :disabled="isEdit || !canReadKeys" :loading="loadingCloudKeys" :options="cloudKeyOptions" @change="onKeyChange" />
      </a-form-item>
      <a-form-item v-if="form.provider === 'gcp'" :label="t('assets.account.project_id')" name="project_id">
        <a-input v-model:value="form.project_id" :disabled="isEdit" @change="onProjectChange" />
        <p v-if="selectedKey?.credential_project_id">{{ t('assets.account.credential_project_info') }}: {{ selectedKey.credential_project_id }}</p>
      </a-form-item>
      <a-form-item :label="t('assets.account.regions')" name="enabled_regions">
        <a-button data-testid="load-regions" :disabled="!canLoadRegions || loadingRegions" :loading="loadingRegions" @click="loadRegions">{{ t('assets.account.load_regions') }}</a-button>
        <a-select v-model:value="form.enabled_regions" data-testid="regions" mode="multiple" :options="regionOptions" />
      </a-form-item>
      <a-form-item :label="t('assets.account.enabled')"><a-switch v-model:checked="form.enabled" /></a-form-item>
      <a-form-item :label="t('assets.account.description')"><a-textarea v-model:value="form.description" :rows="3" :maxlength="2000" /></a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import type { CloudKeySummary } from '@/types/api'
import type { CloudAccountDetail, CreateCloudAccountRequest, Provider, RegionOption, UpdateCloudAccountRequest } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const props = defineProps<{ open: boolean; editing?: CloudAccountDetail | null }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'saved'): void }>()
const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!
const { t } = useI18n()
const permission = usePermission()
const canReadKeys = computed(() => permission.has('credentials:cloud_key:read'))
const isEdit = computed(() => props.editing != null)
const formRef = ref<FormInstance>()
const submitting = ref(false)
const loadingCloudKeys = ref(false)
const loadingRegions = ref(false)
const errorMessage = ref('')
const cloudKeys = ref<CloudKeySummary[]>([])
const regions = ref<RegionOption[]>([])
const form = reactive({ name: '', provider: '' as Provider | '', cloudkey_id: undefined as number | undefined, project_id: '', enabled_regions: [] as string[], enabled: true, description: '' })
let epoch = 0
let keyGeneration = 0
let regionGeneration = 0
let submissionGeneration = 0
let active = true
const current = (version: number) => active && props.open && version === epoch
const selectedKey = computed(() => cloudKeys.value.find(key => key.id === form.cloudkey_id))
const providerLabel = computed(() => ({ aws: 'AWS', aliyun: 'Aliyun', gcp: 'GCP', '': '' })[form.provider])
const cloudKeyOptions = computed(() => isEdit.value ? [{ value: props.editing!.cloudkey_id, label: props.editing!.cloudkey_name || String(props.editing!.cloudkey_id) }] : cloudKeys.value.map(key => ({ value: key.id, label: key.name })))
const regionOptions = computed(() => {
  const options = new Map(regions.value.map(region => [region.id, { value: region.id, label: region.name || region.id }]))
  for (const id of form.enabled_regions) if (!options.has(id)) options.set(id, { value: id, label: id })
  return [...options.values()]
})
const canLoadRegions = computed(() => !!form.provider && !!form.cloudkey_id && (form.provider !== 'gcp' || !!form.project_id.trim()))
const rules = computed(() => ({
  name: [{ required: true, max: 128, message: t('form.required') }],
  cloudkey_id: [{ required: true, type: 'number' as const, min: 1, message: t('form.required') }],
  project_id: [{ required: form.provider === 'gcp', whitespace: true, message: t('form.required') }],
  enabled_regions: [{ required: true, type: 'array' as const, min: 1, message: t('form.required') }],
}))
function invalidateRegions(clear: boolean) {
  regionGeneration++; regions.value = []; loadingRegions.value = false; errorMessage.value = ''
  if (clear) form.enabled_regions = []
  formRef.value?.clearValidate()
}
function clearDraft() {
  epoch++; keyGeneration++; invalidateSubmission(); invalidateRegions(true); cloudKeys.value = []; loadingCloudKeys.value = false
  Object.assign(form, { name: '', provider: '', cloudkey_id: undefined, project_id: '', enabled: true, description: '' })
}
function close() { clearDraft(); emit('close') }
function invalidateSubmission() { submissionGeneration++; submitting.value = false }
function onKeyChange() {
  invalidateSubmission()
  form.provider = selectedKey.value?.provider ?? ''; form.project_id = ''; invalidateRegions(true)
}
function onProjectChange() { invalidateSubmission(); invalidateRegions(true) }
function usable(key: CloudKeySummary) {
  return !key.needs_configuration && (key.provider === 'gcp' ? key.auth_type === 'service_account_json' : key.auth_type === 'access_key')
}
async function loadCloudKeys() {
  const version = epoch; const generation = ++keyGeneration; const owns = () => current(version) && generation === keyGeneration && !isEdit.value && canReadKeys.value
  loadingCloudKeys.value = true
  try {
    const all: CloudKeySummary[] = []
    for (let page = 1; ; page++) {
      const result = await cloudKeyApi.list({ page, page_size: 100 })
      if (!owns()) return
      all.push(...result.items)
      if (all.length >= result.total) break
      if (!result.items.length) throw new Error('Incomplete credential summaries')
    }
    cloudKeys.value = all.filter(usable)
  } catch (error) { if (owns()) errorMessage.value = isBizError(error) ? error.message : t('network.error') }
  finally { if (owns()) loadingCloudKeys.value = false }
}
async function loadRegions() {
  if (!canLoadRegions.value || loadingRegions.value || !form.provider || !form.cloudkey_id) return
  const version = epoch; const generation = ++regionGeneration; const owns = () => current(version) && generation === regionGeneration
  const params = { provider: form.provider, cloudkey_id: form.cloudkey_id, ...(form.provider === 'gcp' ? { project_id: form.project_id.trim() } : {}) }
  loadingRegions.value = true; errorMessage.value = ''
  try { const result = await accountApi.regionOptions(params); if (owns()) regions.value = result.items }
  catch (error) { if (owns()) errorMessage.value = isBizError(error) ? error.message : t('network.error') }
  finally { if (owns()) loadingRegions.value = false }
}
// One batched watcher observes the parent's coherent open/record pair.
watch(() => [props.open, props.editing] as const, ([open, account]) => {
  clearDraft()
  if (!open) return
  if (account) Object.assign(form, { name: account.name, provider: account.provider, cloudkey_id: account.cloudkey_id, project_id: account.project_id, enabled_regions: [...account.enabled_regions], enabled: account.enabled, description: account.description })
  else if (canReadKeys.value) void loadCloudKeys()
}, { immediate: true })
watch(canReadKeys, allowed => {
  if (!props.open || isEdit.value) return
  keyGeneration++; loadingCloudKeys.value = false; cloudKeys.value = []; form.cloudkey_id = undefined; onKeyChange()
  if (allowed) void loadCloudKeys()
})
onBeforeUnmount(() => { active = false; clearDraft() })
async function onOk() {
  if (submitting.value || !props.open) return
  const version = epoch; const editID = props.editing?.id
  const generation = ++submissionGeneration
  const owns = () => current(version) && generation === submissionGeneration
  submitting.value = true
  try {
    await formRef.value?.validate()
    if (!owns()) return
    if (!form.name.trim() || !form.provider || !form.cloudkey_id || !form.enabled_regions.length || (form.provider === 'gcp' && !form.project_id.trim()) || (editID == null && (!canReadKeys.value || !selectedKey.value))) return
    const metadata: UpdateCloudAccountRequest = { name: form.name, enabled_regions: [...form.enabled_regions], enabled: form.enabled, description: form.description }
    if (editID != null) await accountApi.update(editID, metadata)
    else {
      const request: CreateCloudAccountRequest = { name: form.name, provider: form.provider, cloudkey_id: form.cloudkey_id, enabled_regions: [...form.enabled_regions], enabled: form.enabled, description: form.description, ...(form.provider === 'gcp' ? { project_id: form.project_id.trim() } : {}) }
      await accountApi.create(request)
    }
    if (!owns()) return
    message.success(t('common.message.done')); clearDraft(); emit('saved')
  } catch (error) {
    // Ant Form validation errors already render beside their fields.
    if (owns() && !(typeof error === 'object' && error !== null && 'errorFields' in error)) errorMessage.value = isBizError(error) ? error.message : t('network.error')
  } finally { if (owns()) submitting.value = false }
}
</script>
