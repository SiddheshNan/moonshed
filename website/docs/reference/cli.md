# CLI

```
moonshed <command> [-e ENV] [--runtime DIR]
```

## Commands

| Command | What it does |
|---|---|
| `serve` | Runs Moonshed in the foreground. The service runs this for you. |
| `install` | Registers Moonshed as a systemd unit (Linux) or a Windows service, and starts it. Needs root or Administrator. |
| `uninstall` | Stops and removes the service. The runtime folder is kept. |
| `setup-link` | Prints the link to create the first admin. After setup it prints the panel address and exits with 1. |
| `reset-password <user>` | Sets and prints a new random password for a user. |
| `version` | Prints the version. |

## Flags

| Flag | Environment variable | Meaning |
|---|---|---|
| `-e ENV` | `ENV` | `development` or `production`. Picks `runtime/config/<ENV>.yaml`. Required for `serve`, `install` and `setup-link`. |
| `--runtime DIR` | `RUNTIME_DIR` | The [runtime folder](/docs/reference/runtime-folder). Defaults to `runtime/` next to the binary, then `./runtime`. |

`install` saves both in the service definition, so the service starts with the same environment and runtime folder.

## Examples

```bash
sudo ./moonshed install -e production
./moonshed serve -e production --runtime /srv/moonshed
sudo ./moonshed setup-link -e production
sudo ./moonshed reset-password admin
```
