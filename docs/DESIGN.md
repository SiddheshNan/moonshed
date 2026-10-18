# Moonshed — Design (v1)

Moonshed is a lightweight, self-hosted deploy platform like Dokploy, but:

- **One Go binary + SQLite.** No Postgres, Redis or separate proxy; the reverse proxy (Caddy) is built in, and Docker is optional.
- **Runs on Linux and Windows.**
- **Apps run natively or in Docker**, detected per app from the bundle.
- **Small footprint.** Must run comfortably on 1 core / 1GB RAM.

---

## 1. Concepts

| Term | Meaning |
|---|---|
| **Project** | A flat group of apps. |
| **App** | Something you deploy. Has a name, runtime, port, env vars, domains. |
| **Bundle** | A zip of the app: code, plus `moonshed.yaml` and/or a Dockerfile. Uploaded, or created from a git checkout. |
| **Release** | A bundle unpacked and built into a folder, ready to run. |
| **Build** | Running `pre` for a release. Can be killed. Has its own log. |
| **Deployment** | Stopping the old release and starting the new one, with a health check. |
| **Domain** | A hostname routed by the built-in proxy to an app. |

---

## 2. App config: `moonshed.yaml`

`moonshed.yaml` sits at the root of the zip/repo (or the configured folder). Every key is optional, and unknown keys are rejected.

```yaml
pre: |                  # native: runs on every deploy, before start (install deps, migrations)
  python -m venv .venv
  .venv/bin/pip install -r requirements.txt
run: .venv/bin/gunicorn app:app --bind 0.0.0.0:$PORT    # native: the app itself
post: echo "deployed $RELEASE_ID"                        # native: after the new version is healthy

port: 8080              # used when the app has no port set in the UI

persist:                # kept across deploys (folders end with /)
  - data/
  - config.ini

health:
  http: /healthz        # or: tcp: true. Without a health block, only the process is checked.
  interval: 30s         # default 30s
  timeout: 5s           # default 5s
  failures: 3           # failed checks in a row before the app is down; default 3
  grace: 10s            # wait after start before the first check; default 10s
restart_unhealthy: true # restart the app when it goes down; default false

build:                  # Docker apps
  dockerfile: docker/Dockerfile.prod   # path from the repo root; default <folder>/Dockerfile
  folder: api                          # what the Dockerfile can COPY from; default the repo root
  stage: runtime                       # optional: which stage of a multi-stage Dockerfile to build
  args:                                # optional: build arguments (ARG in the Dockerfile)
    NODE_ENV: production
```

`health: /healthz` is shorthand for `health: { http: /healthz }`.

### The file and the UI

Every setting above can also be set in the app's UI. **A value set in the UI wins, then the file, then the default.** The UI shows "From moonshed.yaml" next to values taken from the file and "Changed here" with a way back to the file on overridden ones. Overrides are stored on the app (`apps.config`); a release keeps the file exactly as found in its bundle, and what runs is that file with the app's overrides on top.

- **Build settings** (scripts, persist, `build`) apply on the next deploy.
- **Health and restart settings** apply on the next check, without a deploy.
- **Blocks replace as a whole:** an overridden `health` or `build` block replaces the file's block; scripts and `persist` override one by one.
- **No file at all** is fine: an app can be configured entirely in the UI.

In the UI, scripts and the runtime live in the **Build** card on Overview, the health check and auto-restart on **Monitoring**, versions to keep on **Deployments**, log retention on **Logs**, the port on Overview, and moving the app to another project behind the pencil next to its name.

### Script values

Each of `pre` / `run` / `post` can be:

- **One-liner:** `run: python app.py`
- **Multi-line block:** `pre: |` + lines
- **File path:** `pre: scripts/setup.sh` (anything ending in `.sh`, `.ps1`, `.cmd` is run as a file)
- **Per OS:**
  ```yaml
  run:
    linux: ./myapp
    windows: myapp.exe
  ```

How they execute:

- **Linux:** `sh -e` (stops at the first failing line).
- **Windows:** PowerShell with `$ErrorActionPreference = 'Stop'`.
- If there is no script for the current OS, the build fails.

### Script stages

| Script | When | On failure |
|---|---|---|
| `pre` | Every deploy, in the new release folder. Install deps, migrations. Like `RUN` in a Dockerfile. | Deploy aborts. Old version keeps running. |
| `run` | Starts the app. Must stay in the foreground (like a Docker entrypoint). | Restarted by the platform. Repeated crashes = deploy failed. |
| `post` | After the new release is healthy. | Logged as a warning. No rollback. |

