# Contributing to Optimus

For a bug report, include the version, installation method, expected behavior and a minimal reproduction. Remove credentials, tokens, kubeconfigs, Service Account JSON, provider payloads and personal data. Report security vulnerabilities through [SECURITY.md](SECURITY.md).

Discuss substantial features in a GitHub issue before implementation so their scope and operational impact can be reviewed. Dashboard and Observability Center remain placeholders; cloud collection and Kubernetes inspection have explicit read-only boundaries.

Read [AGENTS.md](AGENTS.md) and the [developer guide](docs/development/getting-started.md) before changing code. Keep API, permissions, menus and UI changes together. Use English code comments, maintain both UI locales, and use Bun for web dependencies. Published migrations are immutable; append the next migration (currently 00020).

Open a pull request describing the problem, resulting behavior and relevant validation. Run checks appropriate to the affected area: `make test`, `make lint`, `make web-check`, and database integration tests for persistence changes. Regenerate API and permission artifacts when their sources change. Use dedicated disposable databases for tests, and include any checks you could not run in the pull request.

Contributions are made under the project's [MIT License](LICENSE).
