---
title: CLI Reference
description: Complete command-line interface documentation for webhook-replay-proxy.
---

# CLI Reference

`webhook-replay-proxy` provides intuitive commands for capturing, listing, inspecting, exporting, replaying, and generating curl requests.

---

## Global Flags

| Flag | Description |
|---|---|
| `-v`, `--version`, `version` | Display version, commit, build date, and license information |
| `-h`, `--help`, `help` | Display usage instructions |

---

## `listen`

Starts the webhook capture proxy daemon and forwards traffic to your target application.

```bash
webhook-replay-proxy listen [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--port`, `-port` | int | `8080` | Port to bind the HTTP proxy server |
| `--forward-to` | string | `""` | Target backend URL (e.g. `http://localhost:3000/webhooks`) |
| `--db` | string | `./webhooks.db` | Path to pure-Go SQLite database file |
| `--config` | string | `""` | Optional path to YAML configuration file |
| `--ui` | bool | `false` | Enable embedded web dashboard at `http://localhost:<port>/_dashboard` |

---

## `list`

Lists captured webhook records from the SQLite flight recorder.

```bash
webhook-replay-proxy list [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--db` | string | `./webhooks.db` | Path to SQLite database file |
| `--limit` | int | `50` | Maximum number of events to display |

---

## `inspect`

Inspects complete details of a recorded webhook event, including headers, pretty-printed JSON body, target response, and previous replay executions.

```bash
webhook-replay-proxy inspect <event_id> [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--db` | string | `./webhooks.db` | Path to SQLite database file |
| `--raw` | bool | `false` | Print raw unformatted payload body |

---

## `export`

Exports a recorded webhook to a standalone, version-controllable JSON fixture file.

```bash
webhook-replay-proxy export <event_id> [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--output`, `-o` | string | stdout | Output file path (e.g. `fixtures/stripe_checkout.json`) |
| `--db` | string | `./webhooks.db` | Path to SQLite database file |

---

## `replay`

Replays a captured webhook or fixture against a target URL with fresh timestamps, field mutations, chaos fault injection, and HMAC re-signing.

```bash
webhook-replay-proxy replay [<event_id> | --file <path>] [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--file` | string | `""` | Replay directly from a fixture JSON file without SQLite |
| `--target` | string | recorded | Override destination target URL |
| `--provider` | string | `auto` | Signing provider (`stripe`, `github`, `slack`, `shopify`, `standard`, `auto`) |
| `--secret` | string | `""` | Webhook HMAC secret to re-sign payload |
| `--set` | string | `[]` | Mutate JSON field (`key.path=value`, repeatable) |
| `--concurrency` | int | `1` | Number of simultaneous requests fired via synchronized barrier |
| `--delay` | duration | `0` | Delay between spaced sequential requests (e.g. `100ms`, `1s`) |
| `--diff` | bool | `false` | Display unified visual diff before sending |
| `--corrupt-sig` | bool | `false` | Chaos mode: deliberately corrupt HMAC signature to verify 401 rejection |
| `--corrupt-json` | bool | `false` | Chaos mode: deliberately malform payload JSON to test parser error handling |
| `--drop-header` | string | `[]` | Chaos mode: strip specified header before dispatching (repeatable) |
| `--time-offset` | duration | `0` | Offset signing timestamp from current time (e.g. `-10m`, `+1h`) |
| `--fixed-timestamp` | int64 | `0` | Exact fixed unix epoch timestamp for signing |
| `--assert-status` | string | `""` | Assert response status distribution (e.g. `200:1,409:9` or `200`) |
| `--max-latency` | duration | `0` | Fail assertion if any request latency exceeds threshold |
| `--db` | string | `./webhooks.db` | Path to SQLite database file |

---

## `curl`

Generates an executable `curl` command with fresh HMAC signatures, chaos tampering, and mutated payload.

```bash
webhook-replay-proxy curl [<event_id> | --file <path>] [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--file` | string | `""` | Generate curl command directly from a fixture JSON file |
| `--target` | string | recorded | Override destination target URL |
| `--provider` | string | `auto` | Signing provider (`stripe`, `github`, `slack`, `shopify`, `standard`, `auto`) |
| `--secret` | string | `""` | Webhook HMAC secret to re-sign payload |
| `--set` | string | `[]` | Mutate JSON field (`key.path=value`, repeatable) |
| `--corrupt-sig` | bool | `false` | Deliberately corrupt HMAC signature |
| `--corrupt-json` | bool | `false` | Deliberately malform payload JSON |
| `--drop-header` | string | `[]` | Strip specified header from generated command (repeatable) |
| `--time-offset` | duration | `0` | Offset signing timestamp from current time |
| `--fixed-timestamp` | int64 | `0` | Exact fixed unix epoch timestamp for signing |
| `--db` | string | `./webhooks.db` | Path to SQLite database file |
