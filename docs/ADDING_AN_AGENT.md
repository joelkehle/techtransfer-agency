# Adding a New Agent to the Bus

This guide walks through adding a new agent that communicates over the Agent Bus. An agent is any process that registers with the bus, receives messages, does work, and optionally sends messages to other agents.

## Prerequisites

- A running bus instance (`go run ./cmd/techtransfer-agency`) or an in-process bus
- Your agent's **ID** (unique string, e.g. `"my-agent"`)
- Your agent's **secret** (shared secret for HMAC-SHA256 signing)
- Your agent's **capabilities** (machine-readable tags other agents use to discover you)

## Overview

Every agent follows the same lifecycle:

```
Register → Poll Inbox → Receive Message → Ack → Do Work (with Progress) → Final/Error
```

The bus is a plain HTTP service. Your agent can be written in any language that speaks HTTP. The examples below use `curl` for clarity, then show the Go client from `internal/patentteam/client.go` as a reference.

---

## Step 1: Register Your Agent

Register by POSTing to `/v1/agents/register`. This must happen at startup and be repeated at `TTL/2` intervals to keep the registration alive.

```bash
curl -X POST http://localhost:8080/v1/agents/register \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "my-agent",
    "capabilities": ["summarize", "translate"],
    "description": "Summarizes and translates text",
    "mode": "pull",
    "ttl": 120,
    "secret": "my-agent-secret"
  }'
```

| Field | Required | Description |
|-------|----------|-------------|
| `agent_id` | yes | Unique identifier for your agent |
| `capabilities` | yes | Tags that let other agents discover you via `GET /v1/agents?capability=X` |
| `mode` | yes | `"pull"` (you poll the inbox) or `"push"` (bus POSTs to your callback URL) |
| `ttl` | no | Registration lifetime in seconds (default: 60). Re-register before expiry. |
| `secret` | yes | Shared secret used to sign requests with HMAC-SHA256 |
| `callback_url` | if push | Required when `mode` is `"push"` |
| `description` | no | Human-readable description of your agent |

**Heartbeat:** Re-register at `TTL/2` intervals. If your TTL is 120s, re-register every 60s. After expiry, there is a 30s grace period before the bus stops accepting messages for your agent.

### Go Example

```go
client := patentteam.NewClient("http://localhost:8080")
err := client.RegisterAgent(ctx, "my-agent", "my-agent-secret", []string{"summarize", "translate"})
```

---

## Step 2: Poll Your Inbox

Use long-polling to receive messages. The bus returns new messages and an updated cursor.

```bash
# Sign the query string with HMAC-SHA256(secret, "agent_id=my-agent&cursor=0&wait=30")
QUERY="agent_id=my-agent&cursor=0&wait=30"
SIG=$(echo -n "$QUERY" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl "http://localhost:8080/v1/inbox?${QUERY}" \
  -H "X-Bus-Signature: ${SIG}"
```

Response:

```json
{
  "events": [
    {
      "message_id": "msg-abc123",
      "type": "request",
      "from": "other-agent",
      "conversation_id": "conv-001",
      "body": "{\"task\": \"summarize\", \"text\": \"...\"}",
      "attachments": [],
      "created_at": "2026-02-18T12:00:00Z"
    }
  ],
  "cursor": "1"
}
```

Save the returned `cursor` value and pass it in the next poll to avoid re-processing messages.

| Parameter | Description |
|-----------|-------------|
| `agent_id` | Your agent ID |
| `cursor` | Position in your inbox stream (start at `0`) |
| `wait` | Long-poll timeout in seconds (max 60) |

### Go Example

```go
events, nextCursor, err := client.PollInbox(ctx, "my-agent", "my-agent-secret", cursor, 30)
// Save nextCursor for the next poll
```

---

## Step 3: Acknowledge the Message

When you receive a `request` message, you must acknowledge it within **10 seconds** or the bus marks it as an error.

```bash
PAYLOAD='{"agent_id":"my-agent","message_id":"msg-abc123","status":"accepted","reason":"processing"}'
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/acks \
  -H "Content-Type: application/json" \
  -H "X-Bus-Signature: ${SIG}" \
  -d "$PAYLOAD"
```

| Field | Description |
|-------|-------------|
| `status` | `"accepted"` to proceed, or `"rejected"` to decline the work |
| `reason` | Optional human-readable reason |

### Go Example

```go
err := client.Ack(ctx, "my-agent", "my-agent-secret", evt.MessageID, "accepted", "processing")
```

---

## Step 4: Report Progress (Optional)

While doing work, post progress events. These are throttled to one every 2 seconds per message.

```bash
PAYLOAD='{"message_id":"msg-abc123","type":"progress","body":"50% complete"}'
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: my-agent" \
  -H "X-Bus-Signature: ${SIG}" \
  -d "$PAYLOAD"
```

---

## Step 5: Complete the Work

When done, post a `final` event (or `error` on failure):

```bash
# Success
PAYLOAD='{"message_id":"msg-abc123","type":"final","body":"done"}'
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: my-agent" \
  -H "X-Bus-Signature: ${SIG}" \
  -d "$PAYLOAD"
```

```bash
# Failure
PAYLOAD='{"message_id":"msg-abc123","type":"error","body":"extraction failed: unsupported format"}'
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: my-agent" \
  -H "X-Bus-Signature: ${SIG}" \
  -d "$PAYLOAD"
```

### Go Example

