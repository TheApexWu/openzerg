#!/usr/bin/env python3
"""Attack: fs_mount_host — Attempt mount syscall (requires CAP_SYS_ADMIN)"""
import json, subprocess, sys, time

def run():
    result = {"vector": "fs_mount_host", "category": "filesystem",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    fs_type = "proc"

    print(json.dumps({"type": "log", "line": f"attempting mount -t {fs_type} none /mnt"}))
    sys.stdout.flush()

    try:
        r = subprocess.run(
            ["mount", "-t", fs_type, "none", "/mnt"],
            capture_output=True, text=True, timeout=5
        )
        if r.returncode == 0:
            result["status"] = "BREACH"
            result["evidence"] = f"mount succeeded — CAP_SYS_ADMIN available"
        else:
            result["evidence"] = f"mount() denied: {r.stderr.strip()[:80]}"
    except FileNotFoundError:
        result["evidence"] = "mount binary not found in container"
    except subprocess.TimeoutExpired:
        result["evidence"] = "mount timed out"
    except Exception as e:
        result["evidence"] = f"error: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
