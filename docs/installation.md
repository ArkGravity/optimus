# Install Optimus

[中文](installation_zh.md) · [Documentation](README.md)

**[v0.1.2 was published on 2026-11-12 (UTC)](https://github.com/ArkGravity/optimus/releases/tag/v0.1.2).** The commands below pin that released version. The operator's real-cloud acceptance of `main-1bbb8fe` is separate from completed verification of these exact versioned artifacts.

Use Linux amd64 or arm64. Runtime prerequisites are Docker Engine with the Docker Compose plugin for the bundled stack, or PostgreSQL 17 for a native binary. Neither installation requires Go, Bun or a source checkout.

## Choose and verify a download

The release has six attachments:

| File | Purpose |
| --- | --- |
| `optimus_v0.1.2_linux_amd64.tar.gz` | Linux x86-64 binary |
| `optimus_v0.1.2_linux_arm64.tar.gz` | Linux ARM64 binary |
| `optimus_v0.1.2_deploy.tar.gz` | Published-image Compose stack |
| `INSTALL.md` | Standalone installation instructions |
| `IMAGES.txt` | Published image references and digests |
| `SHA256SUMS` | Attachment checksums |

Download from the version's GitHub release, not a moving branch. For example, for Compose:

```bash
curl -fLO https://github.com/ArkGravity/optimus/releases/download/v0.1.2/optimus_v0.1.2_deploy.tar.gz
curl -fLO https://github.com/ArkGravity/optimus/releases/download/v0.1.2/SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
tar -xzf optimus_v0.1.2_deploy.tar.gz
```

Run the checksum command in the directory holding the downloaded files and require every downloaded listed file to pass. SHA256SUMS detects corruption relative to the published manifest; obtain both from the trusted release page.

## Docker Compose

The deployment archive contains `docker-compose.yml`, `.env.example`, `LICENSE` and `INSTALL.md`. Run from its extracted directory. It has no image build step.

```bash
docker compose version
cp .env.example .env
chmod 600 .env
openssl rand -hex 32
openssl rand -base64 48
docker run --rm ghcr.io/arkgravity/optimus:v0.1.2 vault-keygen
```

Record the three independently generated outputs securely. Edit `.env` before startup:

- Set `COMPOSE_PROJECT_NAME=optimus-prod`, `IMAGE_REPOSITORY=ghcr.io/arkgravity` (or `docker.io/logic3579`) and `OPTIMUS_VERSION=v0.1.2`.
- Use the first output as `POSTGRES_PASSWORD`, and use exactly the same password, user and database in `OPTIMUS_DATABASE_DSN`. For this stack the DSN host is `postgres`.
- Use the second output as `OPTIMUS_JWT_SECRET` (at least 32 bytes).
- Use the `vault-keygen` output as `OPTIMUS_VAULT_MASTER_KEY`: base64 encoding of exactly 32 bytes. Generate this once for the database, back it up and keep it stable. Do not casually rotate it.
- Keep `HTTP_BIND=127.0.0.1` and `POSTGRES_BIND=127.0.0.1`. For remote users, put a TLS reverse proxy in front of HTTP and set `OPTIMUS_SERVER_TRUSTED_PROXIES` to that proxy's IP/CIDR only.
- Set the bootstrap username/email if needed. The embedded UI is same-origin and needs no CORS entry; extra origins use a comma-separated allowlist.
- Use `OPTIMUS_LOG_LEVEL=info` for ordinary operation. Protect `.env` and seed logs.

The release image is available at both `ghcr.io/arkgravity/optimus:v0.1.2` and `docker.io/logic3579/optimus:v0.1.2`; Docker selects amd64 or arm64 automatically. Check `IMAGES.txt` for recorded digests. Anonymous registry access must be available, or authenticate before pulling.

**For an existing database, stop here and read [recovery](operations/recovery.md).** This release's fresh schema cannot be applied over historical 00022/00023/00028.

```bash
docker compose pull
docker compose up -d --no-build
docker compose ps -a
docker compose logs migrate seed
```

Compose starts PostgreSQL 17, waits for it to be healthy, runs `migrate`, then `seed`, and starts Optimus after both exit successfully. Expect healthy `postgres` and `optimus`, and exit code 0 for the one-shot services.

Open `http://127.0.0.1:8080` locally or your HTTPS address. Login uses `OPTIMUS_BOOTSTRAP_ADMIN_USERNAME` (`admin` by default) and the password printed **once** by seed:

```bash
docker compose logs seed
curl -fsS http://127.0.0.1:8080/api/v1/health
```

Seed is repeatable; it does not print an existing administrator's password again. Record the initial password in a password manager. Losing it requires an administrator password reset through the database; rerunning seed does not reset it.

## Native Linux binary

Download the appropriate binary archive and SHA256SUMS, verify them as above, and extract the archive. Its root contains `optimus`, `configs/config.yaml`, `LICENSE` and `INSTALL.md`. The binary includes the UI and migrations and is built with `CGO_ENABLED=0`. Install your Linux distribution's CA certificate trust store for HTTPS connections to cloud and Kubernetes endpoints; CGO-disabled binaries do not bundle CA certificates. The container image already includes CA certificates.

Create an empty PostgreSQL 17 database and a dedicated database user. Configure `configs/config.yaml` for your database and environment, or export overrides. Development JWT/vault values in the example configuration must be replaced.

```bash
chmod 700 optimus
export OPTIMUS_DATABASE_DSN='host=127.0.0.1 port=5432 user=optimus password=REPLACE_WITH_DB_PASSWORD dbname=optimus sslmode=disable'
export OPTIMUS_JWT_SECRET="$(openssl rand -base64 48)"
export OPTIMUS_VAULT_MASTER_KEY="$(./optimus vault-keygen)"
export OPTIMUS_SERVER_HOST=127.0.0.1
./optimus version
./optimus migrate -config configs/config.yaml -dir up
./optimus seed -config configs/config.yaml
./optimus server -config configs/config.yaml
```

Replace the DSN password before running these commands; choose PostgreSQL TLS settings for your network. Securely persist the exported JWT and vault values for every subsequent startup. The sample key-generation exports are for initial setup only. For managed operation, configure your service manager with a protected environment/configuration and the same values.

The binary **does not load `.env`**. Compose reads it for interpolation; a native process uses YAML and its actual environment. To use `OPTIMUS_VAULT_MASTER_KEY_FILE`, first set `vault.master_key: ""` in the selected YAML and unset `OPTIMUS_VAULT_MASTER_KEY`, then point the file setting to a protected key file readable by the process. Inline keys take precedence over the file; an empty environment value alone falls back to the YAML's development key. Preserve the file's matching key with backups. Use a TLS reverse proxy for remote access and trust only its IP/CIDR. Seed prints the administrator password to its output once; login is the same as for Compose.

The server serves the UI at `/`, API at `/api/v1` and Swagger at `/swagger/index.html`. `GET /api/v1/health` is a raw database/version probe.

## First connection

1. Create a Cloud Key: AWS/Aliyun AK/SK or GCP Service Account JSON.
2. Create a Cloud Account with its provider, bound credential and regions. For GCP, specify the target Project ID explicitly.
3. Request Sync and check Sync Runs before browsing VPCs, Firewalls, Instances and Databases. Supply only read permissions; see the [AWS](../scripts/p4-smoke.md) and [multi-cloud](../scripts/p4-multicloud-smoke.md) checklists.
4. For Kubernetes, store a kubeconfig, create a referencing cluster and select it in Kubernetes pages. `exec`/`auth-provider` plugins are unsupported.

The application does not use ambient cloud authentication. GCP's `GOOGLE_API_USE_CLIENT_CERTIFICATE` must be unset or case-insensitive `false`; other values fail configuration.

## Maintenance

Existing v0.1.0/v0.1.1 databases already using head19 need no recreation for this update. Preserve the matching vault key and configuration, update the image/binary, and run `optimus seed` to refresh built-in menus and grants. Custom roles need `k8s:storage:read` and `docker:environment:read` for the new entries.

Keep the Compose project name and persistent volume identity stable. Changing `.env` does not change the password already stored in an initialized PostgreSQL volume. Do not use `docker compose down -v` to rotate passwords, upgrade or recover data. Back up PostgreSQL with the matching vault key and application configuration; see [recovery and database transitions](operations/recovery.md) before changing versions.

For local source development, use the separate [developer setup](development/getting-started.md).
