#!/usr/bin/env python3
"""
Attack: res_fork_bomb — Test PID cgroup limits
Category: resource

Spawns child processes until hitting the cgroup PID limit.
If the limit holds, sandbox is protected against fork bombs.

SCAFFOLD — real implementation on hack day.
"""
import json
import os
import sys
import time


def run():
    result = {
        "vector": "res_fork_bomb",
        "category": "resource",
        "status": "HELD",
        "evidence": "",
        "duration_ms": 0,
    }

    start = time.time()
    pids = []
    max_forks = 512  # Safety cap so we don't actually kill the host

    print(json.dumps({"type": "log", "line": "spawning child processes to test PID limit"}))
    sys.stdout.flush()

    try:
        for i in range(max_forks):
            pid = os.fork()
            if pid == 0:
                # Child: sleep and exit
                time.sleep(30)
                os._exit(0)
            else:
                pids.append(pid)
                if (i + 1) % 64 == 0:
                    print(json.dumps({"type": "log", "line": f"forked {i + 1} processes..."}))
                    sys.stdout.flush()

        # If we get here without error, PID limit is not enforced
        result["status"] = "BREACH"
        result["evidence"] = f"forked {len(pids)} processes without hitting PID limit"

    except OSError as e:
        # EAGAIN = resource temporarily unavailable (PID limit hit)
        result["evidence"] = f"cgroup PID limit enforced at {len(pids)} processes — {e}"

    finally:
        # Cleanup: kill all children
        for pid in pids:
            try:
                os.kill(pid, 9)
                os.waitpid(pid, 0)
            except OSError:
                pass

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()


if __name__ == "__main__":
    run()
