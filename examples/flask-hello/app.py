import os
import platform
import socket
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

from flask import Flask, jsonify, render_template
from waitress import serve

app = Flask(__name__)

# data/ is listed under persist in moonshed.yaml, so the counter survives deploys.
VISITS = Path("data/visits.txt")
STARTED = time.time()
lock = threading.Lock()


def count_visit() -> int:
    with lock:
        VISITS.parent.mkdir(exist_ok=True)
        count = int(VISITS.read_text() or 0) + 1 if VISITS.exists() else 1
        VISITS.write_text(str(count))
        return count


def uptime() -> str:
    secs = int(time.time() - STARTED)
    days, secs = divmod(secs, 86400)
    hours, secs = divmod(secs, 3600)
    mins, secs = divmod(secs, 60)
    parts = [f"{days}d"] if days else []
    if days or hours:
        parts.append(f"{hours}h")
    if days or hours or mins:
        parts.append(f"{mins}m")
    parts.append(f"{secs}s")
    return " ".join(parts)


def info() -> dict:
    return {
        "app": os.environ.get("APP_NAME", "flask-hello"),
        "release": os.environ.get("RELEASE_ID", "local"),
        "started": datetime.fromtimestamp(STARTED, timezone.utc).isoformat(timespec="seconds"),
        "uptime": uptime(),
        "host": socket.gethostname(),
        "platform": f"{platform.system()} {platform.machine()}",
        "python": platform.python_version(),
        "port": int(os.environ.get("PORT", 8000)),
    }


@app.get("/")
def index():
    return render_template("index.html", visits=count_visit(), **info())


@app.get("/api/info")
def api_info():
    return jsonify(info())


@app.get("/healthz")
def healthz():
    return "ok"


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8000))
    print(f"flask-hello listening on :{port}", flush=True)
    serve(app, host="0.0.0.0", port=port)
