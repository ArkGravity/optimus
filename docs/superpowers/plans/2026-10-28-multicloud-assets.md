# Multi-cloud Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Aliyun and GCP assets with cloud-independent AK/SK and Service
Account JSON authentication, native provider tables and unified asset pages.

**Architecture:** Keep AWS tables and IDs; wrap its collectors behind a provider
registry. Add Aliyun/GCP collectors and tables, a shared lifecycle engine and
common SQL read projections. Credentials, accounts, runs, permissions and audit
remain shared. The GCP database table is **`gcp_databases`**.

**Tech Stack:** Go 1.25.0, Gin, GORM, Goose, PostgreSQL 17, AWS SDK v2, Alibaba
Cloud Go SDK product clients, Google API Go client, Vue 3, Ant Design Vue, Bun,
Vitest. No new frontend dependencies.

---

## State and execution policy

- Design: [approved scope and contracts](../specs/2026-10-28-multicloud-assets-design.md).
- Branch: `feat/multicloud-assets`; base: `3bd0add47e4f333d7984e8164010fc04de3c3037`.
- Baseline [CI 40702433368](https://github.com/ArkGravity/optimus/actions/runs/40702433368)
  passed all six gates. Execution started on 2026-10-28 using subagents at the
  operator's explicit request. Task completion is recorded below after review.
- The user requested a new branch; it already exists in the current checkout.
  Honor that workspace preference before creating an additional worktree.
- Initial implementation authorized verified checkpoints only. On 2026-11-05,
  after basic local functional acceptance, the operator explicitly authorized
  committing/pushing all changes, creating a PR and merging it into main.
  Production rebuilding/deployment and full acceptance remain with the operator.
- Run shell commands through `rtk`, web work through Bun, and Go with 1.25.0.
  Keep TMPDIR/GOCACHE/GOLANGCI_LINT_CACHE under the repository's ignored tmp/.
- Verify Colima before database tests; use disposable PostgreSQL 17 and never
  an application DSN for OPTIMUS_TEST_POSTGRES_DSN.
- Tests below are behavior tests. Red must be caused by the missing behavior,
  not a broken environment. Fix routine implementation errors autonomously.
- All plan source paths are relative to the repository root. Code comments
  and code identifiers remain English. Runtime menu source is internal/seed.

## File map and task order

| Area | New files | Existing files to change |
|---|---|---|
| Common identity | `internal/modules/assets/resource/{identity,location,firewall}.go` and tests | family DTOs/web types |
| Credentials | `cloudkey/{auth,service_account}.go`, tests; 00024 | credential model/DTO/repo/service/Consumer/modal |
| Target scope | `account/{scope,region_options}.go`, tests; 00025 | account model/DTO/repo/service/handler |
| Native storage | `internal/models/assets_{aliyun,gcp}.go`; 00026/00027 | account cascade and migration tests |
| Read projections | `assets/query/{repo,instances,vpcs,subnets,databases,firewalls}.go`, tests | family repos/services and Consumer |
| Firewalls HTTP | `assets/firewall/{dto,repo,service,handler}.go`, tests | module wiring; legacy securitygroup retained |
| Shared engine | `assets/sync/{contracts,registry,scope}.go`, tests; 00028 | engine/runs/scheduler/module |
| AWS adapter | `assets/providers/aws/{provider,identity}.go`, tests | awsclient/existing fetchers |
| Aliyun adapter | `assets/providers/aliyun/{client,provider,instances,network,databases,firewalls,persist,maperror}.go`, tests | composition |
| GCP adapter | `assets/providers/gcp/{client,provider,instances,network,databases,firewalls,persist,maperror}.go`, tests | composition |
| Shared UI | `web/src/views/assets/firewalls/`, `assets/components/`, filter hook | lists/account form/API/router/locales/seed fixture |
| Acceptance | `tests/integration/assets_multicloud_test.go`, `scripts/p4-multicloud-smoke.md` | Swagger/README/AGENTS/P4 checklist |

In the file map, cloudkey is `internal/modules/credentials/cloudkey`; account
and assets prefixes are `internal/modules/assets/account` and
`internal/modules/assets`. Expanded paths appear in the tasks below.

Execute 1 -> 2 -> 3. Task 4 follows 3. Task 5 follows 3; 6 follows 5.
Tasks 7/8 follow 1/3/5/6. Task 9 follows 1/6; 10 follows 9. Provider tasks
11..14 follow 7/8. UI task 15 integrates 4/5/10..14. Tasks 16/17 close delivery.
Do not run implementation agents concurrently against shared migrations,
models, wiring, locale files or generated artifacts.

## Task 1: Define common resource identity and rule contracts

**Create:** `internal/modules/assets/resource/identity.go`, `identity_test.go`,
`location.go`, `location_test.go`, `firewall.go`.

- [x] **1.1 Write this failing identity test:**

```go
package resource

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestParseAssetKey(t *testing.T) {
    key, err := ParseAssetKey("gcp:database:42", "database")
    require.NoError(t, err)
    require.Equal(t, AssetKey{Provider: "gcp", Family: "database", ID: 42}, key)
    legacy, err := ParseAssetKey("42", "vpc")
    require.NoError(t, err)
    require.Equal(t, "aws", legacy.Provider)
    for _, raw := range []string{"gcp:instance:42", "azure:database:42", "gcp:database:0", "gcp:database:9223372036854775808"} {
        _, err := ParseAssetKey(raw, "database")
        require.Error(t, err, raw)
    }
}
```

- [x] **1.2 Run red:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/resource -run TestParseAssetKey -count=1`.
  Expected: missing AssetKey/ParseAssetKey.
- [x] **1.3 Implement the parser and canonical string:**

```go
package resource

import (
    "errors"
    "strconv"
    "strings"
)

type AssetKey struct { Provider, Family string; ID uint64 }

func ParseAssetKey(raw, family string) (AssetKey, error) {
    invalid := errors.New("invalid asset key")
    switch family { case "instance", "vpc", "subnet", "database", "firewall": default: return AssetKey{}, invalid }
    parts := strings.Split(raw, ":")
    if len(parts) == 1 { parts = []string{"aws", family, raw} }
    if len(parts) != 3 || parts[1] != family { return AssetKey{}, invalid }
    switch parts[0] { case "aws", "aliyun", "gcp": default: return AssetKey{}, invalid }
    id, err := strconv.ParseUint(parts[2], 10, 63)
    if err != nil || id == 0 || strconv.FormatUint(id, 10) != parts[2] { return AssetKey{}, invalid }
    return AssetKey{Provider: parts[0], Family: family, ID: id}, nil
}

func (k AssetKey) String() string {
    return k.Provider + ":" + k.Family + ":" + strconv.FormatUint(k.ID, 10)
}
```

- [x] **1.4 Define location and rule types.** Location fields are Type/Name/
  Region/Zone; valid Type is global/region/zone. Global has neither region nor
  zone; regional has region only; zonal has both. Firewall Rule retains protocol,
  nullable ports and targets and adds action, nullable priority/enabled and
  typed source/target ranges/tags/service accounts. Use pointer fields for
  unsupported facts. The exact behavior is design sections 6/7.
- [x] **1.5 Add round-trip/invalid-location tests and run package with race.**
  Assert equal local IDs from different providers yield distinct asset keys.
- [x] **1.6 Verified checkpoint:** common names/families are frozen; no commit.

Task 1 verification (2026-10-28): Go 1.25 race tests passed; separate spec and
code-quality reviewers approved all six resource contract/test files.

## Task 2: Migrate typed cloud credentials safely

**Create:** `migrations/00024_cloud_key_auth_types.sql`,
`cmd/optimus/migrate_multicloud_test.go`.
**Modify:** `internal/models/credential_cloud_key.go`.

- [x] **2.1 Write dbtests beginning at Goose UpTo 23.** Insert AWS/Aliyun/GCP
  legacy AK records with known ciphertext, plus live/deleted account references.
  Upgrade twice; verify bytes and FK links survive, GCP becomes legacy_invalid,
  and invalid auth-shape inserts fail with PostgreSQL 23514.
- [x] **2.2 Run red:** `rtk proxy env GOTOOLCHAIN=go1.25.0 OPTIMUS_TEST_POSTGRES_DSN="$OPTIMUS_TEST_POSTGRES_DSN" TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./cmd/optimus -tags=dbtest -run TestCloudCredentialAuthMigration -race -count=1`.
  Expected: migration/columns are missing.
- [x] **2.3 Write the Up body below, using Goose StatementBegin/End:**

```sql
ALTER TABLE credentials_cloud_keys
  ADD COLUMN auth_type VARCHAR(32) NOT NULL DEFAULT 'access_key',
  ADD COLUMN service_account_json_enc BYTEA,
  ADD COLUMN service_account_email TEXT NOT NULL DEFAULT '',
  ADD COLUMN credential_project_id TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials_cloud_keys ALTER COLUMN access_key_id_enc DROP NOT NULL;
ALTER TABLE credentials_cloud_keys ALTER COLUMN secret_access_key_enc DROP NOT NULL;
UPDATE credentials_cloud_keys SET auth_type = 'legacy_invalid' WHERE provider = 'gcp';
ALTER TABLE credentials_cloud_keys ADD CONSTRAINT ck_cloud_key_auth_shape CHECK (
  (provider IN ('aws','aliyun') AND auth_type = 'access_key'
    AND access_key_id_enc IS NOT NULL AND secret_access_key_enc IS NOT NULL
    AND service_account_json_enc IS NULL)
  OR (provider = 'gcp' AND auth_type = 'service_account_json'
    AND access_key_id_enc IS NULL AND secret_access_key_enc IS NULL
    AND service_account_json_enc IS NOT NULL)
  OR (provider = 'gcp' AND auth_type = 'legacy_invalid'
    AND access_key_id_enc IS NOT NULL AND secret_access_key_enc IS NOT NULL
    AND service_account_json_enc IS NULL)
);
```

- [x] **2.4 Implement Down with an explicit preservation guard:**

```sql
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM credentials_cloud_keys WHERE service_account_json_enc IS NOT NULL) THEN
    RAISE EXCEPTION 'cannot downgrade while service account credentials exist';
  END IF;
