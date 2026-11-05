# Multi-cloud Assets manual smoke checklist

Run only on Dev with separately supplied disposable read-only AWS, Aliyun and
GCP credentials. The application performs inventory reads and no cloud writes.
Do not import private environment identifiers or real secret material into tests.
Fixture coverage is separate from real-provider acceptance; real AWS Security
Groups, Aliyun and GCP acceptance is pending until this checklist is executed.
Production remains accepted at `d0016d0` / schema 00022.

## Fresh database baseline and application permissions

1. Use a fresh disposable Dev database for the approved 19-file baseline.
   Existing 00022/00023/00028 databases have no in-place path with these files.
   Back up any required data and retain its vault key, stop application writers,
   and actually recreate the target database (including `goose_db_version`),
   then run `make migrate-up` through 00019 and `make seed`. Container recreation
   alone retains the database. Expect 31 business tables plus the Goose table;
   the next new migration is 00020. This checklist does not record a completed
   Dev/Production rebuild; last accepted production remains d0016d0/schema22.
2. Rerun seed. Expect Cloud Accounts, VPCs, Firewalls, Instances, Databases,
   Sync Runs, one Firewalls menu retaining the former built-in Security Groups
   row ID and existing permission grants and custom child references, and no duplicate old/new menus. Verify the old
   browser URL redirects under `assets:resource:read`.
3. Reuse `credentials:cloud_key:*`, `assets:account:read/write/delete`,
   `assets:resource:read` and `assets:sync:read`. Verify unauthenticated requests
   return 401, missing application permissions return 403 and authorized reads
   return 200. Provider filters do not provide row authorization.
4. If a legacy-invalid GCP fixture is supplied separately, verify it remains visible with
   `auth_type=legacy_invalid` / `needs_configuration`, without revealing old
   secrets. Repair it by supplying valid Service Account JSON in Cloud Keys.
   Metadata-only edits preserve secret material without a credential reveal.
5. Treat Down migrations as destructive maintenance, not rollout recovery:
   the baseline drops inventory and credential tables after dependants, with no
   intermediate SA-auth downgrade guard. Test recovery with a backup and its
   matching binary and vault key. Retain legacy_invalid compatibility in the
   application; fresh migration performs no old-GCP or sync-run backfill.

## Cloud prerequisites

For AWS, use the policy in [p4-smoke.md](p4-smoke.md), including
`ec2:DescribeRegions`, EC2/VPC/subnet/Security Group reads and
`rds:DescribeDBInstances`. Planning and every fresh unit verify the selected
identity. Valid AK/SK is required; [AWS GetCallerIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html)
requires no IAM permission and does not need a mandatory added policy action.

For Aliyun, use an AK/SK belonging to a disposable RAM identity with the following
read actions for the actual collection methods (no write or manage policy):

```json
{
  "Version": "1",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "ecs:DescribeRegions",
      "ecs:DescribeInstances",
      "ecs:DescribeNetworkInterfaces",
      "ecs:DescribeSecurityGroups",
      "ecs:DescribeSecurityGroupAttribute",
      "vpc:DescribeVpcs",
      "vpc:DescribeVSwitches",
      "rds:DescribeDBInstances",
      "rds:DescribeDBInstanceAttribute",
      "rds:DescribeDBInstanceNetInfo"
    ],
    "Resource": "*"
  }]
}
```

