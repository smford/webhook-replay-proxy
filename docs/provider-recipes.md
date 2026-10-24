---
title: Provider Recipes
description: Step-by-step guides for testing webhooks from Stripe, GitHub, Slack, Shopify, and Svix.
---

# Provider Recipes

`webhook-replay-proxy` provides first-class cryptographic HMAC signing for all major webhook specifications. This guide provides exact setup instructions, signature header formats, and verification code snippets for each provider.

---

## 1. Stripe Webhooks

Stripe signs webhook payloads using HMAC-SHA256 with timestamp protection to prevent replay attacks.

### Header Format
```http
Stripe-Signature: t=1792882862,v1=5257a869e7eceeda32affa62cd49fa7b473e3c9039e072d22aa1f62737636e01
```

- **Signed Payload**: `<timestamp>.<raw_body_bytes>`
- **Secret Format**: `whsec_...`
- **Output Encoding**: Hexadecimal

### Capturing & Replaying Stripe Webhooks
```bash
# 1. Listen and forward to your local app
webhook-replay-proxy listen --port 8080 --forward-to http://localhost:3000/api/webhooks/stripe

# 2. Replay with mutated amount and fresh signature
webhook-replay-proxy replay evt_1792882862 \
  --secret whsec_test_secret_123 \
  --set data.object.amount=5000 \
  --diff
```

### Verification Gotcha (Node.js / Express)
Stripe requires the **unmodified raw request body bytes**. In Express, do not use `express.json()` globally:
```javascript
import express from 'express';
import Stripe from 'stripe';

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY);
const app = express();

app.post('/api/webhooks/stripe',
  express.raw({ type: 'application/json' }),
  (req, res) => {
    const sig = req.headers['stripe-signature'];
    try {
      const event = stripe.webhooks.constructEvent(req.body, sig, process.env.STRIPE_WEBHOOK_SECRET);
      res.json({ received: true });
    } catch (err) {
      res.status(400).send(`Webhook Error: ${err.message}`);
    }
  }
);
```

---

## 2. GitHub Webhooks & GitHub Apps

GitHub sends webhooks for repository events (`push`, `pull_request`, `issues`, `workflow_job`) signed with HMAC-SHA256.

### Header Format
```http
X-Hub-Signature-256: sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c469e4637cb87cf9f
```

- **Signed Payload**: `<raw_body_bytes>`
- **Secret Format**: Custom secret string configured in GitHub Webhook settings
- **Output Encoding**: Hexadecimal with `sha256=` prefix

### Capturing & Replaying GitHub Webhooks
```bash
webhook-replay-proxy replay evt_github_pr \
  --provider github \
  --secret my_github_secret \
  --set action=closed \
  --set pull_request.merged=true \
  --diff
```

---

## 3. Slack Events API

Slack uses an HMAC-SHA256 signature prefixed with version string `v0:`.

### Header Format
```http
X-Slack-Request-Timestamp: 1792882862
X-Slack-Signature: v0=a2114d57b48eac39b9ad189dd81e622ac72cdd3b90a5068fbfa3747b601e47da
```

- **Signed Payload**: `v0:<timestamp>:<raw_body_bytes>`
- **Secret Format**: Slack App "Signing Secret"
- **Freshness Window**: 5 minutes

### Testing Slack Webhooks
```bash
webhook-replay-proxy replay evt_slack_event \
  --provider slack \
  --secret slack_signing_secret_32chars \
  --set event.text="Deploying to production"
```

---

## 4. Shopify Webhooks

Shopify signs payloads with HMAC-SHA256 and encodes the signature as Base64.

### Header Format
```http
X-Shopify-Hmac-Sha256: 3q2+796tvu/N9HKslFMWp32IjBs=
X-Shopify-Topic: orders/create
```

- **Signed Payload**: `<raw_body_bytes>`
- **Secret Format**: Shopify App API Secret Key
- **Output Encoding**: Base64

### Testing Shopify Webhooks
```bash
webhook-replay-proxy replay evt_shopify_order \
  --provider shopify \
  --secret shpss_api_secret_key \
  --set total_price="199.99"
```

---

## 5. Svix / Standard Webhooks Specification

Standard Webhooks (used by Svix and modern SaaS) signs using symmetric base64 secrets (`whsec_...`).

### Header Format
```http
webhook-id: msg_2X1...
webhook-timestamp: 1792882862
webhook-signature: v1,g0hM9SsE+OTPJTGtUVW6aujmWoUTL3visPrNaAsdaJU=
```

- **Signed Payload**: `<webhook_id>.<webhook_timestamp>.<raw_body_bytes>`
- **Secret Format**: Base64 key or `whsec_<base64>`
- **Output Encoding**: Base64 with `v1,` prefix

### Testing Standard / Svix Webhooks
```bash
webhook-replay-proxy replay evt_svix_msg \
  --provider standard \
  --secret whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw \
  --set data.status=completed
```