END $$;
ALTER TABLE credentials_cloud_keys DROP CONSTRAINT ck_cloud_key_auth_shape;
ALTER TABLE credentials_cloud_keys ALTER COLUMN access_key_id_enc SET NOT NULL;
ALTER TABLE credentials_cloud_keys ALTER COLUMN secret_access_key_enc SET NOT NULL;
ALTER TABLE credentials_cloud_keys DROP COLUMN auth_type,
  DROP COLUMN service_account_json_enc, DROP COLUMN service_account_email,
  DROP COLUMN credential_project_id;
```

- [x] **2.5 Add matching GORM fields and verify upgrade/safe/refused downgrade.**
  Preserve existing cloud-key lifecycle tests and old SQL files verbatim.
- [x] **2.6 Verified checkpoint:** existing encrypted material is unchanged.

Task 2 verification (2026-10-28): migration red/green, all 128 credential-shape
cases and full cmd/optimus dbtest race suite passed; independent spec and quality
reviews approved. Existing GCP AK/SK service-test assumptions are intentionally
updated in Task 3; no migration constraint was weakened.

## Task 3: Validate, encrypt and consume GCP Service Account JSON

**Create:** `internal/modules/credentials/cloudkey/auth.go`, `auth_test.go`,
`service_account.go`, `service_account_test.go`.
**Modify:** cloudkey `dto.go`, `repo.go`, `service.go` and their tests;
`internal/modules/credentials/consume.go`, `wipe_test.go`.

- [x] **3.1 Add generated-key fixtures and auth-matrix tests.** The reusable
  test helper generates RSA in memory, returns JSON bytes, and never writes PEM:

```go
func serviceAccountFixture(t *testing.T) []byte {
    t.Helper()
    key, err := rsa.GenerateKey(rand.Reader, 2048)
    require.NoError(t, err)
    der, err := x509.MarshalPKCS8PrivateKey(key)
    require.NoError(t, err)
    data, err := json.Marshal(map[string]string{
        "type": "service_account", "project_id": "test-project",
        "client_email": "collector@test-project.iam.gserviceaccount.com",
        "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
        "token_uri": "https://oauth2.googleapis.com/token",
    })
    require.NoError(t, err)
    return data
}
```

Import crypto/rand, crypto/rsa, crypto/x509, encoding/json, encoding/pem, testing
and require in the test file. Test `ValidateServiceAccountJSON([]byte)` returning
canonical JSON and metadata `{Email, ProjectID}`. Reject duplicate keys, >64KiB,
trailing documents, malformed RSA, external_account, authorized_user, hostile
token_uri and executable credential sources.

- [x] **3.2 Run red:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/credentials/... -run 'AuthType|ServiceAccount|Wipe' -race -count=1`.
- [x] **3.3 Implement the typed validator:** token-stream duplicate checking,
  single-document decoding, allowed fields and exact fixed endpoint checks from
  design 3.2, then canonical JSON. Validation errors use common.validation and
  never contain rejected source bytes. Accept and discard standard downloaded
  auth_uri/auth_provider_x509_cert_url/client_x509_cert_url metadata without
  following URLs. Include a standard full downloaded-key shape in tests.
- [x] **3.4 Implement conditional request/secret handling.** Omitted auth_type
  derives from provider. GCP create accepts only SA JSON; metadata update omits
  secret replacement; valid JSON repairs legacy_invalid and clears AK columns.
  Mixed shapes and new legacy_invalid inputs fail. Store canonical JSON through
  the existing cipher; return only auth metadata in GET/list.
- [x] **3.5 Lock conversion and binding together.** Reuse LockCloudKey,
  credential FOR UPDATE and live-account counter inside a transaction. Disabled
  live accounts block provider/type conversion. Same-type rotation remains
  available. Secret-shape mutations and shared audit must commit atomically.
- [x] **3.6 Extend Consumer with AuthType/ServiceAccountJSON and Wipe.**

```go
for i := range key.ServiceAccountJSON { key.ServiceAccountJSON[i] = 0 }
key.ServiceAccountJSON = nil
key.AuthType = ""
```

Retain current AK/SK best-effort clearing. Test the original slice reference is
overwritten and no secret reaches errors/audit/summary. Add dbtests for rotation,
repair, audit rollback, referenced conversion, concurrent binding and nil counter.
- [x] **3.7 Verify credential race/dbtests; checkpoint before SDK consumption.**

Task 3 verification (2026-10-28): validator/service TDD, complete credentials
PostgreSQL race suite, HTTP non-disclosure, Go lint, regenerated Swagger and
consistency checks passed. Separate spec and quality reviewers approved.

## Task 4: Make Cloud Keys forms provider-specific

**Modify:** `web/src/views/credentials/cloud-keys/components/CloudKeyEditModal.vue`
and its tests, `web/src/views/credentials/cloud-keys/List.vue`,
`web/src/api/credentials/cloud-key.ts`, `web/src/types/api.ts`, both locales.

- [x] **4.1 Add failing modal tests** for AK/SK versus JSON input, provider
  switch clearing, 64KiB file limit, metadata-only edits, legacy GCP repair,
  close/unmount clearing and no localStorage/usePageState secret retention.
- [x] **4.2 Run red from web/:** `rtk proxy bun x vitest run src/views/credentials/cloud-keys/components/CloudKeyEditModal.test.ts`.
- [x] **4.3 Type create requests as metadata intersected with this union:**

```ts
type CloudKeySecret =
  | { provider: 'aws' | 'aliyun'; auth_type: 'access_key'; access_key_id: string; secret_access_key: string }
  | { provider: 'gcp'; auth_type: 'service_account_json'; service_account_json: string }
```

Updates allow omitted secrets; legacy_invalid is summary-only, not a create
variant. Preserve name/description/region metadata and the existing API paths.
- [x] **4.4 Implement upload/paste JSON and conditional key inputs.** Backend
  validation remains authoritative. Uploaded bytes exist only in component
  memory; GET never preloads secrets. Provider switches reset auth-specific
  validation and secret drafts, without carrying an old provider payload.
- [x] **4.5 Run focused tests, typecheck and locale parity.** Existing provider
  selection and metadata edits must pass with updated typed fixtures.
- [x] **4.6 Verified checkpoint:** UI can store GCP JSON; no extra auth methods.

Task 4 verification (2026-10-28): spec and quality reviews approved. The
review-discovered parent-prop scheduling bug was fixed with three permanent
optimized-parent red/green regressions; all 32 focused tests, typecheck, lint
and locale parity passed. The full post-fix frontend suite passed 194 tests.

## Task 5: Model connection targets, regions and identity pinning

**Create:** `migrations/00025_cloud_account_multicloud_scope.sql`,
`internal/modules/assets/account/scope.go`, `scope_test.go`,
`region_options.go`, `region_options_test.go`.
**Modify:** account DTO/service/repo/handler/tests, `internal/models/assets.go`,
`cmd/optimus/migrate_multicloud_test.go`.

- [x] **5.1 Add failing tests:** GCP target project differs from credential
  project; wrong-provider/legacy credentials fail binding; region edits advance
  revision; name/description edits do not; target fields are immutable on edit.
- [x] **5.2 Add target fields and constraints:**

```sql
ALTER TABLE cloud_accounts
  ADD COLUMN project_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_account_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN scope_revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE cloud_accounts ADD CONSTRAINT ck_cloud_account_target CHECK (
  (provider = 'gcp' AND project_id <> '')
  OR (provider IN ('aws','aliyun') AND project_id = '')
);
ALTER TABLE cloud_accounts ADD CONSTRAINT ck_cloud_account_revision CHECK (scope_revision > 0);
```

Accepted AWS data passes. A manually inserted invalid GCP connection fails
migration rather than receiving an invented target. Down drops these constraints
and columns, and is not an operator production action in this plan.
- [x] **5.3 Replace existence-only credential checking** with metadata-only
  provider/auth validation inside the existing credential advisory lock. Do not
  decrypt keys in account CRUD. Validate explicit nonempty region lists by
  provider, with no AWS regex applied to Aliyun/GCP.
- [x] **5.4 Add region-options endpoint:** GET
  `/assets/cloud-accounts/region-options`, parameters provider/cloudkey_id/
  project_id, response `{items:[{id,name}]}`, gate assets:account:write. Inject
  catalog adapters, consume with actor/purpose and discard fresh clients. No
  implicit scan or arbitrary endpoint URL. Preserve existing valid AWS region
  IDs even when absent from the old UI's short region whitelist.
- [x] **5.5 Add account identity pinning.** Under FOR UPDATE compare captured
  scope revision/provider/target/enabled state. Set empty external_account_id
  from verified AWS/Aliyun STS identity, otherwise require an exact match. Return
  IdentityMismatch before writes/pruning when rotated keys target another cloud
  account. GCP always uses the explicit target project instead of identity home.
- [x] **5.6 Run account and credential-binding concurrency dbtests with race.**
  Retain disabled-account reference blocking, historical detach and atomic audit.
- [x] **5.7 Verified checkpoint:** CRUD/region catalogs are provider-aware.

Task 5 verification (2026-10-28): account/cloud-key/migration database race
suites, affected HTTP/module tests, Go lint and regenerated Swagger consistency
passed. Separate spec and quality reviews approved. Real SDK catalog adapters
and STS calls remain explicitly owned by Tasks 8/11/13.

