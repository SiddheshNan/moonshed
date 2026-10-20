# Upgrading

An upgrade replaces two things: the `moonshed` binary and the `runtime/web/` folder. Keep the rest of `runtime/` as it is. It holds your configuration, databases, apps and backups.

1. Read the [changelog](/changelog) for the new version.
2. Make a platform backup from **Settings › Backups**.
3. Download the new release, stop Moonshed, copy the two parts over and start it again.

::: code-group

```bash [Linux]
cd /tmp
curl -fsSL https://github.com/SiddheshNan/moonshed/releases/latest/download/moonshed-linux-amd64.tar.gz | tar -xz

sudo systemctl stop moonshed
sudo cp moonshed/moonshed /opt/moonshed/moonshed
sudo rm -rf /opt/moonshed/runtime/web
sudo cp -r moonshed/runtime/web /opt/moonshed/runtime/web
sudo systemctl start moonshed
```

```powershell [Windows]
# In an Administrator PowerShell
cd $env:TEMP
Invoke-WebRequest https://github.com/SiddheshNan/moonshed/releases/latest/download/moonshed-windows-amd64.zip -OutFile moonshed.zip
Expand-Archive moonshed.zip -DestinationPath . -Force

Stop-Service moonshed
Copy-Item moonshed\moonshed.exe C:\moonshed\moonshed.exe -Force
Remove-Item C:\moonshed\runtime\web -Recurse -Force
Copy-Item moonshed\runtime\web C:\moonshed\runtime\web -Recurse
Start-Service moonshed
```

:::

::: warning
Don't unpack the new archive straight over your install. It contains a default `runtime/config/production.yaml` that would replace yours.
:::

Native Process apps are stopped and started again along with Moonshed. Docker containers keep running and are reattached.

::: warning Pre-release
Until the first stable release, an upgrade can include breaking changes.
:::
