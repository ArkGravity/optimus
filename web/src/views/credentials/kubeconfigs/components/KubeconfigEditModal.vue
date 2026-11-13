<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('credentials.action.edit') : $t('credentials.action.create')"
    :confirm-loading="saving"
    width="720px"
    @ok="onOk"
    @cancel="close"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('credentials.field.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.description')" name="description">
        <a-textarea v-model:value="form.description" :rows="2" :maxlength="4096" />
      </a-form-item>
      <a-form-item :label="$t('credentials.field.default_namespace')" name="default_namespace">
        <a-input v-model:value="form.default_namespace" :maxlength="64" />
      </a-form-item>
      <a-form-item :label="$t('credentials.gke.authentication')" name="auth_type">
        <a-select v-model:value="form.auth_type" :disabled="isEdit" :options="authOptions" />
      </a-form-item>
      <template v-if="isGKE">
        <a-alert class="u-mb-16" type="info" :message="$t('credentials.gke.hint')" show-icon />
        <a-form-item :label="$t('credentials.gke.cloud_key')" name="cloud_key_id">
          <a-select v-model:value="form.cloud_key_id" :loading="loadingKeys" :options="cloudKeys" show-search option-filter-prop="label" />
        </a-form-item>
        <a-form-item :label="$t('credentials.gke.project')" name="project">
          <a-input v-model:value="form.project" :disabled="isEdit" :maxlength="128" />
        </a-form-item>
        <a-form-item :label="$t('credentials.gke.location')" name="location">
          <a-input v-model:value="form.location" :disabled="isEdit" placeholder="us-central1 / us-central1-a" :maxlength="64" />
        </a-form-item>
        <a-form-item :label="$t('credentials.gke.cluster')" name="cluster">
          <a-input v-model:value="form.cluster" :disabled="isEdit" :maxlength="64" />
        </a-form-item>
        <a-form-item :label="$t('credentials.gke.endpoint')" name="endpoint">
          <a-select v-model:value="form.endpoint" :disabled="isEdit" :options="endpointOptions" />
        </a-form-item>
      </template>
      <a-form-item v-else :label="$t('credentials.field.kubeconfig')" name="kubeconfig">
        <a-textarea
          v-model:value="form.kubeconfig"
          :rows="12"
          :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
          class="mono"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { KubeconfigApi } from '@/api/credentials/kubeconfig'
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import type { GKEConnection, KubeconfigSummary, KubeconfigUpdateRequest } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: KubeconfigSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const kubeconfigApi = inject<KubeconfigApi>('kubeconfigApi')!
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()
const loadingKeys = ref(false)
const cloudKeys = ref<{ value: number; label: string }[]>([])
let generation = 0
let keyGeneration = 0

const form = reactive({
  name: '',
  description: '',
  default_namespace: '',
  kubeconfig: '',
  auth_type: 'kubeconfig' as 'kubeconfig' | 'gke_service_account',
  cloud_key_id: undefined as number | undefined,
  project: '', location: '', cluster: '',
  endpoint: 'default' as GKEConnection['endpoint'],
})
let initialSnapshot = { name: '', description: '', default_namespace: '', cloud_key_id: undefined as number | undefined }
const isGKE = computed(() => form.auth_type === 'gke_service_account')
const authOptions = computed(() => [
  { value: 'kubeconfig', label: t('credentials.gke.kubeconfig') },
  { value: 'gke_service_account', label: t('credentials.gke.service_account') },
])
const endpointOptions = computed(() => ['default', 'public', 'private', 'dns'].map(value => ({ value, label: t(`credentials.gke.endpoint_${value}`) })))

const rules = computed(() => ({
  name:       [{ required: true, max: 128, message: t('form.required') }],
  kubeconfig: isEdit.value || isGKE.value ? [] : [{ required: true, message: t('form.required') }],
  ...(isGKE.value ? Object.fromEntries(['cloud_key_id', 'project', 'location', 'cluster', 'endpoint'].map(key => [key, [{ required: true, message: t('form.required') }]])) : {}),
}))