## Task 6: Add independent native storage and snapshot transactions

Execute this large task as two reviewed implementation substeps: 6a covers
6.1–6.3 (schemas, models and shared structured JSON contracts); 6b covers
6.4–6.7 (transactional persistence and cleanup). Keep one implementer active.

**Create:** `migrations/00026_aliyun_assets.sql`, `00027_gcp_assets.sql`,
`internal/models/assets_aliyun.go`, `assets_gcp.go`,
`internal/modules/assets/providers/aliyun/persist.go`, `persist_test.go`,
`internal/modules/assets/providers/gcp/persist.go`, `persist_test.go`.
**Modify:** account cascade and `cmd/optimus/migrate_multicloud_test.go`.

- [x] **6.1 Write schema/upgrade tests** for the ten exact tables from design 5,
  native uniqueness, restrictive FKs, JSON object/array constraints and unchanged
  AWS rows/IDs/ciphertext. Repeated Up is a no-op.
- [x] **6.2 Add typed native schemas.** Design 5 defines complete required
  family fields and keys. This is the database-family DDL for GCP:

```sql
CREATE TABLE gcp_databases (
  id BIGSERIAL PRIMARY KEY,
  cloud_account_id BIGINT NOT NULL REFERENCES cloud_accounts(id),
  database_id TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  product TEXT NOT NULL DEFAULT 'cloud_sql',
  engine TEXT NOT NULL DEFAULT '', engine_version TEXT NOT NULL DEFAULT '',
  instance_class TEXT NOT NULL DEFAULT '', native_status TEXT NOT NULL DEFAULT '',
  region VARCHAR(32) NOT NULL, zone VARCHAR(64) NOT NULL DEFAULT '',
  endpoints JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(endpoints) = 'array'),
  storage_gb BIGINT, ha_mode TEXT, publicly_accessible BOOLEAN,
  tags JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(tags) = 'object'),
  native_details JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(native_details) = 'object'),
  last_seen_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ,
  UNIQUE (cloud_account_id, region, database_id)
);
CREATE INDEX idx_gcp_databases_live ON gcp_databases(cloud_account_id, region, last_seen_at DESC, id DESC) WHERE deleted_at IS NULL;
```

Every new resource table has the lifecycle/account fields above and the exact
family columns in design 5; global network/firewall region is empty, with global
scope interpreted by their table. Add account/location/network/last_seen indexes
and GIN indexes for searched tags/address collections.
- [x] **6.3 Add typed GORM models.** GCPDatabase.TableName returns gcp_databases;
  preserve nullable facts, TEXT native IDs and full network references. Do not
  embed AWS native models. Native detail is an allowlisted object, not raw SDK
  metadata/user-data. Each provider retains its own structured collections.
  Add resource/network.go and resource/database.go for design 5's exact NIC/IP
  and endpoint JSON contracts, and native_ref on new network tables so queries
  can match canonical network_ref without joining numeric GCP IDs to self-links.
Substep 6a verification (2026-10-28): schema/model/JSON contracts passed both
reviews, complete cmd migration/resource race tests, all-ten model roundtrips
and tagged lint. NIC nil-address serialization was hardened with zero/partial
red-green regressions and reviewer recheck. Source head is 27.

- [x] **6.4 Test persistence before implementing it:** first sweep creates two;
  failed fetch changes none; next success prunes one; reappearance revives its ID;
  empty success prunes all; another account/location remains untouched; stale
  revision or disabled/deleted account refuses commit.
- [x] **6.5 Implement native batch upserts** inside the shared engine's
  transaction, stamping account/location/time from trusted scope. Set DeletedAt
  to NULL on conflict and prune only older rows in the exact unit. Aliyun
  VPC/vSwitch share a transaction; GCP global network and regional subnets are
  independent units. Batch size 500 keeps parameter counts bounded.
- [x] **6.6 Extend account cascade** through a static provider/table allowlist.
  Region removal touches regional/zonal rows only, while account deletion also
  removes GCP global snapshots. Verify all-provider cascade counts and rollback.
- [x] **6.7 Verified checkpoint:** native storage can preserve complete snapshots.

Substep 6b verification (2026-10-28): final account/Aliyun/GCP database race
suites and tagged lint passed, with spec recheck and quality approval. Tests
cover GCP zone/region mismatch rejection, region-shrink late-failure rollback,
Unscoped history retention, original-ID revival and global/regional isolation.
Shared account.LockScopeTx and typed publishers are ready for engine adapters.

## Task 7: Introduce the provider registry and scoped sync engine

**Create:** `internal/modules/assets/sync/contracts.go`, `registry.go`,
`scope.go`, `registry_test.go`, `scope_test.go`,
`migrations/00028_assets_sync_scope.sql`.
**Modify:** `internal/modules/assets/sync/engine.go`, `engine_test.go`,
`engine_unit_test.go`, `scheduler.go`, `scheduler_test.go`,
`internal/modules/assets/sync/runs/{dto,repo}.go`,
`internal/models/assets.go`.

- [x] **7.1 Add registry/planning behavior tests.** A duplicate provider is an
  error; unknown providers never fall back to AWS. Two regions produce eight
  AWS/Aliyun units. GCP plans global network/firewall once, regional subnet/DB
  once per selected region and instance units for every discovered zone.
  Planning failure creates a failed planning run with no asset writes.
- [x] **7.2 Run red:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/sync -run 'TestRegistry|TestProviderPlan|TestScopeRevision' -race -count=1`.
  Expected: registry/scope behavior is missing.
- [x] **7.3 Define the engine-facing contract in contracts.go:**

```go
package sync

import (
    "context"
    "time"
    "gorm.io/gorm"
    "github.com/ArkGravity/optimus/internal/models"
    "github.com/ArkGravity/optimus/internal/modules/assets/account"
    "github.com/ArkGravity/optimus/internal/modules/credentials"
)

type Scope = account.Scope
type Unit struct {
    Key, ResourceType, LocationType, Location, Region, Zone string
    Scope Scope
    Budget time.Duration
}
type Plan struct { Units []Unit; ExternalAccountID string }
type Stats struct { Seen, SoftDeleted int64 }
type Snapshot struct {
    Complete bool
    Publish func(context.Context, *gorm.DB) (Stats, error)
}
type Provider interface {
    Name() string
    Plan(context.Context, models.CloudAccount, *credentials.CloudKey) (Plan, error)
    Fetch(context.Context, Unit, *credentials.CloudKey) (Snapshot, error)
}
```

Adapters return typed native rows captured in Publish; SDK objects never cross
this interface. Registry construction returns an error for duplicate names.
Only an internal adapter can construct a Snapshot; handlers cannot submit one.
- [x] **7.4 Migrate run scope.** Add provider VARCHAR(16), location_type
  VARCHAR(16), location TEXT and scope_revision BIGINT; backfill provider from
  the linked account, location_type='region', location=region and revision=1.
  Then set NOT NULL and add provider/location/revision checks. Preserve region,
  all historical resource_type/status values and soft-deleted account history.
  Add a resource_type check allowing existing instance/network/database/
  security_group plus subnet/firewall/planning; the baseline has no such check.
- [x] **7.5 Implement the publication gate:**

```go
package sync

import (
    "context"
    "errors"
    "gorm.io/gorm"
    "github.com/ArkGravity/optimus/internal/modules/assets/account"
    "github.com/ArkGravity/optimus/internal/modules/assets/resource"
)

var ErrIncompleteSnapshot = errors.New("incomplete asset snapshot")

