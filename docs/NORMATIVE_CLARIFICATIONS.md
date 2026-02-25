# TechTransfer Agency V2 Normative Clarifications

This document makes ambiguous parts of the v2 spec deterministic for implementation.

## 1. Error Code to HTTP Status Mapping

| Error code | HTTP status |
|---|---|
| `validation` | 400 |
| `unauthorized` | 401 |
| `not_found` | 404 |
| `rejected` | 409 |
| `rate_limited` | 429 |
| `timeout` | 408 |
| `unavailable` | 503 |
| `internal` | 500 |

All errors use:

```json
{
  "ok": false,
  "error": {
    "code": "validation",
    "message": "...",
    "transient": false,
    "retry_after": 0
  }
}
```

`retry_after` is omitted unless applicable.

## 2. Idempotency Scope and Retention

- Idempotency key is `(from, to, request_id)`.
- Dedupe window is 24 hours from first accepted request.
- Same key returns the original `message_id` and does not re-deliver.

## 3. Ack/Event Authorization

- `/v1/acks`: `agent_id` must equal target `to` on the message.
- `/v1/events`: caller must provide header `X-Agent-ID`; value must equal target `to` on the message.

## 4. Progress Throttle

- Progress events are limited to one every 2 seconds per message.
- Faster calls return `429 rate_limited` with `retry_after`.

## 5. Expired Agent Grace Period

- If target agent just expired, requests are accepted and queued for up to 30 seconds.
- If agent does not re-register within that period, message transitions to `error` with `not_found` semantics.

## 6. Inbox Cursor Semantics

- Cursor is an integer offset into the per-agent inbox stream.
- Response returns next cursor as stringified integer.
- Cursor is stable and monotonic within process lifetime.

## 7. Observe Stream Resume

- SSE supports `Last-Event-ID` header and `cursor` query parameter.
- Resume starts strictly after the provided event id.

## 8. Human Allowlist

- Human identities are checked against `HUMAN_ALLOWLIST` env var (comma-separated).
- If unset/empty, all identities are accepted for local development.

## 9. Signature Encoding and Inbox Query Signing

- `X-Bus-Signature` accepts lowercase/uppercase hex digest, and optional `sha256=<hex>` prefix.
- Inbox signatures are computed over the exact raw query string (`r.URL.RawQuery`), so parameter order and encoding must match exactly.
