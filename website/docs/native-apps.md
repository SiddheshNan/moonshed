# Native apps

A native app runs as a plain process that Moonshed starts and supervises, much like pm2. There is no container, so it uses whatever runtime is installed on the server (Python, Node, a compiled binary and so on).

An app runs natively when it has a `run` command, from `moonshed.yaml` or the **Build** card on Overview, even if it also has a Dockerfile.

## Scripts

There are three scripts, all optional except `run`:

| Script | When it runs | If it fails |
|---|---|---|
| `pre` | On every deploy, in the new release folder, before the app starts. Install dependencies and run migrations here, like `RUN` in a Dockerfile. | The deploy stops. The old version keeps running. |
| `run` | Starts the app. It must stay in the foreground. | Moonshed restarts it. If it keeps crashing during a deploy, the deploy fails. |
| `post` | After the new version is healthy. | Logged as a warning. Nothing is rolled back. |

Dependencies are installed again on every deploy, the same as a Docker build. Use `$CACHE_DIR` to make that fast:

```yaml
pre: .venv/bin/pip install --cache-dir "$CACHE_DIR/pip" -r requirements.txt
```

### Writing scripts

A script can be a single line, a block, or the path to a script file:

```yaml
run: node server.js

pre: |
  npm ci
  npm run build

post: scripts/notify.sh     # .sh, .ps1 and .cmd paths are run as files
```

On Linux scripts run with `sh -e`, so they stop at the first failing line. On Windows they run in PowerShell with `$ErrorActionPreference = 'Stop'`.

If the app runs on both, give each OS its own script:

```yaml
run:
  linux: ./myapp
  windows: myapp.exe
```

If there is no script for the server's OS, the build fails.

### Environment

Every script gets the app's env vars plus:

| Variable | Value |
|---|---|
| `PORT` | The app's port, when it has one |
| `APP_DIR` | The current release folder |
| `SHARED_DIR` | The persistent folder that survives deploys |
| `CACHE_DIR` | A persistent cache folder |
| `APP_NAME` | The app's name |
| `RELEASE_ID` | The ID of the release being run |

## Keeping data across deploys

Each deploy unpacks into a new folder, so anything the app writes there is gone on the next deploy. List the paths to keep under `persist` (folders end with `/`):

```yaml
persist:
  - data/
  - config.ini
```

These paths live in the app's `shared/` folder and are linked into every release with symlinks.

- On the first deploy, if the code contains one of these paths, it is copied into `shared/` once.
- After that `shared/` always wins. A deploy never overwrites persisted data.

## How the process is supervised

- **On boot**, Moonshed starts every app that should be running.
- **Crashes** are restarted with backoff: 1s, 2s, 4s, up to 60s.
- **Stopping** is graceful first, then forced after 10 seconds. On Linux that is SIGTERM then SIGKILL to the process group; on Windows it is CTRL_BREAK, then the Job Object is terminated. Either way every child process is stopped too.
- **Logs** are captured from stdout and stderr and written to daily files. You see them live on the **Logs** tab.

When Moonshed itself restarts, for an upgrade for example, it stops its native apps and starts them again.

## Running as another OS user

By default apps run as the same account as Moonshed: root on Linux, LocalSystem on Windows. To limit what an app can touch, set **Run as OS user** on the **Advanced** tab. The user must already exist on the server.

`pre`, `run` and `post` all run as that user, and the release, `shared/` and cache folders are made writable for them.

- **Linux:** Moonshed starts the process with the user's UID and GID and sets `HOME` and `USER`. Only the username is needed.
- **Windows:** enter the user's password too. It is stored encrypted. Moonshed logs the user on and loads their profile, so `%APPDATA%` and `%USERPROFILE%` work.

::: warning
Native apps are not sandboxed. Without a separate user, an app can read anything Moonshed can.
:::
