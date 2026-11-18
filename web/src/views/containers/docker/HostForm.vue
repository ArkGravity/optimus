<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('docker.host.edit') : $t('docker.host.create')"
    :confirm-loading="saving"
    width="640px"
    destroy-on-close
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('docker.host.name')" name="name">
        <a-input v-model:value="form.name" :maxlength="128" />
      </a-form-item>
      <a-form-item :label="$t('docker.host.description')" name="description">
        <a-textarea v-model:value="form.description" :rows="2" :maxlength="4096" />
      </a-form-item>
      <div class="row">
        <a-form-item class="grow" :label="$t('docker.host.host')" name="host">
          <a-input v-model:value="form.host" :maxlength="255" placeholder="10.0.0.12" />
        </a-form-item>
        <a-form-item :label="$t('docker.host.port')" name="port">
          <a-input-number v-model:value="form.port" :min="1" :max="65535" :precision="0" />
        </a-form-item>
      </div>
      <a-form-item :label="$t('docker.host.auth')" name="auth_type">
        <a-radio-group v-model:value="form.auth_type">
          <a-radio-button value="ssh_key">{{ $t('docker.host.auth_ssh_key') }}</a-radio-button>
          <a-radio-button value="password">{{ $t('docker.host.auth_password') }}</a-radio-button>
        </a-radio-group>
      </a-form-item>
      <a-form-item v-if="form.auth_type === 'ssh_key'" :label="$t('docker.host.ssh_key')" name="ssh_key_id">
        <a-select
          v-model:value="form.ssh_key_id"
          :options="keyOptions"
          :loading="loadingKeys"
          :placeholder="$t('docker.host.ssh_key')"
          option-filter-prop="label"
          show-search
        />
      </a-form-item>
      <a-form-item :label="$t('docker.host.username')" name="username">
        <a-input v-model:value="form.username" :maxlength="64" autocomplete="off" />
        <div v-if="form.auth_type === 'ssh_key'" class="hint">{{ $t('docker.host.username_hint') }}</div>
      </a-form-item>
      <a-form-item v-if="form.auth_type === 'password'" :label="$t('docker.host.password')" name="password">
        <a-input-password
          v-model:value="form.password"
          :maxlength="1024"
          autocomplete="new-password"
          :placeholder="keepsPassword ? $t('docker.host.password_keep') : undefined"
        />
      </a-form-item>
      <a-form-item :label="$t('docker.host.socket_path')" name="socket_path">
        <a-input v-model:value="form.socket_path" :maxlength="255" :placeholder="DEFAULT_SOCKET" />
      </a-form-item>
      <a-checkbox v-model:checked="testAfterSave">{{ $t('docker.host.test_after_save') }}</a-checkbox>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { errorText } from './scope'
import type { DockerHostApi } from '@/api/docker/host'
import type { SshKeyApi } from '@/api/credentials/ssh-key'
import type { DockerAuthType, DockerHost, DockerHostUpdateRequest } from '@/types/docker'

const DEFAULT_SOCKET = '/var/run/docker.sock'
const props = defineProps<{ open: boolean; initial?: DockerHost | null }>()
const emit = defineEmits<{ 'update:open': [open: boolean]; saved: [] }>()

const { t } = useI18n()
const hostApi = inject<DockerHostApi>('dockerHostApi')!
const sshKeyApi = inject<SshKeyApi>('sshKeyApi')!
const formRef = ref<FormInstance>()
const saving = ref(false)
const testAfterSave = ref(true)
const isEdit = computed(() => !!props.initial)
// Editing a password host may leave the password blank to keep the stored one.
const keepsPassword = computed(() => props.initial?.auth_type === 'password')

interface FormShape {
  name: string
  description: string
  host: string
  port: number
  auth_type: DockerAuthType
  ssh_key_id: number | undefined
  username: string
  password: string
  socket_path: string
}
const blank = (): FormShape => ({
  name: '', description: '', host: '', port: 22, auth_type: 'ssh_key',
  ssh_key_id: undefined, username: '', password: '', socket_path: DEFAULT_SOCKET,
})
const form = reactive<FormShape>(blank())

