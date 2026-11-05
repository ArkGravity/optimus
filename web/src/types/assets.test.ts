import { describe, expectTypeOf, it } from 'vitest'
import type { AssetIdentity, AssetKey, CloudAccountSummary, DatabaseSummary, FirewallRule, InstanceSummary, SubnetSummary, UpdateCloudAccountRequest, VPCSummary } from './assets'

describe('asset wire contracts', () => {
  it('requires common identities and preserves nullable facts independently of false and zero', () => {
    expectTypeOf<VPCSummary['is_default']>().toEqualTypeOf<boolean | null>()
    expectTypeOf<DatabaseSummary['multi_az']>().toEqualTypeOf<boolean | null>()
    expectTypeOf<DatabaseSummary['publicly_accessible']>().toEqualTypeOf<boolean | null>()
    expectTypeOf<DatabaseSummary['storage_gb']>().toEqualTypeOf<number | null>()
    expectTypeOf<DatabaseSummary['ha_mode']>().toEqualTypeOf<string | null>()
    expectTypeOf<FirewallRule['priority']>().toEqualTypeOf<number | null>()
    expectTypeOf<FirewallRule['enabled']>().toEqualTypeOf<boolean | null>()
    expectTypeOf<InstanceSummary>().toExtend<AssetIdentity>()
    expectTypeOf<SubnetSummary>().toExtend<AssetIdentity>()
    expectTypeOf<AssetKey>().toEqualTypeOf<`${'aws' | 'aliyun' | 'gcp'}:${'instance' | 'vpc' | 'subnet' | 'database' | 'firewall'}:${number}`>()
    expectTypeOf<CloudAccountSummary['provider']>().toEqualTypeOf<'aws' | 'aliyun' | 'gcp'>()
  })
  it('forbids immutable edit targets even when a request was first assigned to a variable', () => {
    expectTypeOf<{ name: string; provider: 'gcp' }>().not.toExtend<UpdateCloudAccountRequest>()
    expectTypeOf<{ name: string; project_id: string }>().not.toExtend<UpdateCloudAccountRequest>()
    expectTypeOf<{ name: string; cloudkey_id: number }>().not.toExtend<UpdateCloudAccountRequest>()
    expectTypeOf<{ name: string; enabled_regions: string[]; enabled: boolean }>().toExtend<UpdateCloudAccountRequest>()
  })
})
