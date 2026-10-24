---
title: Cheat Sheet & Decision Matrix
description: Quick-reference card and decision guide for common webhook testing tasks.
---

# Cheat Sheet & Decision Matrix

Find the right `webhook-replay-proxy` command and flags for your specific testing goal.

---

## Decision Matrix: What do you want to test?

| Testing Objective | Command / Flags to Use | Example Command |
|---|---|---|
| **Capture traffic passively** | `listen --forward-to <url>` | `webhook-replay-proxy listen --port 8080 --forward-to http://localhost:3000/webhook` |
| **Inspect captured payload** | `inspect <event_id>` | `webhook-replay-proxy inspect evt_123` |
| **Test race conditions / duplicate webhooks** | `replay --concurrency <N>` | `webhook-replay-proxy replay evt_123 --concurrency 10 --assert-status "200:1,409:9"` |
| **Test unauthorized signature rejection** | `replay --corrupt-sig` | `webhook-replay-proxy replay evt_123 --secret whsec_test --corrupt-sig --assert-status 401` |
| **Test parser error handling** | `replay --corrupt-json` | `webhook-replay-proxy replay evt_123 --corrupt-json --assert-status 400` |
| **Test clock drift / expired timestamps** | `replay --time-offset <duration>` | `webhook-replay-proxy replay evt_123 --secret whsec_test --time-offset -10m --assert-status 401` |
| **Test missing header defense** | `replay --drop-header <name>` | `webhook-replay-proxy replay evt_123 --drop-header Stripe-Signature --assert-status 401` |
| **Mutate nested JSON fields** | `replay --set key.path=val --diff` | `webhook-replay-proxy replay evt_123 --set data.object.status=failed --diff` |
| **Save test case to Git** | `export <event_id> -o <path>` | `webhook-replay-proxy export evt_123 -o test/fixtures/checkout.json` |
| **Run in CI without database** | `replay --file <fixture>` | `webhook-replay-proxy replay --file fixtures/checkout.json --target http://localhost:3000/webhook` |
| **Copy curl command** | `curl <event_id>` | `webhook-replay-proxy curl evt_123 --secret whsec_test --set amount=500` |
| **Visual browser dashboard** | `listen --ui` | `webhook-replay-proxy listen --ui` &rarr; open `http://localhost:8080/_dashboard` |

---

## Common Flag Combinations

### Concurrency Stress Test with Assertion
```bash
webhook-replay-proxy replay evt_123 \
  --concurrency 10 \
  --assert-status "200:1,409:9" \
  --max-latency 150ms
```

### Full Security & Parser Negative Test
```bash
# 1. Invalid signature test
webhook-replay-proxy replay evt_123 --secret whsec_key --corrupt-sig --assert-status 401

# 2. Malformed JSON test
webhook-replay-proxy replay evt_123 --corrupt-json --assert-status 400

# 3. Replay attack / expired timestamp test
webhook-replay-proxy replay evt_123 --secret whsec_key --time-offset -15m --assert-status 401
```

### Standalone CI Smoke Test
```bash
webhook-replay-proxy replay \
  --file test/fixtures/order.json \
  --target http://localhost:3000/api/webhooks \
  --secret ${{ secrets.WEBHOOK_SECRET }} \
  --assert-status 200
```