const required = computed(() => [{ required: true, message: t('form.required') }])
const rules = computed(() => ({
  name: required.value,
  host: required.value,
  port: [{ required: true, type: 'number' as const, min: 1, max: 65535, message: t('form.required') }],
  ssh_key_id: [{ required: true, type: 'number' as const, message: t('form.required') }],
  username: form.auth_type === 'password' ? required.value : [],
  password: form.auth_type === 'password' && !keepsPassword.value ? required.value : [],
}))

const loadingKeys = ref(false)
const keys = ref<Array<{ id: number; name: string; username: string }>>([])
const keyOptions = computed(() => {
  const options = keys.value.map(key => ({ value: key.id, label: `${key.name} (${key.username})` }))
  const current = props.initial
  if (current?.ssh_key_id && !options.some(option => option.value === current.ssh_key_id)) {
    options.unshift({ value: current.ssh_key_id, label: current.ssh_key_name || `#${current.ssh_key_id}` })
  }
  return options
})

async function loadKeys() {
  if (loadingKeys.value || keys.value.length) return
  loadingKeys.value = true
  try {
    keys.value = (await sshKeyApi.list({ page: 1, page_size: 200 })).items
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    loadingKeys.value = false
  }
}

watch(() => form.auth_type, type => { if (props.open && type === 'ssh_key') void loadKeys() })
watch(() => form.ssh_key_id, (id, previousId) => {
  const next = keys.value.find(key => key.id === id)?.username
  const previous = keys.value.find(key => key.id === previousId)?.username
  if (next && (!form.username || form.username === previous)) form.username = next
})

watch(() => props.open, open => {
  form.password = ''
  if (!open) return
  keys.value = []
  testAfterSave.value = !props.initial
  const initial = props.initial
  Object.assign(form, blank(), initial ? {
    name: initial.name,
    description: initial.description ?? '',
    host: initial.host,
    port: initial.port,
    auth_type: initial.auth_type,
    ssh_key_id: initial.ssh_key_id,
    username: initial.username,
    socket_path: initial.socket_path,
  } : {})
  if (form.auth_type === 'ssh_key') void loadKeys()
}, { immediate: true })

function updatePatch(initial: DockerHost): DockerHostUpdateRequest {
  const patch: DockerHostUpdateRequest = {}
  if (form.name !== initial.name) patch.name = form.name
  if (form.description !== (initial.description ?? '')) patch.description = form.description
  if (form.host !== initial.host) patch.host = form.host
  if (form.port !== initial.port) patch.port = form.port
  if (form.socket_path !== initial.socket_path) patch.socket_path = form.socket_path
  if (form.auth_type !== initial.auth_type) patch.auth_type = form.auth_type
  if (form.username !== initial.username) patch.username = form.username
  if (form.auth_type === 'ssh_key' && form.ssh_key_id !== initial.ssh_key_id) patch.ssh_key_id = form.ssh_key_id
  if (form.auth_type === 'password' && form.password) patch.password = form.password
  return patch
}

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    let id: number
    if (props.initial) {
      id = props.initial.id
      const patch = updatePatch(props.initial)
      if (Object.keys(patch).length) await hostApi.update(id, patch)
      message.success(t('docker.host.updated'))
    } else {
      const created = await hostApi.create({
        name: form.name,
        description: form.description || undefined,
        host: form.host,
        port: form.port,
        socket_path: form.socket_path || undefined,
        auth_type: form.auth_type,
        ssh_key_id: form.auth_type === 'ssh_key' ? form.ssh_key_id : undefined,
        username: form.username || undefined,
        password: form.auth_type === 'password' ? form.password : undefined,
      })
      id = created.id
      message.success(t('docker.host.created'))
    }
    emit('update:open', false)
    if (testAfterSave.value) {
      try {
        const result = await hostApi.test(id)
        if (result.ok) message.success(t('docker.host.test_ok', { version: result.server_version || '-' }))
        else message.error(result.message || t('docker.host.test_failed'))
      } catch (error) {
        message.error(errorText(error, t('network.error')))
      }
    }
    emit('saved')
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped lang="scss">
.row { display: flex; gap: 12px; }
.grow { flex: 1; }
.hint { color: var(--ant-color-text-tertiary); font-size: 12px; margin-top: 4px; }
</style>