Dependencies are reinstalled on every deploy, the same as a Docker build. This keeps the platform language-agnostic.

### Environment injected into every script

```
PORT         the app's port (set in the UI, or `port:` in moonshed.yaml)
APP_DIR      current release folder
SHARED_DIR   persistent folder (survives updates)
CACHE_DIR    persistent cache, optional for users (e.g. pip --cache-dir $CACHE_DIR)
APP_NAME
RELEASE_ID
+ env vars set in the UI
```

### `persist`

- Listed paths live in `shared/` and are linked into every release:
  - **Linux:** symlinks.
  - **Windows:** symlinks too. Creating them needs admin rights, which the platform has when it runs as a service (LocalSystem).
- **First deploy:** if the zip contains the path, it is copied into `shared/` once.
- **After that:** `shared/` always wins. A deploy never overwrites persisted data.

### Docker apps

For an app that runs in Docker:

- **Dockerfile:** `Dockerfile` at the root, or wherever `build.dockerfile` / `build.folder` point.
- **Scripts:** `pre` / `run` / `post` are not used. The Dockerfile's `RUN` and `CMD`/`ENTRYPOINT` replace them.
- **Build:** BuildKit through the Docker API → image `moonshed/<app>:<release-id>`, with the configured folder, stage and build args. The output goes to the build log, and the build can be killed like a native one.
- **Run:** a container created through the Docker API with:
  - the env vars from the UI, plus `PORT`;
  - port `-p <port>:<port>`;
  - `persist` paths bind-mounted from `shared/`.
- **`moonshed.yaml` is optional.** `persist`, `health`, `restart_unhealthy`, `port` and `build` are read from it.
- **Docker must be installed.** If Docker is missing at deploy time, the build fails with a clear error.

---

## 3. Runtimes (detected per app)

| Runtime | Linux | Windows |
|---|---|---|
| **Native** | Child process of the platform, in its own process group | Child process of the platform, in a Job Object |
| **Docker** | Docker Engine API (unix socket) | Docker Engine API (named pipe) |

The runtime is detected on the first deploy: a `run` command (from the file or the UI) runs natively, even if a Dockerfile is also present; otherwise a Dockerfile runs in Docker. It can be picked in the Build card. Switching a deployed app stops it and drops its release, since the other engine can't run it; the next deploy builds it for the new runtime.

### Native: the platform is the supervisor

The platform runs as a system service (systemd on Linux, Windows service on Windows) and starts on boot. It supervises native apps directly, like pm2:

- **On platform start:** start every app that should be running.
- **Running `run`:** with the app's env, working dir = the release folder.
- **Process tree:** the whole tree is grouped so stop/kill gets every child:
  - **Linux:** a process group (`Setpgid`).
  - **Windows:** a Job Object.
- **Logs:** stdout/stderr are captured through pipes and written to the app's log files.
- **Crash:** restart with backoff (1s, 2s, 4s… max 60s).
- **Stop:** graceful first, then forced after a timeout (default 10s):
  - **Linux:** SIGTERM to the group, then SIGKILL.
  - **Windows:** CTRL_BREAK, then terminate the Job Object.
- **Metrics:** per-app CPU/RAM is summed over the process tree (gopsutil, same code on both OSes).
- **Windows graceful stop:** the platform gives itself a hidden console at startup so `CTRL_BREAK` reaches the app's process group even when running as a service.

When the platform itself restarts (crash or upgrade), it stops its apps and starts them again. This is the same as Docker's default behaviour when its daemon restarts.

### Run as OS user (native apps)

- **Setting:** per app, optional. If empty, the app runs as the platform's service account.
- **Which scripts:** `pre`, `run` and `post` all run as that user, so files created during the build have the right owner.
- **The user must already exist on the machine.** A wrong user or password fails the deploy (or start) with a clear error.
- **Linux:**
  - The platform runs as root and starts the process with that user's UID/GID (`SysProcAttr.Credential`).
  - `releases/<id>/`, `shared/` and `cache/` are chowned to that user.
  - `HOME` and `USER` are set for the user.
