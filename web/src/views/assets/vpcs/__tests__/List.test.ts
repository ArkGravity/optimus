import { vi } from 'vitest'
import List from '../List.vue'
import { resourceListContract, permission } from '../../components/__tests__/resource-list-contract'
import { vpc } from '../../components/__tests__/fixtures'
resourceListContract(List, 'vpc', 'listVPCs', vpc)

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => permission.allowed }) }))
