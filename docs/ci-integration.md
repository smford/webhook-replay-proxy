---
title: CI/CD Integration
description: Automated webhook idempotency testing, payload mutations, and race condition regression checks in CI pipelines.
---

# CI/CD Integration

`webhook-replay-proxy` is available as a native **GitHub Action** (`smford/webhook-replay-proxy`) to automate webhook regression and idempotency testing directly in your CI/CD pipelines.

No Go toolchain or manual compilation is required on runner environments—the action downloads the official precompiled release binary, adds it to `$GITHUB_PATH`, and can declaratively execute replays with cryptographic HMAC re-signing, payload mutations, and response distribution assertions (`--assert-status`).

---

## 1. Quickstart

Add the action as a step in your GitHub Actions workflow:

```yaml
- name: Test Stripe Webhook Idempotency Burst
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: test/fixtures/stripe_checkout_completed.json
    target: http://localhost:3000/api/webhooks/stripe
    secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
    concurrency: 10
    assert-status: "200:1,409:9"
    max-latency: 250ms
```

---

## 2. Action Reference

### Inputs

| Input | Type | Default | Description |
|---|---|---|---|
| `version` | string | `latest` | Version of `webhook-replay-proxy` to install (e.g. `latest`, `v0.2.1`, or `0.2.1`). |
| `install-only` | boolean | `false` | If `true`, only installs the CLI into `$GITHUB_PATH` without executing a command. |
| `command` | string | `replay` | Command to run (`replay`, `curl`, `inspect`, `list`, `export`, `version`). |
| `file` | string | `""` | Path to webhook fixture JSON file to replay (`--file`). |
| `event-id` | string | `""` | Recorded webhook event ID to replay from SQLite database. |
| `target` | string | `""` | Destination target URL override (`--target`). |
| `provider` | string | `auto` | Signing provider (`stripe`, `github`, `slack`, `shopify`, `standard`, `auto`). |
| `secret` | string | `""` | Webhook HMAC secret to re-sign payload with fresh timestamp (`--secret`). |
| `set` | string | `""` | Dot-path payload mutations (`key.path=value`), one per line (`--set`). |
| `concurrency` | integer | `1` | Number of simultaneous concurrent requests (`--concurrency`). |
| `delay` | string | `""` | Delay between paced sequential requests (`--delay`, e.g. `100ms`, `1s`). |
| `diff` | boolean | `false` | Display visual diff of mutated JSON payload before replaying (`--diff`). |
| `corrupt-sig` | boolean | `false` | Deliberately corrupt HMAC signature to verify 401 rejection (`--corrupt-sig`). |
| `corrupt-json` | boolean | `false` | Deliberately malform payload JSON to test error handling (`--corrupt-json`). |
| `drop-header` | string | `""` | Strip specified header before dispatching, one per line (`--drop-header`). |
| `time-offset` | string | `""` | Offset signing timestamp from current time (`--time-offset`, e.g. `-10m`, `+1h`). |
| `fixed-timestamp` | integer | `0` | Use an exact fixed unix epoch timestamp for signing (`--fixed-timestamp`). |
| `assert-status` | string | `""` | Assert response status distribution (`--assert-status`, e.g. `'200:1,409:9'`). |
| `max-latency` | string | `""` | Fail assertion if any request latency exceeds threshold (`--max-latency`, e.g. `250ms`). |
| `db` | string | `""` | Path to SQLite database (`--db`). |
| `args` | string | `""` | Additional arbitrary CLI arguments to pass. |
| `github-token` | string | `${{ github.token }}` | GitHub token for authenticated release asset API queries. |

### Outputs

| Output | Type | Description |
|---|---|---|
| `installed-version` | string | Clean version string of the installed binary (e.g. `0.2.1`). |
| `binary-path` | string | Absolute path to the installed executable binary. |

---

## 3. Worked Examples

### Example 1: Production Webhook Regression Workflow

This complete workflow launches a local Node.js or Go microservice in the background, waits for its health check endpoint, and runs four distinct verification suites:

```yaml
name: Webhook Regression Suite

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  webhook-test:
    name: Webhook Idempotency & Security Verification
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Start Application Service
        run: |
          npm ci && npm start &
          # Wait for application health check endpoint to be ready
          npx wait-on http://localhost:3000/healthz --timeout 30000

      # Test 1: Concurrency burst with 10 simultaneous requests
      - name: Test 1: Idempotency Barrier (10 Concurrent Requests)
        uses: smford/webhook-replay-proxy@v0.2.1
        with:
          file: examples/fixtures/stripe_checkout_completed.json
          target: http://localhost:3000/api/webhooks/stripe
          secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
          concurrency: 10
          assert-status: "200:1,409:9"
          max-latency: 250ms

      # Test 2: Ensure tampered signature returns 401 Unauthorized
      - name: Test 2: Security Verification (Tampered HMAC Signature)
        uses: smford/webhook-replay-proxy@v0.2.1
        with:
          file: examples/fixtures/stripe_checkout_completed.json
          target: http://localhost:3000/api/webhooks/stripe
          secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
          corrupt-sig: true
          assert-status: "401"

      # Test 3: Anti-replay clock drift protection (timestamp 10m in past)
      - name: Test 3: Anti-Replay Verification (Expired Timestamp)
        uses: smford/webhook-replay-proxy@v0.2.1
        with:
          file: examples/fixtures/stripe_checkout_completed.json
          target: http://localhost:3000/api/webhooks/stripe
          secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
          time-offset: -10m
          assert-status: "401"

      # Test 4: Dynamic payload mutation (test unpaid checkout session)
      - name: Test 4: Payload Mutation (Unpaid Session Handling)
        uses: smford/webhook-replay-proxy@v0.2.1
        with:
          file: examples/fixtures/stripe_checkout_completed.json
          target: http://localhost:3000/api/webhooks/stripe
          secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
          set: |
            data.object.payment_status=unpaid
            data.object.amount_total=999999
          diff: true
          assert-status: "200"
```