- **Windows:**
  - **Credentials:** the user's password is entered in the UI and stored encrypted, the same as env vars.
  - **Starting the process:** the platform (LocalSystem) logs the user on (`LogonUser`), loads their profile (`LoadUserProfile`) so `%APPDATA%`, `%USERPROFILE%` etc. work, then starts the process with `CreateProcessAsUser`. The process still goes into the app's Job Object.
  - **Folder access:** the user is given Modify rights on `releases/<id>/`, `shared/` and `cache/`.
- **Logs:** log files stay owned by the platform, because it writes them from the pipes.
- **Docker apps:** this setting is not available. The container's user comes from the Dockerfile.

### Docker

- **Image:** `moonshed/<app>:<release-id>`.
- **Port:** the app's port is published on the host.
- **Restart policy:** `unless-stopped`.
- **Everything goes through Go libraries, never the `docker` command:** the official Docker client (`github.com/moby/moby/client`) for containers, logs, stats and images, and BuildKit (`github.com/moby/buildkit`) for builds, so modern Dockerfile features like `RUN --mount=type=cache` work.
- **Engines without BuildKit** (e.g. a Windows-containers daemon) fall back to the API's classic builder.
- **Logs and stats** are streamed per container and reconnect when the container restarts.
- **Containers survive platform restarts.** On startup the platform re-attaches to running containers instead of recreating them.
- **Env vars** are passed in the create request, never written to disk.

---

## 4. Folder layout

Everything lives in a `runtime/` folder next to the binary (override with `--runtime DIR`). In the repo, `runtime/` at the root is the dev runtime; only its `config/` is committed.

```
moonshed(.exe)
runtime/
  config/<env>.yaml      development.yaml / production.yaml, picked with the required -e flag
  data/
    moonshed.db          main SQLite DB (WAL mode)
    metrics.db           metrics tiers (separate to avoid write contention)
    requests.db          recorded requests
    secret.key           key used to encrypt env vars and passwords at rest
  apps/<app>/
    bundles/<id>.zip     last X bundles (default 5, configurable per app)
    releases/<id>/       unpacked + built release (current, plus the new one during a deploy)
    shared/              persistent data (persist paths)
    cache/               $CACHE_DIR
    logs/app-<date>.log  runtime logs, rotated daily + at 50MB
    deploys/<id>.log     deployment logs
  certs/
    acme/                Let's Encrypt account + certificates
    custom/<host>.crt    uploaded certificates (+ .key)
  backups/
    apps/<app>/          app backups (.tar.gz)
    platform/            platform DB backups (.tar.gz)
  logs/moonshed-<date>.log   the platform log
  web/                   the built UI
```

`config/production.yaml` (every key is required; unknown keys are rejected):

```yaml
panel_addr: ":5050"     # panel only, always on, independent of the proxy
http_addr: ":80"        # reverse proxy
https_addr: ":443"      # reverse proxy
concurrent_builds: 1
ui: "/"                 # built UI from runtime/web; a URL (http://localhost:5173) forwards to a dev server
log_level: "info"       # debug, info, warn or error
```

- **Bundles:** every deploy stores its bundle as a zip, including git deploys (the checkout is zipped). Only the last X are kept.
- **Release folders:** the old release folder is deleted after a successful deploy.
- **Rollback:** pick an older bundle and redeploy it (runs `pre` again).

---

## 5. Deploy flow

Each app has one fixed port. A deploy causes a short gap while the old version stops and the new one starts. This is by design for v1.

1. **Source:** a zip upload, or a git clone (webhook / manual). The bundle is stored in `bundles/`.
2. **Unpack** into `releases/<id>/`, read `moonshed.yaml`, merge the app's UI settings on top, detect the runtime on the first deploy, and take the port from the file if the app has none.
3. **Link** `persist` paths from `shared/`.
4. **Build:** run `pre` (or `docker build`). Output streams to the build log. The build can be killed. The old version keeps running during the build.
5. **Stop** the old release.
6. **Start** the new release on the app's port.
7. **Health check** until healthy or timeout. If there's no `health` section, the check is "process still alive after 5s".
8. **On success:** run `post`, mark the deployment successful, delete the old release folder, prune old bundles.
9. **On failure:** stop the new release, start the old release again, and mark the deployment failed.

### Builds

- **Queue:** one build at a time by default (small servers). Concurrency is configurable.
- **Kill build:** cancel the context, then kill the whole process tree (process group / Job Object).
- **States:** `queued → running → success | failed | killed`.

---

## 6. Logs

