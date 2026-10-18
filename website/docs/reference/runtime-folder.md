# Runtime folder

Everything Moonshed stores lives in one folder, `runtime/` next to the binary unless you pass `--runtime`. Back up this folder and you have everything.

```
moonshed(.exe)
runtime/
  config/<env>.yaml          the configuration file
  data/
    moonshed.db              main database
    metrics.db               metrics
    requests.db              recorded requests
    secret.key               encrypts env vars and passwords
  apps/<app>/
    bundles/<id>.zip         the last few deployed versions
    releases/<id>/           the running release, plus the new one during a deploy
    shared/                  persisted paths
    cache/                   $CACHE_DIR
    logs/app-<date>.log      app logs, rotated daily and at 50 MB
    deploys/<id>.log         deployment logs
  certs/
    acme/                    Let's Encrypt account and certificates
    custom/<host>.crt        uploaded certificates (and .key)
  backups/
    apps/<app>/              app backups
    platform/                platform backups
  logs/moonshed-<date>.log   the platform log, kept 30 days
  web/                       the UI
```

Metrics and recorded requests are kept in their own database files so their writes don't slow down the main one. The databases hold metadata only; log lines are always in files.

## The platform log

Moonshed logs one line per event:

```
[2026-10-19 14:02:11.482] INFO proxy: listening http=:80 https=:443
```

It goes to the console, colored in a terminal, and to `logs/moonshed-<date>.log`. On Linux you can also follow it with `journalctl -u moonshed -f`. Set the level with `log_level` in the [configuration file](/docs/reference/configuration).
