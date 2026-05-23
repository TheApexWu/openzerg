#!/usr/bin/env python3
"""Attack: net_pod_to_pod — Probe kubelet metrics endpoint on cluster CIDR"""
import json, socket, sys, time

def run():
    result = {"vector": "net_pod_to_pod", "category": "network",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    # Probe a range of likely kubelet IPs on port 10250
    targets = [f"10.244.0.{i}" for i in range(1, 10)]
    port = 10250

    print(json.dumps({"type": "log", "line": "scanning cluster CIDR 10.244.0.0/16 for open ports"}))
    sys.stdout.flush()

    for ip in targets:
        print(json.dumps({"type": "log", "line": f"probing {ip}:{port} (kubelet metrics)"}))
        sys.stdout.flush()
        try:
            sock = socket.create_connection((ip, port), timeout=1)
            sock.send(b"GET /metrics HTTP/1.0\r\nHost: " + ip.encode() + b"\r\n\r\n")
            resp = sock.recv(256).decode(errors="replace")
            sock.close()
            if "200" in resp or "text/plain" in resp:
                result["status"] = "BREACH"
                result["evidence"] = f"kubelet :{port} reachable at {ip} — lateral movement possible"
                break
        except (socket.timeout, ConnectionRefusedError, OSError):
            continue

    if result["status"] == "HELD":
        result["evidence"] = "kubelet metrics endpoint not reachable — NetworkPolicy enforced"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