- **App logs:** files per app per day, also rotated by size. Retention is set in days, globally or per app.
- **Build logs:** one file per build, kept as long as the build record.
- **Live tail in the UI:** Server-Sent Events. A new stream starts with the tail of the newest log file, so it isn't empty after a restart.
- **SQLite stores only metadata** (build/deployment rows, log file paths, exit codes), never log lines.
- **Platform log:** one line per event, `[time] LEVEL component: message key=value`, on the console (colored in a terminal) and in `logs/moonshed-<date>.log`, rotated like app logs and kept 30 days. `log_level` in the config sets the minimum level. Caddy's own logs are reduced to warnings, errors and certificate events.

---

## 7. Reverse proxy, domains and TLS

The proxy is an embedded Caddy server. Moonshed generates its Caddyfile from the database, so Caddy handles HTTP/2, HTTP/3, WebSockets and certificates while Moonshed stays the only thing to configure.

- **Turning it on:** the user decides whether the proxy is enabled (Settings › Proxy). When it's off, apps are reached directly on their port.
- **Domains:** each app can have one or more domains (like Dokploy), each with a path prefix (default `/`). The proxy forwards to `127.0.0.1:<app port>`; apps without a port get no routes.
- **TLS options per domain:**
  - **Let's Encrypt:** automatic issue and renewal (HTTP-01).
  - **Custom cert:** uploaded per domain (cert + key as PEM, or a PFX), checked to actually cover the host and not be expired. Stored per host, so paths on the same host share it. Removed automatically when no domain uses it any more.
  - **None:** plain HTTP.
