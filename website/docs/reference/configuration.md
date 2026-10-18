# Configuration file

Moonshed reads `runtime/config/<env>.yaml`, where `<env>` is `development` or `production` and is picked with `-e`. Every key must be set, and unknown keys are rejected. Moonshed refuses to start if the file is missing or incomplete.

Most settings live in the panel. This file only holds what Moonshed needs before it can open its database and listen.

```yaml
# The panel is always served here, whatever the reverse proxy is doing.
# Use "127.0.0.1:5050" to keep it off the network and reach it only through its domain.
panel_addr: ":5050"

# Reverse proxy listeners, used once the proxy is turned on in Settings.
http_addr: ":80"
https_addr: ":443"

# Builds that may run at the same time. Keep 1 on small servers.
concurrent_builds: 1

# "/" serves the built UI from runtime/web.
ui: "/"

# debug, info, warn or error.
log_level: "info"
```

## Keys

| Key | Meaning |
|---|---|
| `panel_addr` | Where the panel listens, as `host:port`. Always on, independent of the proxy. |
| `http_addr` | Where the reverse proxy listens for HTTP. |
| `https_addr` | Where the reverse proxy listens for HTTPS. |
| `concurrent_builds` | How many builds can run at once. At least 1. |
| `ui` | `"/"` to serve the built UI from `runtime/web/`. A URL such as `http://localhost:5173` forwards UI requests to a dev server instead, which is only useful when working on Moonshed itself. |
| `log_level` | Lowest level written to the platform log: `debug`, `info`, `warn` or `error`. |

Restart Moonshed after changing this file.
