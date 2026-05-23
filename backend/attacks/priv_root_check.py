#!/usr/bin/env python3
"""Attack: priv_root_check — Check if running as root"""
import json, os, sys, time

def run():
    result = {"vector": "priv_root_check", "category": "privilege",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    uid = os.getuid()
    print(json.dumps({"type": "log", "line": f"checking effective uid: {uid}"}))
    sys.stdout.flush()

    if uid == 0:
        result["status"] = "BREACH"
        result["evidence"] = "running as root (uid=0) — container not using runAsNonRoot"
    else:
        result["evidence"] = f"running as uid={uid} — not root"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
