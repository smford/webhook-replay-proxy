---
title: Troubleshooting & Framework Gotchas
description: Solutions for raw body parsing, secret formatting, clock drift, and reverse proxy issues.
---

# Troubleshooting & Framework Gotchas

Webhook signature verification is notoriously fragile because cryptographic HMAC hashes depend on the exact byte-for-byte stream received over the wire. Here are solutions to the most common real-world failure modes.

---

## 1. The "Raw Body" Dilemma

The #1 reason developers get `400 Bad Request` or `Invalid Signature` when testing webhooks is that standard JSON middleware parses and re-encodes the body before your handler runs, altering whitespace, key ordering, or Unicode characters.

### Node.js / Express
Do **not** apply `app.use(express.json())` globally before your webhook route. Use `express.raw()` specifically for the webhook endpoint:

```javascript
// Correct: Preserve exact raw Buffer
app.post('/api/webhooks', express.raw({ type: 'application/json' }), (req, res) => {
  const rawBody = req.body; // Buffer!
  const signature = req.headers['stripe-signature'];
  // Verify with rawBody...
});

// Other API routes can use json parser after
app.use(express.json());
```

### Next.js App Router (`route.ts`)
Next.js provides `req.text()` which preserves raw payload bytes:

```typescript
import { NextRequest, NextResponse } from 'next/server';

export async function POST(req: NextRequest) {
  const rawBody = await req.text(); // Do NOT use req.json() here!
  const signature = req.headers.get('stripe-signature');
  // Pass rawBody to verification function...
  return NextResponse.json({ ok: true });
}
```

### Python / FastAPI
Use `await request.body()` to read the unparsed bytes instead of relying on a Pydantic model directly:

```python
from fastapi import FastAPI, Request, HTTPException

app = FastAPI()

@app.post("/webhooks")
async def handle_webhook(request: Request):
    payload = await request.body() # Raw bytes
    sig_header = request.headers.get("Stripe-Signature")
    # Verify signature using payload...
    return {"status": "success"}
```

### Go (`net/http`)
In Go, reading `r.Body` drains the reader. Read with `io.ReadAll` and reassign if downstream middleware needs it:

```go
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "cannot read body", http.StatusBadRequest)
        return
    }
    // Verify HMAC over body bytes...
}
```

---

## 2. Using with `ngrok` or `cloudflared` Tunnels

When testing webhooks from public providers on the internet:

```
Provider (Stripe/GitHub)
       │
       ▼
Public URL (e.g. ngrok or cloudflared)
       │
       ▼
webhook-replay-proxy (:8080)  ──[Flight Recorder SQLite]
       │
       ▼
Local Backend (:3000)
```

1. **Start `webhook-replay-proxy`**:
   ```bash
   webhook-replay-proxy listen --port 8080 --forward-to http://localhost:3000/webhooks --ui
   ```

2. **Point your tunnel to port 8080**:
   ```bash
   ngrok http 8080
   ```

3. **Configure your provider's webhook URL** to point to the `ngrok` URL (e.g. `https://xyz.ngrok-free.app/webhooks`).

Every incoming webhook is now recorded into `./webhooks.db` and immediately forwarded to your backend. You can replay, mutate, or chaos-test it locally without making another external transaction!

---

## 3. Clock Drift & Timestamp Tolerance

If your backend rejects valid signatures with `Timestamp outside tolerance window`:
- Stripe and Slack reject events where the header timestamp differs by more than **300 seconds (5 minutes)** from server time.
- If testing locally, ensure your machine clock is synchronized with NTP:
  - macOS: `sudo sntp -sS time.apple.com`
  - Linux: `sudo timedatectl set-ntp on`
- Alternatively, use `--time-offset` or `--fixed-timestamp` to inspect your application's behavior at specific intervals.
