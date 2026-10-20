# Users and security

## Users

The first user is created when you open the panel for the first time. Add more under **Settings › Users**. Every user is an admin.

Logins last 30 days. After 10 failed logins from one IP within 15 minutes, that IP is blocked until the window passes.

If you lose a password, reset it on the server:

```bash
sudo ./moonshed reset-password admin
```

It prints a new random password.

## API tokens

Tokens for CI and scripts are made under **Settings › API tokens**. They start with `msh_`, are shown once, and only a hash is stored. Use one as `Authorization: Bearer msh_...`. See [Deploying from CI](/docs/ci).

A token can do everything a signed-in user can, so give each pipeline its own. API tokens are not used for cloning repositories; that's what the GitHub App and [git logins](/docs/git#other-git-hosts) are for.

## Projects

Every app belongs to a project, a simple group of apps. Projects can only be deleted when they're empty.

## Audit log

**Settings › Audit log** records every change: who made it (a user, an API token or a webhook), when, from which IP, and what changed. Failed logins are recorded too. Env var changes list the keys, never the values.

## Secrets at rest

Env vars, git tokens and Windows passwords are encrypted with AES-GCM. The key is `runtime/data/secret.key`, created on first run.

::: danger Keep secret.key safe
Without it the encrypted values can't be read. Platform backups include it, so treat those backups as secrets too.
:::

## App isolation

Native Process apps aren't sandboxed. By default they run as the same account as Moonshed. Set [Run as OS user](/docs/native-apps#running-as-another-os-user) per app to limit what each one can touch.
