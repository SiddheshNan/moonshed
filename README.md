<p align="center"><img src="assets/logo.svg" width="96" alt="Moonshed logo"></p>

<h1 align="center">Moonshed</h1>

<p align="center">Self-hosted app deployment in one binary.<br>Runs on Linux and Windows.</p>

<p align="center">
  <a href="https://moonshed.siddhesh.me">Website</a> ·
  <a href="https://moonshed.siddhesh.me/docs/">Docs</a> ·
  <a href="https://moonshed.siddhesh.me/docs/reference/moonshed-yaml">moonshed.yaml</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

> Moonshed is in early development. Expect rough edges and breaking changes.

![Moonshed dashboard](website/public/screenshots/dashboard.png)

## Why Moonshed

Tools like Dokploy and Coolify need Postgres, Redis, Traefik and Docker before your first app runs. Moonshed is a single binary with SQLite and a built-in Caddy proxy. It runs fine on 1 core and 1 GB of RAM, and uses about 15 MB of memory itself.

Each app runs either as a Native Process that Moonshed supervises (like pm2) or as a Docker container. Settings go in a `moonshed.yaml` in your repo or in the panel.

## Features

- Deploy from a GitHub App, any git URL, a zip upload or CI
- Deploy on every push, or deploy new commits when you're ready
- A new version only replaces the old one after its health check passes
- Redeploy any recent version
- Docker builds with BuildKit
- Domains, path routing and automatic HTTPS
- Per-domain redirects, compression, basic auth, IP allow lists and headers
- Encrypted env vars and data folders that survive deploys
- Live logs, resource graphs, uptime and a request log
- App and platform backups, on a schedule or on demand
- Projects, users, API tokens and an audit log

| App overview | Deployments |
|---|---|
| ![App overview](website/public/screenshots/app-overview.png) | ![Deployments](website/public/screenshots/deployments.png) |
| Monitoring | Backups |
| ![Monitoring](website/public/screenshots/monitoring.png) | ![Backups](website/public/screenshots/backups.png) |

## Quick start

Download the latest release on your server and install it as a service:

```bash
cd /opt
curl -fsSL https://github.com/SiddheshNan/moonshed/releases/latest/download/moonshed-linux-amd64.tar.gz | sudo tar -xz
cd moonshed
sudo ./moonshed install -e production
```

Open `http://<server>:5050` and create the admin account.

Releases are built for Linux (x86-64 and ARM64) and Windows (x86-64). See the [installation guide](https://moonshed.siddhesh.me/docs/installation) for Windows, putting the panel behind HTTPS, upgrading, and building from source.

## Deploy an app

Add a `moonshed.yaml` to your app, or set the same things in the panel:

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

## Current limits

- One server only.
- A short gap during each deploy while the old version stops.
- No per-app CPU or memory limits.
- Native Process apps aren't sandboxed. Give an app its own OS user to limit what it can touch.

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
