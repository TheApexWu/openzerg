#!/usr/bin/env python3
"""Attack: priv_capabilities — Check for dangerous Linux capabilities in CapEff"""
import json, sys, time

DANGEROUS_CAPS = {
    0x200000:  "CAP_SYS_ADMIN",
    0x4000000: "CAP_BPF",
    0x10000:   "CAP_SYS_MODULE",
    0x1000:    "CAP_NET_ADMIN",
    0x2000:    "CAP_SYS_PTRACE",
    0x8000:    "CAP_SYS_BOOT",
    0x400000:  "CAP_SYS_NICE",
}

def get_cap_eff() -> int:
    with open("/proc/1/status") as f:
        for line in f:
            if line.startswith("CapEff:"):
                return int(line.split()[1], 16)
    return 0

def run():
    result = {"vector": "priv_capabilities", "category": "privilege",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    print(json.dumps({"type": "log", "line": "reading /proc/1/status for CapEff..."}))
    sys.stdout.flush()

    try:
        caps = get_cap_eff()
        print(json.dumps({"type": "log", "line": f"CapEff: {hex(caps)}"}))
        sys.stdout.flush()

        found = [name for bit, name in DANGEROUS_CAPS.items() if caps & bit]
        if found:
            result["status"] = "BREACH"
            result["evidence"] = f"dangerous capabilities active: {', '.join(found)} — eBPF/kernel attacks possible"
        else:
            result["evidence"] = f"no dangerous capabilities active (CapEff: {hex(caps)})"
    except (FileNotFoundError, PermissionError, ValueError) as e:
        result["evidence"] = f"could not read capabilities: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
