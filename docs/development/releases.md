# Versioned releases

[v0.1.2](../releases/v0.1.2.md) was published on 2026-11-12 with Linux
amd64/arm64 binary archives, a deployment archive and versioned images in both
registries. Exact-source main CI, native runtime smoke, independent attachment
and registry verification, and public post-publication checks passed. See the
[release record](../superpowers/plans/2026-11-12-v0.1.2-release.md).

[v0.1.1](../releases/v0.1.1.md) was published on 2026-11-12 as a source-only
Release after operator acceptance and an explicit request to publish directly
without waiting for or reviewing CI or release artifacts. It has no binary
attachments and asserts no versioned-image availability. This one-off direct
publication did not change the automated artifact release workflow or the
standard sequence below.

[v0.1.0 was published](https://github.com/ArkGravity/optimus/releases/tag/v0.1.0) on 2026-11-06 (UTC), after explicit operator approval of the verified draft. The [release notes](../releases/v0.1.0.md) track operator acceptance separately from completed [release artifact verification](https://github.com/ArkGravity/optimus/actions/runs/42292847315/attempts/2) and publication. The selected source is `7102a3f`; the trusted recovery workflow definition is `ac605a5`. See the [release plan](../superpowers/plans/2026-11-05-v0.1.0-release.md) for exact CI, digest and attachment evidence.

## Release contract

Use an annotated `vX.Y.Z` tag on the reviewed main revision. Never overwrite a published version tag, image tag or release asset; publish a new version for corrections. A moving `latest` tag is not required.

Artifact releases need no source checkout or Go/Bun installation. For v0.1.2 the exact six attachments are:

- `optimus_v0.1.2_linux_amd64.tar.gz`
- `optimus_v0.1.2_linux_arm64.tar.gz`
- `optimus_v0.1.2_deploy.tar.gz`
- `INSTALL.md`
- `IMAGES.txt`
- `SHA256SUMS`

Each binary archive contains `optimus`, `configs/config.yaml`, `LICENSE` and `INSTALL.md`. Build binaries with embedded UI/migrations and `CGO_ENABLED=0`. The deployment archive contains a no-build `docker-compose.yml`, version-pinned `.env.example`, `LICENSE` and `INSTALL.md`. Its secrets require generation before startup.

[INSTALL.md](../releases/INSTALL.md) is a standalone source template; substitute `@VERSION@` when packaging. `IMAGES.txt` records the final image references/digests for both registries. `SHA256SUMS` covers the other five attachments, not itself.

## Delivery sequence

1. Merge the release preparation PR to main. Require full CI on the exact main revision; a prose-only skipped run is insufficient. Confirm schema compatibility, license, documentation and version metadata.
2. Create the annotated version tag on that revision. Release automation must verify tag/revision provenance and refuse existing version destinations.
3. Build each platform's release image once, extract its binary and configuration, and package the release archives. Use native Linux amd64 and arm64 jobs to execute the exact packaged binaries, including version, fresh migrate/seed and authenticated embedded UI/API smoke. Test the actual images and deployment archive, including its secrets contract, then save those tested image outputs for publication. Cross-compilation alone is not execution evidence.
4. Publish the saved, tested image bytes to GHCR and Docker Hub without rebuilding. Inspect both platform manifests and digests, confirm configuration/digest equality with the tested outputs and between registries, and verify anonymous reads of the published manifests. Record the final references and digests in `IMAGES.txt`. Do not substitute a different main image after testing.
5. Render INSTALL, produce the three archives and checksum manifest, and verify the final exact attachments. Upload all six to a **draft** release with the reviewed notes and verification evidence.
6. Review the draft's tag, attachments, checksums, image digests, installation instructions and database warning. Publish only after the final review. Set the actual release date in the changelog and notes after publication is confirmed.

A tag, green source tests, operator report or draft release alone is not completed publication. If any job fails, preserve the failure evidence and fix it before proceeding; do not replace public artifacts in place.

### Retry without moving a version tag

Check out raw commit IDs rather than the Actions tag ref: checkout's fallback can
replace an annotated tag with a lightweight runner-local ref. Never move the
remote version tag to repair this. A workflow-definition repair can be reviewed
and merged to main, then retried from main with an existing stable tag:

```bash
gh workflow run release.yml --ref main -f version=v0.1.0
```

Manual retries are main-only and must still resolve an annotated stable tag,
select its exact source commit, verify main ancestry and require that commit's
six successful main CI jobs. The workflow run's own main revision can be newer;
artifact checkout, OCI revision metadata and binary verification use the selected
tagged source, not that newer workflow revision. A missing, lightweight, arbitrary
branch or invalid version is rejected. A defect requiring different product
source needs a new version, not replacement of an existing tag or attachment.

## Database and recovery requirements

The initial version uses fresh head19: 31 business tables and Goose history. Old 00022/00023/00028 databases require actual recreation, including migration history; this is not an in-place upgrade. Stop writers and retain database backups with their matching vault key, config and application revision. Fresh Down operations drop data and are not rollback. See [recovery](../operations/recovery.md).

These released migrations are immutable. The next schema change appends 00020. Keep the API specification's version independent from the product release version; do not rewrite Swagger's API version solely to match a release tag.

## Verification record

Record the exact Git revision, source CI run, native platform and actual-image smoke results, archive checksums, image digests, published manifest/configuration equality, anonymous manifest reads and draft review. Do not record planned checks as passed. Real-provider acceptance should identify the operator, revision and date without exposing credentials or raw SDK responses. Production deployment remains an operator action unless separately authorized.