func PublishSnapshot(ctx context.Context, db *gorm.DB, unit Unit, snapshot Snapshot) (Stats, error) {
    if !snapshot.Complete || snapshot.Publish == nil {
        return Stats{}, ErrIncompleteSnapshot
    }
    location := resource.Location{Type: unit.LocationType, Name: unit.Location, Region: unit.Region, Zone: unit.Zone}
    var stats Stats
    err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if _, err := account.LockScopeTx(ctx, tx, unit.Scope, location); err != nil {
            return err
        }
        var err error
        stats, err = snapshot.Publish(ctx, tx)
        return err
    })
    if err != nil { return Stats{}, err }
    return stats, nil
}
```

Fetch outside a DB transaction. If Complete is false or Publish is nil, finish
the unit as failed without calling Publish. Otherwise start a transaction,
lock the account FOR UPDATE, verify Matches, then publish. A changed scope is a
failed unit, never an empty success. Create each sync-run before fetching and
finish it with the existing independent five-second cleanup context.
- [x] **7.6 Consume and wipe separately for planning and every unit.** Purpose
  includes trigger/account/provider/unit; preserve system prefix and actor.
  Resolve/pin AWS/Aliyun identity before any unit publishes; reject a different
  verified identity. Capture refreshed account.Scope from PinIdentityTx's
  returned row and stamp planned units with it, including the newly pinned
  identity, binding and revision. Reuse 6b's account.LockScopeTx rather than
  introducing a second eligibility implementation. Continue independent units after a unit error, joining
  errors at the end. Cancellation or account identity failure stops new calls.
- [x] **7.7 Add dbtest races:** region removal, disabling and account deletion
  while Fetch blocks cannot revive rows; one failed unit leaves its previous
  rows unchanged while another succeeds; cancellation still terminates runs.
  Re-run existing manual-lock, scheduler and shutdown tests.
- [x] **7.8 Verified checkpoint:** the shared lifecycle is provider-independent.

Task 7 verification (2026-10-28): independent spec recheck and quality review
approved. Final sync/runs/account/inuse/module/cmd database race suites, normal
and dbtest-tagged lint, generated Swagger and consistency checks passed. Nil
registry, fixed-safe-error and identity-stop regressions are green. Source
head is 28. Task 8 must switch production AWS to NewProviderEngine and remove
staged legacy orchestration, retaining module injection compatibility.

## Task 8: Preserve AWS behavior behind the adapter and pin compatible SDKs

**Create:** `internal/modules/assets/providers/aws/provider.go`, `identity.go`,
`provider_test.go`, `identity_test.go`.
**Modify:** `go.mod`, `go.sum`, `internal/modules/assets/awsclient/factory.go`,
`factory_test.go`, `internal/modules/assets/module/wire.go`, `wire_test.go`,
`internal/modules/assets/sync/{sweep,security_group_sweep}.go`.

- [x] **8.1 Pin these researched SDK candidates with Go 1.25.0:**

```sh
rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go get github.com/alibabacloud-go/ecs-20140526/v7@v7.11.7 github.com/alibabacloud-go/vpc-20160428/v7@v7.3.1 github.com/alibabacloud-go/rds-20140815/v17@v17.0.2 github.com/alibabacloud-go/sts-20150401/v2@v2.1.0 google.golang.org/api@v0.290.0
```

Google v0.309.0 requires Go 1.26 and is excluded. Make AWS STS v1.44.1 a
direct dependency when imported. Verify the complete resolved graph with Go
1.25, preserving `go 1.25`, `ignore ./web/node_modules`, Kubernetes and Helm
pins. Pin an incompatible transitive dependency rather than raising Go.
- [x] **8.2 Add AWS adapter tests** for the four units, all pagination,
  partial SG detail failure and empty complete snapshots. Identity fixtures
  return account 111111111111, then 222222222222: the second run must fail
  identity validation without changing prior assets.
- [x] **8.3 Wrap existing fetchers, do not duplicate AWS mapping.** Move only
  transaction ownership out of existing sweep helpers so Publish uses the
  transaction supplied by the shared engine. Preserve AWS network/subnet
  atomicity, natural keys, legacy fields and security_group resource_type.
- [x] **8.4 Construct explicit AWS credentials/region/STS clients.** No ambient
  credentials, AssumeRole or metadata fallback. STS verification is a separate
  planning call and is repeated for each newly consumed unit key against the
  pinned identity; resource calls retain their four-per-region client factories.
  Test same-type rotation between planning and a later unit: a different
  account must cause IdentityMismatch before any resource fetch/publication.
  Preserve module Input.ClientFactory/Fetcher test seams by adapting them.
- [x] **8.5 Run:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/awsclient ./internal/modules/assets/providers/aws ./internal/modules/assets/sync ./internal/modules/assets/module -race -count=1`.
  Expected: AWS pagination, cleanup, worker locking and all adapter tests pass.
- [x] **8.6 Verified checkpoint:** AWS behavior remains covered before new clouds.

Task 8 verification (2026-10-29): independent spec and quality reviews approved.
Go 1.25 unit/database race suites for awsclient/providers/aws/sync/module,
affected engine/manual-sync HTTP integration, normal/tagged focused lint and
diff checks passed. Real SDK pagination, partial SG detail, empty/restoration,
second-run identity and mid-run rotation regressions are covered. Quality review
found repeated identity-error sanitization losing context sentinels; eight
permanent real-CallerIdentity cases and the reviewer's original overlay passed
after the fix. The full post-fix AWS provider database race suite passed.
All 554 resolved modules declare Go <=1.25 and `go mod verify` passed. Both
ignore entries and Kubernetes/Helm pins remain intact; unused researched SDK
pins are retained until Tasks 11/13 import them. No commit was made.

## Task 9: Build common queries across native tables

Execute as two reviewed substeps: 9a freezes the shared query contracts and
PostgreSQL fixtures; 9b wires existing family DTOs/repositories/services and
compatibility regressions. Keep one implementer active. The original query
implementer was interrupted after its green core fixture; its unfinished
family-adapter assertions remain preserved for 9b.

**Create:** `internal/modules/assets/query/repo.go`, `instances.go`, `vpcs.go`,
`subnets.go`, `databases.go`, `firewalls.go`, `repo_test.go`.
**Modify:** `internal/modules/assets/instance/{dto,repo,service}.go`,
`internal/modules/assets/vpc/{dto,repo,service}.go`,
`internal/modules/assets/database/{dto,repo,service}.go`.

- [x] **9.1 Add PostgreSQL fixtures** with local row ID 42 in all three
  providers, duplicate private IP 10.0.0.8 in two accounts, multiple NICs,
  tied last_seen_at, a soft-deleted row and nullable GCP DB facts. Assert count
  and page across providers, stable ordering, exact secondary-IP matching,
  tag filtering, scoped subnet lookup and absence of fabricated values.
- [x] **9.2 Run red:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/query -tags=dbtest -run TestUnifiedResourceQueries -race -count=1`.
  Expected: unified queries do not exist.
- [x] **9.3 Build static provider projections.** Each selected family branch
  exposes common columns id/provider/account/project/location/native_id/
  native_status/last_seen_at/deleted_at and typed family columns; union them
  before applying common count/order/limit/offset. Keep provider-native columns
  in allowlisted detail projections. For a common relation named assets:

```sql
SELECT * FROM assets
WHERE ($1 = '' OR provider = $1)
  AND ($2::bigint IS NULL OR cloud_account_id = $2)
  AND ($3 OR deleted_at IS NULL)
ORDER BY (deleted_at IS NOT NULL) ASC, last_seen_at DESC,
         provider ASC, id DESC
LIMIT $4 OFFSET $5;
```

Build SQL branches only from static provider/family maps, never identifiers
from request strings. Use one read-only repeatable-read transaction for count
and page. Reject invalid provider/size/key before executing queries.
- [x] **9.4 Implement exact IP queries over normalized NIC address arrays.**
  Use JSONB containment or jsonb_array_elements with equality and parameterized
  input; never LIKE. AWS scalar IP fields project into the same address shape.
  Preserve IPs as strings and labels separately from GCP network tags.
- [x] **9.5 Implement provider-key details** by parsed provider/family/local ID.
  Constrain subnets by the parent's account plus full native network reference
  and applicable scope; Shared VPC host references are retained even if their
  network is outside the current managed project. Preserve old AWS aliases.
Substep 9a verification (2026-10-29): independent spec and quality reviews
approved the query contracts and PostgreSQL fixtures. The 11-subtest core
database race suite, normal/tagged focused lint and diff checks passed.
Regressions cover same-NIC network/IP predicates, rejection of zoned IPv6
before SQL, native nullable facts and complete AWS firewall aliases. Quality
review also required indexed typed native AWS rule facts to preserve target
owner/VPC/peering metadata; its permanent red/green regression and final core
suite passed. Representative EXPLAIN checks show bounded pages and existing
account/scope index access. Existing family wiring and the preserved adapter
runtime fixture remain pending in 9b; Task 9 is not complete yet.

- [x] **9.6 Re-run query and existing family repository dbtests.** Check query
  plans on representative fixtures use account/scope indexes and bounded
  pagination. No materialized copy/index table is introduced.
- [x] **9.7 Verified checkpoint:** combined queries are correct on PostgreSQL 17.

Substep 9b verification (2026-10-29): independent spec and quality reviews
approved family DTO/repository/service integration. Query and all affected
family PostgreSQL race suites, normal family race suites, normal/tagged focused
lint, Swagger regeneration/consistency and diff checks passed. Preserved adapter
fixtures observed red before unified reads; regressions cover native arrays,
nullable JSON, int64 storage, safe validation, provider keys, Shared VPC scope,
AWS aliases and mapped-IP normalization. The review's nullable ha_mode schema
annotation was fixed and rechecked in all generated copies. Task 9 is complete;
canonical HTTP details, Firewalls and scoped Consumer follow in Task 10.

## Task 10: Expose unified Firewalls and scoped asset lookup

**Create:** `internal/modules/assets/firewall/dto.go`, `repo.go`, `service.go`,
`handler.go`, `handler_test.go`; `internal/modules/assets/consume_multicloud_test.go`.
**Modify:** `internal/modules/assets/module/wire.go`, `wire_test.go`,
`internal/modules/assets/{instance,vpc,database}/handler.go`,
`internal/modules/assets/consume.go`, `consume_test.go`.

- [x] **10.1 Add HTTP tests:** 401 unauthenticated, 403 without
  assets:resource:read and 200 with permission for each canonical family and
  Firewalls detail. Parse gcp:database:42 separately from aws:database:42;
  reject malformed and wrong-family keys. Numeric legacy routes return AWS.
- [x] **10.2 Mount Firewalls and update family detail key parsing.** Use one
  wildcard per existing path, not separate conflicting :id and :asset_key
  routes. Legacy /security-groups remains an AWS-only adapter. All routes
  return fixed envelopes and use nested read-permission middleware groups.
- [x] **10.3 Extend the Consumer contract:**

```go
type InstanceScope struct {
    Provider string
    CloudAccountID *uint64
    NetworkRef string
}
var ErrAssetsInstanceAmbiguous = errors.New("multiple assets match instance lookup")
```

Add LookupInstanceByPrivateIPScoped(ctx context.Context, ip netip.Addr,
scope InstanceScope) (*Instance, error), retaining the existing netip type and
adding provider/key/location to Instance. LookupInstanceByPrivateIP calls it
with an empty scope. Preserve the two other Consumer methods and their existing
AWS semantics during this increment. Zero matches preserve
ErrAssetsInstanceNotFound; more than one yields ErrAssetsInstanceAmbiguous;
query all live private NIC IPv4/IPv6 addresses and apply optional account/network/provider
constraints. Reads never add audit rows.
- [x] **10.4 Add Consumer dbtests** for duplicate private IPs, scoped success,
  secondary NIC match, deleted matches and unchanged not-found identity.
- [x] **10.5 Run:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/... -race -count=1`.
  Expected: unit/HTTP contracts and legacy AWS adapters pass. Run the matching
  dbtest packages with the disposable DSN as well.
