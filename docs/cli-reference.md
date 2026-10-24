---
title: CLI Reference
description: Complete command-line interface documentation for webhook-replay-proxy.
---

# CLI Reference

`webhook-replay-proxy` provides intuitive commands for capturing, listing, inspecting, replaying, and generating curl requests.

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

## `replay`

Replays a captured webhook against a target URL with fresh timestamps, field mutations, and HMAC re-signing.

```bash
webhook-replay-proxy replay <event_id> [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--target` | string | recorded | Override destination target URL |
| `--provider` | string | `auto` | Signing provider (`stripe`, `github`, `slack`, `shopify`, `standard`, `auto`) |
| `--secret` | string | `""` | Webhook HMAC secret to re-sign payload |
| `--set` | string | `[]` | Mutate JSON field (`key.path=value`, repeatable) |
| `--concurrency` | int | `1` | Number of simultaneous requests fired via synchronized barrier |
| `--delay` | duration | `0` | Delay between spaced sequential requests (e.g. `100ms`, `1s`) |
| `--diff` | bool | `false` | Display unified visual diff before sending |
| `--db` | string | `./webhooks.db` | Path to SQLite database file |

---

## `curl`

Generates an executable `curl` command with fresh HMAC signatures and mutated payload.

```bash
webhook-replay-proxy curl <event_id> [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--target` | string | recorded | Target URL for curl command |
| `--provider` | string | `auto` | Signing scheme |
| `--secret` | string | `""` | Secret used to calculate signature |
| `--set` | string | `[]` | JSON mutations |
| `--db` | string | `./webhooks.db` | Path to SQLite database |