```go
// Success
err := client.Event(ctx, "my-agent", "my-agent-secret", evt.MessageID, "final", "done", nil)

// Failure
err := client.Event(ctx, "my-agent", "my-agent-secret", evt.MessageID, "error", "extraction failed", nil)
```

---

## Step 6: Send Messages to Other Agents (Optional)

If your agent needs to forward work or reply, send a message:

```bash
PAYLOAD='{
  "to": "next-agent",
  "from": "my-agent",
  "conversation_id": "conv-001",
  "request_id": "req-unique-001",
  "type": "request",
  "body": "{\"task\": \"translate\", \"text\": \"...\"}",
  "attachments": []
}'
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "my-agent-secret" | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -H "X-Bus-Signature: ${SIG}" \
  -d "$PAYLOAD"
```

| Field | Required | Description |
|-------|----------|-------------|
| `to` | yes | Target agent ID |
| `from` | yes | Your agent ID |
| `conversation_id` | yes | Conversation context |
| `request_id` | yes | Unique per `(from, to)` pair. Same key within 24h is idempotent. |
| `type` | yes | `"request"`, `"response"`, or `"inform"` |
| `body` | yes | Message payload (typically JSON-encoded) |
| `in_reply_to` | for responses | The `message_id` you are replying to |
| `attachments` | no | Array of `{url, name, content_type, size, sha256}` |
| `meta` | no | Arbitrary metadata |

**Message types:**
- `request` — asks the target to do work (expects ack + final/error)
- `response` — reply to a request (set `in_reply_to`)
- `inform` — fire-and-forget notification (no ack expected)

### Go Example

```go
msgID, err := client.SendMessage(ctx,
    "my-agent", "my-agent-secret",  // from, secret
    "next-agent",                   // to
    "conv-001",                     // conversation_id
    "req-unique-001",               // request_id
    "request",                      // type
    `{"task":"translate"}`,         // body
    nil,                            // attachments
    map[string]any{"stage": "translate"}, // meta
)
```

---

## Authentication

All signed endpoints require an `X-Bus-Signature` header containing an HMAC-SHA256 digest of the request payload using your agent's secret.

**What gets signed:**

| Endpoint | Signed Content |
|----------|---------------|
| `POST /v1/messages` | Raw request body |
| `GET /v1/inbox` | Raw query string (e.g. `agent_id=X&cursor=0&wait=30`) |
| `POST /v1/acks` | Raw request body |
| `POST /v1/events` | Raw request body (also requires `X-Agent-ID` header) |

**Signature format:** Raw hex digest or `sha256=<hex>` prefix. Both are accepted.

```go
func sign(secret string, payload []byte) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    return hex.EncodeToString(mac.Sum(nil))
}
```

---

## Complete Agent Loop (Go Pseudocode)

```go
const (
    agentID = "my-agent"
    secret  = "my-agent-secret"
    busURL  = "http://localhost:8080"
)

client := patentteam.NewClient(busURL)
ctx := context.Background()

// 1. Register
client.RegisterAgent(ctx, agentID, secret, []string{"summarize"})

// 2. Start heartbeat
go func() {
    for {
        time.Sleep(60 * time.Second)  // TTL/2
        client.RegisterAgent(ctx, agentID, secret, []string{"summarize"})
    }
}()

// 3. Main loop
cursor := 0
for {
    events, next, err := client.PollInbox(ctx, agentID, secret, cursor, 30)
    if err != nil {
        log.Printf("poll error: %v", err)
        time.Sleep(time.Second)
        continue
    }
    cursor = next

    for _, evt := range events {
        // 4. Ack
        client.Ack(ctx, agentID, secret, evt.MessageID, "accepted", "")

        // 5. Do work
        result, err := doWork(evt)

        if err != nil {
            // 6a. Report error
            client.Event(ctx, agentID, secret, evt.MessageID, "error", err.Error(), nil)
            continue
        }

        // 6b. Report completion
        client.Event(ctx, agentID, secret, evt.MessageID, "final", "done", nil)

        // 7. Optionally send result to next agent or reply
        client.SendMessage(ctx, agentID, secret,
            "next-agent", evt.ConversationID, "req-"+evt.MessageID,
            "response", result, nil, nil,
        )
    }
}
```

---

## Adding Your Agent to a Team Workflow

If you're building a multi-agent pipeline (like the patent team in `internal/patentteam/`), you need to:

1. **Add the agent ID and secret** to the team's secrets map in `team.go`
2. **Register the agent** in `registerAgents()`
3. **Add a handler function** (e.g. `handleMyAgent`) that processes inbox events
4. **Wire it into the polling loop** in `processInbox()`
5. **Connect it in the message chain** — have the upstream agent send requests to yours, and have yours forward to the next agent downstream

See `internal/patentteam/team.go` for a working example of a five-agent pipeline.

---

## Checklist

- [ ] Agent ID is unique across the bus
- [ ] Secret is set and used for all signed requests
- [ ] Registration happens at startup and repeats at `TTL/2`
- [ ] Incoming `request` messages are acked within 10 seconds
- [ ] Work completes with a `final` or `error` event
- [ ] `request_id` values are unique per `(from, to)` pair within 24 hours
- [ ] Signatures match the exact bytes sent (no re-encoding between sign and send)

## Reference

- Protocol spec: `docs/PROTOCOL_SPEC_v2.md`
- Normative details: `docs/NORMATIVE_CLARIFICATIONS.md`
- Working example: `internal/patentteam/`
- HTTP client: `internal/patentteam/client.go`
