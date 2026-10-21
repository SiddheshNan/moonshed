<p align="center"><img src="assets/logo.svg" width="96" alt="Moonshed logo"></p>

<h1 align="center">Moonshed</h1>

<p align="center">A small, self-hosted platform (PaaS) for deploying your apps.<br>One binary and SQLite. Runs on Linux and Windows.</p>

<p align="center">
  <a href="https://moonshed.siddhesh.me">Website</a> ·
  <a href="https://moonshed.siddhesh.me/docs/">Docs</a> ·
  <a href="https://moonshed.siddhesh.me/docs/reference/moonshed-yaml">moonshed.yaml</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

>  **Status: early development.** Moonshed is pre-release.  Expect rough edges and breaking changes.

![Moonshed dashboard](website/public/screenshots/dashboard.png)

## Why Moonshed

Tools like Dokploy and Coolify need Postgres, Redis, Traefik and Docker before your first app runs. Moonshed is a single binary with SQLite and a built-in Caddy reverse proxy. It runs fine on 1 core and 1 GB of RAM.

Each app runs either as a Native Process that Moonshed supervises (like pm2) or as a Docker container. The app can be simply configured using `moonshed.yaml` in your repo, or from directly on the moonshed webui.

## Features

- Deploy from a GitHub App, any git URL, a zip upload or CI
- Supports app to be deployed as a Native Process or Docker container
- A new version of app only replaces the old one (after its health check passes)
- Redeploy any older version of app
- Docker builds support with BuildKit
- Domains, path routing and automatic HTTPS
- Health checks, logs, resource graphs and uptime monitoring per app
- persistent data folders and encrypted environment variables that survive deploys
- App and platform backups, on a schedule or on demand
- Projects, users, API tokens and an audit log



| App overview | Deployments |
|---|---|
| ![App overview](website/public/screenshots/app-overview.png) | ![Deployments](website/public/screenshots/deployments.png) |
| Monitoring | Backups |
| ![Monitoring](website/public/screenshots/monitoring.png) | ![Backups](website/public/screenshots/backups.png) |

## Quick start

On your Linux server, download the [install script](website/public/install.sh), read it, then run it:

```bash
curl -fsSL https://moonshed.siddhesh.me/install.sh -o install.sh
less install.sh
sh install.sh
```

It downloads the latest release into `/opt/moonshed`, checks it against the release checksums and installs it as a service. Releases are built by [GitHub Actions](.github/workflows/release.yml) from the tagged commit, with signed provenance ([how to check](https://moonshed.siddhesh.me/docs/installation#check-what-you-download)).

Open `http://<server>:5050` and create the admin account.

Releases are built for Linux (x86-64 and ARM64) and Windows (x86-64). See the [installation guide](https://moonshed.siddhesh.me/docs/installation) for Windows, putting the panel behind HTTPS, upgrading, and building from source.

## Deploy an app

Add a `moonshed.yaml` to your app, or set the same things in the webui:

```yaml
pre: |
  python3 -m venv .venv
  .venv/bin/pip install -r requirements.txt
run: .venv/bin/python app.py
port: 8000
persist:                 # kept across deploys
  - data/
health:
  http: /healthz
restart_unhealthy: true
```

Create the app in the panel, pick a repository or upload a zip, and click Deploy. From CI, upload a zip with an API token:

```bash
curl -fsS -H "Authorization: Bearer $MOONSHED_TOKEN" -H "Content-Type: application/zip" \
  --data-binary @app.zip https://panel.example.com/api/apps/$APP_ID/deploy
```

To try it, deploy one of the [examples](examples): a Flask hello world, a FastAPI todo list or a Snake game in Node. The [moonshed.yaml reference](https://moonshed.siddhesh.me/docs/reference/moonshed-yaml) lists every key.

## Current limitations

- One server only, no clustering or multi-server deployments.
- A short gap during each deploy while the old version stops, this is by design on v1
- No per-app CPU or memory limits.
- Native Process apps aren't sandboxed. You will need to give an app its own OS user to limit what it can touch.

## Documentation

- [Docs](https://moonshed.siddhesh.me/docs/): install, deploy, domains, monitoring and backups. Source in [website/](website).
- [DESIGN.md](docs/DESIGN.md): how Moonshed works inside.

Run the docs site locally:

```bash
cd website && npm install && npm run dev
```

## Development

```bash
go run ./cmd/moonshed serve -e development
cd ui && npm install && npm run dev
```

Open http://localhost:5050. In development the panel forwards the UI to the Vite dev server, so changes show up live. See [CONTRIBUTING.md](CONTRIBUTING.md) for the project layout and conventions.

## Contributing

Issues and pull requests are welcome. Moonshed is early, so please open an issue to discuss bigger changes first. See [CONTRIBUTING.md](CONTRIBUTING.md), and report security issues as described in [SECURITY.md](SECURITY.md).

## License

[AGPL-3.0](LICENSE). You can use, modify and self-host Moonshed freely; if you offer a modified version to others as a network service, you must publish your changes under the same license.
