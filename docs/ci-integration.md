---
title: CI/CD Integration
description: Automated webhook idempotency testing and race condition regression checks in CI pipelines.
---

# CI/CD Integration

`webhook-replay-proxy` enables automated webhook regression testing directly inside your CI/CD pipelines. Using checked-in JSON fixtures and status distribution assertions (`--assert-status`), you can guarantee that race conditions and idempotency bugs never hit production.

---

## 1. GitHub Actions Workflow

Here is a production-ready GitHub Actions workflow that:
1. Starts your backend service in the background.
2. Installs `webhook-replay-proxy`.
3. Runs simultaneous concurrency bursts using checked-in fixtures.
4. Asserts that idempotency locks prevent double-processing.

```yaml
name: Webhook Regression Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  webhook-test:
    name: Idempotency & Security Verification
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Install Webhook Replay Proxy
        run: |
          go install github.com/smford/webhook-replay-proxy/cmd/webhook-replay-proxy@latest

      - name: Start Application Service
        run: |
          npm install && npm start &
          # Wait for application to be healthy
          npx wait-on http://localhost:3000/healthz --timeout 30000

      - name: Test 1: Concurrency Idempotency Burst (10 simultaneous requests)
        run: |
          webhook-replay-proxy replay \
            --file test/fixtures/stripe_checkout_completed.json \
            --target http://localhost:3000/api/webhooks/stripe \
            --secret ${{ secrets.STRIPE_DEV_WEBHOOK_SECRET }} \
            --concurrency 10 \
            --assert-status "200:1,409:9" \
            --max-latency 250ms

      - name: Test 2: Security Validation (Tampered Signature Rejection)
        run: |
          webhook-replay-proxy replay \
            --file test/fixtures/stripe_checkout_completed.json \
            --target http://localhost:3000/api/webhooks/stripe \
            --secret ${{ secrets.STRIPE_DEV_WEBHOOK_SECRET }} \
            --corrupt-sig \
            --assert-status 401

      - name: Test 3: Anti-Replay Clock Drift Protection
        run: |
          webhook-replay-proxy replay \
            --file test/fixtures/stripe_checkout_completed.json \
            --target http://localhost:3000/api/webhooks/stripe \
            --secret ${{ secrets.STRIPE_DEV_WEBHOOK_SECRET }} \
            --time-offset -10m \
            --assert-status 401
```

---

## 2. Docker Compose Integration

For teams testing full microservice architectures locally or in test environments:

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

## 3. GitLab CI Pipeline

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
