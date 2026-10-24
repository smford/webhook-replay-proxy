# webhook-replay-proxy

[![CI](https://github.com/smford/webhook-replay-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/smford/webhook-replay-proxy/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/smford/webhook-replay-proxy?color=blue&logo=github)](https://github.com/smford/webhook-replay-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/smford/webhook-replay-proxy/badge)](https://securityscorecards.dev/viewer/?repo=github.com/smford/webhook-replay-proxy)

Production-grade webhook capture, replay, and inspection proxy designed for edge environments and cloud microservices.

---

## Features

- **Zero-Dependency Static Binary**: Compiled hermetically with `CGO_ENABLED=0` for minimal attack surface.
- **Webhook Ingestion**: Captures incoming HTTP payloads, timestamps, and headers.
- **In-Memory Ring Buffer**: Configurable event retention limits to maintain predictable memory usage.
- **Inspection & Replay API**: Query received webhook events and replay them against destination targets.
- **Production Observability**: Built-in health check probe (`/healthz`) and version metadata endpoint (`/version`).
- **Zero-Trust Security**: Hardened with SAST (CodeQL & Semgrep), DAST (OWASP ZAP baseline), and OpenSSF Scorecard supply-chain auditing.

---

## Installation

### Homebrew (macOS / Linux)

```bash
brew install smford/homebrew-tap/webhook-replay-proxy
```

### Docker / GHCR

```bash
docker pull ghcr.io/smford/webhook-replay-proxy:latest
docker run -d -p 8080:8080 ghcr.io/smford/webhook-replay-proxy:latest
```

### Precompiled Standalone Binaries

Download signed binaries for Linux and macOS (`amd64`, `arm64`) from [GitHub Releases](https://github.com/smford/webhook-replay-proxy/releases).

---

## Quick Start

### 1. Launch the Server

```bash
webhook-replay-proxy -port 8080 -config config.example.yaml
```

### 2. Ingest a Webhook

```bash
curl -X POST http://localhost:8080/api/v1/webhooks \
  -H "Content-Type: application/json" \
  -d '{"event": "invoice.paid", "id": "inv_12345", "amount": 9900}'
```

### 3. List Ingested Webhooks

```bash
curl http://localhost:8080/api/v1/webhooks
```

### 4. Health Probe

```bash
curl http://localhost:8080/healthz
```

---

## Configuration

Configuration can be provided via YAML or command line flags:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

storage:
  max_events: 1000

upstream:
  default_target_url: "https://example.com/webhook-receiver"
```

| Flag | Description | Default |
|---|---|---|
| `-config` | Path to YAML configuration file | `""` |
| `-port` | HTTP server port override | `8080` |
| `-version` | Output version and build information | `false` |

---

## Security & Supply Chain

- **Keyless Signing**: Release artifacts and container images are cryptographically signed using [Cosign](https://sigstore.dev/).
- **Automated Dependency Updates**: Managed and grouped by Dependabot with automated verification and patch tagging.
- **Shift-Left Pre-Commit**: Pre-commit hooks enforce formatting, gitleaks secret detection, and conventional commits locally and in CI.

---

## License

Released under the [MIT License](LICENSE). Copyright &copy; 2026 Simon Ford.
