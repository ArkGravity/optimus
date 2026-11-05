# P4 assets — manual smoke checklist

> Run on a Docker-enabled workstation with a disposable, read-only AWS
> credential. This is a manual release sign-off; it is not an automated test
> because it talks to a real AWS account.

Use a fresh disposable Dev database for the current 19-migration baseline
(head 00019, next 00020), then run `make migrate-up` and `make seed`. An old
00022/00023/00028 database must actually be recreated, including
`goose_db_version`, first; containers alone retain its data and version history.
For Aliyun/GCP and authentication checks, also use [the multi-cloud checklist](p4-multicloud-smoke.md).
Real-provider acceptance is pending; fixture results alone are not release sign-off.

## IAM policy for the smoke credential

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeRegions",
        "ec2:DescribeInstances",
        "ec2:DescribeVpcs",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "rds:DescribeDBInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

Authenticated `DescribeRegions` needs `ec2:DescribeRegions`. Planning and every
fresh resource unit also verify `GetCallerIdentity` with the selected valid AK/SK;
AWS documents that this STS operation requires no IAM permissions, so no extra
policy action is mandatory. See [AWS GetCallerIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html).

## Steps

1. Sign in as an admin and create an AWS cloud key at
   `/credentials/cloud-keys`. Use a smoke-only access key and select a region
   with known resources.
2. Create a Cloud Account at `/assets/cloud-accounts`. Select that cloud key,
   enable the matching region, and leave the account enabled.
3. Click **Sync now** (or wait for the configured cron). Confirm the request
   returns immediately and the row becomes available for another sync after
   the worker completes.
4. Open `/assets/sync-runs`. Expect `instance`, `network`, `database`,
   and `security_group` runs plus a successful `planning` run with status `success`; verify `items_seen` matches the account's
   discovered resources. A failed run exposes its error code and reveals the
   detailed error only on the error control's tooltip.
5. Open `/assets/instances`, `/assets/vpcs`, and `/assets/databases`.
   Check account/region/search filters and pagination. Open a VPC and verify
   its subnet list; verify keyboard Enter and Space also open a selected VPC.
6. Open `/assets/firewalls` (the legacy `/assets/security-groups` URL redirects), filter by account, region and VPC, and open
   a group. Compare inbound/outbound rules, IPv4/IPv6 ranges, group references,
   prefix lists, port ranges and ICMP type/code with AWS. The menu order is
   Cloud Accounts, VPCs, Firewalls, Instances, Databases, Sync Runs.
   No AWS write actions are exposed.
7. Remove one enabled region from the Cloud Account. With **Include deleted**
   enabled, verify resources in that region become soft-deleted; with it
   disabled, verify they are hidden.
8. If the owner has independently changed a disposable resource, run another
   sync and confirm the missing resource is soft-deleted only after a successful
   full sweep. The smoke credential performs no cloud writes.
9. While the Cloud Account exists, try deleting its cloud key. Expect error
   `43001` (`assets.cloud_account.in_use`). Delete the Cloud Account and
   verify the response reports its cascaded soft-delete count; retrying the
   cloud-key delete should now succeed.

## Reporting a failure

Capture the response envelope's `code` and `message_key`, the related
`assets_sync_runs` and `audit_logs` rows, and sanitized backend logs. Never capture credentials, tokens, raw SDK payloads
or full provider error bodies.
