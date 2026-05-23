#!/usr/bin/env python3
"""Attack: res_memory_bomb — Allocate memory until OOMKilled or limit hit"""
import json, sys, time

def run():
    result = {"vector": "res_memory_bomb", "category": "resource",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    chunk_mb = 64
    allocated = 0
    chunks = []

    print(json.dumps({"type": "log", "line": f"allocating memory in {chunk_mb}MB chunks"}))
    sys.stdout.flush()

    try:
        while allocated < 2048:  # cap at 2GB — OOMKill should fire well before
            chunks.append(bytearray(chunk_mb * 1024 * 1024))
            allocated += chunk_mb
            print(json.dumps({"type": "log", "line": f"allocated {allocated}MB..."}))
            sys.stdout.flush()
        # If we reach here the limit wasn't enforced
        result["status"] = "BREACH"
        result["evidence"] = f"allocated {allocated}MB without OOMKill — memory limit not enforced"
    except MemoryError:
        result["evidence"] = f"MemoryError at {allocated}MB — cgroup limit enforced"
    finally:
        del chunks

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
