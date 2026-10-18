# Deploying from CI

CI can build your app and upload the result as a zip. This is the way to deploy compiled apps, for example a PyInstaller build made on the right OS, or anything that needs a build step you'd rather not run on the server.

## 1. Create an API token

Go to **Settings › API tokens** and create one. Tokens start with `msh_` and are shown once. Only a hash is stored.

Save it as a secret in your CI, for example `MOONSHED_TOKEN`.

## 2. Upload the zip

Send the zip to the app's deploy endpoint:

```bash
curl -fsS \
  -H "Authorization: Bearer $MOONSHED_TOKEN" \
  -H "Content-Type: application/zip" \
  --data-binary @app.zip \
  "https://panel.example.com/api/apps/$APP_ID/deploy"
```

A multipart upload with the file in a field named `bundle` works too:

```bash
curl -fsS -H "Authorization: Bearer $MOONSHED_TOKEN" \
  -F bundle=@app.zip \
  "https://panel.example.com/api/apps/$APP_ID/deploy"
```

The app ID is in the app's URL in the panel. The response is `202 Accepted` with the new deployment as JSON. The deploy then runs in the background, and you can follow it on the **Deployments** tab.

Bundles can be up to 4 GB.

## GitHub Actions example

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # Build your app here.

      - name: Zip
        run: zip -r app.zip . -x '.git/*'

      - name: Deploy to Moonshed
        run: |
          curl -fsS \
            -H "Authorization: Bearer ${{ secrets.MOONSHED_TOKEN }}" \
            -H "Content-Type: application/zip" \
            --data-binary @app.zip \
            "${{ vars.MOONSHED_URL }}/api/apps/${{ vars.MOONSHED_APP_ID }}/deploy"
```

## Triggering a git deploy

If the app deploys from git and you only want CI to say "deploy now", send the request with no body:

```bash
curl -fsS -X POST -H "Authorization: Bearer $MOONSHED_TOKEN" \
  "https://panel.example.com/api/apps/$APP_ID/deploy"
```

Or use the app's [webhook URL](/docs/git#the-per-app-webhook) with `?token=`, which needs no API token.
