# API

The panel is built on the same HTTP API you can call from scripts. It's served on the panel's address under `/api`.

::: warning Not stable yet
The API can change between pre-release versions. Request and response shapes aren't documented here yet. The panel's [`ui/src/types.ts`](https://github.com/SiddheshNan/moonshed/blob/main/ui/src/types.ts) is the most up-to-date description.
:::

## Authentication

Send an [API token](/docs/security#api-tokens) as a bearer token:

```bash
curl -H "Authorization: Bearer msh_..." https://panel.example.com/api/apps
```

The panel itself uses a session cookie. Webhook endpoints and the first-run setup need neither.

## Conventions

- Request and response bodies are JSON. Unknown fields in a request body are rejected.
- IDs in paths are UUIDs.
- Errors return a status code and `{"error": "message"}`. A deploy or action on an app that's busy returns `409 Conflict`.
- Live logs (`/logs/stream`, `/deployments/{id}/log`) are Server-Sent Events.

## Endpoints

### Auth and users

```
GET    /api/auth/status
POST   /api/auth/setup                      first-run admin account (needs the setup link's token)
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/users                           POST /api/users
PUT    /api/users/{id}/password             DELETE /api/users/{id}
GET    /api/tokens                          POST /api/tokens
DELETE /api/tokens/{id}
```

### Projects and apps

```
GET    /api/projects                        POST /api/projects
GET    /api/projects/{id}                   PUT/DELETE /api/projects/{id}
GET    /api/apps                            POST /api/apps
GET    /api/apps/{id}                       PUT/DELETE /api/apps/{id}
POST   /api/apps/{id}/start
POST   /api/apps/{id}/stop
POST   /api/apps/{id}/restart
GET    /api/apps/{id}/env                   PUT /api/apps/{id}/env
```

### Deployments

```
POST   /api/apps/{id}/deploy                zip body, multipart "bundle", or empty for git
GET    /api/apps/{id}/deployments
GET    /api/apps/{id}/bundles
POST   /api/apps/{id}/bundles/{bundle}/redeploy
GET    /api/deployments                     all apps; ?status=&before=&limit=
GET    /api/deployments/{id}
POST   /api/deployments/{id}/kill
GET    /api/deployments/{id}/log            SSE
```

See [Deploying from CI](/docs/ci) for examples.

### Domains

```
GET    /api/apps/{id}/domains               POST /api/apps/{id}/domains
PUT    /api/domains/{id}                    DELETE /api/domains/{id}
PUT    /api/domains/{id}/certificate        cert and key, or a PFX, as multipart
POST   /api/domains/{id}/check              validate a draft without saving
```

### Logs, metrics and uptime

```
GET    /api/apps/{id}/logs?date=
GET    /api/apps/{id}/logs/days
GET    /api/apps/{id}/logs/stream           SSE
GET    /api/apps/{id}/metrics?range=
GET    /api/apps/{id}/uptime?range=
GET    /api/server
GET    /api/server/metrics?range=
GET    /api/requests
GET    /api/requests/recording              PUT /api/requests/recording
```

### Backups

```
GET    /api/apps/{id}/backups               POST /api/apps/{id}/backups
GET    /api/platform/backups                POST /api/platform/backups
POST   /api/backups/{id}/restore
GET    /api/backups/{id}/download
DELETE /api/backups/{id}
```

### Settings and proxy

```
GET    /api/settings                        PUT /api/settings
POST   /api/settings/panel-certificate
POST   /api/settings/proxy-check
GET    /api/proxy/caddyfile
GET    /api/audit                           ?action=&actor=&before=&limit=
```

### Git

```
GET    /api/git
POST   /api/git/credentials                 PUT/DELETE /api/git/credentials/{id}
POST   /api/git/github/manifest
GET    /api/git/github/callback
GET    /api/git/github/setup
POST   /api/git/github/{id}/sync            DELETE /api/git/github/{id}
GET    /api/git/installations/{id}/repos
GET    /api/git/installations/{id}/branches?repo=
```

### Webhooks

These need no login. They're verified with a signature or secret instead.

```
POST   /api/hooks/github                    GitHub App webhooks
POST   /api/hooks/apps/{id}                 per-app push webhook or ?token=
```

See [Git and webhooks](/docs/git#the-per-app-webhook).
