# 安装 Optimus

[English](installation.md) · [文档索引](README.md)

**v0.1.0 正在准备发布。** 以下命令固定使用此版本，正式发布后才能执行下载和拉取。请先检查 [Releases](https://github.com/ArkGravity/optimus/releases)。操作员对 `main-1bbb8fe` 的实云验收与本次版本化产物验证分别记录。

支持 Linux amd64 与 arm64。Compose 方式需要 Docker Engine 和 Docker Compose 插件；原生二进制需要 PostgreSQL 17。运行发布包无需 Go、Bun 或源码仓库。

## 下载与校验

发布包含六个附件：

| 文件 | 用途 |
| --- | --- |
| `optimus_v0.1.0_linux_amd64.tar.gz` | Linux x86-64 二进制 |
| `optimus_v0.1.0_linux_arm64.tar.gz` | Linux ARM64 二进制 |
| `optimus_v0.1.0_deploy.tar.gz` | 使用发布镜像的 Compose 部署包 |
| `INSTALL.md` | 独立安装说明 |
| `IMAGES.txt` | 镜像引用与摘要 |
| `SHA256SUMS` | 附件校验和 |

从对应版本的 GitHub Release 下载。例如：

```bash
curl -fLO https://github.com/ArkGravity/optimus/releases/download/v0.1.0/optimus_v0.1.0_deploy.tar.gz
curl -fLO https://github.com/ArkGravity/optimus/releases/download/v0.1.0/SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
tar -xzf optimus_v0.1.0_deploy.tar.gz
```

在下载目录执行校验，确认所有已下载的清单文件均通过。校验和用于检测文件与清单是否一致；二者都应来自可信发布页。

## Docker Compose 部署

解压包包含 `docker-compose.yml`、`.env.example`、`LICENSE` 和 `INSTALL.md`，没有构建镜像步骤。在解压目录执行：

```bash
docker compose version
cp .env.example .env
chmod 600 .env
openssl rand -hex 32
openssl rand -base64 48
docker run --rm ghcr.io/arkgravity/optimus:v0.1.0 vault-keygen
```

安全保存三次独立生成的结果，启动前编辑 `.env`：

- 设置 `COMPOSE_PROJECT_NAME=optimus-prod`、`IMAGE_REPOSITORY=ghcr.io/arkgravity`（或 `docker.io/logic3579`）、`OPTIMUS_VERSION=v0.1.0`。
- 第一次输出用于 `POSTGRES_PASSWORD`，并同步写入 `OPTIMUS_DATABASE_DSN`，确保用户名、密码、数据库名完全一致。容器内 DSN 的主机名是 `postgres`。
- 第二次输出用于 `OPTIMUS_JWT_SECRET`，至少 32 字节。
- `vault-keygen` 输出用于 `OPTIMUS_VAULT_MASTER_KEY`，它是 32 字节密钥的 base64 编码。每个数据库初始化时生成一次，安全备份并长期保持一致，不要随意更换。
- 保持 `HTTP_BIND=127.0.0.1` 和 `POSTGRES_BIND=127.0.0.1`。远程访问通过 TLS 反向代理，`OPTIMUS_SERVER_TRUSTED_PROXIES` 仅填写该代理的 IP/CIDR。
- 可修改初始化管理员用户名和邮箱。内嵌 UI 与 API 同源，无需 CORS 项；额外浏览器来源使用逗号分隔的白名单。
- 日常运行使用 `OPTIMUS_LOG_LEVEL=info`，保护 `.env` 和 seed 日志。

镜像为 `ghcr.io/arkgravity/optimus:v0.1.0` 和 `docker.io/logic3579/optimus:v0.1.0`，Docker 自动选择 amd64/arm64。摘要以 `IMAGES.txt` 为准；仓库需允许匿名拉取，否则先认证。

**使用已有数据库前，必须先阅读 [恢复与数据库切换指南](operations/recovery.md)。** 历史 00022/00023/00028 不能直接运行本版本重新编号的迁移。

```bash
docker compose pull
docker compose up -d --no-build
docker compose ps -a
docker compose logs migrate seed
```

Compose 先等待 PostgreSQL 17 健康，再依次执行 migrate、seed，最后启动 Optimus。预期 postgres 和 optimus 健康，migrate/seed 以 0 退出。

本机打开 `http://127.0.0.1:8080`，远程使用已配置的 HTTPS 地址。用户名为 `OPTIMUS_BOOTSTRAP_ADMIN_USERNAME`（默认 `admin`），密码由 seed **仅打印一次**：

```bash
docker compose logs seed
curl -fsS http://127.0.0.1:8080/api/v1/health
```

seed 可重复执行，但不会再次打印或重置已有管理员密码。请将首次密码存入密码管理器。丢失后需通过数据库重置管理员密码。

## 原生 Linux 二进制

下载对应架构二进制包与 SHA256SUMS，按上文校验后解压。根目录包含 `optimus`、`configs/config.yaml`、`LICENSE` 和 `INSTALL.md`。UI 与迁移均内嵌，构建时使用 `CGO_ENABLED=0`。访问云平台和 Kubernetes HTTPS 地址需要安装 Linux 发行版的 CA 证书信任库；禁用 CGO 不代表二进制内置 CA 证书。容器镜像已包含 CA 证书。

创建空的 PostgreSQL 17 数据库与专用用户。修改 `configs/config.yaml` 或导出配置覆盖值，必须替换示例中的开发 JWT/vault 密钥：

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

执行前替换 DSN 密码，并按网络需求配置 PostgreSQL TLS。安全持久化 JWT 和 vault 密钥供后续每次启动使用；上述生成命令只用于首次初始化。由服务管理器运行时，应使用受保护的环境或配置文件并保持密钥一致。

原生进程 **不会读取 `.env`**。Compose 使用它插值，二进制只读取 YAML 与实际环境变量。如需使用 `OPTIMUS_VAULT_MASTER_KEY_FILE`，必须先将所选 YAML 中的 `vault.master_key` 设为 `""`，并取消设置 `OPTIMUS_VAULT_MASTER_KEY`，再将文件配置指向进程可读取的受保护密钥文件。内联密钥优先于文件；仅把环境变量设为空会回退到 YAML 中的开发密钥。备份时保留该文件中对应的密钥。远程访问使用 TLS 反向代理，仅信任该代理的 IP/CIDR。seed 首次输出管理员密码，登录方式同 Compose。

同一端口提供 `/` UI、`/api/v1` API 和 `/swagger/index.html`。健康检查为 `GET /api/v1/health`，返回原始数据库/版本信息。

## 首次连接

1. 创建 Cloud Key：AWS/阿里云 AK/SK 或 GCP Service Account JSON。
2. 创建关联凭据的 Cloud Account，选择云平台与区域；GCP 明确填写目标 Project ID。
3. 请求 Sync，通过 Sync Runs 确认结果，再查看 VPC、防火墙、实例、数据库。只授予读取权限，见 [AWS](../scripts/p4-smoke.md) 和 [多云](../scripts/p4-multicloud-smoke.md) 检查单。
4. Kubernetes 先保存 kubeconfig，再创建引用它的集群并在页面选择。禁止 `exec`/`auth-provider` 插件。

应用不使用环境中隐式的云认证。GCP 的 `GOOGLE_API_USE_CLIENT_CERTIFICATE` 只能未设置或为大小写不敏感的 `false`，其他值会导致配置失败。

## 维护

保持 Compose 项目名和持久化卷一致。修改 `.env` 不会改变已初始化 PostgreSQL 卷内的密码。不要用 `docker compose down -v` 轮换密码、升级或恢复，它会删除持久数据。备份数据库时同时保留对应 vault 密钥、应用版本及配置，版本切换前阅读 [恢复指南](operations/recovery.md)。

源码开发请使用独立的 [开发指南](development/getting-started.md)。
