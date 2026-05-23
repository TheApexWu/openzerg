#!/usr/bin/env bash
# nimble_fetch — pod-side shell wrapper for Nimble /v1/extract.
#
# Usage from inside the attacker pod (the Pi skill instructs the model to
# call this via bash):
#
#   ./tools/nimble_fetch.sh <target-url>
#
# Behaviour:
#   - Reads NIMBLE_API_KEY from the env (injected by the openzerg-keys k8s
#     Secret via envFrom on the pod spec).
#   - POSTs to https://sdk.nimbleway.com/v1/extract with render=true so JS
#     SPAs (e.g., Juice Shop on Angular) come back as fully-rendered HTML.
#   - On success, prints a JSON object with shape:
#       {"ok":true,"status_code":<int>,"html_len":<int>,"markdown":"<snip>"}
#     The full HTML is *not* printed (too big for the model context); the
#     model can ask for a follow-up shorter excerpt via curl or grep.
#   - On any failure (missing key, network, non-2xx, malformed body) prints
#     a one-line JSON error: {"ok":false,"error":"<reason>"} and exits 1.
#
# The script NEVER echoes NIMBLE_API_KEY to stdout/stderr. It is only set in
# the curl Authorization header.

set -u
set -o pipefail

target_url="${1:-}"
if [ -z "$target_url" ]; then
  printf '{"ok":false,"error":"nimble_fetch: target-url required"}\n'
  exit 1
fi

if [ -z "${NIMBLE_API_KEY:-}" ]; then
  printf '{"ok":false,"error":"nimble_fetch: NIMBLE_API_KEY not set"}\n'
  exit 1
fi

if [ "${OPENZERG_DISABLE_NIMBLE:-0}" = "1" ]; then
  printf '{"ok":false,"error":"nimble disabled via OPENZERG_DISABLE_NIMBLE"}\n'
  exit 1
fi

payload="$(jq -c -n --arg url "$target_url" \
  '{url: $url, render: true, formats: ["html","markdown"]}')"

http_status=0
response_body="$(mktemp)"
trap 'rm -f "$response_body"' EXIT

# --fail-with-body is curl 7.76+; node:22-bookworm-slim ships ≥7.88.
# -w writes the HTTP status to stdout while -o sends the body to a temp file
# so we can shape the final report without re-fetching.
http_status="$(curl -sS \
  -H "Authorization: Bearer ${NIMBLE_API_KEY}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  --max-time 45 \
  -o "$response_body" \
  -w '%{http_code}' \
  -X POST 'https://sdk.nimbleway.com/v1/extract' \
  -d "$payload")" || true

if [ "$http_status" -lt 200 ] || [ "$http_status" -ge 300 ]; then
  err="$(jq -c -n \
    --arg http_status "$http_status" \
    --arg snippet "$(head -c 200 "$response_body" 2>/dev/null || true)" \
    '{ok:false, error:("nimble upstream " + $http_status), body_snippet:$snippet}')"
  printf '%s\n' "$err"
  exit 1
fi

# Render shape: {url, status_code, data:{html, markdown}}. We summarise so
# stdout stays bounded and the model can fit the result in its context.
summary="$(jq -c '
  {ok: true,
   status_code: .status_code,
   url: .url,
   html_len: (.data.html // "" | length),
   markdown: ((.data.markdown // "")[0:1200])}
' "$response_body" 2>/dev/null)" || summary=""

if [ -z "$summary" ]; then
  printf '{"ok":false,"error":"nimble_fetch: malformed response body"}\n'
  exit 1
fi

printf '%s\n' "$summary"
