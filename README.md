<p align="center"><img src="assets/logo.svg" width="96" alt="Moonshed logo"></p>

<h1 align="center">Moonshed</h1>

<p align="center">A small, self-hosted platform for deploying your apps.<br>One Go binary + SQLite. Runs on Linux and Windows.</p>

> **Status: early development.** Moonshed is pre-release. Expect rough edges and breaking changes.

![Moonshed dashboard](docs/screenshots/dashboard.png)

| App overview | Deployments | Monitoring |
|---|---|---|
| ![App overview](docs/screenshots/app-overview.png) | ![Deployments](docs/screenshots/deployments.png) | ![Monitoring](docs/screenshots/monitoring.png) |

## Why Moonshed

Moonshed is a lightweight alternative to platforms like Dokploy, built to run comfortably on 1 core / 1 GB RAM:

- **One binary, one SQLite file.** No Postgres, Redis or Traefik. Docker is optional.
- **Linux and Windows.** Runs as a systemd unit or a Windows service.
- **Native or Docker, per app.** Run an app as a plain supervised process, or as a container.

## Features

- **Deploy** from a zip upload, a git repo, or CI with an API token. Builds have their own log and can be killed.
- **Safe rollouts.** A new release starts with a health check, and you can redeploy earlier versions.
- **Live logs** streamed to the UI.
- **Built-in reverse proxy** with domains and automatic HTTPS (Let's Encrypt), or your own certificate.
- **Monitoring.** CPU, memory, disk and network graphs, uptime from real health checks, and a request log.
- **Backups** for each app and for Moonshed itself, on a schedule or on demand.
- **Teams.** Projects, users, API tokens and an audit log.

## Install

Needs Go 1.26+ and Node 20+ to build.

```bash
./build.sh
```

This produces a folder per target (default: linux/amd64, linux/arm64, windows/amd64):

```
dist/linux-amd64/
  moonshed
  runtime/web/      the UI
```

Pick targets with `TARGETS="linux/amd64 windows/amd64" ./build.sh`.

Copy the `dist/<os>-<arch>/` folder to your server, then as root / Administrator:

```bash
./moonshed install
```

This registers a systemd unit (Linux) or a Windows service and starts it on boot. Open `http://<server>:5050` and create the admin account. The rest of `runtime/` (config, database, apps, logs) is created on first run.

Other commands:

```bash
./moonshed serve                   # run in the foreground
./moonshed uninstall
./moonshed reset-password admin
```

## Deploy an app

A bundle is a zip with `app.yaml` at the root (or a `Dockerfile` for Docker apps):

```yaml
pre: |
  python3 -m venv .venv
  .venv/bin/pip install -r requirements.txt
run: .venv/bin/python app.py
persist:
  - data/
health:
  http: /healthz
```

Upload it in the UI, or from CI with an API token (Settings → API tokens):

```bash
curl -fsS -H "Authorization: Bearer $MOONSHED_TOKEN" -H "Content-Type: application/zip" \
  --data-binary @app.zip https://panel.example.com/api/apps/$APP_ID/deploy
```

Scripts can be one-liners, multi-line blocks, a path to a `.sh`/`.ps1`/`.cmd` file, or per-OS (`linux:` / `windows:`). See [examples/python-app](examples/python-app) for a working bundle.

## Development

```bash
cd backend && go run ./cmd/moonshed serve --runtime ../.dev
cd frontend && npm install && npm run dev     # http://localhost:5173, proxies /api to :5050
```

```
backend/    Go server (cmd/moonshed)
frontend/   React + Mantine UI
examples/   sample apps
build.sh    builds dist/<os>-<arch>/ ready to copy to a server
```

## Docs

[DESIGN.md](DESIGN.md) explains how it works: the bundle format, runtimes, the deploy flow, the proxy, monitoring, backups and the API.

## Contributing

Issues and pull requests are welcome. Moonshed is early, so please open an issue to discuss bigger changes first.
