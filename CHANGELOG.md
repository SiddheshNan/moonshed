# Changelog

Notable changes to Moonshed. Versions follow [semantic versioning](https://semver.org).

## Unreleased

- One-line install on Linux: `curl -fsSL https://moonshed.siddhesh.me/install.sh | sh`. It picks the x86-64 or ARM64 build and checks the download against the release's `SHA256SUMS`.
- Release archives come with signed build provenance, checkable with `gh attestation verify`.

## v0.3.0 (2026-10-20)

First public release. Builds for Linux (x86-64, ARM64) and Windows (x86-64).

- One binary with SQLite and a built-in Caddy reverse proxy; installs as a systemd unit or Windows service.
- Apps run as Native Processes (supervised, restarted on crash) or Docker containers built with BuildKit.
- Deploy from a GitHub App, any git URL, a zip upload or CI with an API token.
- `moonshed.yaml` for build, run, health check and persistent folders, with the same settings editable in the panel.
- A new version replaces the old one only after its health check passes; failed deploys roll back. Any older release can be redeployed.
- Domains, path routing and automatic HTTPS, with raw Caddyfile for advanced setups.
- Logs, resource graphs, health checks, uptime and incidents per app.
- Encrypted environment variables and persistent data folders that survive deploys.
- App and platform backups, on a schedule or on demand.
- Projects, users, API tokens and an audit log.