---

### Example 2: Idempotency Barrier Stress Testing

Test database row locking, Redis mutex locks, or distributed idempotency keys under load. The concurrency barrier ensures that all $N$ requests hit your server at the exact same millisecond:

```yaml
- name: Concurrency Burst (25 Requests at the Same Millisecond)
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/svix_invoice_paid.json
    target: http://localhost:8080/webhooks/svix
    secret: ${{ secrets.SVIX_WEBHOOK_SECRET }}
    concurrency: 25
    assert-status: "200:1,409:24"
    max-latency: 350ms
```

---

### Example 3: Security & Chaos Verification Auditing

Verify that all webhook security guards properly reject corrupted headers, tampered payloads, and missing signatures:

```yaml
# 1. Corrupt signature test
- name: Verify Rejection of Corrupted Signature
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/github_pull_request.json
    target: http://localhost:8080/api/webhooks/github
    secret: ${{ secrets.GH_WEBHOOK_SECRET }}
    corrupt-sig: true
    assert-status: "401"

# 2. Corrupt JSON syntax test
- name: Verify Rejection of Malformed JSON Syntax
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/github_pull_request.json
    target: http://localhost:8080/api/webhooks/github
    secret: ${{ secrets.GH_WEBHOOK_SECRET }}
    corrupt-json: true
    assert-status: "400"

# 3. Missing signature header test
- name: Verify Rejection When Signature Header is Stripped
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/stripe_checkout_completed.json
    target: http://localhost:8080/api/webhooks/stripe
    drop-header: Stripe-Signature
    assert-status: "401"
```

---

### Example 4: Setup-Only Mode for Custom Scripts & Makefiles

If you prefer writing custom bash scripts, invoking `Makefile` targets, or testing multiple fixtures in a loop, configure `install-only: true`:

```yaml
- name: Setup Webhook Replay Proxy
  id: setup-proxy
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    install-only: true

- name: Run Test Matrix Across Multiple Fixtures
  run: |
    echo "Running tests with version: ${{ steps.setup-proxy.outputs.installed-version }}"

    for fixture in examples/fixtures/*.json; do
      echo "Testing fixture: $fixture"
      webhook-replay-proxy replay \
        --file "$fixture" \
        --target "http://localhost:3000/webhooks" \
        --secret "test_secret_123" \
        --concurrency 5 \
        --assert-status 200
    done
```

---

### Example 5: Multi-Provider Webhook Testing

Test multiple third-party webhook integrations in a single pipeline:

```yaml
# Stripe
- name: Test Stripe Webhook
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/stripe_checkout_completed.json
    target: http://localhost:3000/webhooks/stripe
    secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
    assert-status: 200

# GitHub
- name: Test GitHub Webhook
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/github_pull_request.json
    target: http://localhost:3000/webhooks/github
    secret: ${{ secrets.GITHUB_WEBHOOK_SECRET }}
    set: pull_request.merged=true
    assert-status: 200

# Slack
- name: Test Slack Webhook
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/slack_event_callback.json
    target: http://localhost:3000/webhooks/slack
    secret: ${{ secrets.SLACK_SIGNING_SECRET }}
    assert-status: 200

# Svix / Standard Webhooks
- name: Test Svix Webhook
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/svix_invoice_paid.json
    target: http://localhost:3000/webhooks/svix
    secret: ${{ secrets.SVIX_WEBHOOK_SECRET }}
    assert-status: 200
```

---

## 4. Docker Compose Integration

For teams testing full microservice architectures locally or in containerized test environments:

```yaml
version: '3.8'

services:
  backend:
    build: .
    ports:
      - "3000:3000"
    environment:
      - STRIPE_WEBHOOK_SECRET=whsec_local_docker_test_secret

  webhook-proxy:
    image: ghcr.io/smford/webhook-replay-proxy:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    command:
      - "listen"
      - "--port=8080"
      - "--forward-to=http://backend:3000/webhooks"
      - "--db=/data/webhooks.db"
      - "--ui"
```

---

## 5. GitLab CI Pipeline

```yaml
webhook_regression:
  image: golang:1.24
  stage: test
  services:
    - name: your-app-image:test
      alias: app
  script:
    - go install github.com/smford/webhook-replay-proxy/cmd/webhook-replay-proxy@latest
    - webhook-replay-proxy replay --file fixtures/order.json --target http://app:3000/webhook --concurrency 5 --assert-status "200:1,409:4"
```
