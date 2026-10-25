# Installation

Moonshed ships as one archive per platform: the binary, a default configuration file and the UI.

| Platform | Download |
|---|---|
| Linux x86-64 | `moonshed-linux-amd64.tar.gz` |
| Linux ARM64 | `moonshed-linux-arm64.tar.gz` |
| Windows x86-64 | `moonshed-windows-amd64.zip` |

All releases are on the [GitHub releases page](https://github.com/SiddheshNan/moonshed/releases), with a `SHA256SUMS` file to check the downloads against.

## Quick install on Linux

Download the install script, read it, then run it:

```bash
curl -fsSL https://moonshed.siddhesh.me/install.sh -o install.sh
less install.sh
sh install.sh
```

Or in one line: `curl -fsSL https://moonshed.siddhesh.me/install.sh | sh`.

The script picks the x86-64 or ARM64 build, downloads the latest release from GitHub, checks it against the release's `SHA256SUMS`, unpacks it into `/opt/moonshed` and installs it as a systemd service. Set `MOONSHED_DIR` to install somewhere else, or `MOONSHED_VERSION` (e.g. `v0.3.0`) for a specific release. When it finishes it prints a setup link; open it to create the admin account. If you lose the link, run `sudo /opt/moonshed/moonshed setup-link -e production` to print it again.

To install by hand, or on Windows, follow the steps below.

## Check what you download

- **The script** is [install.sh on GitHub](https://github.com/SiddheshNan/moonshed/blob/main/website/public/install.sh), about 70 lines.
- **The binary** is built by the public [release workflow](https://github.com/SiddheshNan/moonshed/blob/main/.github/workflows/release.yml) from the tagged commit, never on a personal machine. You can see every run on the [Actions tab](https://github.com/SiddheshNan/moonshed/actions/workflows/release.yml).
- **Checksums:** each release has a `SHA256SUMS` file. Check a download with `sha256sum -c SHA256SUMS --ignore-missing`.
- **Provenance:** each archive has a signed record from GitHub of the commit and workflow that built it. Check it with the [GitHub CLI](https://cli.github.com):

  ```bash
  gh attestation verify moonshed-linux-amd64.tar.gz -R SiddheshNan/moonshed
  ```

- **Or build it yourself** from source, see [Build from source](#build-from-source).

## Install on the server

Download the latest release and unpack it where Moonshed should live. Everything it stores goes into a `runtime/` folder next to the binary.

::: code-group

```bash [Linux]
cd /opt
tag=$(curl -fsSL "https://api.github.com/repos/SiddheshNan/moonshed/releases?per_page=1" | grep -m1 '"tag_name"' | cut -d'"' -f4)
curl -fsSL "https://github.com/SiddheshNan/moonshed/releases/download/$tag/moonshed-linux-amd64.tar.gz" | sudo tar -xz
cd moonshed
```

```powershell [Windows]
# In an Administrator PowerShell
$tag = (Invoke-RestMethod "https://api.github.com/repos/SiddheshNan/moonshed/releases?per_page=1")[0].tag_name
Invoke-WebRequest "https://github.com/SiddheshNan/moonshed/releases/download/$tag/moonshed-windows-amd64.zip" -OutFile moonshed.zip
Expand-Archive moonshed.zip -DestinationPath C:\
cd C:\moonshed
```

:::

On an ARM server, download `moonshed-linux-arm64.tar.gz` instead.

Check `runtime/config/production.yaml` (see [Configuration file](/docs/reference/configuration)), then install Moonshed as a service:

::: code-group

```bash [Linux]
sudo ./moonshed install -e production
```

```powershell [Windows]
.\moonshed.exe install -e production
```

:::

On Linux this writes a systemd unit to `/etc/systemd/system/moonshed.service`. On Windows it registers a Windows service. Either way Moonshed starts now and on every boot.

`install` ends with a setup link like `http://203.0.113.10:5050/setup?token=...`. Open it to create the admin account. The IP is your server's network card; behind NAT, swap in the public IP. Until the admin exists, the panel only lets you set up through this link, so nobody else who reaches port 5050 can take it over.

Lost the link? Run `moonshed setup-link -e production` in the same folder (with `sudo` on Linux) to print it again.

![Login page](/screenshots/login.png)

The rest of `runtime/` (databases, apps, logs, backups) is created on first run. See [Runtime folder](/docs/reference/runtime-folder) for what goes where.

## Before you put it on the internet

- The panel on port 5050 is plain HTTP. Give it a domain with HTTPS (Settings › Reverse Proxy (Caddy) › Panel access) and set `panel_addr: "127.0.0.1:5050"` so it is only reachable through that domain. See [Domains and HTTPS](/docs/domains#the-panel).
- Ports 80 and 443 must be free and open for the reverse proxy and Let's Encrypt.
- Install Docker if you want to run Docker apps. Moonshed works without it.

## Run in the foreground

To try Moonshed without installing a service:

```bash
./moonshed serve -e production
```

## Uninstall

```bash
sudo ./moonshed uninstall
```

This stops and removes the service. The `runtime/` folder is left as it is.

## Build from source

You need Go 1.26+, Node 20+ and Python 3.

```bash
git clone https://github.com/SiddheshNan/moonshed.git
cd moonshed
./scripts/build.py
```

The script asks for the version, the target platforms, the output folder and whether to run the tests. Press Enter to accept the defaults. It writes one folder per platform into `dist/`, laid out the same as a release archive. The binary is built without CGO, so you can build for any platform from any machine.
