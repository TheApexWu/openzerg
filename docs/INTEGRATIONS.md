# OpenZerg Integrations

## Nimble API key

Do not put the Nimble API key in browser JavaScript and do not commit it to GitHub.

Set it locally:

```bash
cp .env.example .env
```

Edit `.env`:

```bash
NIMBLE_API_KEY=your_real_key_here
```

Then run the local dev server:

```bash
python3 scripts/dev_server.py
```

Open the prototype gallery from the printed local URL.

The frontend checks:

```txt
/api/integrations/nimble
```

The endpoint reports whether the key is configured without exposing the secret to the browser.

## Choosing the exact Nimble endpoint

The current proxy supports an optional `NIMBLE_API_URL` for the exact Nimble endpoint once selected from the Nimble docs/product flow.

```bash
NIMBLE_API_URL=https://...
```

Until that is set, the integration shows as configured/demo rather than making a live Nimble request.

## ClickHouse

Use the same pattern for ClickHouse credentials. Put them in `.env`, not frontend code.

```bash
CLICKHOUSE_HOST=
CLICKHOUSE_USER=
CLICKHOUSE_PASSWORD=
CLICKHOUSE_DATABASE=openzerg
```
