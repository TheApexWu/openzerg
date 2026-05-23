#!/usr/bin/env bash
# OpenZerg production run script.
#
# This is the canonical demo invocation: one evolutionary swarm against the
# authorized OWASP Juice Shop instance on Railway. Defaults are tuned for
# the recorded demo (pop=3, gens=2, ~90s wall clock, gen-1 BREACH expected
# via sqli_login).
#
# Prereqs (one-time):
#   1. .env at repo root with OPENROUTER_API_KEY and NIMBLE_API_KEY.
#   2. kubectl context pointed at the DO cluster (kubeconfig path below).
#   3. openzerg namespace + openzerg-keys Secret already created:
#        kubectl create namespace openzerg
#        kubectl -n openzerg create secret generic openzerg-keys \
#          --from-env-file=.env
#   4. pi-attacker image pushed:
#        bash scripts/build-and-push-attacker.sh
#   5. backend/bin/openzerg built:
#        cd backend && go build -o ./bin/openzerg ./cmd/openzerg
#
# Override any of these via env, e.g.:
#   POPULATION=5 GENERATIONS=3 bash scripts/run-prod.sh
#   TARGET=https://other-authorized-target bash scripts/run-prod.sh

set -euo pipefail

# --- repo root ---------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# --- config (overridable) ----------------------------------------------------
TARGET="${TARGET:-https://juice-shop-production-d0c5.up.railway.app}"
POPULATION="${POPULATION:-3}"
GENERATIONS="${GENERATIONS:-2}"
OUT_DIR="${OUT_DIR:-./out}"
KUBECONFIG_PATH="${KUBECONFIG:-$REPO_ROOT/k8s-1-36-0-do-0-nyc1-1779544226353-kubeconfig.yaml}"
BIN="${BIN:-$REPO_ROOT/backend/bin/openzerg}"

# --- pre-flight --------------------------------------------------------------
if [ ! -x "$BIN" ]; then
  echo "error: openzerg binary not found at $BIN" >&2
  echo "build it first: (cd backend && go build -o ./bin/openzerg ./cmd/openzerg)" >&2
  exit 1
fi
if [ ! -f "$KUBECONFIG_PATH" ]; then
  echo "error: kubeconfig not found at $KUBECONFIG_PATH" >&2
  echo "set KUBECONFIG=/path/to/kubeconfig.yaml and re-run" >&2
  exit 1
fi
mkdir -p "$OUT_DIR"

export KUBECONFIG="$KUBECONFIG_PATH"

# --- doctor (fast, no cluster mutation) --------------------------------------
echo "=== openzerg doctor ==="
"$BIN" doctor
echo

# --- run ---------------------------------------------------------------------
echo "=== openzerg run ==="
echo "target:      $TARGET"
echo "population:  $POPULATION"
echo "generations: $GENERATIONS"
echo "out dir:     $OUT_DIR"
echo

exec "$BIN" run \
  --target "$TARGET" \
  --population "$POPULATION" \
  --generations "$GENERATIONS" \
  --out-dir "$OUT_DIR"
