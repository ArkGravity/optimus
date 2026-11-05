import { vi } from 'vitest'
import List from '../List.vue'
import { resourceListContract, permission } from '../../components/__tests__/resource-list-contract'
import { firewall } from '../../components/__tests__/fixtures'
resourceListContract(List, 'firewall', 'listFirewalls', firewall)
vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => permission.allowed }) }))
