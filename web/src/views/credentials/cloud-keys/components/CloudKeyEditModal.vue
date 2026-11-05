<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('credentials.action.edit') : $t('credentials.action.create')"
    :confirm-loading="saving"
    width="640px"
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
      <a-form-item :label="$t('credentials.field.provider')" name="provider">
        <a-radio-group v-model:value="form.provider">
          <a-radio-button value="aliyun">Aliyun</a-radio-button>
          <a-radio-button value="aws">AWS</a-radio-button>
          <a-radio-button value="gcp">GCP</a-radio-button>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="$t('credentials.field.region')" name="region">
        <a-input v-model:value="form.region" :maxlength="32" />
      </a-form-item>
      <template v-if="isGcp">
        <a-alert
          v-if="initial?.provider === 'gcp' && initial.needs_configuration"
          type="warning"
          show-icon
          class="u-mb-16"
          :message="$t('credentials.cloud_keys.needs_configuration')"
          :description="$t('credentials.cloud_keys.repair_hint')"
        />
        <dl v-if="initial?.provider === 'gcp' && initial.service_account_email" class="identity">
          <dt>{{ $t('credentials.field.service_account_email') }}</dt>
          <dd>{{ initial.service_account_email }}</dd>
          <dt>{{ $t('credentials.field.credential_project_id') }}</dt>
          <dd>{{ initial.credential_project_id }}</dd>
        </dl>
        <a-form-item :label="$t('credentials.field.service_account_json')" name="service_account_json">
          <input
            ref="fileInput"
            type="file"
            accept=".json,application/json"
            :aria-label="$t('credentials.cloud_keys.upload_json')"
            @change="onFileChange"
          />
          <p>{{ $t('credentials.cloud_keys.json_hint') }}</p>
          <a-textarea
            v-model:value="form.service_account_json"
            :rows="8"
            :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : $t('credentials.cloud_keys.paste_json')"
            autocomplete="off"
            spellcheck="false"
            @update:value="invalidateFileRead"
          />
        </a-form-item>
      </template>
      <template v-else>
        <a-form-item :label="$t('credentials.field.access_key_id')" name="access_key_id">
          <a-input
            v-model:value="form.access_key_id"
            :maxlength="256"
            :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
            autocomplete="off"
          />
        </a-form-item>
        <a-form-item :label="$t('credentials.field.secret_access_key')" name="secret_access_key">
          <a-input-password
            v-model:value="form.secret_access_key"
            :placeholder="isEdit ? $t('credentials.placeholder.unchanged') : ''"
            autocomplete="new-password"
          />
        </a-form-item>
      </template>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { CloudKeyApi } from '@/api/credentials/cloud-key'
import type { CloudKeySecret, CloudKeySummary, CloudKeyUpdateRequest } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: CloudKeySummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const cloudKeyApi = inject<CloudKeyApi>('cloudKeyApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()
const fileInput = ref<HTMLInputElement>()
let draftVersion = 0
let fileReadVersion = 0

const form = reactive<{
  name: string
  description: string
  provider: CloudKeySummary['provider']
  region: string
  access_key_id: string
  secret_access_key: string
  service_account_json: string
}>({
  name: '',
  description: '',
  provider: 'aws',
  region: '',
  access_key_id: '',
  secret_access_key: '',
  service_account_json: '',
})
let initialSnapshot = {
  name: '', description: '', region: '',
}

const isGcp = computed(() => form.provider === 'gcp')
const hasSecretDraft = computed(() => isGcp.value
  ? !!form.service_account_json
  : !!(form.access_key_id || form.secret_access_key))
const needsReplacement = computed(() => !isEdit.value
  || form.provider !== props.initial?.provider || hasSecretDraft.value)

const rules = computed(() => ({
  name:              [{ required: true, max: 128, message: t('form.required') }],
  provider:          [{ required: true, message: t('form.required') }],
  access_key_id:     !isGcp.value && needsReplacement.value ? [{ required: true, message: t('form.required') }] : [],
  secret_access_key: !isGcp.value && needsReplacement.value ? [{ required: true, message: t('form.required') }] : [],
  service_account_json: isGcp.value && needsReplacement.value ? [{ required: true, message: t('form.required') }] : [],
}))

function invalidateFileRead() { fileReadVersion++ }

function clearSecrets() {
  draftVersion++
  invalidateFileRead()
  form.access_key_id = ''
  form.secret_access_key = ''
  form.service_account_json = ''
  if (fileInput.value) fileInput.value.value = ''
  formRef.value?.clearValidate(['access_key_id', 'secret_access_key', 'service_account_json'])
}

watch(() => form.provider, clearSecrets, { flush: 'sync' })
watch(() => props.open, (open) => {
  if (!open) clearSecrets()
}, { flush: 'sync' })
onBeforeUnmount(clearSecrets)

// Vue patches dynamic props in order; initialize after both props are updated.
watch(
  [() => props.open, () => props.initial],
  ([open, initial]) => {
    clearSecrets()
    if (!open) return
    formRef.value?.clearValidate()
    if (initial) {
      form.name = initial.name
      form.description = initial.description
      form.provider = initial.provider
      form.region = initial.region
      initialSnapshot = {
        name: initial.name,
        description: initial.description,
        region: initial.region,
      }
    } else {
      form.name = ''
      form.description = ''
      form.provider = 'aws'
      form.region = ''
    }
  },
  { immediate: true }
)

function close() {
  clearSecrets()
  emit('update:open', false)
}

async function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  const version = ++fileReadVersion
  if (!file || !props.open || !isGcp.value) return
  if (file.size > 64 * 1024) {
    message.error(t('credentials.cloud_keys.json_too_large'))
    return
  }
  try {
    const text = await file.text()
    if (version !== fileReadVersion || !props.open || !isGcp.value) return
    form.service_account_json = text
    formRef.value?.clearValidate('service_account_json')
  } catch {
    if (version === fileReadVersion && props.open && isGcp.value) {
      message.error(t('credentials.cloud_keys.json_read_failed'))
    }
  }
}

function secretPayload(): CloudKeySecret {
  return form.provider === 'gcp'
    ? { provider: 'gcp', auth_type: 'service_account_json', service_account_json: form.service_account_json }
    : {
      provider: form.provider, auth_type: 'access_key',
      access_key_id: form.access_key_id, secret_access_key: form.secret_access_key,
    }
}

async function onOk() {
  if (saving.value || !props.open) return
  const version = draftVersion
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (version !== draftVersion || !props.open) return
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const metadata = formDiff(initialSnapshot, {
        name: form.name,
        description: form.description,
        region: form.region,
      })
      const patch: CloudKeyUpdateRequest = needsReplacement.value
        ? { ...metadata, ...secretPayload() }
        : metadata
      if (Object.keys(patch).length > 0) {
        await cloudKeyApi.update(props.initial.id, patch)
      }
      message.success(t('credentials.toast.updated'))
    } else {
      await cloudKeyApi.create({
        name: form.name,
        description: form.description || undefined,
        region: form.region || undefined,
        ...secretPayload(),
      })
      message.success(t('credentials.toast.created'))
    }
    emit('saved')
    if (version === draftVersion) close()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.identity dd {
  margin-inline-start: 0;
  overflow-wrap: anywhere;
}
</style>
