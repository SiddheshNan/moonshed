---
layout: home

hero:
  name: Moonshed
  text: A small, self-hosted platform (PaaS) for deploying your apps.
  tagline: Single binary with built-in proxy, and SQLite. Runs on Linux and Windows, even on 1 core and 1 GB of RAM.
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
  - title: One binary
    details: No Postgres, Redis or Traefik. Docker is optional.
  - title: Native Process or Docker
    details: Run each app as a supervised process, like pm2, or as a container.
  - title: Safe deploys
    details: A new version only replaces the old one after its health check passes.
  - title: Domains and HTTPS
    details: Built-in Caddy Reverse Proxy with Let's Encrypt or your own certificates.
  - title: Monitoring
    details: Resource graphs, uptime from health checks, and logs.
  - title: Backups
    details: App and platform backups.
---

<div class="home-section">

## See it deploy

<p class="lead">Deploy a FastAPI app from a git repo, give it a domain, then redeploy. The todos survive because <code>data/</code> is persisted.</p>

<video class="demo" src="/demo/demo.mp4" poster="/demo/demo-poster.jpg" autoplay muted loop playsinline controls></video>

</div>

<div class="home-section">

## One file per app

<p class="lead">Add a <code>moonshed.yaml</code> to your app, or set the same things in the panel. A Dockerfile alone works too.</p>

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

## Install

<p class="lead">Download the release and install the service, then open the setup link it prints.</p>

::: code-group

```bash [Linux]
curl -fsSL https://moonshed.siddhesh.me/install.sh | sh
```

```powershell [Windows]
$tag = (Invoke-RestMethod "https://api.github.com/repos/SiddheshNan/moonshed/releases?per_page=1")[0].tag_name
Invoke-WebRequest "https://github.com/SiddheshNan/moonshed/releases/download/$tag/moonshed-windows-amd64.zip" -OutFile moonshed.zip
Expand-Archive moonshed.zip -DestinationPath C:\
cd C:\moonshed; .\moonshed.exe install -e production
```

:::

[Installation guide →](/docs/installation) · [Read install.sh](https://github.com/SiddheshNan/moonshed/blob/main/website/public/install.sh)

</div>

<div class="home-section">

## Screenshots

<div class="screens">

![Dashboard](/screenshots/dashboard.png)

![App overview](/screenshots/app-overview.png)

![Monitoring](/screenshots/monitoring.png)

![Backups](/screenshots/backups.png)

</div>

</div>