- [x] **10.6 Verified checkpoint:** unified APIs and downstream lookup are usable.

Task 10 verification (2026-10-29): independent spec and quality reviews approved
with no remaining findings. All Assets unit/database race suites, real JWT/RBAC
canonical-route integration, normal/tagged focused lint, generated Swagger
consistency and diff checks passed. Canonical details and Firewalls preserve
numeric AWS compatibility, typed native rule facts and shared permissions.
Consumer regressions cover private-only ambiguity, canonical requested IP,
matched-NIC network/subnet/public context, wrong-network/public-only exclusions,
deletion and audit-free reads. Existing AWS ID/VPC methods remain intact.

## Task 11: Implement Aliyun identity, instances and VPCs

**Create:** `internal/modules/assets/providers/aliyun/client.go`, `provider.go`,
`instances.go`, `network.go`, `persist.go`, `maperror.go`, `client_test.go`,
`provider_test.go`, `instances_test.go`, `network_test.go`, `persist_test.go`.

- [x] **11.1 Create httptest fixtures** for GetCallerIdentity, DescribeRegions,
  DescribeInstances, DescribeNetworkInterfaces, DescribeVpcs and
  DescribeVSwitches. Generate IDs/IPs in
  fixtures rather than importing private environment output. Test stopped
  instances, multiple NICs, no public IP, tags and two-page responses. Inject
  test transports into SDK clients; do not expose arbitrary endpoints in the
  production account API.
- [x] **11.2 Construct explicit AK/SK clients.** Use the validated access_key
  credential bytes and selected region for product clients; no default
  credential chain. Set connect/read/context deadlines and retry attempts <=3.
  Plan calls STS GetCallerIdentity and emits instance/network/database/
  security_group units for every selected region. Region-options uses the
  same bounded authenticated client seam and validates returned IDs.
  Verify each fresh unit key's STS identity against its pinned Scope as well;
  test a mid-sweep rotation into a different account with zero resource writes.
- [x] **11.3 Implement page completion checks.** Track seen pagination tokens
  and declared totals, reject repeated cursors or truncated totals. Only set
  Snapshot.Complete after every page and required detail request succeeds.
  Preserve all ECS statuses and interface addresses. Map VPC/vSwitch native
  IDs and relations without cross-region name joins. The pinned DescribeInstances
  payload lacks complete per-NIC references/public-IP associations, so enrich
  its returned instance attachments through read-only DescribeNetworkInterfaces.
  Cross-check attachment instance IDs, finish required pages and never import
  standalone, NAT or unattached interfaces as additional resources. Enrichment
  failure fails the unit before publication.
- [x] **11.4 Publish through native repositories.** Use natural keys
  (account,region,native_id), typed allowlisted native details and the existing
  account-locked transaction. Both VPC and vSwitch changes commit together.
- [x] **11.5 Add failure tests:** page two returns 403/429/timeout, malformed
  response, repeated cursor and vSwitch failure after successful VPC fetch;
  every case leaves the prior complete network snapshot intact.
