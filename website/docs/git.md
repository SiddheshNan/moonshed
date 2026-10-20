# Git and webhooks

An app's source is either a zip upload or a git repository. You choose it when you create the app and can change it later in the **Source** card on Overview.

For git sources you set the branch and, optionally, a **Folder** if the app lives in a subfolder of the repository. Every git deploy is zipped into a bundle, so redeploying an older version never needs the repository.

## GitHub App

The easiest way to deploy from GitHub is to connect a GitHub App. Clones then use short-lived tokens, so private repositories need no personal token, and you get a repository and branch picker.

1. Go to **Settings › Git**, choose who should own the GitHub App (your personal account or an organization you own) and click **Continue on GitHub**.
2. GitHub opens with the app's settings filled in. Click **Create GitHub App**.
3. Install it on the accounts you want to deploy from and choose which repositories Moonshed may read. Use **Install on another account** later to add more.
4. When creating an app, choose **GitHub App** under **Deploy from** and pick the repository and branch.

All pushes arrive at one webhook, `POST /api/hooks/github`, signed with the GitHub App's secret.

::: warning
GitHub has to reach your panel over HTTPS to send webhooks. Give the panel a domain first, see [Domains and HTTPS](/docs/domains#the-panel).
:::

## Other git hosts

For GitLab, Bitbucket, Gitea or anything else, choose **Git URL** and paste the repository URL. Public repositories need nothing else.

For private repositories, add a **git login** under **Settings › Git**: your username and a personal access token from that host. Then pick it as the app's **Git login**. One login can be used by many apps.

## Deploying on push

Turn on **Deploy automatically on every push** for the app.

- **With the GitHub App**, that's all. Pushes to the branch deploy straight away.
- **With a git URL**, the Source card shows a webhook URL and secret. Add them to your repository as a push webhook. On GitHub set the content type to `application/json`.

With automatic deploys off, a push only shows up on the app as **New commit waiting to deploy**, and you press **Deploy** when you're ready.

Each deployment records the commit and its message.

### The per-app webhook

The per-app webhook is `POST /api/hooks/apps/<app-id>` and accepts:

- GitHub push webhooks signed with the app's secret (`X-Hub-Signature-256`),
- GitLab push webhooks with the secret in `X-Gitlab-Token`,
- a plain request with `?token=<secret>` from anything else, which always deploys.

```bash
curl -fsS -X POST "https://panel.example.com/api/hooks/apps/$APP_ID?token=$WEBHOOK_SECRET"
```
