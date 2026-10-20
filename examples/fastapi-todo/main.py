import os
import sqlite3
from contextlib import contextmanager
from pathlib import Path

import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field

# data/ is listed under persist in moonshed.yaml, so the database outlives every release.
DB_PATH = Path("data/todos.db")
INDEX = Path(__file__).parent / "index.html"


@contextmanager
def db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    try:
        with conn:
            yield conn
    finally:
        conn.close()


def init_db():
    DB_PATH.parent.mkdir(exist_ok=True)
    with db() as conn:
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute(
            """CREATE TABLE IF NOT EXISTS todos (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                title TEXT NOT NULL,
                done INTEGER NOT NULL DEFAULT 0,
                created_at TEXT NOT NULL DEFAULT (datetime('now'))
            )"""
        )


class TodoIn(BaseModel):
    title: str = Field(min_length=1, max_length=200)


class TodoPatch(BaseModel):
    title: str | None = Field(default=None, min_length=1, max_length=200)
    done: bool | None = None


def row_to_todo(row: sqlite3.Row) -> dict:
    return {"id": row["id"], "title": row["title"], "done": bool(row["done"]), "created_at": row["created_at"]}


def get_or_404(conn: sqlite3.Connection, todo_id: int) -> sqlite3.Row:
    row = conn.execute("SELECT * FROM todos WHERE id = ?", (todo_id,)).fetchone()
    if row is None:
        raise HTTPException(status_code=404, detail="Todo not found")
    return row


init_db()
app = FastAPI(title="Moonshed todo example")


@app.get("/", include_in_schema=False)
def index():
    return FileResponse(INDEX)


@app.get("/healthz", include_in_schema=False)
def healthz():
    with db() as conn:
        conn.execute("SELECT 1")
    return {"status": "ok"}


@app.get("/api/info")
def info():
    return {"app": os.environ.get("APP_NAME", "fastapi-todo"), "release": os.environ.get("RELEASE_ID", "local")}


@app.get("/api/todos")
def list_todos():
    with db() as conn:
        rows = conn.execute("SELECT * FROM todos ORDER BY done, id DESC").fetchall()
    return [row_to_todo(r) for r in rows]


@app.post("/api/todos", status_code=201)
def create_todo(body: TodoIn):
    with db() as conn:
        cur = conn.execute("INSERT INTO todos (title) VALUES (?)", (body.title.strip(),))
        return row_to_todo(get_or_404(conn, cur.lastrowid))


@app.patch("/api/todos/{todo_id}")
def update_todo(todo_id: int, body: TodoPatch):
    with db() as conn:
        get_or_404(conn, todo_id)
        if body.title is not None:
            conn.execute("UPDATE todos SET title = ? WHERE id = ?", (body.title.strip(), todo_id))
        if body.done is not None:
            conn.execute("UPDATE todos SET done = ? WHERE id = ?", (int(body.done), todo_id))
        return row_to_todo(get_or_404(conn, todo_id))


@app.delete("/api/todos/{todo_id}", status_code=204)
def delete_todo(todo_id: int):
    with db() as conn:
        get_or_404(conn, todo_id)
        conn.execute("DELETE FROM todos WHERE id = ?", (todo_id,))


@app.delete("/api/todos", status_code=204)
def clear_done():
    with db() as conn:
        conn.execute("DELETE FROM todos WHERE done = 1")


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.environ.get("PORT", 8001)))
