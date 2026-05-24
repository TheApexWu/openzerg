#!/usr/bin/env bash
# serve-ui.sh — one-shot launcher for the OpenZerg web UI.
#
# What it does:
#   1. Builds ./backend/bin/openzerg if it is missing or stale.
#   2. Starts `openzerg serve` on :8080 (override with $PORT or --port).
#   3. Waits until /healthz returns 200.
#   4. Opens http://localhost:$PORT in your default browser (best-effort).
#   5. Tails the server log; Ctrl-C cleanly shuts it down.
#
# Usage:
#   ./scripts/serve-ui.sh                       # default :8080
#   PORT=9000 ./scripts/serve-ui.sh             # custom port via env
#   ./scripts/serve-ui.sh --port 9000           # custom port via flag
#   ./scripts/serve-ui.sh --frontend ./frontend # dev mode: bypass the embed
#                                               # and serve from disk so you
#                                               # can edit HTML/CSS/JS live.
#   ./scripts/serve-ui.sh --no-open             # don't open a browser
#
# The frontend is baked into the binary via go:embed, so the binary alone
# is sufficient — no external file tree is required at runtime.

set -euo pipefail

# Resolve repo root from this script's location so it works no matter where
# you invoke it from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN="${REPO_ROOT}/backend/bin/openzerg"

PORT="${PORT:-8080}"
FRONTEND_OVERRIDE=""
OPEN_BROWSER=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --frontend) FRONTEND_OVERRIDE="$2"; shift 2 ;;
    --no-open) OPEN_BROWSER=0; shift ;;
    -h|--help)
      sed -n '1,/^set -euo pipefail/p' "$0" | sed 's/^# \{0,1\}//' | sed '$d'
      exit 0
      ;;
    *)
      echo "serve-ui.sh: unknown flag $1" >&2
      exit 2
      ;;
  esac
done

# --- 1. Build if needed ---------------------------------------------------
need_build=0
if [[ ! -x "${BIN}" ]]; then
  need_build=1
else
  # Rebuild if any Go source is newer than the binary.
  if [[ -n "$(find "${REPO_ROOT}/backend" -name '*.go' -newer "${BIN}" -print -quit 2>/dev/null)" ]]; then
    need_build=1
  fi
  # Rebuild if any embedded frontend file is newer than the binary.
  if [[ -n "$(find "${REPO_ROOT}/backend/internal/api/frontend_embed" -type f -newer "${BIN}" -print -quit 2>/dev/null)" ]]; then
    need_build=1
  fi
fi

if [[ "${need_build}" -eq 1 ]]; then
  echo "==> building ./backend/bin/openzerg"
  (cd "${REPO_ROOT}/backend" && go build -o ./bin/openzerg ./cmd/openzerg)
else
  echo "==> using cached binary at ${BIN}"
fi

# --- 2. Start the server --------------------------------------------------
ADDR=":${PORT}"
LOG_FILE="$(mktemp -t openzerg-serve-XXXXXX.log)"

EXTRA_FLAGS=()
if [[ -n "${FRONTEND_OVERRIDE}" ]]; then
  EXTRA_FLAGS+=(--frontend "${FRONTEND_OVERRIDE}")
  echo "==> dev mode: serving frontend from ${FRONTEND_OVERRIDE}"
fi

echo "==> starting openzerg serve on ${ADDR}"
"${BIN}" serve --addr "${ADDR}" "${EXTRA_FLAGS[@]}" \
  > "${LOG_FILE}" 2>&1 &
SERVER_PID=$!

cleanup() {
  if kill -0 "${SERVER_PID}" 2>/dev/null; then
    echo ""
    echo "==> stopping openzerg serve (pid ${SERVER_PID})"
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -f "${LOG_FILE}"
}
trap cleanup EXIT INT TERM

# --- 3. Wait for /healthz -------------------------------------------------
HEALTH_URL="http://localhost:${PORT}/healthz"
for attempt in {1..40}; do
  if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
    echo "!! server exited before becoming healthy. Log:" >&2
    cat "${LOG_FILE}" >&2
    exit 1
  fi
  sleep 0.25
done

if ! curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
  echo "!! /healthz never returned 200 after 10s. Log:" >&2
  cat "${LOG_FILE}" >&2
  exit 1
fi

URL="http://localhost:${PORT}"
echo "==> ready: ${URL}"
echo "    healthz: ${HEALTH_URL}"
echo "    events:  ${URL}/api/events  (SSE)"
echo "    log:     ${LOG_FILE}"

# --- 4. Open browser (best-effort) ----------------------------------------
if [[ "${OPEN_BROWSER}" -eq 1 ]]; then
  if command -v xdg-open >/dev/null 2>&1; then
    (xdg-open "${URL}" >/dev/null 2>&1 &) || true
  elif command -v open >/dev/null 2>&1; then
    (open "${URL}" >/dev/null 2>&1 &) || true
  else
    echo "    (no xdg-open / open found; visit ${URL} manually)"
  fi
fi

# --- 5. Tail the log until Ctrl-C -----------------------------------------
echo "==> Ctrl-C to stop. tailing server log:"
echo "----------------------------------------------------------------"
tail -f "${LOG_FILE}" &
TAIL_PID=$!
wait "${SERVER_PID}"
kill "${TAIL_PID}" 2>/dev/null || true
