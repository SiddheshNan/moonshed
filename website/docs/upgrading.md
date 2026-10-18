# Upgrading

1. [Build](/docs/installation#build) the new version.
2. Make a platform backup from **Settings › General**.
3. Stop Moonshed, replace the `moonshed` binary and the `runtime/web/` folder, and start it again.

::: code-group

```bash [Linux]
sudo systemctl stop moonshed
# copy the new moonshed binary and runtime/web/ into place
sudo systemctl start moonshed
```

```powershell [Windows]
Stop-Service moonshed
# copy the new moonshed.exe and runtime\web\ into place
Start-Service moonshed
```

:::

Keep the rest of `runtime/` as it is. It holds your databases, apps and backups.

Native apps are stopped and started again along with Moonshed. Docker containers keep running and are reattached.

::: warning Pre-release
Until the first stable release, an upgrade can include breaking changes. Check the [changelog](https://github.com/SiddheshNan/moonshed/blob/main/CHANGELOG.md) before upgrading.
:::
