# OpenZerg Ralph State

STATUS: RUNNING

Last updated: 2026-05-23T18:11:45Z (iter 0019)

This file is the ledger of milestone state. The ralph loop reads it every
iteration. Milestones marked `ACCEPTED` are sticky — the agent will not
re-verify them unless dependencies change.

See `RALPH_README.md` for state machine rules and update format.

---

## Milestones

### M0 — Repo hygiene + Go module bootstrap
- status: ACCEPTED
- accepted_at: 2026-05-23T17:32:03Z
- summary: Go module scaffolded; all internal packages declared; binary prints version. build + vet green.
- verify_evidence: |
    cd backend && go build ./...  -> ok
    cd backend && go vet ./...    -> ok
    ./bin/openzerg version        -> "openzerg 0.1.0-dev"

### M1 — Config, secrets, doctor command
- status: ACCEPTED
- accepted_at: 2026-05-23T17:35:24Z
- summary: secrets/config/k8s probes implemented; doctor prints multi-line status; run --dry-run prints planned pod spec; tests green.
- verify_evidence: |
    cd backend && go build ./...                                    -> ok
    cd backend && go vet ./...                                      -> ok
    cd backend && go test ./...                                     -> ok (secrets + k8s)
    ./bin/openzerg doctor                                            -> kubeconfig + secret report, exit 0
    ./bin/openzerg run --target https://example.invalid --dry-run    -> planned-pod-spec preview, exit 0

### M2 — K8s pod spawn + log streaming (no PI yet)
- status: ACCEPTED
- accepted_at: 2026-05-23T18:11:45Z
- summary: client-go spawns 3 busybox stub pods in openzerg ns on DO cluster, streams logs, parses final JSON line, and deletes pods. RunAsUser/RunAsGroup pinned to 65532 so busybox passes RunAsNonRoot admission. All 3 results printed; zero leftover pods.
- verify_evidence: |
    cd backend && go build ./...  -> ok
    cd backend && go vet ./...    -> ok
    cd backend && go test ./...   -> ok (evolve, k8s, secrets, spawn)
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 1
        -> [pod 0..2] {"type":"result",...} ; run: ok
    kubectl -n openzerg get pods  -> No resources found (clean)

### M3 — Attacker pod image with PI + Gemma 4 (no Nimble yet)
- status: PENDING
- summary: (not started)

### M4 — Nimble integration inside the attacker pod
- status: PENDING
- summary: (not started)

### M5 — Evolution loop, fitness scoring, mutation, summary
- status: PENDING
- summary: (not started)

### M6 — Cleanup, docs, demo script
- status: PENDING
- summary: (not started)

---

## Iteration Log

<!-- Append one line per iteration. Format: -->
<!-- - iter NNNN | ISO-timestamp | M<n> | progress|accepted|blocked | one-line note -->
- iter 0002 | 2026-05-23T17:32:03Z | M0 | accepted | go module + package skeleton verified; build/vet green; binary prints version
- iter 0003 | 2026-05-23T17:35:24Z | M1 | accepted | secrets loader + flags + kubeconfig probe; doctor and run --dry-run wired; tests green
- iter 0004 | 2026-05-23T17:37:13Z | M2 | progress | evolve.ParseLastJSONLine + hostile-input tests; client-go spawn next
- iter 0005 | 2026-05-23T17:38:24Z | M2 | progress | added backend/deploy/namespace.yaml manifest for openzerg ns
- iter 0006 | 2026-05-23T17:40:29Z | M2 | progress | spawn.BuildBusyboxPod renders busybox pod manifest with k8s.io/api types; tests pass
- iter 0007 | 2026-05-23T17:42:07Z | M2 | progress | k8s.BuildClientset added (client-go, explicit/$KUBECONFIG/in-cluster); 3 unit tests green
- iter 0008 | 2026-05-23T17:43:26Z | M2 | progress | k8s.CreatePod wrapper + 3 fake-clientset tests; build/vet/test green
- iter 0009 | 2026-05-23T17:45:00Z | M2 | progress | k8s.DeletePod wrapper (idempotent on NotFound) + 3 fake-clientset tests; build/vet/test green
- iter 0010 | 2026-05-23T17:46:30Z | M2 | progress | k8s.WaitForCompletion polls until Succeeded/Failed, ctx-cancellable; 4 fake-clientset tests; build/vet/test green
- iter 0011 | 2026-05-23T17:48:00Z | M2 | progress | k8s.StreamLogs (follow=true) returns io.ReadCloser; 2 fake-clientset tests; build/vet/test green
- iter 0013 | 2026-05-23T17:49:46Z | M2 | progress | spawn.RunPod ties create/stream/wait/parse/delete for one pod; nil-guard tests; build/vet/test green
- iter 0014 | 2026-05-23T17:51:42Z | M2 | progress | spawn.RunPods concurrent fan-out (sync.WaitGroup, ordered outcomes); 2 nil/empty tests; build/vet/test green
- iter 0015 | 2026-05-23T17:53:36Z | M2 | progress | cmd/openzerg run non-dry-run wired: builds clientset, renders N busybox stubs, fans out via spawn.RunPods, prints outcomes; build/vet/test green
- iter 0019 | 2026-05-23T18:11:45Z | M2 | accepted | live DO smoke green: 3/3 busybox stub pods spawn, emit JSON, parse, and delete; pinned RunAsUser=65532 to clear RunAsNonRoot admission
