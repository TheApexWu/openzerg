#!/usr/bin/env python3
"""
Attack: priv_docker_sock — Check if Docker socket is exposed
Category: privilege

If /var/run/docker.sock is mounted into the pod, any process can
control the host Docker daemon = full container escape.

SCAFFOLD — real implementation on hack day.
"""
import json
import os
import socket
import sys
import time


def run():
    result = {
        "vector": "priv_docker_sock",
        "category": "privilege",
        "status": "HELD",
        "evidence": "",
        "duration_ms": 0,
    }

    start = time.time()
    sock_path = "/var/run/docker.sock"

    print(json.dumps({"type": "log", "line": f"probing {sock_path}"}))
    sys.stdout.flush()

    if not os.path.exists(sock_path):
        result["evidence"] = "docker.sock not mounted"
        result["duration_ms"] = int((time.time() - start) * 1000)
        print(json.dumps({"type": "result", **result}))
        sys.stdout.flush()
        return

    print(json.dumps({"type": "log", "line": "socket found — sending GET /containers/json"}))
    sys.stdout.flush()

    try:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.connect(sock_path)
        sock.send(b"GET /containers/json HTTP/1.1\r\nHost: localhost\r\n\r\n")
        response = sock.recv(4096).decode(errors="replace")
        sock.close()

        if "200 OK" in response:
            result["status"] = "BREACH"
            result["evidence"] = "docker.sock exposed — full host container listing via Docker API"
            print(json.dumps({"type": "log", "line": "200 OK — host Docker daemon fully accessible"}))
        else:
            result["evidence"] = f"docker.sock present but API returned: {response[:80]}"

    except (PermissionError, ConnectionRefusedError, OSError) as e:
        result["evidence"] = f"docker.sock present but inaccessible: {e}"

    sys.stdout.flush()
    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()


if __name__ == "__main__":
    run()