The attached NIC enrichment specifically needs
[ecs:DescribeNetworkInterfaces](https://next.api.alibabacloud.com/document/Ecs/2014-05-26/DescribeNetworkInterfaces).
The database detail path uses
[rds:DescribeDBInstanceNetInfo](https://www.alibabacloud.com/help/en/rds/developer-reference/api-rds-2014-08-15-describedbinstancenetinfo),
whose action has no dependent action. Authenticated STS identity verification is
also a prerequisite; use valid AK/SK and verify it succeeds, without inventing a
required RAM action for
[GetCallerIdentity](https://help.aliyun.com/zh/ram/developer-reference/api-sts-2015-04-01-getcalleridentity).

For GCP, supply an explicitly selected Service Account JSON key. Grant
[Compute Viewer (`roles/compute.viewer`)](https://docs.cloud.google.com/iam/docs/roles-permissions/compute)
and [Cloud SQL Viewer (`roles/cloudsql.viewer`)](https://docs.cloud.google.com/iam/docs/roles-permissions/cloudsql)
to that service account on the **target project**. Enable Compute Engine API and
Cloud SQL Admin API there before the smoke, and select explicit enabled regions.
The target Project ID may differ from the credential JSON's home `project_id`;
permissions on the home project alone do not authorize target-project discovery.
Do not grant Editor or Owner. API enablement and key provisioning are operator
prerequisites, not operations performed by Optimus.

Google uses fixed official universe/endpoints. Ambient client-certificate discovery
is unsupported: `GOOGLE_API_USE_CLIENT_CERTIFICATE` is accepted only when unset or
case-insensitive `false`; `true` or unknown values fail Config before SDK discovery.
The application neither changes process-wide environment variables nor enables
raw SDK debug-payload logging.

## Provider inventory and scope

1. Create one typed credential per provider: AWS/Aliyun `access_key` with AK/SK,
   GCP `service_account_json`. Inspect list/get/create/update envelopes: only
   metadata summaries return; no SA JSON, private key, access key, secret or token
   is returned. Keep all screenshots and reports free of secrets.
2. Create Cloud Accounts with authenticated region options. For GCP explicitly
   enter a target project distinct from the home project when testing cross-project
   access. Provider, cloud-key binding and target project are immutable; use a new
   account for a different binding. Region selection never scans all regions.
3. Click Sync now. The HTTP response queues immediately; STS/zone planning belongs
   to the worker. Check a successful planning run, followed by AWS/Aliyun's four
   regional units (`instance`, `network`, `database`, `security_group`). For GCP,
   check global `network`/`firewall`, regional `subnet`/`database` and one `instance`
   unit per authoritatively discovered zone under the selected regions. An empty
   zone list must still allow the global and regional units.
4. Compare Instances, VPCs, Firewalls and Databases to cloud consoles for the chosen
   scope. Exercise provider/account/project/region/zone and native family filters,
   pagination and canonical `provider:family:rowID` links. Include stopped instances,
   all NIC IPs (including private-only instances), native tags, nullable facts,
   64-bit capacities, full network references and scoped subnet lists.
5. Compare AWS/Aliyun inbound/outbound rules and GCP allow/deny, priority, enabled
   state, source/target ranges, tags and service accounts. Check classic Aliyun
   Security Groups and managed port-list references if present. GCP VPC/firewall
   resources are global and must not appear copied once per region.
6. Use independently provisioned inventories with duplicate private IPs in different
   accounts/projects/networks. Check scoped downstream lookups preserve ambiguity
   when scope is insufficient and identify the matching NIC when scope resolves it.
   These checks require an approved consumer test harness, not a new public API.
7. Rotate a disposable GCP SA JSON key on the existing cloud-key record and sync
   again. Confirm fresh clients use the new key with the account's existing target
   project. Keep old key disposal under the owner's normal key-management process.
   For AWS/Aliyun verify cross-account credential rotation fails IdentityMismatch
   before resource publication; restoring the intended identity permits a later sync.
8. Remove one selected region. Confirm regional/zonal snapshots there soft-delete,
   scope_revision increments and stale in-flight work cannot revive them. GCP global
   networks/firewalls remain. Delete a disposable Cloud Account and verify all native
   resource families cascade; sync history remains available.
9. Attempt cloud-key deletion while an enabled or disabled live account references
   it: expect `43001`. Delete the account, then retry; deletion detaches historical
   references and records the shared audit event atomically.

## Failures, deadlines and evidence

Use only independently prepared read-only failure credentials/projects. Confirm
OAuth acquisition errors remain distinct from target API Forbidden/APIDisabled.
Provider Unauthorized returns a domain envelope, without initiating application
JWT refresh. Numeric categories are `43201` Unauthorized, `43202` Forbidden,
`43203` Unreachable, `43204` Throttled, `43205` Config, `43206` Partial,
`43207` APIDisabled, `43208` Other and `43209` IdentityMismatch; AWS
`43102`–`43107` stay compatible. Partial pages, required detail failures and zone
catalog failures must preserve previous snapshots and must not delete missing rows.

Use `OPTIMUS_ASSETS_REQUEST_TIMEOUT=30s` for Aliyun/GCP and independently
`OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT=60s` to exercise AWS compatibility through
STS planning, region options and resource calls. `OPTIMUS_ASSETS_SYNC_TIMEOUT`
defaults to `15m`; a valid `25m` ceiling must remain 25m. Each unit is at most 15m,
actual planned global/regional/zonal units determine the summed budget and planning
elapsed time consumes the account deadline. Shared cron/startup/retention remain
configured with their existing variables. Cancellation/shutdown releases account
locks and finalizes running records without exposing provider payloads.

Record date, application revision/schema, provider, nonsecret scope alias,
application permission role, envelope code/message_key, run IDs/status/counts and
sanitized audit/log evidence. Never attach SA JSON, JWTs, AK/SK, private identifiers
or raw SDK request/response/error bodies. Record each real provider as pending,
passed or failed separately; passing fixtures is never real-provider acceptance.
