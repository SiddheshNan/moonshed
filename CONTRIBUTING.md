# Contributing to Moonshed

Thanks for helping. Moonshed is early, so for anything bigger than a bug fix please open an issue first and describe what you want to change. It saves both of us a rewrite.

## Setup

You need Go 1.26+, Node 20+ and, for release builds, Python 3. Docker is optional and only needed to work on Docker apps.

```bash
go run ./cmd/moonshed serve -e development
cd ui && npm install && npm run dev
```

Open http://localhost:5050 and create the first user. The panel forwards the UI to Vite (`ui:` in `runtime/config/development.yaml`), so UI changes show up live. Everything Moonshed creates at runtime (database, apps, logs, certificates) lands in `runtime/` and is gitignored.

## Layout

```
cmd/moonshed/       entry point: commands and flags
internal/
  server/           wires everything together and runs it
  api/              HTTP handlers for the panel
  deploy/           build queue and the deploy pipeline
  engine/           native process and Docker runtimes
  proxy/            reverse proxy and TLS (Caddy)
  store/            SQLite schema and queries
  metrics/ health/ requestlog/   monitoring
  backup/ githost/ logs/ logger/ config/ manifest/ proc/ service/ secret/ archive/ version/
ui/                 React + Mantine panel
runtime/config/     development.yaml and production.yaml
scripts/build.py    release build
docs/               design doc
website/            website and user docs (VitePress)
examples/           sample apps
```

[docs/DESIGN.md](docs/DESIGN.md) is the spec. Read the relevant section before changing how something behaves, and update it in the same pull request when you do.

## Before you open a pull request

```bash
go vet ./...
go test ./...
GOOS=windows go vet ./...     # Moonshed runs on Windows too
cd ui && npm run typecheck
```

- Keep changes focused; one pull request per fix or feature.
- Match the code around you. Comments explain why, not what, and only where it isn't obvious.
- Platform code is split by file suffix (`_unix.go`, `_windows.go`). Keep both compiling.
- Dependencies must be pure Go (builds use `CGO_ENABLED=0`) and light. Moonshed targets 1 core and 1 GB of RAM.
- Schema changes are a new step appended to `internal/store/schema.go`, never an edit to an existing one.
- When the API's JSON changes, update `ui/src/types.ts` in the same change.

## Releases

`./scripts/build.py` asks for the version and targets, runs the checks and writes a bundle per platform to `dist/`.

By contributing you agree that your work is licensed under the [AGPL-3.0](LICENSE).
