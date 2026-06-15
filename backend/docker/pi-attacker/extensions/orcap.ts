// orcap — OpenRouter provider with an ENFORCED output-token cap.
//
// Why this exists: pi sends `max_tokens: 65536` in the actual chat request for
// qwen/qwen3.7-plus regardless of the models.json `maxTokens` override (that
// override only changes the capability display, not the wire request). When the
// OpenRouter account balance is thin, OpenRouter rejects any request whose
// max_tokens it cannot pre-authorize with HTTP 402, killing every probe.
//
// Registering the model under a CUSTOM provider is the documented mechanism
// (docs/custom-provider.md) where the model's `maxTokens` IS sent on the wire.
// We point it at OpenRouter's OpenAI-compatible endpoint, reuse the same
// OPENROUTER_API_KEY, keep supportsDeveloperRole=false (Alibaba upstream 400
// fix), and cap output at 8192 — plenty for a focused probe + the one-line JSON
// result, and small enough to clear OpenRouter's pre-flight on a tight budget.
//
// The entrypoint selects this with `--provider orcap --model qwen/qwen3.7-plus`.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  pi.registerProvider("orcap", {
    name: "OpenRouter (capped)",
    // Point at the in-pod maxtokens_proxy (tools/maxtokens_proxy.js), which the
    // entrypoint starts on 127.0.0.1:8799. It forwards to openrouter.ai but
    // clamps max_tokens down to a safe cap, because pi hard-sends 65536 which
    // 402s on a thin balance. Override with MAXTOK_PROXY_PORT if needed.
    baseUrl: "http://127.0.0.1:8799/api/v1",
    apiKey: "$OPENROUTER_API_KEY",
    api: "openai-completions",
    authHeader: true,
    compat: {
      supportsDeveloperRole: false,
      // qwen models take thinking control via a top-level enable_thinking flag,
      // not reasoning_effort. Tell pi to use the qwen format and that the model
      // has no reasoning_effort, so defaultThinkingLevel:"off" actually
      // disables reasoning instead of leaking ~870 reasoning tokens/call.
      thinkingFormat: "qwen",
      supportsReasoningEffort: false,
    },
    models: [
      {
        id: "qwen/qwen3.7-plus",
        name: "Qwen 3.7 Plus (capped)",
        reasoning: false,
        input: ["text", "image"],
        contextWindow: 128000,
        maxTokens: 16384,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        compat: {
          supportsDeveloperRole: false,
        },
      },
    ],
  });
}
