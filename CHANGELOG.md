# Changelog

## v0.1.0 — release preparation

The first versioned release is being prepared; no publication date or completed release-artifact verification is claimed here.

- Self-hosted Go binary with embedded Vue UI, PostgreSQL 17 and Docker Compose deployment.
- Authentication, RBAC, menu administration and audit records.
- Encrypted credential vault for kubeconfigs and typed AWS/Aliyun AK/SK and GCP Service Account JSON.
- Read-only Kubernetes inspection and log streaming, plus Helm application management.
- Read-only AWS, Aliyun and GCP inventory with provider-native resource storage, shared lists/details, Firewalls and asynchronous sync history.
- English and Simplified Chinese UI, light/dark themes and workspace tabs.
- Fresh migration baseline ending at 00019. Historical 00022/00023/00028 databases require recreation; see [recovery](docs/operations/recovery.md).
- MIT License and installation, contribution and release documentation.

The operator confirmed real-cloud acceptance of `main-1bbb8fe` on 2026-11-05. Versioned native artifacts, image digests, checksums and publication are separate release gates. See [v0.1.0 notes](docs/releases/v0.1.0.md).
