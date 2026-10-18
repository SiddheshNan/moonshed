<p align="center"><img src="assets/logo.svg" width="96" alt="Moonshed logo"></p>

<h1 align="center">Moonshed</h1>

<p align="center">A small, self-hosted platform for deploying your apps.<br>One Go binary + SQLite. Runs on Linux and Windows.</p>

> **Status: early development.** Moonshed is pre-release. Expect rough edges and breaking changes.

![Moonshed dashboard](website/public/screenshots/dashboard.png)

| App overview | Deployments |
|---|---|
| ![App overview](website/public/screenshots/app-overview.png) | ![Deployments](website/public/screenshots/deployments.png) |
| **Monitoring** | **Backups** |
| ![Monitoring](website/public/screenshots/monitoring.png) | ![Backups](website/public/screenshots/backups.png) |

## Why Moonshed

Moonshed is a lightweight alternative to platforms like Dokploy, built to run comfortably on 1 core / 1 GB RAM:

- **One binary, one SQLite file.** No Postgres, Redis or Traefik. Docker is optional.
- **Linux and Windows.** Runs as a systemd unit or a Windows service.
- **Native or Docker, per app.** Run an app as a plain supervised process, or as a container.

## Features

- **Deploy** from GitHub (through a GitHub App), any git URL, a zip upload, or CI with an API token. Pushes can deploy automatically, and each deployment links to its commit.
- **Configure in the repo or the UI.** `moonshed.yaml` describes how to build, run and check the app; the same settings can be changed in the UI, which wins over the file.
- **Docker builds** with BuildKit: custom Dockerfile path, build folder, stage and build arguments.
- **Safe rollouts.** A new version must pass its health check before it replaces the old one, and earlier versions can be redeployed.
- **Live logs** for every app and every deployment.
- **Built-in reverse proxy** (Caddy) with domains and automatic HTTPS (Let's Encrypt), or your own certificate.
- **Monitoring.** CPU, memory, disk and network graphs, uptime and incidents from real health checks, optional auto-restart, and a request log.
- **Backups** of an app's code, data, env vars and logs, as an archive that unpacks to a plain copy of the app. On a schedule or on demand, restorable part by part. Moonshed backs up its own database too.
- **Teams.** Projects, users, API tokens and an audit log.

## Install

Needs Go 1.26+, Node 20+ and Python 3 to build.

```bash
./scripts/build.py
```

It asks for the version, the target platforms, the output folder and whether to run the tests (Enter accepts the defaults), then writes a bundle per target:

```
dist/linux-amd64/
  moonshed
  runtime/
    config/production.yaml
    web/                      the UI
```

Copy a bundle to your server, check `runtime/config/production.yaml`, then as root / Administrator:

```bash
./moonshed install -e production
```

This registers a systemd unit (Linux) or a Windows service and starts it on boot. Open `http://<server>:5050` and create the admin account. The rest of `runtime/` (database, apps, logs, backups) is created on first run.

Other commands:

```bash
./moonshed serve -e production     # run in the foreground
./moonshed uninstall
./moonshed reset-password admin
```

`-e` (or `ENV`) picks `runtime/config/<env>.yaml` and is required; Moonshed exits if the file is missing or incomplete. `--runtime` (or `RUNTIME_DIR`) defaults to `runtime/` next to the binary.

## Deploy an app

Deploy from a git repository or a zip. Describe how to run it in a `moonshed.yaml` at the root, or set the same things in the app's Build settings; a `Dockerfile` alone also works:

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
  interval: 30s
  failures: 3
restart_unhealthy: true
```

Settings made in the UI override the file. Zips can be uploaded in the UI, or from CI with an API token:

```bash
curl -fsS -H "Authorization: Bearer $MOONSHED_TOKEN" -H "Content-Type: application/zip" \
  --data-binary @app.zip https://panel.example.com/api/apps/$APP_ID/deploy
```

Scripts can be one-liners, multi-line blocks, a path to a `.sh`/`.ps1`/`.cmd` file, or per-OS (`linux:` / `windows:`). Docker apps can add a `build:` block (`dockerfile`, `folder`, `stage`, `args`). [DESIGN.md](docs/DESIGN.md) lists every key, and [examples/python-app](examples/python-app) is a working example.

## Development

```bash
go run ./cmd/moonshed serve -e development
cd ui && npm install && npm run dev
```

Open http://localhost:5050. In development the panel forwards the UI to the Vite dev server (`ui:` in `runtime/config/development.yaml`), so changes show up live. See [CONTRIBUTING.md](CONTRIBUTING.md) for the project layout and conventions.

## Docs

The user docs are a VitePress site in [website/](website). Run them locally with:

```bash
cd website && npm install && npm run dev
```

[DESIGN.md](docs/DESIGN.md) explains how it works: `moonshed.yaml`, runtimes, the deploy flow, the proxy, monitoring, backups and the API.

## Contributing

Issues and pull requests are welcome. Moonshed is early, so please open an issue to discuss bigger changes first. See [CONTRIBUTING.md](CONTRIBUTING.md), and report security issues as described in [SECURITY.md](SECURITY.md).

## License

[AGPL-3.0](LICENSE). You can use, modify and self-host Moonshed freely; if you offer a modified version to others as a network service, you must publish your changes under the same license.
