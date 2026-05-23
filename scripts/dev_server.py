#!/usr/bin/env python3
"""OpenZerg local dev server.

Serves static files and exposes safe local integration status endpoints.
Secrets are read from environment or .env and are never sent to the browser.
"""
from __future__ import annotations

import json
import os
import subprocess
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.error import URLError, HTTPError
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
OPENAGE_PROCESS = None
OPENAGE_LOG = Path("/tmp/openzerg-openage-engine.log")


def load_dotenv() -> None:
    env_path = ROOT / ".env"
    if not env_path.exists():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ.setdefault(key.strip(), value.strip().strip('"').strip("'"))


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(ROOT), **kwargs)

    def send_json(self, status: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):  # noqa: N802 - stdlib handler API
        if self.path.startswith("/api/integrations/nimble"):
            return self.nimble_status()
        if self.path.startswith("/api/integrations/clickhouse"):
            return self.clickhouse_status()
        if self.path.startswith("/api/openage/status"):
            return self.openage_status()
        if self.path.startswith("/api/openage/launch"):
            return self.openage_launch()
        return super().do_GET()

    def nimble_status(self) -> None:
        key = os.environ.get("NIMBLE_API_KEY", "").strip()
        url = os.environ.get("NIMBLE_API_URL", "").strip()
        if not key:
            return self.send_json(200, {
                "service": "nimble",
                "mode": "demo",
                "configured": False,
                "message": "Set NIMBLE_API_KEY in .env to enable Nimble integration.",
            })

        # If no endpoint is selected yet, confirm configuration without exposing the key.
        if not url:
            return self.send_json(200, {
                "service": "nimble",
                "mode": "configured",
                "configured": True,
                "message": "NIMBLE_API_KEY loaded. Set NIMBLE_API_URL when the exact endpoint is chosen.",
            })

        try:
            req = Request(url, headers={"Authorization": f"Bearer {key}", "Accept": "application/json"})
            with urlopen(req, timeout=8) as res:
                data = res.read(4096).decode("utf-8", errors="replace")
            return self.send_json(200, {
                "service": "nimble",
                "mode": "live",
                "configured": True,
                "status": "reachable",
                "sample": data[:1000],
            })
        except HTTPError as exc:
            return self.send_json(200, {"service": "nimble", "mode": "live", "configured": True, "status": f"http_{exc.code}"})
        except URLError as exc:
            return self.send_json(200, {"service": "nimble", "mode": "live", "configured": True, "status": "error", "error": str(exc.reason)})

    def clickhouse_status(self) -> None:
        configured = bool(os.environ.get("CLICKHOUSE_HOST", "").strip())
        return self.send_json(200, {
            "service": "clickhouse",
            "mode": "configured" if configured else "demo",
            "configured": configured,
        })

    def openage_status(self) -> None:
        global OPENAGE_PROCESS
        running = bool(OPENAGE_PROCESS and OPENAGE_PROCESS.poll() is None)
        tail = ""
        if OPENAGE_LOG.exists():
            tail = "\n".join(OPENAGE_LOG.read_text(errors="replace").splitlines()[-30:])
        return self.send_json(200, {
            "service": "openage",
            "built": (ROOT / "vendor/openage/bin/run").exists(),
            "running": running,
            "pid": OPENAGE_PROCESS.pid if running else None,
            "log_tail": tail,
        })

    def openage_launch(self) -> None:
        global OPENAGE_PROCESS
        if OPENAGE_PROCESS and OPENAGE_PROCESS.poll() is None:
            return self.openage_status()

        bin_dir = ROOT / "vendor/openage/bin"
        run_bin = bin_dir / "run"
        if not run_bin.exists():
            return self.send_json(409, {
                "service": "openage",
                "running": False,
                "message": "OpenAge is not built yet. Build vendor/openage first.",
            })

        env = os.environ.copy()
        data_root = ROOT / "vendor/openage-data"
        env.setdefault("XDG_DATA_HOME", str(data_root))
        env.setdefault("XDG_CONFIG_HOME", str(data_root))
        env.setdefault("XDG_CACHE_HOME", str(data_root / ".cache"))
        OPENAGE_LOG.parent.mkdir(parents=True, exist_ok=True)
        log_file = OPENAGE_LOG.open("a")
        log_file.write("\n--- launching OpenAge engine demo ---\n")
        log_file.flush()
        OPENAGE_PROCESS = subprocess.Popen(
            [str(run_bin), "test", "--demo", "main.tests.engine_demo", "0"],
            cwd=str(bin_dir),
            env=env,
            stdout=log_file,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        return self.openage_status()


if __name__ == "__main__":
    load_dotenv()
    host = "127.0.0.1"
    port = int(os.environ.get("PORT", "5177"))
    print(f"OpenZerg dev server: http://{host}:{port}/prototypes/")
    ThreadingHTTPServer((host, port), Handler).serve_forever()
