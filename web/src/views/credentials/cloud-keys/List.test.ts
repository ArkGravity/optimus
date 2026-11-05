/* eslint-disable vue/require-prop-types */
import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { CloudKeySummary } from '@/types/api'
import List from './List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePageState', () => ({ usePageState: vi.fn() }))

const Table = defineComponent({
  props: ['columns', 'dataSource'],
  setup(props, { slots }) {
    return () => h('div', props.dataSource.map((record: CloudKeySummary) =>
      h('div', props.columns.map((column: { key: string }) => slots.bodyCell?.({ record, column }))),
    ))
  },
})

describe('Cloud Key identity summaries', () => {
  it.each([false, true])('shows safe GCP identity and configuration status (legacy: %s)', async (legacy) => {
    const summary: CloudKeySummary = {
      id: 1, name: 'gcp', description: '', provider: 'gcp', region: '', created_at: '', updated_at: '',
      auth_type: legacy ? 'legacy_invalid' : 'service_account_json', needs_configuration: legacy,
      service_account_email: legacy ? '' : 'fixture@example.iam.gserviceaccount.com',
      credential_project_id: legacy ? '' : 'fixture-project',
    }
    const wrapper = mount(List, {
      global: {
        mocks: { $t: (key: string) => key },
        provide: { cloudKeyApi: { list: vi.fn().mockResolvedValue({ items: [summary], total: 1 }) } },
        directives: { permission: () => {} },
        stubs: {
          'a-card': { template: '<div><slot/></div>' }, 'a-table': Table,
          'a-space': { template: '<span><slot/></span>' }, 'a-tag': { template: '<span><slot/></span>' },
          'a-button': true, 'a-input-search': true, 'a-select': true, 'a-pagination': true,
          'a-popconfirm': true, PageHeader: true, CloudKeyEditModal: true,
        },
      },
    })
    await flushPromises()
    if (legacy) expect(wrapper.text()).toContain('credentials.cloud_keys.needs_configuration')
    else {
      expect(wrapper.text()).toContain(summary.service_account_email)
      expect(wrapper.text()).toContain(summary.credential_project_id)
    }
    wrapper.unmount()
  })
})