- [x] **11.6 Run:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/providers/aliyun -race -count=1`.
  Expected: HTTP mapping/pagination tests pass; then run persist_test with
  dbtest and the disposable PostgreSQL DSN.
- [x] **11.7 Verified checkpoint:** Aliyun compute and network inventory works.

Task 11 verification (2026-10-29): independent spec and quality reviews approved
explicit pinned-SDK identity/catalog/compute/network clients and production
registry wiring. Aliyun/module/sync database race suites, affected account/sync/
manual HTTP integration, normal race tests, normal/tagged focused lint and diff
checks passed. All 554 resolved modules remain Go 1.25 compatible. Tests cover
fresh-unit rotation, cancellation, complete/failed/empty snapshots, strict page
completion and accurate NIC associations; a secondary-public-IP duplication
regression observed red then green. SDK source established ECS page size 100,
VPC/vSwitch 50 and STS's context-bound HttpClient seam. The review's retry clarity
fix explicitly sets Config.RetryOptions to one disabled-retry attempt; focused
tests and recheck passed. Task 12 database/security-group units still fail safely
without publication. Real Aliyun acceptance remains pending; its RAM checklist
must include ecs:DescribeNetworkInterfaces.

## Task 12: Implement Aliyun databases and Firewalls

**Create:** `internal/modules/assets/providers/aliyun/databases.go`,
`firewalls.go`, `databases_test.go`, `firewalls_test.go`, `maperror_test.go`.
**Modify:** `internal/modules/assets/providers/aliyun/{client,persist}.go`.

- [x] **12.1 Add HTTP fixtures** for DescribeDBInstances,
  DescribeDBInstanceAttribute, DescribeDBInstanceNetInfo, DescribeSecurityGroups
  and DescribeSecurityGroupAttribute. Include MySQL/PostgreSQL, internal and
  public endpoints, nullable capacity facts, IPv6, all ports, ICMP type/code
  zero and referenced security groups.
- [x] **12.2 Fetch list plus required detail pages.** Bound detail concurrency
  to four per unit, sharing its context/deadline and retry budget. RDS detail
  failure fails the database unit; one SG-rule failure fails its firewall unit.
  Do not publish list-only records with silently missing endpoints/rules.
- [x] **12.3 Map native facts** into AliyunDatabase/AliyunSecurityGroup and the
  resource rule contract. Retain RDS product/engine/version/class/status/
  endpoints/HA. Preserve allow/deny, directions, port/protocol semantics and
  source/target references; no guessed engine, CPU or memory values.
- [x] **12.4 Map provider errors** to design 9's neutral codes using official
  SDK error code/status fields; store only sanitized summaries. Test invalid
  AK, missing RAM permission, throttling, timeout and unknown response.
- [x] **12.5 Run the full Aliyun package with race and its persist dbtests.**
  Assert complete empty sweeps prune only their native unit; incomplete detail
  fetches never write or prune, and revived rows keep their IDs.
- [x] **12.6 Verified checkpoint:** all four Aliyun regional units are covered.

Task 12 verification (2026-10-29): independent spec and quality reviews approved
all four regional Aliyun units. Affected unit/database race suites, measured
four-worker cancellation fixtures, normal/tagged focused lint, generated
Swagger, locale parity and diff checks passed. RDS attributes/endpoints and SG
rule pages are required before publication; failure preserves prior snapshots
and independent units continue with fixed neutral run codes/messages. Classic
intranet/internet token chains and failure in either scope are covered, along
with typed managed port-list references and unknown nullable ports. Empty full
sweeps, family/region isolation, stable revival IDs and stale-scope guards are
verified. The reviewer independently repeated affected database checks after
sandbox escalation; no findings remain. Schema/persistence implementation and
SDK/Go/Kubernetes/Helm pins are unchanged. Real Aliyun acceptance remains pending.

## Task 13: Implement explicit GCP auth, scope discovery and compute/network

**Create:** `internal/modules/assets/providers/gcp/client.go`, `provider.go`,
`instances.go`, `network.go`, `persist.go`, `maperror.go`, `client_test.go`,
`provider_test.go`, `instances_test.go`, `network_test.go`, `persist_test.go`.

- [x] **13.1 Test the auth boundary** using a generated RSA key, fake token
  transport and Compute HTTP fixtures. Assert client_email signs the JWT,
  token audience is fixed, target project comes from Cloud Account, and the
  metadata server/default ADC chain is never consulted. Error messages must
  not contain the private key, JWT assertion or access token.
- [x] **13.2 Build explicit Google clients** from the canonical validated JSON
  using oauth2/google JWT credentials with cloud-platform scope, fixed
  https://oauth2.googleapis.com/token and a bounded transport. Pass the
  resulting HTTP client explicitly to compute.NewService/sqladmin.NewService.
  Official token-source refresh is in-memory and scoped to the unit client.
- [x] **13.3 Plan authoritative zones** under selected regions before emitting
  zonal instance units. Emit one global network and one global firewall unit,
  plus regional subnet/database units. A zone-catalog failure fails planning;
  no unobserved zone is inferred empty. Enforce one selected project per
  Cloud Account; the JSON's home project does not override it.
- [x] **13.4 Fetch scoped instances and network data.** Use Instances.List per
  zone, Networks.List once globally and Subnetworks.List per region. Complete
  every page. Keep uint64 IDs as decimal strings, all NIC/address collections,
  network tags separately from labels, full Shared VPC references and explicit
  region/zone/global locations. Map TERMINATED to common stopped.
- [x] **13.5 Publish global/regional/zonal tables independently.** Regional
  subnet pruning cannot delete global networks. GCP account region removal
  cleans its zonal instances and regional rows while preserving global rows.
- [x] **13.6 Test** two regions with multiple zones, disabled Compute API,
  missing permissions, empty successful zone, page failure and Shared VPC
  host project outside collection scope. Global fetch count must remain one.
- [x] **13.7 Run:** `rtk proxy env GOTOOLCHAIN=go1.25.0 TMPDIR="$PWD/tmp/work" GOCACHE="$PWD/tmp/go-cache" go test ./internal/modules/assets/providers/gcp -race -count=1`.
  Expected: explicit auth, location mapping and complete-scope behavior pass;
  then run native persistence dbtests on disposable PostgreSQL.
- [x] **13.8 Verified checkpoint:** GCP compute/network scope is authoritative.

Task 13 verification (2026-11-04): independent spec and quality reviews approved.
Earlier affected unit/database race suites, normal/tagged lint, Swagger and
checksum checks passed; final warning/subnet-fact and explicit SQL-client
regressions also passed after the interruption. The credentials-package local
validator forwards existing validation without adding a retrieval path. Token
deadline fixtures verify the official JWT source finishes consuming response
bodies before cancellation. Review found ambient universe settings could alter
bearer destinations and client-certificate discovery could execute a signer
before the SDK returned the supplied HTTP client. Permanent red/green fixtures
reproduced both; shared constructor options now pin official universe/endpoints
and fail closed for enabled/unknown certificate discovery, without environment
mutation or validation bypass. Final GCP/module/credentials race suites,
normal/tagged lint and independent security regressions passed; no findings
remain. Catalog/zone/instance/global-network/regional-subnet fixtures preserve
explicit target projects, Shared VPC references, typed NIC facts and scope-safe
publication. Task 14 collectors still fail safely without publishers. Real GCP
acceptance remains pending.

## Task 14: Implement GCP databases and Firewalls

**Create:** `internal/modules/assets/providers/gcp/databases.go`,
`firewalls.go`, `databases_test.go`, `firewalls_test.go`, `maperror_test.go`.
**Modify:** `internal/modules/assets/providers/gcp/{client,persist}.go`.

- [x] **14.1 Add Cloud SQL/Compute fixtures.** Include multiple SQL regions,
  private/public endpoints, disk size, regional HA, engine versions and missing
  optional facts; include enabled/disabled allow/deny rules, priorities 0 and
  65535, several protocols/port ranges, IPv6 and tag/service-account targets.
- [x] **14.2 Implement database collection.** Cloud SQL Instances.List is
  project-wide: finish its entire paginated response, then partition by the
  unit's selected region before publishing gcp_databases. Keep each regional
  unit authoritative even when this first version repeats a project list;
  optimize only after profiling. Never prune an unselected region or substitute
  another database product. Exclude explicit ON_PREMISES_INSTANCE registrations
  after validating list identity/project and pagination: the official SDK
  identifies them as external databases not managed by Cloud SQL. Preserve
  unspecified managed-instance values as null.
- [x] **14.3 Implement global Firewalls.List.** One native firewall row retains
  its parent ID/name/network/priority/disabled flag; map allowed/denied protocol
  entries to displayed rules. Empty port lists mean all ports, ICMP zero is a
  value, and no entry creates a fictitious AWS security group. Retain GCP
  source/target ranges/tags/service accounts and action.
- [x] **14.4 Test incomplete responses** including page token loops, SQL page
  failure, Firewalls failure, API-disabled and token-exchange failure. If a
  future aggregated response is used, unreachable scopes and error warnings
  fail publication; NO_RESULTS_ON_PAGE by itself permits an empty result.
- [x] **14.5 Run GCP unit/race and persistence dbtests.** Assert writes go to
  gcp_databases, globally scoped firewall rows survive region changes, and
  identity/account/revision guards apply to every publication.
- [x] **14.6 Verified checkpoint:** both new providers cover all approved families.

Task 14 verification (2026-11-04): independent spec and quality reviews approved
Cloud SQL regional snapshots and global VPC firewalls. Affected unit/database
race suites, five-family SDK-to-engine integration, targeted response-presence/
body-lifetime/cancellation/security fixtures, normal/tagged lint, generated
Swagger, checksum/tidy-diff and whitespace checks passed. Explicit priority
zero remains distinct from the documented omitted default; the firewall-only
observer restores bounded response bytes for official SDK decoding. Typed SQL
provenance excludes external registrations and egress-only connection endpoints.
The SDK debug-log leak observed red then green after an explicit private discard
logger was added through the supported shared constructor option. No findings
remain. Selected modules are Go 1.25 compatible and required pins/ignore entries
are intact; native persistence and migrations are unchanged. SQL project lists
repeat per selected region, and firewall pages over 16 MiB fail before
publication. Real GCP acceptance remains pending.

## Task 15: Integrate shared menus, filters, details and account forms

Execute as three sequential reviewed substeps: 15a freezes TypeScript/API
contracts, shared filters and rule formatting; 15b integrates shared details,
resource/sync pages and canonical detail routes; 15c completes account forms,
seed/menu identity upgrades and the guarded legacy browser redirect. Each
substep uses a fresh implementer, then spec and quality review. Keep one
implementer active and complete all 15.1–15.7 requirements before closing Task 15.
Split 15b further into reviewed 15b1 (typed details, route wrappers and VPC/
subnet surface) and fresh 15b2 (resource/sync list integration) to keep each
implementation context bounded.

Substep 15a verification (2026-11-04): independent spec and quality reviews
approved the exact TS/native wire contracts, canonical/legacy APIs, shared
filter/request-generation controls and rule helpers. Affected suites passed
59 tests, typecheck, full lint, locale parity and whitespace checks; reviewers
independently passed 22 focused tests and full typecheck. Tests cover permission
revocation/unmount cleanup, initial saved-state restoration, loading/error
ownership, large server-key identity, nullable facts and indexed port-list
metadata. No useTable/backend/dependency rewrite was introduced. Page/detail
integration and forms/menus remain pending in 15b/15c.

Substep 15b1 verification (2026-11-04): independent spec and quality reviews
approved typed shared details, canonical detail routes and parent-key subnet
reads. Affected suites passed 87 tests, final focused detail/routes/workspace
checks, typecheck, lint and locale parity. Exact MaxInt64 legacy/canonical keys
remain precise. Quality review used actual Ant Design event chains to expose
subnet search and duplicate size-change requests; permanent red/green fixtures
now verify draft/submitted search and one size-change request resetting page 1.
Factual booleans use localized Yes/No, while enforcement keeps Enabled/Disabled.
Reviewer rechecks passed all three fixes with no remaining findings. Resource/
sync list integration remains pending in fresh 15b2, then forms/menus in 15c.

Substep 15b2 verification (2026-11-04): resource/Firewalls/sync lists passed
independent spec and quality reviews. Affected suites passed 120 tests,
typecheck, full lint, locale parity and diff checks. Real Ant Design controls
verify provider/account changes, multiword typing and one-request pagination.
Non-AWS collections render when legacy scalar aliases are empty. The spec
display correction localizes empty/absent facts without losing zero/false;
focused rechecks passed. Quality's missing keyboard/click navigation assertions
were restored for colliding provider IDs and MaxInt64 keys, with no production
change. No findings remain. Account forms and seed/menu/redirect integration
remain pending in 15c.

**Create:** `web/src/views/assets/firewalls/List.vue`, `rules.ts`,
`__tests__/List.test.ts`, `__tests__/rules.test.ts`,
`web/src/views/assets/components/AssetFilters.vue`, `AssetDetail.vue`,
`AssetFilters.test.ts`, `AssetDetail.test.ts`,
`web/src/hooks/use-asset-filters.ts`, `use-asset-filters.test.ts`.
**Modify:** `web/src/api/assets/{resource,account,sync}.ts`, `web/src/types/assets.ts`,
`web/src/views/assets/{instances,vpcs,databases,sync-runs}/List.vue`,
`web/src/views/assets/vpcs/Detail.vue`,
`web/src/views/assets/cloud-accounts/{Form,List}.vue`,
`web/src/stores/assets.ts`, `web/src/router/static-routes.ts`,
`internal/seed/seed.go`, `seed_test.go`,
`web/src/test/fixtures/menu-contract.json`,
`web/src/locales/{en-US,zh-CN}.json`, and their existing tests.

- [x] **15.1 Add table/filter tests** for duplicate numeric IDs across
  providers, provider/account/region changes resetting page and stale request
  rejection. Account options load only with account:read; resource-only users
  retain the numeric account filter. Details show all NIC IPs and typed native
  fields; null is displayed as localized not-applicable.
- [x] **15.2 Implement common identity/types.** The API type contract includes:

```ts
export type Provider = 'aws' | 'aliyun' | 'gcp'
export type AssetFamily = 'instance' | 'vpc' | 'subnet' | 'database' | 'firewall'
export type AssetKey = `${Provider}:${AssetFamily}:${number}`
export type LocationType = 'global' | 'region' | 'zone'
```

Use the server-returned asset_key as row key and encode it in detail links.
Keep legacy response properties available in API types during compatibility.
Native IDs remain strings. Use one detail component with family/provider
sections rather than displaying raw JSON. Rules use the common contract from
  Task 1 and the existing tested AWS formatting behavior. Indexed native rule
  facts retain AWS/Aliyun target associations and Aliyun managed port-list
  references; display a port-list ID/name instead of interpreting its unknown
  nullable common ports as an all-ports rule.
- [x] **15.3 Update account forms.** Provider comes from selected usable
  credential; GCP exposes required target Project ID. Fetch authenticated
  provider-specific region options and require explicit regions. Keep provider,
  project and binding immutable on edit. Clear dependent fields and increment
  a request-generation token on selection changes/unmount so late responses
  cannot overwrite a new selection. GCP home-project metadata is informational.
- [x] **15.4 Upgrade seed menus idempotently.** Find the existing built-in
  assets.security_groups row, preserve ID, change key/path/component to
  assets.firewalls, /assets/firewalls, assets/firewalls/List. Apply menu order
  Cloud Accounts/VPCs/Firewalls/Instances/Databases/Sync Runs. If only the new
  row exists, update it without inserting another. Rename visible EC2/RDS
  labels while preserving their existing route/component identities. If both
  reserved menu codes are active, fail transactionally with an actionable
  conflict instead of deleting or merging operator content; retain all rows,
  child references and existing permission grants for review.
- [x] **15.5 Add a guarded browser redirect** from /assets/security-groups to
  /assets/firewalls; update route fixture/locales/menu tests together. Keep
  permission codes unchanged. Do not add security-group/firewall category
  switches inside the Firewalls page. Clear detail drafts on close/unmount;
  persist only approved filters/page/scroll under workspace-tabs policy.
- [x] **15.6 Run focused web tests** from web/: `rtk proxy bun x vitest run src/views/assets src/hooks/use-asset-filters.test.ts src/router/static-routes.test.ts src/router/dynamic-routes.test.ts src/stores/assets.test.ts src/api/assets/__tests__/api.test.ts`.
  Expected: unified display, route parity, filters and request-race tests pass.
- [x] **15.7 Verified checkpoint:** all providers use the approved six menus.

Substep 15c verification (2026-11-05): independent spec and quality reviews
approved account forms, six-menu seed upgrades and the guarded legacy browser
route. The initial affected integration passed 150 tests; the full frontend
passed 296 tests, typecheck, lint, locale parity and production build. Seed
database race tests passed, preserving menu IDs, child references and permission
grants, including transactional collision rollback. Actual router bootstrap
tests verify denied access without a Firewalls menu and allowed query/hash
preservation. Quality exposed stale submission and account-list ownership
races with real Ant Design controls. Five permanent red/green regressions now
cover validation/save feedback, rows, totals, loading, errors and unmount.
Affected typecheck/lint/diff checks passed after the fixes; both reviewers
independently passed all four account suites (26 tests) on recheck. Existing
metadata edits remain independent of credential reads and current cloud auth.
No findings remain. Configuration/docs (Task 16) and final acceptance gates
(Task 17) remain; real provider acceptance is pending.

## Task 16: Finish configuration, error localization and operating docs

**Modify:** `internal/infra/config/{config,config_test}.go`,
`internal/modules/assets/module/{wire,wire_test}.go`,
`internal/modules/assets/errs/codes.go`,
`internal/modules/assets/sync/runs/{dto,service,handler}.go`,
`configs/config.yaml`, `.env.example`, `docker-compose.yml`, `README.md`,
`AGENTS.md`, `docs/README.md`, `scripts/p4-smoke.md`,
`web/src/locales/{en-US,zh-CN}.json`, annotated credential/account/resource handlers.
**Create:** `scripts/p4-multicloud-smoke.md`.
**Generate:** `api/docs/{docs.go,swagger.json,swagger.yaml}`,
`docs/api/swagger.json`, `docs/permissions.md` using repository targets.

- [x] **16.1 Add timeout config tests** for defaults, invalid/zero/overflow
  values and existing AWS override. Add request_timeout=30s and
  sync_timeout=15m, forward both env variables in Compose and retain all
  existing cron/retention settings. Derive bounded manual/cron account budgets
  from planned global/regional/zonal units; never use regions*4 for GCP.
  Preserve an AWS override larger than the general timeout through STS planning,
  authenticated region catalogs and resource calls; outer shared contexts must
  not silently truncate it to the general timeout. Test these paths explicitly.
  The manual handler assigns only the account ceiling and returns immediately;
  the worker performs planning and derives the remaining unit budget, including
  elapsed planning time. Never discover zones in the HTTP handler.
- [x] **16.2 Define neutral error codes exactly:**

```text
43201 Unauthorized     43202 Forbidden      43203 Unreachable
43204 Throttled        43205 Config         43206 Partial
43207 APIDisabled      43208 Other          43209 IdentityMismatch
```

Keep AWS 43102..43107 intact. Translate each in both locales and sanitize
responses/runs/logs. Tests assert provider raw errors/SA JSON/JWT/AK/SK are not
present in client envelopes, persisted run messages or audit metadata.
- [x] **16.3 Update API annotations** for typed credential requests/summaries,
  target project/region options, provider/location filters, common keys and
  Firewalls routes. Reuse permission constants; new ones, if an actual
  uncovered operation is found, must originate in permissions/codes.go.
- [x] **16.4 Write the smoke checklist** with minimum read-only RAM permissions
  for the listed Aliyun methods, including DescribeNetworkInterfaces used for
  attached-instance NIC enrichment; GCP Compute Viewer + Cloud SQL Viewer on each
  target project, enabled Compute/Cloud SQL Admin APIs and explicit selected
  regions. Include AWS DescribeRegions and identity verification in the AWS
  checklist. Cover SA rotation, cross-project binding, global scopes, duplicate
  private IPs and no cloud writes. Record real-provider acceptance separately
  from local fixture checks. Never place real secrets in the checklist.
  Use the verified official permission references for
  [Compute Viewer](https://docs.cloud.google.com/iam/docs/roles-permissions/compute),
  [Cloud SQL Viewer](https://docs.cloud.google.com/iam/docs/roles-permissions/cloudsql)
  and [Aliyun ENI discovery](https://next.api.alibabacloud.com/document/Ecs/2014-05-26/DescribeNetworkInterfaces)
  when documenting minimum read-only roles/actions; do not require Editor/Owner.
  Document the fixed Google universe/endpoints and fail-closed handling of
  enabled/unknown GOOGLE_API_USE_CLIENT_CERTIFICATE settings; do not enable
  ambient certificate/command discovery or mutate process environment.
- [x] **16.5 Document schema 23 -> 28 migrate + seed**, preservation/rollback
  limits, menu upgrades, auth repairs, new timeouts and credential deletion.
  Keep the operator-accepted production baseline distinct from unshipped work.
  Update this plan's checkboxes with actual evidence, not planned pass claims.
- [x] **16.6 Generate:** `rtk proxy env GOTOOLCHAIN=go1.25.0 make swag`,
  `rtk proxy env GOTOOLCHAIN=go1.25.0 make dump-perms`; verify
  `rtk proxy env GOTOOLCHAIN=go1.25.0 make swagger-diff` and
  `rtk proxy env GOTOOLCHAIN=go1.25.0 make perm-check`.
  Expected: generated artifacts agree with handlers/constants.
- [x] **16.7 Verified checkpoint:** configuration and documentation match code.

Task 16 verification (2026-11-05): independent spec and quality reviews approved
with no findings. Affected normal/database race suites, normal/tagged focused
lint, locale parity, regenerated Swagger/permission consistency and diff checks
passed. Independent checks cover malformed/overflowing configuration, fixed
catalog errors, actual AWS SDK planning/catalog/resource contexts with a 60s
override versus 30s general timeout, configured account ceilings above 15m and
start-based budgets that consume planning elapsed time. Shared optional timeout
capabilities preserve package direction; manual discovery remains worker-owned.
Current operating docs distinguish accepted production 00022 from source 00028,
describe preservation upgrades and downgrade safeguards, and leave real-provider
acceptance pending. Final repository/HTTP/embedded gates remain in Task 17.

## Task 17: Verify migration, cross-cloud isolation and the embedded product

Execute the remaining work in bounded sequential substeps: 17a adds the HTTP/
upgrade acceptance fixtures and corrects legacy test data for migration 00028;
17b fixes the embedded smoke's discovered conflict between known Assets SPA
routes and the /assets build-output namespace; 17c completes the final gate/
compiled smoke run. Review implementation substeps independently, spec first
and quality second, then perform a fresh overall composition review. The 17b
fix must use a narrow infra/webui route-shape whitelist without an infra-to-assets
dependency, retaining missing bundle 404s, API/Swagger JSON 404s and existing
method/cache/gzip behavior. Keep unrelated green gate evidence instead of
repeating it after isolated fixes.

**Create:** `tests/integration/assets_multicloud_test.go`.
**Modify:** existing credential/account/sync/resource integration tests as needed.

- [x] **17.1 Add a full HTTP integration test** with three providers and
  injected fixture clients: create encrypted keys, bind accounts, trigger async
  sync, wait for terminal runs with a bounded poll, read each family, change
  regions, rerun sync, delete accounts, then delete keys. Check 401/403/200,
  the one manual audit event, no cron mutation audit, in-use 43001 protection
  and historical run/credential behavior. Tests never contact real clouds.
  Reuse module.Input.ProviderRegistry and RegionCatalogs for fixture injection;
  no new production transport API is needed. Extend the integration wiring
  helper as needed so an omitted custom Consumer uses the real credentials
  module Consumer, preserving existing explicit test overrides. Deferred
  provider publishers receive the engine transaction and need no DB capture
  before app setup.
- [x] **17.2 Verify Colima readiness:** `rtk proxy colima status`,
  `rtk proxy docker context show`, `rtk proxy docker info`,
  `rtk proxy kubectl --context colima cluster-info`,
  `rtk proxy docker compose version`. Use its reported socket for tools that
  ignore Docker contexts. Use a dedicated disposable PostgreSQL 17 instance;
  do not set the test DSN to Dev/Production or destroy shared namespaces/data.

Task 17 runtime evidence (2026-11-05): root independently verified running Colima,
Docker context/info, Colima Kubernetes and Compose 5.7.0. The dedicated
optimus-multicloud-tests PostgreSQL 17 instance remains loopback port 32768;
Task 16 database races also passed there. Readiness need not be repeated unless
connectivity fails; no application or production data is used.
- [x] **17.3 Run the repository gates from root:**

```sh
rtk proxy env GOTOOLCHAIN=go1.25.0 make lint
rtk proxy env GOTOOLCHAIN=go1.25.0 GOFLAGS=-count=1 make test
rtk proxy env GOTOOLCHAIN=go1.25.0 make test-int
rtk proxy make web-check
rtk proxy env GOTOOLCHAIN=go1.25.0 make swagger-diff
rtk proxy env GOTOOLCHAIN=go1.25.0 make perm-check
rtk proxy env GOTOOLCHAIN=go1.25.0 make build
rtk git diff --check
```

Expected: lint, all Go race/unit/dbtests, web lint/typecheck/locale parity/tests/
build, generated checks and embedded build pass. Respect Makefile cache
defaults. Run tests with -race -count=1; do not weaken CI to obtain a pass.
- [x] **17.4 Verify upgrade preservation** on schema 23 -> 28, seed twice and
  compare pre/post AWS IDs, ciphertext, accounts, permissions, menu IDs and
  run history. Separately verify fresh initialization. A valid SA key makes
  credential downgrade fail clearly instead of deleting encrypted data.

Substep 17a verification (2026-11-05): independent spec and quality reviews
approved the full three-provider HTTP lifecycle, real encrypted vault Consumer,
engine-gated native publishers and combined upgrade/fresh-init acceptance.
Focused and full database runs passed the new fixtures. Full Go unit race and
lint gates passed. The full database run exposed one legacy deletion-test
fixture missing migration 00028 metadata; the valid AWS location/scope fields
were added without changing assertions, and the entire cloud-key database race
suite passed on recheck (17.877s). Every other package passed in the full run.
Actual validated/encrypted SA JSON remains unchanged after refused downgrade.
No findings remain for 17a. Known Assets SPA fallback repair and remaining
web/generated/build/compiled smoke gates continue in 17b/17c.

Substep 17b verification (2026-11-05): independent spec and quality reviews
approved the narrow infra-only Assets SPA whitelist. Embedded/disk real Gin
tests observed valid pages failing before the repair, then passed canonical/
legacy keys, decoded encoded-colon URIs, parent pages, GET/HEAD and original
index cache/gzip/ETag behavior. Review exposed missing raw-path traversal escapes
and the single-slash parent regression; permanent red/green cases cover both.
Unknown/dotted/nested/bundle paths remain 404 and existing static-file serving
still takes precedence. Full affected race tests, normal/tagged lint and diff
checks passed; reviewers independently repeated the affected race suite with
no findings. Matching README/AGENTS wording is updated. Final compiled smoke and
remaining repository gates continue in 17c.

Compiled verification in 17c exposed one additional composition miss: the
Swagger wildcard returned a plain-text 404 before NoRoute could supply the
documented JSON envelope. Substep 17d (2026-11-05) passed independent spec and
quality reviews after real Gin red/green regressions. Its narrow composition
wrapper retains the pinned library's 13 resources, rejects unknown aliases
with JSON 40401 and prevents query text selecting another resource or poisoning
the handler prefix. Request changes remain local and are restored for outer
middleware. Affected command-package races, normal/tagged lint and independent
spec race checks passed with no findings. Final 17c rebuild/compiled smoke must
now verify this reviewed repair; other successful gate evidence remains valid.
- [x] **17.5 Inspect the final diff** for obsolete names, secret material,
  unpinned dependencies, changed Go/Kubernetes/Helm invariants, generated drift
  and unapproved permission expansion. Use only gcp_databases in schema/model/
  query/test/doc contracts. Run the built binary's help/version and embedded
  UI smoke using disposable data.
- [x] **17.6 Report evidence and remaining real-cloud acceptance.** If the
  operator has not supplied disposable credentials, leave real AWS/Aliyun/GCP
  smoke explicitly pending; local fixtures are not production acceptance.
  Update AGENTS/current checkpoint/mem0 with actual results. Leave changes on
  feat/multicloud-assets for review; do not commit/push/deploy without separate
  user authorization.

Task 17 final verification (2026-11-05): all planned local work is complete.
Full Go 1.25 lint and unit race/coverage gates passed. Database package coverage
passed across the full run and the corrected cloud-key fixture's entire-package
race recheck described in 17a; the initial full database invocation itself was
not a single successful exit. Subsequent webui/Swagger changes passed affected
normal/tagged lint and race suites, with independent reviews. Final web-check
passed lint, typecheck, locale parity and 302 tests in 54 files. Swagger and
permission consistency, whitespace checks, frozen-lockfile frontend build and
the embedded Go build passed; web/dist/.gitkeep remains tracked and present.

The rebuilt binary (3bd0add-dirty) passed the actual compiled smoke, session
75300: help/version, isolated migrate/seed/health, 176 valid Assets GET/HEAD
requests, 78 raw invalid/missing/traversal 404 checks, cache/security/gzip/ETag
behavior, JWT 401/login and seven authenticated list APIs. All 15 unknown API/
Swagger paths returned JSON 40401, and 39 checks covered the 13 pinned Swagger
resources with ordinary and filename-containing queries. Its server exited and
only its unique disposable database was dropped; absence was verified.

A fresh overall composition review independently inspected working-tree and
untracked implementation across credentials, production wiring, scope/query/
Consumer, UI/menu, embedded routing, migration and operating boundaries and
approved with no Critical, Important or Minor findings. HEAD remains
3bd0add47e4f333d7984e8164010fc04de3c3037 on the uncommitted feat/multicloud-assets
branch. No commit, push, deployment or application-data reset was performed.
Real AWS/Aliyun/GCP smoke remains pending separately supplied disposable
read-only credentials; production acceptance remains d0016d0/schema 00022.

## Coverage and handoff

| Approved requirement | Tasks |
|---|---|
| gcp_databases; provider-native tables | 2, 5, 6, 9, 14 |
| Cloud-independent Aliyun AK/SK and GCP SA JSON; no ambient auth | 2–5, 8, 11, 13 |
| Encrypted-only credentials, safe repair/rotation/deletion, atomic audit | 2–5, 17 |
| Explicit account/project identity and region/zone/global scope | 5, 7, 8, 11, 13, 14 |
| Complete-sweep-only pruning, restore, stale-scope/concurrency protection | 6–8, 11–14, 17 |
| Unified list/detail/filter/identity; scoped Consumer | 1, 9, 10, 15 |
| VPCs / Firewalls / Instances / Databases naming and menu order | 10, 15 |
| Shared RBAC/audit, legacy AWS routes and existing data | 2, 7–10, 15–17 |
| Go 1.25 pins, configs, errors/locales, generated artifacts | 8, 16, 17 |
| Upgrade/fresh DB, fixture/HTTP/race/UI tests; real-cloud smoke separated | 2–17 |

Execution choice follows the writing-plans skill: subagent-driven development
with per-task review, or inline execution using executing-plans. The design is
approved; this choice does not reopen architecture or authorize a commit.


## Approved fresh baseline19 execution checkpoint (2026-11-05)

The operator approved replacing the historical migration sequence with exactly
19 sequential fresh-baseline SQL files; head 00019, next new migration 00020.
This supersedes the earlier unchanged-migration/in-place-upgrade tasks above,
whose dated verification remains history. Keep 31 business tables, final DDL
semantics and all runtime/API/UI compatibility (including legacy_invalid).
Old 00022/00023/00028 databases require actual database recreation, including
goose_db_version, before migrate + seed; container recreation is insufficient.
No application/Dev/Production database mutation or commit/push/deploy is part
of this source task. Last production acceptance remains d0016d0/schema 00022.

- [x] Observe RED for exact 19-file layout and real PostgreSQL fresh head19.
- [x] Fold named indexes/FKs into owners; split apps tables; define final typed
  credentials, scoped accounts, AWS/Aliyun/GCP native tables and sync runs.
- [x] Retain final auth-shape, reference/deletion, native shape/model/index and
  scope/history checks; retire obsolete transition/backfill/downgrade promises.
- [x] Verify full populated reverse Down, fresh Up, repeated Up and seed twice.
- [x] Verify old final28 versus fresh19 pg_catalog schema equivalence.
- [x] Update current operating docs and this supersession checkpoint.
- [x] Complete affected Go/database/lint checks and independent spec/quality
  reviews; record actual results without changing historical acceptance.

Baseline19 verification: the exact embedded filename/head tests first failed
against the old 24-file/head28 baseline, then passed at head19. Full changed
command/migration/seed database race packages passed (13.689s/4.115s/21.986s),
including all 31 business tables populated before Down0, fresh reinitialization,
repeated Up and seed twice. Final auth-shape matrix includes legacy_invalid;
valid encrypted SA JSON survives repeated Up and is dropped only by the owned
credential-table Down after dependants. Provider/location/family/status history
checks retain exact persisted values. Normal and dbtest focused lint passed.
An independent PostgreSQL catalog comparison passed: old head28 and fresh19
match across 31 tables, 396 columns, 118 named constraints/FK definitions,
135 indexes and 29 sequences/ownership, with zero custom triggers. Physical
column order and Goose history are deliberately excluded. Both comparison
databases were dropped and absence verified. SQL remained unchanged after
the comparison snapshot. Independent specification and quality reviews both
approved with no findings after inspecting the final SQL, retained assertions,
reference snapshot and current operating documentation.

Final baseline19 verification (2026-11-05): the complete repository
`make test-int` run passed with Go 1.25, `-tags=dbtest -race -count=1`, including
all business and HTTP integration packages. The frozen-lockfile frontend and
embedded Go build passed. The rebuilt binary initialized a unique empty test
database at head19 with 32 total tables, repeated migrate and seed without
duplicates, and passed authentication/seven list APIs plus the existing 176
valid Assets GET/HEAD, 78 invalid-path, 15 JSON404 and 39 Swagger resource/query
smoke checks. Its server exited and its unique database was dropped; absence
was verified. Final SQL hashes still match the independently compared snapshot
and the whitespace check passed. No Dev/Production database reset, deployment,
commit or push was performed; the work remains on `feat/multicloud-assets`.

## Delivery authorization and cleanup (2026-11-05)

The operator confirmed basic local functional acceptance and requested committing
all updates, pushing the feature branch, opening a PR and merging into main for
Production rebuild and full validation. This supersedes the earlier no-commit
checkpoints. Production acceptance remains d0016d0/historical schema22 until the
operator confirms the new rollout. Use the fresh schema19 with an actually
recreated database followed by migrate + seed.

After all local verification passed, the dedicated optimus-multicloud-tests
PostgreSQL container and its anonymous data volume were removed. Both test
PostgreSQL images were removed. Shared Colima Kubernetes resources are retained.
Local temporary verification files and build artifacts are cleaned during
delivery; final PR, merge and publication evidence is recorded in the delivery
report and project memory.
