import os
from pathlib import Path

from flask import Flask

app = Flask(__name__)
visits = Path("data/visits.txt")


@app.get("/")
def index():
    visits.parent.mkdir(exist_ok=True)
    count = int(visits.read_text() or 0) + 1 if visits.exists() else 1
    visits.write_text(str(count))
    return f"Hello from release {os.environ['RELEASE_ID']}, visit #{count}\n"


@app.get("/healthz")
def healthz():
    return "ok"


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=int(os.environ["PORT"]))