- **Domain options:** force HTTPS (on by default), HSTS, www redirect, compression, strip the path prefix, basic auth, IP allow list, request/response header rules and a max body size. Anything else can be added as raw Caddyfile directives per domain ("advanced").
- **Global options:** trusted proxies, raw Caddy global options and extra site blocks, all in Settings › Proxy. The generated Caddyfile can be viewed there.
- **Safety:** every change is validated before it's saved. If the running config fails only because of raw Caddyfile someone typed, the proxy runs without those parts and shows the error, so a bad snippet can't take every app offline.
- **Reloads:** an unchanged config is not reloaded, since a reload closes idle keep-alive connections.
- **Panel:** always reachable on `panel_addr` (default `:5050`, plain HTTP), even if the proxy is off or misconfigured. Bind it to `127.0.0.1:5050` to keep it off the network. Optionally it can also get a domain through the proxy (with Let's Encrypt, a custom certificate or plain HTTP); both ways keep working.

---

## 8. Monitoring

### Metrics (graphs)

- **Server:** CPU %, RAM, disk, network in/out, load. Collected with `gopsutil`.
- **Per app:** CPU %, RAM, process count. Nothing is recorded while an app isn't running.
- **Per domain (from the proxy):** requests/sec, 4xx/5xx per second, average response time.
- **Sampled every 5s and aggregated in memory into fixed-size tiers.** Raw samples are never stored. Each bucket keeps sum, min, max and count, so a coarser bucket is exact (weighted average, max of maxes); counters are shown as per-second rates.

| Tier | Kept for | Chart ranges |
|---|---|---|
| 5s | 15 min | 1m, 5m, 15m |
| 1m | 12 h | 30m, 1h, 6h, 12h |
| 5m | 48 h | 24h, 48h |
| 30m | 7 days | 7d |

- **Storage is fixed:** about 2,000 rows per app or server, ever. Stored in `metrics.db`.
- **Charts** draw the bucket average, with the bucket peak as a dashed line.

### Uptime

- **Source:** the app's health check (UI, else `moonshed.yaml`), run every `interval` (default 30s), after the `grace` period that follows each start. Without one, the check is "process is running", every 30s.
- **After a successful deploy:** one check runs 5s later (if a health check is configured), so the status is known right away.
- **Storage:** each check adds to the app's row for the current hour: passed and total. Individual checks are not stored. Kept 7 days.
- **Uptime % = passed ÷ total checks.** Hours with no checks (not deployed, stopped, platform offline) are "No data", never counted as up.
- **UI:** 24h (24 hourly bars) and 7d (7 daily bars), the current state (up / down / not checked yet), and incidents.
- **Down:** `failures` consecutive failed checks (default 3; 1 for the process check) opens an incident; the next success closes it.
- **Optional auto-restart:** `restart_unhealthy` restarts an app that is running but down. A crashed process is restarted by its engine regardless.

---

## 9. Git and CI/CD

An app's source is a **zip upload** (always available) or a **git repository**.

**Git sources:**
- **GitHub App:** Settings › Git › Connect GitHub creates a private GitHub App through GitHub's manifest flow, then you install it on accounts or repos. Clones use short-lived installation tokens, so private repos need no personal token, and apps get a repo and branch picker. All pushes arrive at one webhook, `POST /api/hooks/github`, verified with the app's webhook secret. GitHub must reach the panel (its public domain) for webhooks.
- **Git URL + token:** for GitLab, Bitbucket, Gitea or anything else. Tokens are saved once as named credentials in Settings › Git and picked per app. Public repos need none.
- **Autodeploy (per app):** on, every push to the branch deploys; off, the push is shown on the app ("new commit waiting") and you press Deploy.

**Hooks for a single app**, `POST /api/hooks/apps/<id>`:
- GitHub push webhooks signed with the app's secret (`X-Hub-Signature-256`).
- GitLab push webhooks with the secret as `X-Gitlab-Token`.
- `?token=<secret>` from any CI, which always deploys.

**Push from CI:** `POST /api/apps/<id>/deploy` with an API token and the zip. This is how PyInstaller and other compiled apps get deployed: CI builds on the right OS and uploads the result.

Deployments record the commit and its message.

---

## Requests

- **Recording is a session.** Pick apps on the Requests page and start; apps can be added or removed while it runs.
- **Stored per request:** time, method, host, path, query, status, duration, bytes, client IP, user agent, referer. No bodies.
- **Per-app cap of 100,000 requests;** the oldest are dropped. There is no time-based retention.
- **Stopping deletes everything recorded,** and removing an app from the session deletes that app's requests.
- Stored in `requests.db`, written in batches so the proxy is never slowed down.

## Projects

Flat groups of apps. None exist until you create one, and every app must be created in a project. Only empty projects can be deleted.

## Audit log

Every change is recorded with who did it (user, API token, or webhook), when, from which IP, the action, the target and a short detail. Env var changes list the keys, never values. Failed logins are recorded too. Kept for one year.

---

## 10. Backups

**App backups** pick any of four parts, each a switch: **Code** (the deployed version), **Data** (the persisted paths), **Env vars** (the Environment tab) and **Logs** (app logs plus the deploy log). The archive unpacks to a folder that looks like the app itself:

```
myapp_2026-10-18_03-00-00.tar.gz
└── myapp/
    ├── app.py, moonshed.yaml …   Code: exactly as deployed
    ├── data/, config.ini         Data: each persisted path in its real place
    ├── .env                      Env vars, as KEY=value (".env.moonshed" if the code has its own .env)
    └── .moonshed/
        ├── logs/                 Logs
        └── backup.json           what's included, release, commit, time
```

- **Back up now** asks which parts to include. A **schedule** has an on/off switch; when on, it needs a cron expression and its own choice of parts, and keeps the last N backups.
- **Restore** offers the parts the backup has: Data replaces the persisted paths (the app is stopped meanwhile), Env vars replace the Environment tab, and Code is deployed as a new deployment (trigger "restore"), even if that version is no longer among the kept versions. Logs are for download only.
- **Downloads** that include env vars warn that the `.env` is plain text.

**Platform backups** hold the database (`VACUUM INTO`) and the key that decrypts it, on their own schedule (same on/off switch) or on demand. Restore by stopping Moonshed and extracting the archive into `runtime/data/`.

Backups live in `runtime/backups/`.

---

## 11. Auth and security

- **Admin user:** created on first run. Login uses a session cookie that lasts 30 days.
- **Login throttling:** 10 failed logins from one IP within 15 minutes blocks further attempts from it until the window passes.
- **Other users:** local accounts. All users are admins.
- **API tokens:** for CI / push deploys, prefixed `msh_`. Only a hash is stored.
- **Env vars:** encrypted at rest with AES-GCM using `secret.key`.
- **Isolation:** native apps are not sandboxed. By default they run as the platform's service account. Set "run as OS user" per app (section 3) to limit what an app can touch.

---

## 12. Data model (SQLite, main DB)

IDs are UUIDv7 strings, except GitHub App and installation IDs (GitHub's own numbers) and the audit log's `seq` (used only for paging). A database from an older development build is refused at startup; delete `runtime/data/` to start fresh.

```
users          (id, username, password_hash, created_at)
sessions       (token_hash, user_id, expires_at)
api_tokens     (id, user_id, name, token_hash, last_used_at, created_at)
settings       (key, value)     proxy_enabled, acme_email, panel_domain, panel_tls, proxy_trusted_proxies,
                                proxy_global_options, proxy_extra_sites, backup_enabled, backup_cron, backup_keep

apps           (id, name, runtime[''|native|docker], port, source[upload|git],
                git_url, git_branch, git_subdir, git_token (encrypted), webhook_secret,
                run_as_user, run_as_password (encrypted, Windows only),
                keep_bundles, log_retention_days, backup_enabled, backup_cron, backup_keep, backup_parts,
                config (JSON: settings changed in the UI on top of moonshed.yaml),
                current_release_id, desired_state[running|stopped], created_at)
env_vars       (app_id, key, value (encrypted))
domains        (id, app_id, host, path, tls[letsencrypt|custom|none],
                options (JSON: force_https, hsts, basic_auth, headers…), advanced (raw Caddyfile))

bundles        (id, app_id, source[upload|git], git_commit, size, created_at)
releases       (id, app_id, bundle_id, manifest (the bundle's moonshed.yaml as JSON), image, created_at)
deployments    (id, app_id, bundle_id, release_id, trigger, status, error,
                created_at, started_at, finished_at)
                trigger: upload | api | manual | github_app | github_webhook | gitlab_webhook | deploy_hook | redeploy
                status: queued → building → starting → success | failed | killed

incidents      (id, app_id, down_at, up_at, reason)
uptime         (app_id, hour, ok, total)                      one row per app per hour, 7 days
backups        (id, app_id (NULL = platform), file, size, status, error, parts, created_at)
projects       (id, name, description, created_at)
audit_log      (id, ts, actor_type, actor_id, actor_name, ip, action, target_type, target_id, target_name, details)
git_credentials      (id, name, host, username, token (encrypted), created_at)
github_apps          (id = GitHub app id, slug, name, owner, html_url, client_id, client_secret, private_key, webhook_secret)
github_installations (id = GitHub installation id, github_app_id, account, account_type, created_at)
```

Apps also have `project_id`, `git_credential_id`, `github_installation_id`, `autodeploy`, `pushed_commit`, `pushed_message` and `record_requests`. Deployments have `git_commit` and `git_message`.

A "build" is the building phase of a deployment, so there is no separate builds table; killing a build kills the deployment.

`metrics.db`: `targets` (server, app or domain) and `points` (target, metric, tier, bucket time, sum, min, max, count), one fixed-size set of buckets per tier (section 8).

`requests.db`: `requests`, one row per recorded request (section "Requests").

---

## 13. API outline

```
GET    /api/auth/status              POST /api/auth/setup | /login | /logout
GET    /api/users                    POST /api/users
PUT    /api/users/:id/password       DELETE /api/users/:id
GET    /api/tokens                   POST /api/tokens       DELETE /api/tokens/:id

GET    /api/apps                     POST /api/apps
GET    /api/apps/:id                 PUT/DELETE /api/apps/:id
POST   /api/apps/:id/start|stop|restart
POST   /api/apps/:id/deploy          multipart "bundle", raw application/zip body, or empty (git)
GET    /api/apps/:id/deployments     GET /api/deployments/:id
POST   /api/deployments/:id/kill     GET /api/deployments/:id/log   (SSE)
GET    /api/apps/:id/bundles         POST /api/apps/:id/bundles/:bundle/redeploy   (rollback)
GET/PUT /api/apps/:id/env
GET/POST /api/apps/:id/domains       PUT/DELETE /api/domains/:id     (custom TLS: multipart with cert/key or pfx)
PUT    /api/domains/:id/certificate  POST /api/domains/:id/check     (validate a draft without saving)
GET    /api/apps/:id/logs?date=      GET /api/apps/:id/logs/days   GET /api/apps/:id/logs/stream (SSE)
GET    /api/apps/:id/metrics?range=  GET /api/apps/:id/uptime?range=
GET/POST /api/apps/:id/backups       GET/POST /api/platform/backups
POST   /api/backups/:id/restore      GET /api/backups/:id/download   DELETE /api/backups/:id
GET    /api/server                   GET /api/server/metrics?range=
GET/PUT /api/settings                POST /api/settings/panel-certificate
POST   /api/settings/proxy-check     GET /api/proxy/caddyfile
GET/POST /api/projects               GET/PUT/DELETE /api/projects/:id
GET    /api/deployments              (all apps; ?status=&before=&limit=)
GET    /api/audit                    (?action=&actor=&before=&limit=)
GET    /api/requests                 GET/PUT /api/requests/recording
GET    /api/git                      POST /api/git/credentials    PUT/DELETE /api/git/credentials/:id
POST   /api/git/github/manifest      GET /api/git/github/callback   GET /api/git/github/setup
POST   /api/git/github/:id/sync      DELETE /api/git/github/:id
GET    /api/git/installations/:id/repos    GET /api/git/installations/:id/branches?repo=
POST   /api/hooks/github             (GitHub App webhooks; no login, signed)
POST   /api/hooks/apps/:app_id       (per-app: GitHub/GitLab push or ?token=; no login)
```

Auth is a session cookie for the UI, or `Authorization: Bearer msh_...` for API tokens.

---

## 14. Install and CLI

```
moonshed serve -e <env>             run the platform (normally run by the service)
moonshed install -e <env>           register a systemd unit / Windows service (auto-start on boot)
moonshed uninstall
moonshed reset-password <user>
moonshed version
```

- **`-e` (or `ENV`):** `development` or `production`, selecting `runtime/config/<env>.yaml`. Required for `serve` and `install`; Moonshed refuses to start when the file is missing, has unknown keys or leaves one out. `install` records it in the service definition.
- **`--runtime` (or `RUNTIME_DIR`):** the runtime folder. Defaults to `runtime/` next to the binary, then `./runtime`.
- **Install:** copy a release bundle (binary + `runtime/config/production.yaml` + `runtime/web/`), run `moonshed install -e production`, open the UI.
- **Upgrade:** replace the binary and `runtime/web/`, then restart the service. Apps restart with it.
- **Release builds:** `scripts/build.py` writes the version, optionally vets and tests, builds the UI and cross-compiles one bundle per platform into `dist/<os>-<arch>/`.
- **Development:** `development.yaml` sets `ui` to the Vite dev server, so the panel forwards every non-API request to it and UI changes show up live, on `:5050` and on the panel's domain alike.

---

## 15. Tech stack

| Need | Choice |
|---|---|
| Language | Go |
| DB | SQLite via `modernc.org/sqlite` (pure Go, no CGO, easy Windows builds) |
| Config | `gopkg.in/yaml.v3`, strict decoding |
| Reverse proxy / TLS / ACME | Embedded Caddy v2 (Caddyfile generated from the DB) |
| Logging | `log/slog` with Moonshed's own handler (`internal/logger`) |
| Metrics | `gopsutil` |
| Windows service / Job Objects | `golang.org/x/sys/windows`, `golang.org/x/sys/windows/svc` |
| Git | `go-git` |
| Docker | Official Docker Go client + BuildKit client |
| Cron | `robfig/cron` |
| UI | React + Vite + Mantine (`ui/`), served from `runtime/web/`. Sidebar: Dashboard, Projects, Apps, Deployments, Requests; Settings: General, Proxy, Git, Certificates, Users, API tokens, Audit log; ⌘K search |

Target memory use: under 50MB RSS for the platform itself (measured ~15MB with 3 apps).

---

## 16. Platform support and license

- **Linux / Windows:** fully supported.
- **macOS:** builds and runs for local testing only. Not supported for production.
- **License:** AGPL-3.0. The repository layout and contribution rules are in [CONTRIBUTING.md](../CONTRIBUTING.md).

---

## 17. Next version (not in v1)

- Notifications (down/up, deploy result, resource thresholds, backup failure).
- Zero-downtime deploys (start new on a temp port, health check, switch proxy, stop old).
- Multi-server (same binary in agent mode).
- Resource limits per app (memory/CPU caps).

---

## Branding

Moonshed is a gold moon rising behind a small shed with one lit window. The mark is `assets/logo.svg`; the same SVG is the UI logo (`Logo` in `ui/src/components/Layout.tsx`) and the favicon.

| Token | Value | Use |
|---|---|---|
| Moon gold | `#f5b02a` | The moon |
| Lantern | `#ffd35e` | The shed window |
| Night ink | `#090c16` | Logo background |
| Shed blue | `#2c3563` | The shed |

The UI itself is neutral (zinc greys) with indigo for actions, so the only colour on screen is meaning: indigo for actions, green / orange / red for state.
