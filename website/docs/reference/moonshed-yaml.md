# moonshed.yaml

`moonshed.yaml` sits at the root of your repository or zip, or in the **Folder** set for a git app. Every key is optional. Unknown keys are rejected, so a typo fails the deploy instead of being ignored.

## Full example

```yaml
pre: |                  # Native Process: runs on every deploy, before start
  python -m venv .venv
  .venv/bin/pip install -r requirements.txt
run: .venv/bin/gunicorn app:app --bind 0.0.0.0:$PORT
post: echo "deployed $RELEASE_ID"

port: 8080

persist:
  - data/
  - config.ini

health:
  http: /healthz
  interval: 30s
  timeout: 5s
  failures: 3
  grace: 10s
restart_unhealthy: true

build:                  # Docker apps
  dockerfile: docker/Dockerfile.prod
  folder: api
  stage: runtime
  args:
    NODE_ENV: production
```

## The file and the UI

Every setting here can also be changed in the app's UI. **A value set in the UI wins, then the file, then the default.** The UI marks values that come from the file, and lets you go back to the file for ones you changed.

- Scripts, `persist` and `build` apply on the next deploy.
- `health` and `restart_unhealthy` apply from the next check, without a deploy.
- A `health` or `build` block set in the UI replaces the file's block as a whole. Scripts and `persist` are overridden one by one.
- An app can be configured entirely in the UI, with no file at all.

## Keys

### `pre`

Script run on every deploy in the new release folder, before the app starts. If it fails, the deploy stops and the old version keeps running. Native Process apps only.

### `run`

Starts the app. It must stay in the foreground. Having a `run` command makes the app a Native Process, even if there is also a Dockerfile.

### `post`

Script run after the new version is healthy. A failure is logged as a warning and nothing is rolled back. Native Process apps only.

### Script formats

`pre`, `run` and `post` each take one of:

```yaml
run: python app.py              # one line

pre: |                          # several lines
  npm ci
  npm run build

pre: scripts/setup.sh           # a file ending in .sh, .ps1 or .cmd

run:                            # one script per OS
  linux: ./myapp
  windows: myapp.exe
```

On Linux scripts run with `sh -e`; on Windows in PowerShell with `$ErrorActionPreference = 'Stop'`. If there's no script for the server's OS, the build fails.

### `port`

The port the app listens on, from 1 to 65535. It's only used when the app has no port set in the UI. Moonshed passes it to the app as `$PORT`.

### `persist`

Paths kept across deploys, relative to the app root. End folders with `/`. See [Keeping data across deploys](/docs/native-apps#keeping-data-across-deploys).

### `health`

| Key | Meaning | Default |
|---|---|---|
| `http` | Path to request. Any status below 400 passes. | |
| `tcp` | `true` to only check that the port accepts connections. | |
| `interval` | Time between checks. At least `1s`. | `30s` |
| `timeout` | How long a check may take. | `5s` |
| `failures` | Failed checks in a row before the app is down. | `3` |
| `grace` | Wait after start before the first check. | `10s` |

A `health` block needs either `http` or `tcp`. `health: /healthz` is short for `health: { http: /healthz }`.

Without a `health` block, Moonshed only checks that the process is running, and one failed check means down.

### `restart_unhealthy`

`true` restarts the app when its health check says it's down. Default `false`. A crashed process is restarted either way.

### `build`

For Docker apps. Paths are relative to the repository root.

| Key | Meaning | Default |
|---|---|---|
| `dockerfile` | Path to the Dockerfile. | `<folder>/Dockerfile` |
| `folder` | Build context, what the Dockerfile can `COPY` from. | repository root |
| `stage` | Stage of a multi-stage Dockerfile to build. | last stage |
| `args` | Build arguments, for `ARG` in the Dockerfile. | |

## Environment variables

Every script and container gets the app's env vars plus:

| Variable | Value |
|---|---|
| `PORT` | The app's port, when it has one |
| `APP_NAME` | The app's name |
| `RELEASE_ID` | The ID of the release being run |
| `APP_DIR` | The current release folder. Native Process apps only. |
| `SHARED_DIR` | The persistent folder that survives deploys. Native Process apps only. |
| `CACHE_DIR` | A persistent cache folder, e.g. `pip --cache-dir $CACHE_DIR`. Native Process apps only. |
