# webhook-replay-proxy

[![CI](https://github.com/smford/webhook-replay-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/smford/webhook-replay-proxy/actions/workflows/ci.yml)
[![GitHub Action](https://img.shields.io/badge/GitHub_Action-smford%2Fwebhook--replay--proxy-blue?logo=githubactions&logoColor=white)](https://github.com/smford/webhook-replay-proxy)
[![Release](https://img.shields.io/github/v/release/smford/webhook-replay-proxy?color=blue&logo=github)](https://github.com/smford/webhook-replay-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/smford/webhook-replay-proxy/badge)](https://securityscorecards.dev/viewer/?repo=github.com/smford/webhook-replay-proxy)
[![Documentation](https://img.shields.io/badge/Docs-md--web--renderer-blue.svg)](https://stephenford.org/webhook-replay-proxy/)

> **Deterministic local webhook recorder, SQLite flight recorder, payload mutator, and cryptographic HMAC re-signer for edge and cloud microservices.**

---

## What is webhook-replay-proxy?

`webhook-replay-proxy` is a single-binary, zero-CGO tool that sits between third-party webhook senders (such as **Stripe**, **GitHub**, **Slack**, **Shopify**, and **Svix / Standard Webhooks**) and your local backend.

When developing or debugging webhook integrations, capturing webhooks is only half the battle. If you replay an event 10 minutes later, your application rejects it because the timestamp expired. If you change a single character in the JSON body (e.g., testing a failed payment instead of a successful one), the HMAC signature breaks.

`webhook-replay-proxy` records every webhook into a local SQLite database (`webhooks.db`) and lets you replay them on demand with **fresh timestamps**, **mutated fields**, and **re-computed cryptographic signatures** so your application middleware accepts them every time.

---

## Problems It Solves & How It Solves Them

| The Real-World Problem | Why It Breaks Local Dev | How `webhook-replay-proxy` Solves It |
|---|---|---|
| **Expired Timestamps** | Modern providers (Stripe, Slack, Svix) sign a Unix timestamp in their headers (`Stripe-Signature: t=1690000000,v1=...`). SDKs enforce a 5-minute tolerance window. Replaying a captured webhook later fails with: `Timestamp outside the tolerance zone`. | On replay, generates a fresh current timestamp, formats the provider-specific payload string (`<ts>.<body bytes>`), and calculates a valid HMAC hash with your local secret. |
| **Payload Tampering Breaks Signatures** | Changing a single character in the JSON body (e.g. mutating `"amount": 2000` to `"amount": 999999` or `"status": "failed"`) invalidates the original signature. | Using `--set <key.path=value>`, mutates the JSON body in-memory and recalculates the exact HMAC-SHA256 signature matching the provider spec. |
| **Idempotency & Race Condition Bugs** | In production, webhook providers retry aggressively when timeouts happen. Does your application duplicate orders or deadlock under load? | Uses `--concurrency <N>` with a synchronized goroutine barrier (`sync.WaitGroup`) to release $N$ identical requests simultaneously at the exact same millisecond. |
| **Sharing Test Fixtures** | Sharing webhooks between team members requires manually copy-pasting curl headers and recalculating hashes. | Generates a ready-to-run curl command via `webhook-replay-proxy curl <event_id>` with all headers pre-calculated. |

---

## Architecture

![Architecture](docs/assets/architecture-diagram.svg)

1. **Capture Proxy (`listen`)**: Binds to `:8080`, logs incoming webhooks, and forwards traffic live to your local development server.
2. **SQLite Flight Recorder**: Pure-Go zero-CGO embedded database (`modernc.org/sqlite`) stores HTTP method, path, query, headers, raw body, latency, and response status.
3. **Payload Mutator**: Navigates dot-path notation (`--set data.object.amount=9999`) and typecasts strings, numbers, booleans, and nested structures.
4. **Cryptographic Re-Signer**: Generates valid signatures for Stripe, GitHub, Slack, Shopify, and Svix/Standard Webhooks.
5. **Concurrency Barrier**: Fires simultaneous requests to stress-test locks and race conditions.

---

## Worked Examples

### 1. Capture Webhooks in Real Time

```bash
webhook-replay-proxy listen --port 8080 --forward-to http://localhost:3000/webhooks
```

![Listen and Capture](docs/assets/capture-listen.svg)

### 2. Inspect Recorded Payload & Headers

```bash
webhook-replay-proxy inspect evt_179285514721
```

![Inspect Event](docs/assets/inspect-event.svg)

### 3. Replay with Mutations & Visual Diff

Mutate the payment status and amount, re-sign with your dev secret, and test 5 concurrent requests:

```bash
webhook-replay-proxy replay evt_179285514721 \
  --secret whsec_dev_local_secret \
  --set data.object.amount_total=999999 \
  --set data.object.payment_status=unpaid \
  --diff \
  --concurrency 5
```

![Replay Mutation Diff](docs/assets/replay-mutation-diff.svg)

### 4. Chaos Engineering, Time-Travel & Assertions

```bash
# Chaos: deliberately corrupt HMAC signature to ensure endpoint rejects with 401
webhook-replay-proxy replay evt_179285514721 --secret whsec_dev_key --corrupt-sig --assert-status 401

# Time-travel: test 5-minute freshness tolerance by setting timestamp 10m in the past
webhook-replay-proxy replay evt_179285514721 --secret whsec_dev_key --time-offset -10m

# CI Idempotency Assertion: verify exactly 1 succeeds and 9 get 409 conflict under storm
webhook-replay-proxy replay evt_179285514721 --concurrency 10 --assert-status "200:1,409:9" --max-latency 200ms
```

### 5. Export Fixtures & Standalone Replay

```bash
# Export to version-controlled JSON fixture
webhook-replay-proxy export evt_179285514721 -o fixtures/payment_intent.json

# Replay directly in CI without needing SQLite database
webhook-replay-proxy replay --file fixtures/payment_intent.json --target http://localhost:3000/webhook
```

### 6. Export Executable `curl` Command

```bash
webhook-replay-proxy curl evt_179285514721 \
  --secret whsec_dev_local_secret \
  --set data.object.amount_total=999999
```

Output:
```bash
curl -X POST "http://localhost:3000/webhooks" \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: t=1792855900,v1=4a5b6c7d8e9f..." \
  --data '{"data":{"object":{"amount_total":999999,"currency":"usd"}},"type":"checkout.session.completed"}'
```

### 7. Interactive Live Dashboard

```bash
webhook-replay-proxy listen --port 8080 --forward-to http://localhost:3000/webhook --ui
```
Open `http://localhost:8080/_dashboard` in your browser for a real-time event inspector, one-click replayer, and field editor.

---

## Supported Providers

| Provider | Signature Header | Signed Payload String | Output Encoding |
|---|---|---|---|
| **Stripe** | `Stripe-Signature: t=<ts>,v1=<hash>` | `<unix_ts>.<body_bytes>` | Hex |
| **GitHub** | `X-Hub-Signature-256: sha256=<hash>` | `<body_bytes>` | Hex |
| **Slack** | `X-Slack-Signature: v0=<hash>` | `v0:<unix_ts>:<body_bytes>` | Hex |
| **Shopify** | `X-Shopify-Hmac-Sha256: <hash>` | `<body_bytes>` | Base64 |
| **Standard / Svix** | `webhook-signature: v1,<hash>` | `<id>.<unix_ts>.<body_bytes>` | Base64 |

---

## Installation

### GitHub Action (CI/CD)

Use `webhook-replay-proxy` directly inside your GitHub Actions workflows without installing Go:

```yaml
# Replay webhook fixture with HMAC re-signing & idempotency storm
- name: Replay Webhook with Cryptographic Verification
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/stripe_checkout_completed.json
    target: http://localhost:3000/api/webhooks/stripe
    secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
    concurrency: 10
    assert-status: "200:1,409:9"
    max-latency: 250ms
```

Or use as a runner setup action to add `webhook-replay-proxy` to `$GITHUB_PATH`:

```yaml
- name: Setup Webhook Replay Proxy
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    install-only: true

- name: Run Custom Webhook Tests
  run: |
    webhook-replay-proxy version
    webhook-replay-proxy replay --file fixtures/event.json --target http://localhost:3000/webhook
```

See [examples/](examples/) and [CI/CD Integration](https://stephenford.org/webhook-replay-proxy/#/ci-integration) for complete worked workflows and fixtures.

### Homebrew (macOS / Linux)

```bash
brew install smford/homebrew-tap/webhook-replay-proxy
```

### Docker

```bash
docker pull ghcr.io/smford/webhook-replay-proxy:latest
docker run -d -p 8080:8080 -v $(pwd)/data:/etc/webhook-replay-proxy ghcr.io/smford/webhook-replay-proxy:latest
```

### Precompiled Standalone Binaries

Download signed standalone binaries for Darwin and Linux (`amd64`, `arm64`) from [GitHub Releases](https://github.com/smford/webhook-replay-proxy/releases).

---

## CLI Usage

```text
webhook-replay-proxy 0.2.0 (commit: dev, date: unknown)
Copyright (c) 2026 smford
Licensed under the MIT License
https://github.com/smford/webhook-replay-proxy

Usage:
  webhook-replay-proxy [command] [flags]

Commands:
  listen             Start webhook capture and forwarding server (default)
  list               List recorded webhooks from SQLite database
  inspect <event_id> Inspect recorded webhook payload, headers, and replay history
  export <event_id>  Export recorded webhook to a standalone JSON fixture file
  replay <event_id>  Replay a captured webhook with mutations and HMAC re-signing
  curl <event_id>    Generate ready-to-run curl command with fresh HMAC signature
  version            Show version information

Flags for 'listen':
  --port, -port <int>        Listen port (default: 8080)
  --forward-to <url>         Forward incoming webhooks to target URL
  --db <path>                Path to SQLite database (default: ./webhooks.db)
  --config <path>            Path to YAML configuration file
  --ui                       Enable embedded web dashboard at /_dashboard

Flags for 'list':
  --db <path>                Path to SQLite database (default: ./webhooks.db)
  --limit <int>              Maximum events to list (default: 50)

Flags for 'inspect':
  --db <path>                Path to SQLite database (default: ./webhooks.db)
  --raw                      Print raw unformatted payload body

Flags for 'export':
  --output, -o <path>        Output file path (default: stdout)
  --db <path>                Path to SQLite database (default: ./webhooks.db)

Flags for 'replay':
  --file <path>              Replay directly from a fixture JSON file without SQLite
  --target <url>             Override destination target URL
  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)
  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp
  --set <key.path=value>     Mutate JSON field (can be specified multiple times)
  --concurrency <int>        Number of concurrent requests fired simultaneously (default: 1)
  --delay <duration>         Delay between spaced requests (e.g. 100ms, 1s)
  --diff                     Display visual diff of mutated JSON payload before replaying
  --corrupt-sig              Deliberately corrupt HMAC signature to verify 401 rejection
  --corrupt-json             Deliberately malform payload JSON to test parser error handling
  --drop-header <name>       Strip specified header before dispatching (can be repeated)
  --time-offset <duration>   Offset signing timestamp from current time (e.g. -10m, +1h)
  --fixed-timestamp <unix>   Use an exact fixed unix epoch timestamp for signing
  --assert-status <pattern>  Assert response status distribution (e.g. '200:1,409:9' or '200')
  --max-latency <duration>   Fail assertion if any request latency exceeds threshold
  --db <path>                Path to SQLite database (default: ./webhooks.db)

Flags for 'curl':
  --file <path>              Generate curl directly from a fixture JSON file
  --target <url>             Override destination target URL
  --provider <name>          Provider signing scheme (stripe, github, slack, shopify, standard, auto)
  --secret <string>          Webhook HMAC secret to re-sign payload with fresh timestamp
  --set <key.path=value>     Mutate JSON field (can be specified multiple times)
  --corrupt-sig              Deliberately corrupt HMAC signature
  --corrupt-json             Deliberately malform payload JSON
  --drop-header <name>       Strip specified header from generated curl command
  --time-offset <duration>   Offset signing timestamp from current time
  --fixed-timestamp <unix>   Use an exact fixed unix epoch timestamp for signing
  --db <path>                Path to SQLite database (default: ./webhooks.db)
```

---

## Documentation

Full documentation, interactive guides, and recipes are available at [**https://stephenford.org/webhook-replay-proxy/**](https://stephenford.org/webhook-replay-proxy/):

- [**Getting Started Guide**](https://stephenford.org/webhook-replay-proxy/#/getting-started) — 5-minute setup and quickstart.
- [**Worked Examples**](https://stephenford.org/webhook-replay-proxy/#/worked-examples) — Real-world scenarios (Stripe payment failures, GitHub PR bots, Slack events, chaos engineering).
- [**Provider Recipes**](https://stephenford.org/webhook-replay-proxy/#/provider-recipes) — Signature formats and verification middleware for Stripe, GitHub, Slack, Shopify, and Svix.
- [**CI/CD Integration**](https://stephenford.org/webhook-replay-proxy/#/ci-integration) — Automated concurrency stress tests and regression suites for GitHub Actions and GitLab.
- [**Troubleshooting & Gotchas**](https://stephenford.org/webhook-replay-proxy/#/troubleshooting-gotchas) — Solving raw body middleware issues in Express, Next.js, FastAPI, and Go.
- [**Cheat Sheet & Decision Matrix**](https://stephenford.org/webhook-replay-proxy/#/cheat-sheet) — 1-page quick-reference card for common testing workflows.
- [**Architecture Deep Dive**](https://stephenford.org/webhook-replay-proxy/#/architecture) — Zero-CGO SQLite flight recorder and crypto signing engine.
- [**CLI Reference**](https://stephenford.org/webhook-replay-proxy/#/cli-reference) — Complete flag reference for all subcommands.

---

## License

Released under the [MIT License](LICENSE). Copyright &copy; 2026 Simon Ford.
