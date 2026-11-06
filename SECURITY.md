# Security policy

Optimus stores sensitive credentials. Keep it behind trusted access controls and TLS, use least-privilege cloud and Kubernetes credentials, and retain the database's matching vault key securely. See [installation](docs/installation.md) and [recovery](docs/operations/recovery.md).

## Reporting a vulnerability

Do not publish exploit details or secrets in a public issue. Use GitHub's private [Report a vulnerability](https://github.com/ArkGravity/optimus/security/advisories/new) channel for this repository. Private vulnerability reporting is enabled. No dedicated security email address or response deadline is currently published.

Privately provide the affected version, impact, reproduction steps and a minimal sanitized example. Never send vault keys, JWTs, cloud secrets, kubeconfigs, Service Account JSON, database backups or raw provider responses.

## Supported versions

v0.1.0 is in preparation. No fixed security maintenance window is promised. Check [Releases](https://github.com/ArkGravity/optimus/releases) and release notes for available fixes before upgrading, and follow the documented database compatibility requirements.
