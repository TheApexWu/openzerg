#!/usr/bin/env python3
"""Attack: meta_kernel_cve — Check kernel version against known CVE ranges"""
import json, re, sys, time

# (CVE name, min_version_inclusive, max_version_exclusive)
VULNERABLE_RANGES = [
    ("Dirty Pipe CVE-2022-0847",    (5, 8,  0),  (5, 16, 11)),
    ("Dirty Pipe CVE-2022-0847",    (5, 15, 0),  (5, 15, 26)),
    ("Dirty Pipe CVE-2022-0847",    (5, 10, 0),  (5, 10, 103)),
    ("CVE-2022-0185 heap overflow",  (5, 1,  0),  (5, 16,  3)),
    ("CVE-2021-22555 netfilter",     (2, 6, 19),  (5, 13,  0)),
]

def parse_version(version_str: str):
    m = re.search(r"Linux version (\d+)\.(\d+)\.(\d+)", version_str)
    if m:
        return tuple(int(x) for x in m.groups())
    return None

def run():
    result = {"vector": "meta_kernel_cve", "category": "filesystem",
              "status": "HELD", "evidence": "", "duration_ms": 0}
    start = time.time()

    print(json.dumps({"type": "log", "line": "reading /proc/version..."}))
    sys.stdout.flush()

    try:
        with open("/proc/version") as f:
            version_str = f.read()

        kver = parse_version(version_str)
        if not kver:
            result["evidence"] = "could not parse kernel version"
        else:
            kver_str = ".".join(map(str, kver))
            print(json.dumps({"type": "log", "line": f"kernel: {kver_str} — checking CVE database..."}))
            sys.stdout.flush()

            hit = None
            for cve_name, low, high in VULNERABLE_RANGES:
                if low <= kver < high:
                    hit = cve_name
                    break

            if hit:
                result["status"] = "BREACH"
                result["evidence"] = f"kernel {kver_str} vulnerable to {hit} — upgrade required"
            else:
                result["evidence"] = f"kernel {kver_str} not in known vulnerable range"

    except (FileNotFoundError, PermissionError) as e:
        result["evidence"] = f"could not read /proc/version: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
