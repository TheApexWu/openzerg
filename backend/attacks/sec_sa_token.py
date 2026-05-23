#!/usr/bin/env python3
"""Attack: sec_sa_token — Check for mounted service account token"""
import json, os, sys, time

def run():
    result = {"vector": "sec_sa_token", "category": "secrets",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    token_path = os.environ.get("TARGET_PATH",
                                "/var/run/secrets/kubernetes.io/serviceaccount/token")

    print(json.dumps({"type": "log", "line": f"checking {token_path}"}))
    sys.stdout.flush()

    try:
        with open(token_path) as f:
            token = f.read().strip()
        if len(token) > 10:
            result["status"] = "BREACH"
            result["evidence"] = f"SA token mounted ({len(token)} chars) — automountServiceAccountToken not disabled"
        else:
            result["evidence"] = "token file empty"
    except FileNotFoundError:
        result["evidence"] = "automountServiceAccountToken: false — no token mounted"
    except PermissionError:
        result["evidence"] = "token path exists but not readable"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
