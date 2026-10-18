---
layout: home

hero:
  name: Moonshed
  text: A small, self-hosted platform (PaaS) for deploying your apps.
  tagline: One Go binary and SQLite. Runs on Linux and Windows, comfortably on 1 core and 1 GB of RAM.
  image:
    src: /logo.svg
    alt: Moonshed
  actions:
    - theme: brand
      text: Get started
      link: /docs/
    - theme: alt
      text: moonshed.yaml reference
      link: /docs/reference/moonshed-yaml
    - theme: alt
      text: GitHub
      link: https://github.com/SiddheshNan/moonshed

features:
  - title: One binary, one database
    details: No Postgres, Redis or Traefik. The reverse proxy is built in and Docker is optional.
  - title: Native or Docker, per app
    details: Run an app as a supervised process, like pm2, or as a container built with BuildKit.
  - title: Safe rollouts
    details: A new version has to pass its health check before it replaces the old one. Any kept version can be redeployed.
  - title: Domains and HTTPS
    details: An embedded Caddy server routes your domains, with Let's Encrypt or your own certificates.
  - title: Monitoring from real checks
    details: CPU, memory, disk and network graphs, uptime and incidents from health checks, and a request log.
  - title: Backups you can read
    details: An app backup unpacks to a plain copy of the app, its data and its env vars. Restore it part by part.
---

<div class="home-section">

## Describe your app in one file

<p class="lead">Put a <code>moonshed.yaml</code> next to your code, or set the same things in the panel. A Dockerfile on its own works too.</p>

```yaml
pre: |
  python3 -m venv .venv
  .venv/bin/pip install -r requirements.txt
run: .venv/bin/python app.py
port: 8000
persist:
  - data/
health:
  http: /healthz
restart_unhealthy: true
```

</div>

<div class="home-section">

## Install it on your server

<p class="lead">Build a bundle, copy it over and register the service. Then open the panel on port 5050.</p>

::: code-group

```bash [Linux]
./scripts/build.py
sudo ./moonshed install -e production
```

```powershell [Windows]
python scripts/build.py
.\moonshed.exe install -e production
```

:::

[Read the installation guide →](/docs/installation)

</div>

<div class="home-section">

## A look inside

<div class="screens">

![Dashboard](/screenshots/dashboard.png)

![App overview](/screenshots/app-overview.png)

![Monitoring](/screenshots/monitoring.png)

![Backups](/screenshots/backups.png)

</div>

</div>
