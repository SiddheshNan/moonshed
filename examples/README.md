# Examples

Small apps to try Moonshed with. Each runs as a Native Process on Linux or Windows.

| Example | Stack | Shows |
|---|---|---|
| [flask-hello](flask-hello) | Python, Flask, waitress | A hello page with a visit counter |
| [fastapi-todo](fastapi-todo) | Python, FastAPI, uvicorn, SQLite | A todo list saved in SQLite, kept across deploys |
| [node-snake](node-snake) | Node, Express | A Snake game with a leaderboard, no database |

## Deploying one

Zip the example's folder (with `moonshed.yaml` at the top of the zip), create a Zip upload app, and upload it from **Deploy**:

```bash
cd examples/fastapi-todo && zip -r ../fastapi-todo.zip . -x '.venv/*' 'data/*'
```

Or create a GitHub App or Git URL app for this repository and set **Folder** to `examples/<name>`.

If an example's port is taken, change it with **Edit** next to Port on the app's Overview.
