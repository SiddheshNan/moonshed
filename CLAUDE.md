# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Moonshed: a lightweight self-hosted deploy platform (think Dokploy) shipped as one binary + SQLite, running on Linux and Windows (macOS for local dev only). Apps run either as supervised native processes or as Docker containers. [docs/DESIGN.md](docs/DESIGN.md) is the authoritative spec (bundle format, deploy flow, proxy, metrics tiers, API list); read the relevant section before changing behavior.

## Commands

Go (1.26, module `github.com/SiddheshNan/moonshed` at the repo root):

```bash
go run ./cmd/moonshed serve -e development                     # panel on :5050, runtime is ./runtime/
go test ./...                                                 # only manifest and metrics have tests
go test ./internal/metrics -run TestName                      # single test
go vet ./...
GOOS=windows go build ./...                                   # keep Windows compiling (see platform split below)
```

UI (`ui/`, React + Vite + Mantine, no test runner or linter):

```bash
cd ui && npm install && npm run dev          # the backend forwards its UI to this (ui: in development.yaml)
npm run typecheck                            # tsc --noEmit (also run by `npm run build`)
```

Release build: `scripts/build.py` (interactive) writes the version into `internal/version` and `ui/package.json`, optionally vets/tests, builds the UI and cross-compiles into `dist/<os>-<arch>/{moonshed,runtime/config/production.yaml,runtime/web}`. Builds use `CGO_ENABLED=0`, so dependencies must be pure Go (hence `modernc.org/sqlite`).

Config is `runtime/config/<env>.yaml`, chosen with the required `-e development|production`; every key must be set (`config.Load` rejects unknown or missing ones), including `log_level`. `ui: "/"` serves the built UI from `<runtime>/web/` (not embedded); a URL forwards non-API requests to that dev server.

## Architecture

`cmd/moonshed/main.go` only parses commands and flags and loads config. `internal/server` wires everything by hand (no DI framework); `server.New` opens the databases and builds the components, and `Server.Run` runs inside `service.Run`, which is either a systemd/foreground runner or a Windows service. Startup order matters: `deploy.Manager.Run` boots apps that should be running, then the proxy loads routes, then metrics/health/request-log goroutines start, then the panel listener.

Packages under `internal/`:

- **`api`**: `net/http` ServeMux handlers. Routes are all registered in `server.go` via `s.public` / `s.private` (private = session cookie or `Bearer msh_...` token). Handlers return `error`; `writeError` maps `*apiError`, `store.ErrNotFound`, `deploy.ErrBusy` to status codes. Request bodies use `DisallowUnknownFields`. Path ids must be UUIDs (`pathID`). State-changing handlers record to the audit log via `s.audit` / `s.auditApp` (`audit.go`); env var changes log keys, never values.
- **`deploy`**: `Manager` owns the app lifecycle: a build queue (`concurrent_builds` workers), per-app locks, and the deploy pipeline in `run.go` (unpack → link `persist` paths → build `pre`/`docker build` → stop old → start new → health check → `post`, with rollback to the old release on failure). Sources are zip uploads or git checkouts (`git.go`), which are zipped into `bundles/` so redeploy/rollback always works from a bundle.
- **`engine`**: `Engine` interface (`Start/Stop/Status`) with two implementations: `Native` (child processes, restart with backoff, process-tree kill via `proc`) and `Docker` (Docker Go client + BuildKit, never the `docker` CLI; containers survive platform restarts and are re-attached on boot).
- **`proc`**: OS process-tree handling split by file suffix: `proc_unix.go` (process groups) and `proc_windows.go` (Job Objects). `service` uses the same `_unix`/`_windows` split. When touching these, verify both with `GOOS=windows go build ./...`.
- **`store`**: SQLite (`modernc.org/sqlite`, WAL). Schema is the `migrations` slice in `schema.go`, applied by `PRAGMA user_version`; **append a new step, never edit an existing one**. IDs are UUIDv7 strings (GitHub ids and the audit log counter are the exceptions). Secrets (env vars, passwords, tokens) are AES-GCM encrypted through `secret.Box` using `data/secret.key`.
- **`proxy`**: an embedded Caddy server. `caddyfile.go` generates the Caddyfile from domains, domain options and the proxy settings (deterministic output; unchanged configs aren't reloaded), and raw Caddyfile typed by users is dropped in safe mode if it breaks the config. `modules.go` holds the custom Caddy modules (traffic tracking, the panel handler, the log writer). The panel is always reachable on `panel_addr` independent of the proxy.
- **`metrics` / `health` / `requestlog`**: metrics are aggregated in memory into fixed-size tiers (5s/1m/5m/30m) in `metrics.db`, raw samples never stored. Health checks feed hourly uptime counters plus incidents in `moonshed.db`. Recorded requests live in `requests.db`. Three separate SQLite files on purpose, to avoid write contention.
- **`manifest`**: parses `moonshed.yaml` strictly (unknown keys rejected); scripts can be one-liners, blocks, file paths, or per-OS maps. `Overrides` (stored as `apps.config`) are the same settings changed in the UI; `Effective(file, overrides)` merges them, and `Check()` fills in health-check defaults. A release stores the file as found; deploy and the health monitor always use the merged result.
- **`logger`**: the default `slog` handler, `[time] LEVEL component: msg key=val`, colored on a terminal, also written to `logs/moonshed-<date>.log`. Put the subsystem in a `component` attr. Caddy's JSON logs are filtered in `proxy/modules.go` to warnings, errors and certificate events.
- **`backup`, `archive`, `githost`, `logs`, `config`**: backups (tar.gz of `shared/`, `VACUUM INTO` for the DB, cron schedules), zip/tar helpers, GitHub App integration, daily/size-rotated log files with SSE tailing, and runtime path layout (`config.Paths` / `AppPaths` are the only place that knows the on-disk layout).

UI (`ui/src/`): pages mirror the sidebar (`pages/`, with per-app tabs in `pages/app/` and settings in `pages/settings/`). All server access goes through `api.ts` (`api.get/post/put/delete`, `uploadBundle` via XHR for progress) with TanStack Query; shared API shapes are in `types.ts`, so update it alongside any backend JSON change. UI is neutral zinc with indigo for actions; colour is reserved for state (see Branding in docs/DESIGN.md).

## Project conventions

- Show only real data in the UI and API. No placeholder defaults such as 100% uptime when nothing has been checked; periods without checks are "No data" (see Uptime in docs/DESIGN.md).
- SQLite stores metadata only; log lines live in files under `apps/<app>/logs` and `deploys/`.
- Footprint matters (target is 1 core / 1 GB RAM, ~15 MB RSS for the platform): avoid adding heavy dependencies or per-request allocations on the proxy path.
