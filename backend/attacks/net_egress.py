#!/usr/bin/env python3
"""Attack: net_egress — Attempt TCP connection to external internet"""
import json, socket, sys, time

def run():
    result = {"vector": "net_egress", "category": "network",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    target = "1.1.1.1"
    port   = 80

    print(json.dumps({"type": "log", "line": f"attempting egress to {target}:{port}"}))
    sys.stdout.flush()
    print(json.dumps({"type": "log", "line": "TCP SYN sent, waiting for response..."}))
    sys.stdout.flush()

    try:
        sock = socket.create_connection((target, port), timeout=5)
        sock.close()
        result["status"] = "BREACH"
        result["evidence"] = f"egress to {target}:{port} succeeded — NetworkPolicy not enforced"
    except (socket.timeout, ConnectionRefusedError, OSError) as e:
        result["evidence"] = f"egress to {target}:{port} blocked — {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
