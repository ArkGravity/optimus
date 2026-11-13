# Install Optimus @VERSION@

This standalone file is rendered for a versioned release. When reading the source template, replace `@VERSION@` with the published version. Check https://github.com/ArkGravity/optimus/releases for published versions before downloading. No source checkout, Go or Bun is needed.

## Verify downloads

The release has six attachments:

- `optimus_@VERSION@_linux_amd64.tar.gz`
- `optimus_@VERSION@_linux_arm64.tar.gz`
- `optimus_@VERSION@_deploy.tar.gz`
- `INSTALL.md`
- `IMAGES.txt`
- `SHA256SUMS`

Download the appropriate archive and SHA256SUMS from the same trusted version's release page. Run `sha256sum --ignore-missing -c SHA256SUMS` in their directory and require every downloaded listed file to pass. IMAGES.txt records final image references/digests.

## Compose installation

Requires Docker Engine with the Docker Compose plugin. Extract `optimus_@VERSION@_deploy.tar.gz`; it contains `docker-compose.yml`, `.env.example`, `LICENSE` and this file. The Compose file uses published images and has no build block.

```bash
docker compose version
cp .env.example .env
chmod 600 .env
openssl rand -hex 32
openssl rand -base64 48
docker run --rm ghcr.io/arkgravity/optimus:@VERSION@ vault-keygen
```

Edit `.env` before startup. Set `COMPOSE_PROJECT_NAME=optimus-prod`, `IMAGE_REPOSITORY=ghcr.io/arkgravity` (or `docker.io/logic3579`), and `OPTIMUS_VERSION=@VERSION@`. Use the first random output for POSTGRES_PASSWORD and the identical password in OPTIMUS_DATABASE_DSN (host `postgres`, matching user/database). Use the second output for OPTIMUS_JWT_SECRET (at least 32 bytes). Use vault-keygen's independently generated 32-byte base64 output for OPTIMUS_VAULT_MASTER_KEY. Back up the vault key once and keep it stable for that database.

Keep HTTP_BIND and POSTGRES_BIND at `127.0.0.1`. Put a TLS reverse proxy in front for remote access and set OPTIMUS_SERVER_TRUSTED_PROXIES only to its IP/CIDR. The embedded UI needs no extra CORS origin. Protect .env and seed logs. Authenticate to the selected registry first if anonymous pulls are unavailable.

**Existing database warning:** migrations preserve the published fresh head19 baseline and append 00020 for native GKE connections (31 business tables plus Goose history). Existing head19 databases upgrade in place; do not recreate them. Historical 00022/00023/00028 databases cannot upgrade in place. Stop writers, retain a tested backup with matching vault key/configuration/application revision, and authorize actual database recreation including Goose history before migrate/seed. Container recreation alone keeps old data. No automatic historical-data conversion is provided. Migration20 Down refuses existing GKE bindings; earlier Down migrations drop data and are not recovery. Never use `docker compose down -v` as an upgrade or password-rotation procedure.

```bash
docker compose pull
docker compose up -d --no-build
docker compose ps -a
docker compose logs migrate seed
curl -fsS http://127.0.0.1:8080/api/v1/health
```

PostgreSQL 17 starts first; migrate and seed must exit 0 before Optimus starts. Expect healthy postgres/optimus. The images support linux/amd64 and linux/arm64 automatically:

- `ghcr.io/arkgravity/optimus:@VERSION@`
- `docker.io/logic3579/optimus:@VERSION@`

Open http://127.0.0.1:8080 locally or your configured HTTPS address. Login with OPTIMUS_BOOTSTRAP_ADMIN_USERNAME (default `admin`) and the password printed **once** in `docker compose logs seed`. Record it securely. Repeated seed does not reset an existing administrator password; losing it requires a database-admin reset.

## Native Linux installation

Use PostgreSQL 17 and the archive matching your CPU. Each binary archive contains `optimus`, `configs/config.yaml`, `LICENSE` and this file. The UI/migrations are embedded; the executable is built with CGO_ENABLED=0. Install your Linux distribution's CA certificate trust store for HTTPS cloud and Kubernetes connections; disabling CGO does not bundle CA certificates. The container image already includes CA certificates.

Create an empty database and dedicated user. Replace the development secrets in the YAML or export overrides:

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

Replace the DSN password and choose PostgreSQL TLS settings for your network before running. Persist the same JWT/vault values securely for all subsequent startups; generation is for initial setup only. The binary does **not** load .env: use YAML or actual exported environment. To use OPTIMUS_VAULT_MASTER_KEY_FILE, first set `vault.master_key: ""` in the selected YAML and unset OPTIMUS_VAULT_MASTER_KEY, then point the file setting to a protected key file readable by the process. Inline keys take precedence over files; an empty environment value alone falls back to the YAML's development key. Keep the file's matching key with backups. Use a service manager for ongoing operation, with the same protected configuration. UI is at /, API at /api/v1, Swagger at /swagger/index.html. Apply the same TLS/proxy and first-login guidance as Compose.

## First connection and recovery

Create a Cloud Key using AWS/Aliyun AK/SK or GCP Service Account JSON. Create a Cloud Account bound to it, select regions, and supply GCP's explicit target Project ID. Request Sync, inspect Sync Runs, then browse VPCs, Firewalls, Instances and Databases. Use read-only provider permissions. Kubernetes requires a stored kubeconfig and referencing cluster; kubeconfigs using exec/auth-provider plugins are rejected.

GCP's GOOGLE_API_USE_CLIENT_CERTIFICATE must be unset or case-insensitive false. Collection uses explicit selected credentials and provides no cloud write/manage APIs. Never share raw cloud payloads, secrets, JWTs or kubeconfigs in reports.

A usable recovery set is the database backup plus its matching vault key, configuration and application revision. Restore them together on an isolated target first. PostgreSQL volumes retain their original password after .env changes. Do not delete volumes to rotate passwords. Published baseline19 migrations are immutable; native GKE appends 00020 and the next new migration is 00021.

Full guides: https://github.com/ArkGravity/optimus/blob/@VERSION@/docs/installation.md and https://github.com/ArkGravity/optimus/blob/@VERSION@/docs/operations/recovery.md

Security reports: https://github.com/ArkGravity/optimus/security/advisories/new
