# OpenZerg Ralph State

STATUS: RUNNING

Last updated: 2026-05-23T17:48:00Z (iter 0011)

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
- status: IN_PROGRESS
- summary: ParseLastJSONLine + namespace.yaml + spawn.BuildBusyboxPod + k8s.BuildClientset + k8s.CreatePod + k8s.DeletePod + k8s.WaitForCompletion + k8s.StreamLogs (follow=true, returns io.ReadCloser, fake-clientset tested); run-loop glue still TODO.

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
