# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Moonshed: a lightweight self-hosted deploy platform (think Dokploy) shipped as one Go binary + SQLite, running on Linux and Windows (macOS for local dev only). Apps run either as supervised native processes or as Docker containers. [DESIGN.md](DESIGN.md) is the authoritative spec (bundle format, deploy flow, proxy, metrics tiers, API list); read the relevant section before changing behavior.

## Commands

Backend (Go 1.26, `backend/`, module name `moonshed`):

```bash
cd backend && go run ./cmd/moonshed serve --runtime ../.dev   # panel on :5050; .dev/ is the gitignored dev runtime dir
go test ./...                                                 # only manifest and metrics have tests
go test ./internal/metrics -run TestName                      # single test
go vet ./...
GOOS=windows go build ./...                                   # keep Windows compiling (see platform split below)
```

Frontend (`frontend/`, React + Vite + Mantine, no test runner or linter):

```bash
cd frontend && npm install && npm run dev    # http://localhost:5173, proxies /api to 127.0.0.1:5050
npm run typecheck                            # tsc --noEmit (also run by `npm run build`)
```

Release build: `./build.sh` builds the UI then cross-compiles into `dist/<os>-<arch>/{moonshed,runtime/web}`. Override with `TARGETS="linux/amd64 windows/amd64"` and `VERSION=...`. Builds use `CGO_ENABLED=0`, so dependencies must be pure Go (hence `modernc.org/sqlite`).

The binary serves the UI from `<runtime>/web/`, not embedded. In dev, either use the Vite server or copy `frontend/dist` into the runtime's `web/`.

## Architecture

`cmd/moonshed/main.go` wires everything by hand (no DI framework) and runs it inside `service.Run`, which is either a systemd/foreground runner or a Windows service. Startup order matters: `deploy.Manager.Run` boots apps that should be running, then the proxy loads routes, then metrics/health/request-log goroutines start, then the panel listener.

Packages under `backend/internal/`:

- **`api`**: `net/http` ServeMux handlers. Routes are all registered in `server.go` via `s.public` / `s.private` (private = session cookie or `Bearer msh_...` token). Handlers return `error`; `writeError` maps `*apiError`, `store.ErrNotFound`, `deploy.ErrBusy` to status codes. Request bodies use `DisallowUnknownFields`. Path ids must be UUIDs (`pathID`). State-changing handlers record to the audit log via `s.audit` / `s.auditApp` (`audit.go`); env var changes log keys, never values.
- **`deploy`**: `Manager` owns the app lifecycle: a build queue (`concurrent_builds` workers), per-app locks, and the deploy pipeline in `run.go` (unpack → link `persist` paths → build `pre`/`docker build` → stop old → start new → health check → `post`, with rollback to the old release on failure). Sources are zip uploads or git checkouts (`git.go`), which are zipped into `bundles/` so redeploy/rollback always works from a bundle.
- **`engine`**: `Engine` interface (`Start/Stop/Status`) with two implementations: `Native` (child processes, restart with backoff, process-tree kill via `proc`) and `Docker` (Docker Go client + BuildKit, never the `docker` CLI; containers survive platform restarts and are re-attached on boot).
- **`proc`**: OS process-tree handling split by file suffix: `proc_unix.go` (process groups) and `proc_windows.go` (Job Objects). `service` uses the same `_unix`/`_windows` split. When touching these, verify both with `GOOS=windows go build ./...`.
- **`store`**: SQLite (`modernc.org/sqlite`, WAL). Schema is the `migrations` slice in `schema.go`, applied by `PRAGMA user_version`; **append a new step, never edit an existing one**. IDs are UUIDv7 strings (GitHub ids and the audit log counter are the exceptions). Secrets (env vars, passwords, tokens) are AES-GCM encrypted through `secret.Box` using `data/secret.key`.
- **`proxy`**: built-in reverse proxy (`httputil.ReverseProxy` + certmagic) with in-memory routes swapped atomically; the panel is always reachable on `panel_addr` independent of the proxy.
- **`metrics` / `health` / `requestlog`**: metrics are aggregated in memory into fixed-size tiers (5s/1m/5m/30m) in `metrics.db`, raw samples never stored. Health checks feed hourly uptime counters plus incidents in `moonshed.db`. Recorded requests live in `requests.db`. Three separate SQLite files on purpose, to avoid write contention.
- **`manifest`**: parses `app.yaml` strictly (unknown keys rejected); scripts can be one-liners, blocks, file paths, or per-OS maps.
- **`backup`, `archive`, `githost`, `logs`, `config`**: backups (tar.gz of `shared/`, `VACUUM INTO` for the DB, cron schedules), zip/tar helpers, GitHub App integration, daily/size-rotated log files with SSE tailing, and runtime path layout (`config.Paths` / `AppPaths` are the only place that knows the on-disk layout).

Frontend (`frontend/src/`): pages mirror the sidebar (`pages/`, with per-app tabs in `pages/app/` and settings in `pages/settings/`). All server access goes through `api.ts` (`api.get/post/put/delete`, `uploadBundle` via XHR for progress) with TanStack Query; shared API shapes are in `types.ts`, so update it alongside any backend JSON change. UI is neutral zinc with indigo for actions; colour is reserved for state (see Branding in DESIGN.md).

## Project conventions

- Show only real data in the UI and API. No placeholder defaults such as 100% uptime when nothing has been checked; periods without checks are "No data" (see Uptime in DESIGN.md).
- SQLite stores metadata only; log lines live in files under `apps/<app>/logs` and `deploys/`.
- Footprint matters (target is 1 core / 1 GB RAM, ~15 MB RSS for the platform): avoid adding heavy dependencies or per-request allocations on the proxy path.
