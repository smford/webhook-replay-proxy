# Examples & Test Fixtures

This directory contains production-ready test fixtures and GitHub Actions workflows for deterministic webhook testing using `webhook-replay-proxy`.

## Directory Layout

```text
examples/
├── fixtures/
│   ├── github_pull_request.json          # GitHub pull_request webhook payload & headers
│   ├── slack_event_callback.json         # Slack app_mention event callback
│   ├── stripe_checkout_completed.json    # Stripe checkout.session.completed event
│   └── svix_invoice_paid.json            # Standard / Svix invoice.paid webhook
└── workflows/
    ├── concurrency-idempotency-storm.yml # High-concurrency race condition testing
    ├── security-chaos-testing.yml        # HMAC corruption, malformed JSON, and clock drift
    ├── setup-and-custom-script.yml       # Using install-only with custom scripts
    └── webhook-regression-test.yml       # Complete end-to-end regression suite
```

## Replaying Fixtures Locally

Replay any fixture against your local development server with fresh cryptographic HMAC signatures:

```bash
# Stripe with payload mutation
webhook-replay-proxy replay \
  --file examples/fixtures/stripe_checkout_completed.json \
  --target http://localhost:3000/api/webhooks/stripe \
  --secret whsec_your_dev_secret \
  --set data.object.amount_total=999999 \
  --diff

# GitHub pull request
webhook-replay-proxy replay \
  --file examples/fixtures/github_pull_request.json \
  --target http://localhost:3000/api/webhooks/github \
  --secret your_github_webhook_secret \
  --set pull_request.merged=true

# Svix / Standard Webhook with concurrency burst
webhook-replay-proxy replay \
  --file examples/fixtures/svix_invoice_paid.json \
  --target http://localhost:3000/api/webhooks/svix \
  --secret your_svix_secret \
  --concurrency 10 \
  --assert-status "200:1,409:9"
```

## Using in GitHub Actions

Reference these fixtures directly in your GitHub Actions workflows:

```yaml
- name: Test Webhook Idempotency
  uses: smford/webhook-replay-proxy@v0.2.1
  with:
    file: examples/fixtures/stripe_checkout_completed.json
    target: http://localhost:3000/api/webhooks/stripe
    secret: ${{ secrets.STRIPE_WEBHOOK_SECRET }}
    concurrency: 10
    assert-status: "200:1,409:9"
```
