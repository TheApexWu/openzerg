#!/usr/bin/env python3
"""Attack: res_disk_fill — Write to /tmp until ENOSPC or limit hit"""
import json, os, sys, time

def run():
    result = {"vector": "res_disk_fill", "category": "resource",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    path = "/tmp/openzerg_fill"
    written_mb = 0
    chunk = b"\x00" * (1024 * 1024)  # 1MB

    print(json.dumps({"type": "log", "line": f"writing {path} with 1MB chunks"}))
    sys.stdout.flush()

    try:
        with open(path, "wb") as f:
            while written_mb < 1024:
                f.write(chunk)
                written_mb += 1
                if written_mb % 50 == 0:
                    print(json.dumps({"type": "log", "line": f"wrote {written_mb}MB..."}))
                    sys.stdout.flush()
        result["status"] = "BREACH"
        result["evidence"] = f"wrote {written_mb}MB without ENOSPC — ephemeral storage limit not enforced"
    except OSError as e:
        if e.errno == 28:  # ENOSPC
            result["evidence"] = f"ENOSPC at {written_mb}MB — ephemeral storage limit enforced"
        else:
            result["evidence"] = f"OS error at {written_mb}MB: {e}"
    finally:
        try:
            os.unlink(path)
        except OSError:
            pass

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
