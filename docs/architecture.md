---
title: Architecture & Cryptography
description: Deep dive into webhook-replay-proxy internal architecture and HMAC cryptographic signing schemes.
---

# Architecture & Cryptography

`webhook-replay-proxy` is built with a zero-CGO philosophy, high concurrency safety, and exact compliance with official provider webhook signature specifications.

---

## High-Level Architecture

![System Architecture](assets/architecture-diagram.svg)

### 1. Pure-Go SQLite Flight Recorder (`internal/store`)
- Built on `modernc.org/sqlite`, a CGO-free, memory-safe SQLite engine compiled into Go bytecode.
- Enables single-binary distribution with zero external C dependencies across `linux/amd64`, `linux/arm64`, and `darwin/arm64`.
- WAL (Write-Ahead Logging) and pooled read connections guarantee reliable capture without blocking incoming proxy streams.

### 2. Dot-Path JSON Payload Mutator (`internal/mutator`)
- Traverses arbitrary JSON objects by dot notation (e.g., `data.object.charges.data.0.status`).
- Automatically typecasts primitives:
  - Strings (`status="failed"`)
  - Integers and Floats (`amount=9999`, `rate=0.05`)
  - Booleans (`livemode=false`, `paid=true`)
  - Nulls (`customer=null`)
  - Nested JSON objects / arrays (`metadata={"retry": true}`)
- Computes unified visual diffs comparing the recorded state to the mutated state.

### 3. Cryptographic Signing Engine (`internal/signer`)

Each webhook provider enforces distinct cryptographic signing specifications. `webhook-replay-proxy` implements each scheme identically to official SDKs:

#### Stripe
- **Header:** `Stripe-Signature: t=<unix_ts>,v1=<hex_hmac_sha256>`
- **Signed String:** `<unix_ts>.<raw_body_bytes>`
- **Digest:** HMAC-SHA256 hex encoded.

#### GitHub
- **Header:** `X-Hub-Signature-256: sha256=<hex_hmac_sha256>`
- **Signed String:** `<raw_body_bytes>`
- **Digest:** HMAC-SHA256 hex encoded prefixed with `sha256=`.

#### Slack
- **Headers:** `X-Slack-Request-Timestamp: <unix_ts>`, `X-Slack-Signature: v0=<hex_hmac_sha256>`
- **Signed String:** `v0:<unix_ts>:<raw_body_bytes>`
- **Digest:** HMAC-SHA256 hex encoded prefixed with `v0=`.

#### Shopify
- **Header:** `X-Shopify-Hmac-Sha256: <base64_hmac_sha256>`
- **Signed String:** `<raw_body_bytes>`
- **Digest:** HMAC-SHA256 base64 encoded.

#### Standard Webhooks / Svix
- **Headers:** `webhook-id: <msg_id>`, `webhook-timestamp: <unix_ts>`, `webhook-signature: v1,<base64_hmac_sha256>`
- **Signed String:** `<msg_id>.<unix_ts>.<raw_body_bytes>`
- **Secret Support:** Handles base64-encoded `whsec_` secrets automatically.

---

## Concurrency Synchronization Barrier (`internal/proxy`)

When `--concurrency N` is specified:
```go
startGate := make(chan struct{})
for i := 0; i < N; i++ {
    go func(idx int) {
        req := prepareRequest(...)
        <-startGate // Wait at synchronized barrier
        client.Do(req)
    }(i)
}
close(startGate) // Release all requests simultaneously
```

This ensures that all $N$ requests hit the network stack and target server concurrently, exposing database race conditions, locking bugs, and duplicate processing issues.
