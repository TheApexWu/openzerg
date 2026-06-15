#!/usr/bin/env node
// maxtokens_proxy.js — a tiny in-pod reverse proxy for OpenRouter that clamps
// the `max_tokens` field of every chat-completions request down to a safe cap.
//
// Why: pi hard-sends `max_tokens: 65536` for qwen/qwen3.7-plus regardless of any
// models.json / custom-provider config. When the OpenRouter balance is thin,
// OpenRouter rejects requests it can't pre-authorize for 65536 tokens with HTTP
// 402, killing every probe — even though a smaller request (<=16k) succeeds and
// bills normally. This proxy sits between pi and OpenRouter, rewrites max_tokens
// down to MAXTOK (default 8192), and forwards everything else untouched. pi's
// `orcap` provider points its baseUrl at http://127.0.0.1:PORT/api/v1.
//
// Zero deps (Node stdlib only). Streams request/response bodies. Buffers only
// JSON chat-completions bodies (small) to rewrite them.
const http = require("http");
const https = require("https");

const PORT = parseInt(process.env.MAXTOK_PROXY_PORT || "8799", 10);
const MAXTOK = parseInt(process.env.MAXTOK_CAP || "8192", 10);
const UPSTREAM_HOST = "openrouter.ai";

const server = http.createServer((req, res) => {
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    let body = Buffer.concat(chunks);

    // Rewrite max_tokens on chat/completions JSON bodies.
    const isChat = req.url.includes("/chat/completions") && body.length > 0;
    if (isChat) {
      try {
        const obj = JSON.parse(body.toString("utf8"));
        if (typeof obj.max_tokens !== "number" || obj.max_tokens > MAXTOK) {
          obj.max_tokens = MAXTOK;
        }
        if (typeof obj.max_completion_tokens === "number" && obj.max_completion_tokens > MAXTOK) {
          obj.max_completion_tokens = MAXTOK;
        }
        body = Buffer.from(JSON.stringify(obj), "utf8");
      } catch (_e) {
        // not JSON we understand; forward as-is
      }
    }

    const headers = { ...req.headers, host: UPSTREAM_HOST };
    headers["content-length"] = Buffer.byteLength(body);

    const upstream = https.request(
      {
        host: UPSTREAM_HOST,
        port: 443,
        method: req.method,
        path: req.url,
        headers,
      },
      (up) => {
        res.writeHead(up.statusCode || 502, up.headers);
        up.pipe(res);
      }
    );
    upstream.on("error", (err) => {
      res.writeHead(502, { "content-type": "application/json" });
      res.end(JSON.stringify({ error: { message: "proxy upstream error: " + err.message } }));
    });
    upstream.end(body);
  });
});

server.listen(PORT, "127.0.0.1", () => {
  process.stderr.write(`[maxtokens_proxy] listening on 127.0.0.1:${PORT} -> ${UPSTREAM_HOST}, cap=${MAXTOK}\n`);
});
