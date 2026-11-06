# Database transitions and recovery

Optimus credentials are encrypted with a vault master key. A usable recovery set includes the PostgreSQL backup, its matching vault key, application revision and configuration. Protect all of them, and test restoration on an isolated database before relying on the backup.

## Fresh baseline for v0.1.0

Current main uses exactly 19 sequential migrations ending at `00019_create_assets_sync_runs.sql`. Initialization creates 31 business tables plus `goose_db_version` (32 tables total), with head version 19. The next new migration is 00020; published migrations are immutable.

Historical databases at schema 00022, 00023 or 00028 **cannot upgrade in place** with these renumbered files. They must actually be recreated, including `goose_db_version`. Recreating an application/PostgreSQL container does not remove the existing named volume or its schema history. Do not attempt to repair this by changing Goose version rows.

For an explicitly planned transition:

1. Stop application writers, including background synchronization and other instances.
2. Back up required database contents and securely retain the matching vault key, application/configuration revision and deployment identity. Verify the backup on a disposable target.
3. Choose and authorize the target database recreation. This removes existing application data; decide how required data will be retained or restored before proceeding. No automatic business-data transfer from the historical baseline is provided.
4. Recreate the target database or remove all its objects, including Goose history, using the operator's database procedure.
5. Run the selected version's migrate and seed, check head19, then verify login and credential/resource behavior before reopening access.

Do not run destructive recreation commands on a persistent environment as a shortcut. This guide deliberately does not prescribe `docker compose down -v`: it erases persistent data. Container recreation alone is also insufficient for the baseline transition.

## Rollback and passwords

Goose Down operations drop tables and data after their dependants, including credentials. They are not rollout recovery. Recover using a tested backup with its matching application revision, configuration and vault key. Never restore an old database and run a mismatched binary's migrations against it without reviewing compatibility.

A PostgreSQL named volume retains its original user password after `.env` changes. Rotate a password through PostgreSQL administration and update every dependent DSN consistently. Do not delete volumes to rotate credentials.

Generate the vault key once for a database using `optimus vault-keygen`. Losing it makes encrypted credentials unrecoverable; replacing it casually does not re-encrypt existing values. To use `OPTIMUS_VAULT_MASTER_KEY_FILE`, first set `vault.master_key: ""` in the selected YAML and unset `OPTIMUS_VAULT_MASTER_KEY`, then point the file setting to a protected key file readable by the process (mounted into the container when applicable). Inline keys take precedence over files; an empty environment value alone falls back to the YAML's development key. Keep the file's matching key across restart, backup and restoration.

Seed prints a newly generated administrator password once; repeated seed does not reset an existing password. Preserve it securely. If lost, an authorized database administrator must reset the password through the database.

## Multi-cloud compatibility and operations

Seed preserves the former built-in Security Groups menu row ID, permission grants and custom children while renaming it to Firewalls. Order is Cloud Accounts, VPCs, Firewalls, Instances, Databases, Sync Runs. The legacy `/assets/security-groups` browser path redirects under the same permission, and legacy AWS API aliases remain supported.

Legacy-invalid GCP credential records remain readable as `auth_type=legacy_invalid` / `needs_configuration`; replace their secret with valid Service Account JSON before collection. GCP uses an explicit target Project ID, possibly different from the credential's home project. Cloud Account provider, credential binding and target project are immutable; create a new account to change them. Rotate secret material on the existing credential to retain a binding.

Credential deletion is blocked with `43001` while any live Cloud Account references it, including disabled accounts. Delete the account first. Only historical deleted-account references are detached during physical credential deletion and its atomic audit event. Kubeconfig deletion likewise refuses live cluster references.

| Setting | Environment variable | Default |
| --- | --- | --- |
| General request timeout (Aliyun/GCP) | `OPTIMUS_ASSETS_REQUEST_TIMEOUT` | `30s` |
| Independent AWS request timeout | `OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT` | `30s` |
| Account sync ceiling | `OPTIMUS_ASSETS_SYNC_TIMEOUT` | `15m` |

These correspond to `assets.request_timeout`, `assets.aws_request_timeout` and `assets.sync_timeout`. Use positive Go durations. Malformed, overflowing, zero or negative values fail configuration/startup validation. AWS's independent override covers identity planning, authenticated catalogs and resource calls. Every call also respects parent cancellation and remaining deadlines. Individual units remain at most 15m; the configured account ceiling may exceed 15m. Actual global/regional/zonal plans determine aggregate budgets, with overflow protection and planning elapsed time deducted.

Manual sync returns immediately; its worker owns planning and collection. `OPTIMUS_ASSETS_SYNC_CRON` is a standard five-field cron evaluated in UTC. `OPTIMUS_ASSETS_SYNC_STARTUP_DELAY` and `OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS` govern scheduling/history. The source YAML retains 90 days; Compose's default is 30 days, so configure retention explicitly when consistency is required.

Google clients use explicit selected credentials and fixed official universe/endpoints. `GOOGLE_API_USE_CLIENT_CERTIFICATE` must be unset or case-insensitive `false`; enabled/unknown discovery fails Config before SDK construction. Never log SA JSON, JWTs, AK/SK or raw provider payloads.

Provider errors are sanitized: `43201` Unauthorized, `43202` Forbidden, `43203` Unreachable, `43204` Throttled, `43205` Config, `43206` Partial, `43207` APIDisabled, `43208` Other and `43209` IdentityMismatch. AWS `43102`–`43107` remain compatible. A provider Unauthorized error is distinct from an application JWT 401. Incomplete collection units preserve the preceding snapshot; only authoritative successful sweeps can remove missing resources.

## Linux Docker network troubleshooting

A remote Linux incident showed `apk add` hanging on Alpine indexes while host/host-network downloads worked. Bridge MTU 1460 resolved that host's issue. After verifying the appropriate MTU for your network, merge Docker daemon `mtu: 1460` and `default-network-opts.bridge.com.docker.network.driver.mtu: "1460"` into existing `/etc/docker/daemon.json`, then restart Docker during a planned window. Existing networks need recreation to adopt defaults; preserve project identity and volumes. Do not use volume deletion as a network repair.

PostgreSQL volumes initialized by version 16 cannot simply start under 17. Use a supported PostgreSQL migration procedure for retained data; only disposable development volumes may be recreated without preserving their contents.

## Acceptance history

The operator confirmed `d0016d0` / historical schema 00022 production recreation, deployment and P0–P4 acceptance on 2026-10-28; its inventory was AWS-only. That historical report is distinct from the operator's 2026-11-05 real AWS/Aliyun/GCP acceptance of `main-1bbb8fe`. The current feature is on main. The v0.1.0 native amd64/arm64 artifacts and versioned images passed [release verification](https://github.com/ArkGravity/optimus/actions/runs/42292847315/attempts/2); the [GitHub Release was published](https://github.com/ArkGravity/optimus/releases/tag/v0.1.0) on 2026-11-06 (UTC) after final operator approval. Publication did not deploy or recreate Production databases.

Use [P3](../../scripts/p3-smoke.md), [AWS](../../scripts/p4-smoke.md) and [multi-cloud](../../scripts/p4-multicloud-smoke.md) smoke checklists with disposable resources and separately supplied read-only cloud credentials. Fixture evidence and operator acceptance should remain separately attributable.
