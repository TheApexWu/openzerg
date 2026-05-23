# Manual Input Queue

The ralph loop appends requests here when it needs information from the human.
The loop **halts** while any `- [ ]` (unchecked) line exists under the
`## Open requests` heading below.

To unblock: resolve the request (set the env var, make the decision, push the
image, etc.), then flip its checkbox from empty to `x`. The loop polls every
few seconds and resumes on the next iteration.

Format used by the agent when appending (illustrated WITHOUT a real checkbox
so it does not count as an open request):

    DASH SPACE [SPACE] <ISO-timestamp> M<n> — <short, specific request>

Example, once filled in below the marker (this is the real shape the agent
writes):

    - [ ] 2026-05-23T18:42:00Z M3 — Put OPENROUTER_API_KEY into /home/carson/openzerg/.env

The loop only inspects content **below** the `## Open requests` marker, so
this doc section is safe to keep as-is.

---

## Open requests

<!-- The agent appends one `- [ ] ...` line per request below this comment.
     Leave the heading above in place; the loop's detector keys off of it. -->