watch(
  () => [props.open, props.initial] as const,
  ([open]) => {
    generation++
    saving.value = false
    form.kubeconfig = ''
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.name = props.initial.name
      form.description = props.initial.description
      form.default_namespace = props.initial.default_namespace
      form.kubeconfig = ''
      form.auth_type = props.initial.auth_type ?? 'kubeconfig'
      form.cloud_key_id = props.initial.cloud_key_id
      form.project = props.initial.gke?.project ?? ''
      form.location = props.initial.gke?.location ?? ''
      form.cluster = props.initial.gke?.cluster ?? ''
      form.endpoint = props.initial.gke?.endpoint ?? 'default'
      initialSnapshot = {
        name: props.initial.name,
        description: props.initial.description,
        default_namespace: props.initial.default_namespace,
        cloud_key_id: props.initial.cloud_key_id,
      }
    } else {
      form.name = ''
      form.description = ''
      form.default_namespace = ''
      form.kubeconfig = ''
      form.auth_type = 'kubeconfig'
      form.cloud_key_id = undefined
      form.project = ''; form.location = ''; form.cluster = ''; form.endpoint = 'default'
    }
  },
  { immediate: true, flush: 'post' }
)

watch(() => [props.open, form.auth_type, props.initial?.id] as const, async ([open, mode]) => {
  const request = ++keyGeneration
  cloudKeys.value = []
  loadingKeys.value = false
  formRef.value?.clearValidate?.()
  if (mode === 'gke_service_account') form.kubeconfig = ''
  if (!open || mode !== 'gke_service_account') return
  loadingKeys.value = true
  try {
    const items: { value: number; label: string }[] = []
    for (let page = 1; ; page++) {
      const result = await cloudKeyApi.list({ page, page_size: 100, provider: 'gcp' })
      if (request !== keyGeneration) return
      items.push(...result.items.filter(key => key.auth_type === 'service_account_json').map(key => ({ value: key.id, label: key.name })))
      if (page * 100 >= result.total || result.items.length === 0) break
    }
    cloudKeys.value = items
    const selected = props.initial
    if (selected?.cloud_key_id && !items.some(key => key.value === selected.cloud_key_id)) {
      cloudKeys.value.push({ value: selected.cloud_key_id, label: `#${selected.cloud_key_id}` })
    }
  } catch (e) {
    if (request === keyGeneration) message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    if (request === keyGeneration) loadingKeys.value = false
  }
}, { immediate: true, flush: 'post' })

function clearDraft() {
  generation++
  keyGeneration++
  form.kubeconfig = ''
}
function close() {
  clearDraft()
  emit('update:open', false)
}
onBeforeUnmount(clearDraft)

async function onOk() {
  if (saving.value || loadingKeys.value) return
  const request = generation
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (request !== generation || !props.open) return
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch: KubeconfigUpdateRequest = formDiff(initialSnapshot, {
        name: form.name,
        description: form.description,
        default_namespace: form.default_namespace,
        cloud_key_id: form.cloud_key_id,
      })
      if (!isGKE.value && form.kubeconfig) patch.kubeconfig = form.kubeconfig
      if (Object.keys(patch).length > 0) {
        await kubeconfigApi.update(props.initial.id, patch)
      }
      if (request === generation) message.success(t('credentials.toast.updated'))
    } else {
      const metadata = {
        name: form.name,
        description: form.description || undefined,
        default_namespace: form.default_namespace || undefined,
      }
      if (isGKE.value) {
        if (form.cloud_key_id === undefined) return
        await kubeconfigApi.create({ ...metadata, auth_type: 'gke_service_account', cloud_key_id: form.cloud_key_id,
          gke: { project: form.project.trim(), location: form.location.trim(), cluster: form.cluster.trim(), endpoint: form.endpoint } })
      } else {
        await kubeconfigApi.create({ ...metadata, kubeconfig: form.kubeconfig })
      }
      if (request === generation) message.success(t('credentials.toast.created'))
    }
    if (request !== generation) return
    emit('saved')
    close()
  } catch (e) {
    if (request === generation) message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    if (request === generation) saving.value = false
  }
}
</script>

<style scoped lang="scss">
.mono {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}
</style>
