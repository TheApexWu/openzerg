#!/usr/bin/env python3
"""Attack: priv_cgroup_escape — Test release_agent writability (CVE-2022-0492 class)"""
import glob, json, sys, time

def run():
    result = {"vector": "priv_cgroup_escape", "category": "privilege",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    print(json.dumps({"type": "log", "line": "probing /sys/fs/cgroup/*/release_agent writability..."}))
    sys.stdout.flush()
    print(json.dumps({"type": "log", "line": "testing CVE-2022-0492 class escape..."}))
    sys.stdout.flush()

    paths = glob.glob("/sys/fs/cgroup/*/release_agent") or ["/sys/fs/cgroup/release_agent"]
    breach_path = None

    for path in paths:
        try:
            # Zero-length write — tests writability, triggers nothing
            with open(path, "w") as f:
                f.write("")
            breach_path = path
            break
        except (PermissionError, FileNotFoundError, OSError):
            continue

    if breach_path:
        result["status"] = "BREACH"
        result["evidence"] = f"{breach_path} is writable — CVE-2022-0492 class host escape possible"
    else:
        result["evidence"] = "release_agent not writable — cgroup escape blocked"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
