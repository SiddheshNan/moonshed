# Examples

Small apps that deploy to Moonshed as native processes. Each has a `moonshed.yaml` with scripts for Linux and Windows, a health check, and a default port.

| Example | Stack | Shows |
|---|---|---|
| [flask-hello](flask-hello) | Python, Flask, waitress | Release info, a visit counter kept in `persist`, a health check |
| [fastapi-todo](fastapi-todo) | Python, FastAPI, uvicorn, SQLite | A todo list whose database lives in `data/` and survives deploys and rollbacks |
| [node-snake](node-snake) | Node, Express | A Snake game with an in-memory leaderboard, no database, `restart_unhealthy` |

## Deploying one

Zip the example's folder so `moonshed.yaml` sits at the root of the zip, create an app in the panel and drop the zip into the **Deploy** dialog:

```bash
cd examples/fastapi-todo && zip -r ../fastapi-todo.zip . -x '.venv/*' 'data/*'
```

Or create a GitHub / Git URL app pointing at this repository and set **Folder** to `examples/<name>`.

The port in each `moonshed.yaml` is only a default; set a different one in the app's UI if it's taken. See the [moonshed.yaml reference](https://moonshed.siddhesh.me/docs/reference/moonshed-yaml) for every key.
