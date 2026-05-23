"""
OpenZerg Attack Vector Registry

Each entry defines:
  - vector:          unique identifier
  - category:        filesystem | network | privilege | secrets | resource
  - params:          genome — mutable fields for evolutionary mutation
  - log_lines:       scaffold mode log output
  - expected_status: HELD or BREACH
  - evidence:        human-readable proof string
"""

ATTACK_VECTORS = [

    # ── Filesystem (3 vectors) ──────────────────────────────────────────
    {
        "vector": "fs_symlink",
        "category": "filesystem",
        "params": {"target_path": "/proc/1/root/etc/shadow", "technique": "symlink", "timeout": 5},
        "log_lines": [
            "creating symlink /tmp/escape → /proc/1/root/etc/shadow",
            "attempting readlink traversal...",
        ],
        "expected_status": "HELD",
        "evidence": "symlink blocked by mount namespace",
    },
    {
        "vector": "fs_proc_root",
        "category": "filesystem",
        "params": {"target_path": "/proc/1/root/etc/shadow", "technique": "direct_open", "timeout": 5},
        "log_lines": [
            "opening /proc/1/root",
            "/proc/1/root accessible — descending to /etc/shadow",
            "reading /etc/shadow contents...",
        ],
        "expected_status": "BREACH",
        "evidence": "read /etc/shadow via /proc/1/root — host filesystem exposed",
    },
    {
        "vector": "fs_mount_host",
        "category": "filesystem",
        "params": {"target_path": "proc", "technique": "mount_syscall", "timeout": 5},
        "log_lines": [
            "attempting mount -t proc none /mnt",
            "trying mount /host from hostPath volume...",
        ],
        "expected_status": "HELD",
        "evidence": "mount() denied — no CAP_SYS_ADMIN",
    },

    # ── Network (3 vectors) ─────────────────────────────────────────────
    {
        "vector": "net_k8s_api",
        "category": "network",
        "params": {"target_path": "10.96.0.1", "technique": "bearer_token", "timeout": 5},
        "log_lines": [
            "resolving kubernetes.default.svc (10.96.0.1)",
            "connecting to 10.96.0.1:443 with service account token...",
        ],
        "expected_status": "HELD",
        "evidence": "NetworkPolicy rejected connection to API server",
    },
    {
        "vector": "net_egress",
        "category": "network",
        "params": {"target_path": "1.1.1.1:80", "technique": "tcp_syn", "timeout": 5},
        "log_lines": [
            "attempting egress to 1.1.1.1:80",
            "TCP SYN sent, waiting for response...",
        ],
        "expected_status": "HELD",
        "evidence": "egress blocked by NetworkPolicy",
    },
    {
        "vector": "net_pod_to_pod",
        "category": "network",
        "params": {"target_path": "10.244.0.0/16", "technique": "port_scan", "timeout": 5},
        "log_lines": [
            "scanning cluster CIDR 10.244.0.0/16 for open ports",
            "probing 10.244.0.5:10250 (kubelet metrics)",
            "200 OK — kubelet metrics endpoint reachable",
        ],
        "expected_status": "BREACH",
        "evidence": "kubelet :10250 reachable from sandbox pod — lateral movement possible",
    },

    # ── Privilege escalation (3 vectors) ────────────────────────────────
    {
        "vector": "priv_root_check",
        "category": "privilege",
        "params": {"technique": "uid_check", "timeout": 2},
        "log_lines": [
            "checking effective uid: id -u",
            "uid=1000 (sandbox)",
        ],
        "expected_status": "HELD",
        "evidence": "running as uid 1000, not root",
    },
    {
        "vector": "priv_docker_sock",
        "category": "privilege",
        "params": {"target_path": "/var/run/docker.sock", "technique": "socket_api", "timeout": 5},
        "log_lines": [
            "probing /var/run/docker.sock",
            "socket found — sending GET /containers/json",
            "200 OK — full container listing returned",
        ],
        "expected_status": "BREACH",
        "evidence": "docker.sock exposed — full host escape via container API",
    },
    {
        "vector": "priv_nsenter",
        "category": "privilege",
        "params": {"technique": "mount", "timeout": 5},
        "log_lines": [
            "attempting nsenter --target 1 --mount --pid",
            "nsenter: operation not permitted",
        ],
        "expected_status": "HELD",
        "evidence": "nsenter blocked — no CAP_SYS_ADMIN or CAP_SYS_PTRACE",
    },

    # ── Secrets (3 vectors) ─────────────────────────────────────────────
    {
        "vector": "sec_sa_token",
        "category": "secrets",
        "params": {"target_path": "/var/run/secrets/kubernetes.io/serviceaccount/token", "timeout": 3},
        "log_lines": [
            "checking /var/run/secrets/kubernetes.io/serviceaccount/token",
            "token file not present",
        ],
        "expected_status": "HELD",
        "evidence": "automountServiceAccountToken: false — no token mounted",
    },
    {
        "vector": "sec_envvar_scan",
        "category": "secrets",
        "params": {"technique": "environ_proc", "timeout": 3},
        "log_lines": [
            "dumping process environment: env | grep -i secret",
            "scanning co-tenant pod env via /proc/*/environ",
        ],
        "expected_status": "HELD",
        "evidence": "pod-level env isolation intact, no cross-pod env access",
    },
    {
        "vector": "sec_configmap_read",
        "category": "secrets",
        "params": {"target_path": "/api/v1/namespaces/default/configmaps", "technique": "bearer_token", "timeout": 5},
        "log_lines": [
            "querying K8s API for ConfigMaps in namespace",
            "GET /api/v1/namespaces/default/configmaps",
        ],
        "expected_status": "HELD",
        "evidence": "RBAC denied — service account has no configmap read permission",
    },

    # ── Resource exhaustion (3 vectors) ─────────────────────────────────
    {
        "vector": "res_fork_bomb",
        "category": "resource",
        "params": {"technique": "fork_loop", "timeout": 10},
        "log_lines": [
            "spawning fork bomb: :(){ :|:& };:",
            "process count climbing... 64... 128... 256",
            "fork() returned EAGAIN at PID 256",
        ],
        "expected_status": "HELD",
        "evidence": "cgroup PID limit (256) enforced — fork bomb contained",
    },
    {
        "vector": "res_memory_bomb",
        "category": "resource",
        "params": {"technique": "bytearray_alloc", "timeout": 10},
        "log_lines": [
            "allocating memory in 64MB chunks",
            "allocated 128MB... 256MB... 384MB...",
            "OOMKilled at 512MB",
        ],
        "expected_status": "HELD",
        "evidence": "cgroup memory limit (512Mi) enforced — OOMKilled",
    },
    {
        "vector": "res_disk_fill",
        "category": "resource",
        "params": {"technique": "dd_zero", "target_path": "/tmp/fill", "timeout": 10},
        "log_lines": [
            "writing /tmp/fill with dd if=/dev/zero bs=1M",
            "wrote 50MB... 100MB... 150MB...",
            "write() returned ENOSPC at 200MB",
        ],
        "expected_status": "HELD",
        "evidence": "ephemeral storage limit (200Mi) enforced",
    },

    # ── Stretch vectors (evolutionary bonus) ────────────────────────────
    {
        "vector": "priv_capabilities",
        "category": "privilege",
        "params": {"technique": "cap_parse", "timeout": 3},
        "log_lines": [
            "reading /proc/1/status for CapEff...",
            "parsing capability bitmask...",
        ],
        "expected_status": "HELD",
        "evidence": "no dangerous capabilities active (CAP_SYS_ADMIN, CAP_BPF, CAP_NET_ADMIN)",
    },
    {
        "vector": "priv_cgroup_escape",
        "category": "privilege",
        "params": {"target_path": "/sys/fs/cgroup/*/release_agent", "technique": "write_test", "timeout": 5},
        "log_lines": [
            "probing /sys/fs/cgroup/*/release_agent writability...",
            "testing CVE-2022-0492 class escape...",
        ],
        "expected_status": "HELD",
        "evidence": "release_agent not writable — cgroup escape blocked",
    },
    {
        "vector": "meta_kernel_cve",
        "category": "filesystem",
        "params": {"technique": "version_check", "timeout": 2},
        "log_lines": [
            "reading /proc/version...",
            "checking kernel version against CVE database...",
        ],
        "expected_status": "HELD",
        "evidence": "kernel version not in known vulnerable range",
    },
]

# Index by vector name for quick lookup
REGISTRY = {a["vector"]: a for a in ATTACK_VECTORS}
