<p align="center"><img src="web/public/optimus-logo.png" width="96" alt="Optimus 标志"></p>

# Optimus

[![CI](https://github.com/ArkGravity/optimus/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ArkGravity/optimus/actions/workflows/ci.yml)
[版本发布](https://github.com/ArkGravity/optimus/releases) · [English](README.md) · [文档](docs/README.md)

Optimus 是可自托管的基础设施控制台，将访问管理、凭据保护、Kubernetes 查看、Helm 应用管理和多云资产查询集中到一个界面。Web UI 与 API 内嵌在同一个 Go 二进制中，使用 PostgreSQL 17 存储数据。

## 功能

- 用户、角色、菜单、权限管理，以及 RBAC 和审计记录。
- 使用 AES-256-GCM 加密保存 kubeconfig 和云凭据。
- 通过只读查看 API 检查 Kubernetes 工作负载、资源和日志。
- 管理 Helm 应用和发布。
- 采集 AWS、阿里云、GCP 的只读资产，使用统一筛选和详情页。
- 支持简体中文、英语以及明暗主题。

| 云平台 | 资产范围 |
| --- | --- |
| AWS | EC2 实例、VPC/子网、安全组、RDS |
| 阿里云 | ECS 实例、VPC/子网、安全组、RDS |
| GCP | Compute Engine 实例、VPC/子网、防火墙规则、Cloud SQL |

资产菜单顺序为 **云账号 → VPC → 防火墙 → 实例 → 数据库 → 同步记录**。采集使用明确选择的凭据，不提供云资源写入或管理 API。Dashboard 和 Observability Center 目前为 Coming Soon 占位页。

## 安装与首次登录

推荐使用 Docker Compose 部署应用与 PostgreSQL，也可使用 Linux 二进制连接现有 PostgreSQL 17。运行发布包无需安装 Go 或 Bun。

**[v0.1.2 已于 2026-11-12（UTC）正式发布](https://github.com/ArkGravity/optimus/releases/tag/v0.1.2)。** Linux amd64/arm64 二进制包、部署包及两个镜像仓库均已通过 [实际产物验证](https://github.com/ArkGravity/optimus/actions/runs/43303096304/attempts/2) 和发布后独立复核。请按 [安装指南](docs/installation_zh.md) 下载发行附件。

1. 按照 [Compose 或二进制安装指南](docs/installation_zh.md) 操作。共享环境启动前必须分别生成数据库密码、JWT 密钥和 vault 密钥。
2. 本机打开 `http://127.0.0.1:8080`，远程访问使用反向代理配置的 HTTPS 地址。
3. 使用初始化用户名（默认 `admin`，可通过 `OPTIMUS_BOOTSTRAP_ADMIN_USERNAME` 配置）和 seed 仅打印一次的密码登录。Compose 部署可执行 `docker compose logs seed` 获取，并安全保存。

恢复方案必须同时保留数据库备份与对应的 vault 密钥；丢失 vault 密钥将无法解密已保存的凭据。

## 连接第一组资源

采集云资产时，先创建 **Cloud Key（云密钥）**，填写 AWS/阿里云 AK/SK 或 GCP Service Account JSON，再创建关联该凭据的 **Cloud Account（云账号）**，选择云平台及区域。GCP 还需明确填写目标 Project ID，可与凭据所属项目不同。执行 **Sync（同步）**，查看 **Sync Runs（同步记录）**，然后浏览 VPC、防火墙、实例和数据库。使用只读云权限，所需权限与验证步骤见 [AWS 检查单](scripts/p4-smoke.md) 和 [多云检查单](scripts/p4-multicloud-smoke.md)。

连接 Kubernetes 时，先在 vault 中保存 kubeconfig，再创建引用它的集群。包含 `exec` 或 `auth-provider` 插件的 kubeconfig 会被拒绝。在 Kubernetes 页面选择集群后即可查看资源。

操作员已于 2026-11-05 确认 `main-1bbb8fe` 的 AWS、阿里云和 GCP 实云验收。该报告与版本化发行产物的自动验证分别记录，详见 [发布说明](docs/releases/v0.1.0.md)。

## 数据库基线

当前 main 使用全新的 19 个迁移文件基线，包含 31 张业务表及 Goose 历史表。历史 00022、00023 或 00028 数据库无法使用重新编号的迁移原地升级。使用已有数据库前，请阅读 [数据库切换与恢复指南](docs/operations/recovery.md)。重建容器不会清除 PostgreSQL 持久化数据。

## 文档与开发

- [中文安装指南](docs/installation_zh.md) · [English installation](docs/installation.md)
- [文档索引](docs/README.md) · [API 规范](docs/api/swagger.json) · [权限目录](docs/permissions.md)
- [开发环境](docs/development/getting-started.md) · [CI](docs/development/ci.md) · [发布流程](docs/development/releases.md)
- [贡献指南](CONTRIBUTING.md) · [安全报告](SECURITY.md)

项目使用 Go 1.25、Vue 3、TypeScript、Ant Design Vue 和 Bun。`make build` 构建内嵌 UI 的二进制，环境配置和检查命令见开发指南。

## 许可证

本项目采用 [MIT 许可证](LICENSE)。Copyright © 2026 ArkGravity contributors.
