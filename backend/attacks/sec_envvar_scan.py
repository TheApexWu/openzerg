#!/usr/bin/env python3
"""Attack: sec_envvar_scan — Scan env vars and /proc/*/environ for secrets"""
import json, os, glob, sys, time

SENSITIVE_KEYS = ["secret", "password", "token", "api_key", "private_key", "credential"]

def run():
    result = {"vector": "sec_envvar_scan", "category": "secrets",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    print(json.dumps({"type": "log", "line": "dumping process environment: env | grep -i secret"}))
    sys.stdout.flush()

    # Check own env
    found = [k for k in os.environ if any(s in k.lower() for s in SENSITIVE_KEYS)]
    if found:
        result["status"] = "BREACH"
        result["evidence"] = f"sensitive env vars found: {', '.join(found[:3])}"
        result["duration_ms"] = int((time.time() - start) * 1000)
        print(json.dumps({"type": "result", **result}))
        sys.stdout.flush()
        return

    # Try cross-pod env scan via /proc
    print(json.dumps({"type": "log", "line": "scanning co-tenant pod env via /proc/*/environ"}))
    sys.stdout.flush()

    for environ_path in glob.glob("/proc/[0-9]*/environ"):
        try:
            with open(environ_path, "rb") as f:
                data = f.read(4096).decode(errors="replace")
            for entry in data.split("\x00"):
                k = entry.split("=")[0].lower()
                if any(s in k for s in SENSITIVE_KEYS):
                    result["status"] = "BREACH"
                    result["evidence"] = f"cross-pod env leak via {environ_path}: key={entry.split('=')[0]}"
                    break
        except (PermissionError, FileNotFoundError, OSError):
            continue
        if result["status"] == "BREACH":
            break

    if result["status"] == "HELD":
        result["evidence"] = "pod-level env isolation intact, no cross-pod env access"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
