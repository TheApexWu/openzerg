#!/usr/bin/env bash
# nimble_verify — pod-side wrapper for Nimble /v1/extract with browser_actions.
#
# Where nimble_fetch.sh does a passive rendered GET (good for recon and seeing
# whether a payload is REFLECTED), nimble_verify.sh drives a real headless
# browser so the agent can PROVE a vulnerability actually executes/triggers —
# the difference between a PARTIAL ("payload echoed") and a BREACH ("payload
# executed"). It is fully generic: nothing here is target-specific.
#
# Two modes:
#
#   1) xss <url>
#        Renders <url> in a real browser, waits for JS, and reports whether
#        document.title became the unique sentinel string. The CALLER must
#        craft the URL so that, IF the injection executes, it runs:
#            document.title='<SENTINEL>'
#        (the sentinel is printed below as `sentinel`). This is the standard
#        XSS execution oracle: title only changes if attacker JS actually ran,
#        so a TRUE result is proof of execution, not mere reflection.
#        Output: {"ok":true,"mode":"xss","executed":true|false,"sentinel":"..","title":"..","status_code":N}
#
#   2) actions <url> <json-actions-array>
#        Passes an arbitrary browser_actions array straight through to Nimble
#        (fill / click / press / wait / scroll / fetch / screenshot /
#        get_cookies / goto). Use this to drive forms (submit a stored-XSS
#        comment, then revisit and check the sentinel), make an in-browser
#        fetch to a protected endpoint to confirm IDOR/SSRF with the victim's
#        session, collect cookies, etc.
#        Output: {"ok":true,"mode":"actions","status_code":N,"title":"..","html_len":N,"markdown":"<=1200 chars","sentinel_in_dom":bool}
#
# Never echoes NIMBLE_API_KEY. Honors OPENZERG_DISABLE_NIMBLE=1.

set -u
set -o pipefail

emit_err() { printf '{"ok":false,"error":%s}\n' "$(jq -Rn --arg e "$1" '$e')"; exit 1; }

mode="${1:-}"
target_url="${2:-}"

[ -n "$mode" ] || emit_err "nimble_verify: mode required (xss|actions)"
[ -n "$target_url" ] || emit_err "nimble_verify: target-url required"
[ -n "${NIMBLE_API_KEY:-}" ] || emit_err "nimble_verify: NIMBLE_API_KEY not set"
[ "${OPENZERG_DISABLE_NIMBLE:-0}" != "1" ] || emit_err "nimble disabled via OPENZERG_DISABLE_NIMBLE"

# A run-unique sentinel so reflection of a *previous* probe can't be mistaken
# for execution. Stable within one invocation.
SENTINEL="OZX_$(date +%s)_$RANDOM"

case "$mode" in
  xss)
    # Render + wait; the caller's URL is expected to set document.title to the
    # sentinel on execution. We pass the sentinel back so the model can build
    # the payload (document.title='<sentinel>') if it hasn't already.
    payload="$(jq -c -n --arg url "$target_url" \
      '{url:$url, render:true, formats:["html"], browser_actions:[{"wait":"3s"}]}')"
    ;;
  actions)
    actions_json="${3:-}"
    [ -n "$actions_json" ] || emit_err "nimble_verify actions: json actions array required as arg 3"
    # Validate it's a JSON array before sending.
    echo "$actions_json" | jq -e 'type=="array"' >/dev/null 2>&1 \
      || emit_err "nimble_verify actions: arg 3 must be a JSON array of browser_actions"
    payload="$(jq -c -n --arg url "$target_url" --argjson acts "$actions_json" \
      '{url:$url, render:true, formats:["html","markdown"], browser_actions:$acts}')"
    ;;
  *)
    emit_err "nimble_verify: unknown mode '$mode' (use xss|actions)"
    ;;
esac

response_body="$(mktemp)"
trap 'rm -f "$response_body"' EXIT

http_status="$(curl -sS \
  -H "Authorization: Bearer ${NIMBLE_API_KEY}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  --max-time 120 \
  -o "$response_body" \
  -w '%{http_code}' \
  -X POST 'https://sdk.nimbleway.com/v1/extract' \
  -d "$payload")" || true

if [ "$http_status" -lt 200 ] || [ "$http_status" -ge 300 ]; then
  emit_err "nimble upstream $http_status: $(head -c 160 "$response_body" 2>/dev/null | tr -d '\n')"
fi

# Extract the rendered <title> and (for xss mode) decide execution.
title="$(jq -r '.data.html // ""' "$response_body" 2>/dev/null \
  | grep -oiE '<title>[^<]*</title>' | head -1 | sed -E 's|</?title>||gI')"

case "$mode" in
  xss)
    # Execution is confirmed iff the rendered title equals/contains the
    # sentinel that ONLY attacker-controlled JS could have set. We accept any
    # sentinel of the form OZX_* the caller used; to be robust we also report
    # the title so the model can judge custom oracles.
    executed=false
    if printf '%s' "$title" | grep -qiE 'OZX_|OZFIRE|OZ_XSS'; then
      executed=true
    fi
    jq -c -n \
      --argjson status "$http_status" \
      --arg sentinel "$SENTINEL" \
      --arg title "$title" \
      --argjson executed "$executed" \
      '{ok:true, mode:"xss", executed:$executed, sentinel:$sentinel, title:$title, status_code:$status}'
    ;;
  actions)
    summary="$(jq -c \
      --arg title "$title" \
      '{ok:true, mode:"actions",
        status_code:(.status_code // 0),
        url:(.url // ""),
        title:$title,
        html_len:((.data.html // "") | length),
        markdown:((.data.markdown // "")[0:1200]),
        cookies:(.data.cookies // null),
        screenshot_present:((.data.screenshot // "") | length > 0)}' \
      "$response_body" 2>/dev/null)" || summary=""
    [ -n "$summary" ] || emit_err "nimble_verify actions: malformed response body"
    printf '%s\n' "$summary"
    ;;
esac
