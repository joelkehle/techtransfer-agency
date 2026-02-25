# TechTransfer Agency Protocol Specification v2

> Refactored from v1 by Claude (Opus 4.6), 2026-02-08.
> Original v1 by Claude (Opus 4.5) and Codex, facilitated by Joel Kehle, 2026-01-22.

## Overview

The TechTransfer Agency Bus is an agent-first communication protocol. Agents communicate via
HTTP. Humans and external systems observe via a streaming observation API.
The bus is transport-agnostic — any client that speaks HTTP and SSE can
consume the observation stream.

## Design Principles

- **Agent-first**: The protocol is designed for machines talking to machines.
  Human observation is a read-only layer on top, not a structural dependency.
- **Transport-agnostic**: The bus defines an HTTP API. Display transports
  (web UI, CLI, CXDB) are consumers of the observation API, not part of the
  core protocol.
- **Pluggable**: Any process that speaks HTTP can register as an agent. The
  protocol is the published interface — implement it and you're in.
- **Observable**: All agent communication flows through the bus and is available
  via a real-time observation stream. Nothing is hidden.
- **At-least-once delivery**: Idempotency via caller-supplied `request_id`.
  The bus does not silently drop messages.

## Architecture

```
                    ┌──────────────────────┐
                    │ TechTransfer Agency  │
                    │                      │
                    │  ┌────────────────┐  │
                    │  │    Registry    │  │
                    │  ├────────────────┤  │
                    │  │  Message Store │  │
                    │  ├────────────────┤  │
                    │  │   Router       │  │
                    │  ├────────────────┤  │
                    │  │  Observation   │  │
                    │  │    Stream      │  │
                    │  └────────────────┘  │
                    └──────────┬───────────┘
                               │ HTTP
            ┌──────────────────┼──────────────────┐
            │                  │                  │
      ┌─────┴─────┐    ┌──────┴─────┐    ┌──────┴──────┐
      │  Market    │    │  Patent    │    │    TDG      │
      │  Analyst   │    │  Assessor  │    │  Assistant  │
      │  (agent)   │    │  (agent)   │    │  (agent)    │
      └───────────┘    └────────────┘    └─────────────┘

Observers (read-only, any number):
  - Web dashboard
  - CLI monitor
  - CXDB logger
```

## Concepts

### Agent

A process that registers with the bus and can send/receive messages. Each agent
has a unique `agent_id` and declares its capabilities at registration.

### Conversation

A threaded sequence of messages between agents on a topic. Conversations have a
`conversation_id` that groups related messages. A conversation can involve two
or more agents over multiple turns.

### Message

A single communication from one agent to another (or to a conversation).
Messages have a type: `request`, `response`, `inform`. Not everything is
request/response — an agent can broadcast information to a conversation.

### Observer

A read-only consumer of the observation stream. Observers see all messages,
acks, progress events, and state transitions. They cannot inject messages
into conversations (use the human injection endpoint for that).

---

## HTTP Endpoints

### POST /v1/agents/register

Register an agent with the bus. Re-registering with the same `agent_id`
refreshes the TTL and updates metadata (idempotent heartbeat).

**Request:**
```json
{
  "agent_id": "market-analyst",
  "capabilities": ["market-analysis", "revenue-estimation"],
  "description": "Analyzes market potential for inventions and estimates licensing revenue ranges",
  "mode": "pull",
  "callback_url": null,
  "ttl": 60
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| agent_id | string | yes | Unique identifier for the agent |
| capabilities | string[] | yes | Machine-readable capability tags for discovery |
| description | string | no | Human-readable description of what this agent does |
| mode | "pull" \| "push" | yes | Pull = long-poll inbox; Push = deliver to callback_url |
| callback_url | string | no | Required if mode=push |
| ttl | int | no | Registration TTL in seconds (default: 60) |

**Response:**
```json
{
  "ok": true,
  "agent_id": "market-analyst",
  "expires_at": "2026-02-08T12:00:00Z"
}
```

**Lifecycle:**

```
┌────────────┐  POST /register   ┌────────────┐
│unregistered│ ───────────────▶  │ registered │
└────────────┘                   └─────┬──────┘
      ▲                                │
      │         POST /register         │ TTL expires
      │        (same agent_id)         │ (no refresh)
      │       ┌────────────────┐       │
      │       │   refreshes    │       │
      │       │   TTL + meta   │       │
      │       └───────┬────────┘       │
      │               │                │
      │               ▼                ▼
      │         ┌────────────┐   ┌────────────┐
      └─────────│ registered │   │  expired   │
                └────────────┘   └────────────┘
```

**Edge cases:**

| Scenario | Behavior |
|----------|----------|
| Re-register before expiry | TTL refreshed, metadata updated, `200 ok` |
| Re-register after expiry | Treated as new registration, `200 ok` |
| Register with conflicting agent_id (different secret) | `401 unauthorized` |
| Agent sends message while expired | `401 unauthorized` (must re-register) |
| Message to expired agent | Queued briefly; if not re-registered within grace period, `404 not_found` |

**Recommended agent behavior:**
- Re-register at `TTL / 2` interval (e.g., every 30s if TTL=60)
- On `401` from any endpoint, re-register immediately
- On startup, always register before polling inbox or sending messages

---

### GET /v1/agents

Discover registered agents and their capabilities.

**Request:**
```
GET /v1/agents
GET /v1/agents?capability=market-analysis
```

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| capability | string | no | Filter by capability tag |

**Response:**
```json
{
  "agents": [
    {
      "agent_id": "market-analyst",
      "capabilities": ["market-analysis", "revenue-estimation"],
      "description": "Analyzes market potential for inventions",
      "status": "active",
      "registered_at": "2026-02-08T11:00:00Z",
      "expires_at": "2026-02-08T12:00:00Z"
    },
    {
      "agent_id": "patent-agent",
      "capabilities": ["patent-eligibility", "prior-art-search"],
      "description": "Evaluates patent eligibility and searches prior art",
      "status": "active",
      "registered_at": "2026-02-08T11:00:00Z",
      "expires_at": "2026-02-08T12:00:00Z"
    }
  ]
}
```

---

### POST /v1/conversations

Create a new conversation. Conversations group related messages into a thread.

**Request:**
```json
{
  "conversation_id": "disclosure-2026-003",
  "title": "Market and patent assessment for nano-coating invention",
  "participants": ["tdg-assistant", "market-analyst", "patent-agent"],
  "meta": {
    "case_number": "UCLA-TT-2026-003",
    "disclosure_type": "invention"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| conversation_id | string | no | Caller-supplied ID; auto-generated if omitted |
| title | string | no | Human-readable title for observers |
| participants | string[] | no | Initial participant list (advisory; not enforced in v2) |
| meta | object | no | Arbitrary metadata attached to the conversation |

**Response:**
```json
{
  "ok": true,
  "conversation_id": "disclosure-2026-003"
}
```

---

### GET /v1/conversations

List conversations, optionally filtered.

**Request:**
```
GET /v1/conversations
GET /v1/conversations?participant=market-analyst
GET /v1/conversations?status=active
```

**Response:**
```json
{
  "conversations": [
    {
      "conversation_id": "disclosure-2026-003",
      "title": "Market and patent assessment for nano-coating invention",
      "participants": ["tdg-assistant", "market-analyst", "patent-agent"],
      "status": "active",
      "message_count": 7,
      "created_at": "2026-02-08T11:00:00Z",
      "last_message_at": "2026-02-08T11:05:00Z",
      "meta": {}
    }
  ]
}
```

---

### POST /v1/messages

Send a message to an agent or into a conversation.

**Request:**
```json
{
  "to": "market-analyst",
  "from": "tdg-assistant",
  "conversation_id": "disclosure-2026-003",
  "request_id": "uuid-1234",
  "type": "request",
  "body": "Analyze the market potential for this nano-coating invention. See attached disclosure.",
  "meta": { "priority": "normal" },
  "attachments": [
    {
      "url": "https://storage.example.com/disclosure-003.pdf",
      "name": "disclosure-003.pdf",
      "content_type": "application/pdf",
      "size": 102400,
      "sha256": "e3b0c44..."
    }
  ],
  "ttl": 600
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| to | string | yes | Target agent_id |
| from | string | yes | Sender agent_id |
| conversation_id | string | no | Groups message into a conversation; auto-created if new |
| request_id | string | yes | Caller-supplied for idempotency |
| type | "request" \| "response" \| "inform" | yes | Message intent |
| body | string | yes | Message content |
| meta | object | no | Arbitrary metadata |
| attachments | array | no | URL-based attachments |
| ttl | int | no | Time-to-live in seconds for request completion |

**Response:**
```json
{
  "ok": true,
  "message_id": "m-5678"
}
```

**Message types:**
- `request` — Asks the recipient to do something. Expects ack → progress → final/error.
- `response` — A reply to a previous request (references `in_reply_to`).
- `inform` — One-way notification. No ack or response expected.

**Optional fields for `response` type:**
```json
{
  "type": "response",
  "in_reply_to": "m-5678",
  "body": "Market analysis complete. Estimated licensing revenue: $2M-8M/year..."
}
```

---

### GET /v1/inbox

Long-poll for incoming messages (pull mode agents).

**Request:**
```
GET /v1/inbox?agent_id=market-analyst&cursor=abc&wait=30
```

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| agent_id | string | yes | Agent requesting messages |
| cursor | string | no | Pagination cursor from previous response |
| wait | int | no | Long-poll timeout in seconds (max 60) |

**Response:**
```json
{
  "events": [
    {
      "message_id": "m-5678",
      "type": "request",
      "from": "tdg-assistant",
      "conversation_id": "disclosure-2026-003",
      "body": "Analyze the market potential...",
      "meta": {},
      "attachments": [],
      "created_at": "2026-02-08T11:00:00Z"
    }
  ],
  "cursor": "xyz"
}
```

---

### POST /v1/acks

Acknowledge receipt of a message.

**Request:**
```json
{
  "agent_id": "market-analyst",
  "message_id": "m-5678",
  "status": "accepted"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| agent_id | string | yes | Agent acknowledging |
| message_id | string | yes | Message being acknowledged |
| status | "accepted" \| "rejected" | yes | Accept or reject the request |
| reason | string | no | Reason for rejection (if rejected) |

**Response:**
```json
{
  "ok": true
}
```

---

### POST /v1/events

Send progress, final result, or error for a message.

**Request:**
```json
{
  "message_id": "m-5678",
  "type": "progress",
  "body": "Searching patent databases...",
  "meta": { "percent": 30 }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| message_id | string | yes | Original request message_id |
| type | "progress" \| "final" \| "error" | yes | Event type |
| body | string | yes | Event content |
| meta | object | no | Additional data (percent, error details, etc.) |

**Response:**
```json
{
  "ok": true
}
```

**Notes:**
- Bus enforces min interval (2-5s) between progress events per message
- `final` or `error` terminates the request flow
- After `final`, the message state transitions to `completed`

---

### GET /v1/observe

Server-Sent Events stream for real-time observation. This is the primary
interface for human visibility, dashboards, loggers, and external systems.

**Request:**
```
GET /v1/observe
GET /v1/observe?conversation_id=disclosure-2026-003
GET /v1/observe?agent_id=market-analyst
```

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| conversation_id | string | no | Filter to a specific conversation |
| agent_id | string | no | Filter to messages involving a specific agent |

**Response (SSE stream):**
```
event: message
data: {"message_id":"m-5678","type":"request","from":"tdg-assistant","to":"market-analyst","conversation_id":"disclosure-2026-003","body":"Analyze the market...","created_at":"2026-02-08T11:00:00Z"}

event: ack
data: {"message_id":"m-5678","agent_id":"market-analyst","status":"accepted","at":"2026-02-08T11:00:01Z"}

event: progress
data: {"message_id":"m-5678","body":"Searching market databases...","meta":{"percent":30},"at":"2026-02-08T11:00:10Z"}

event: state_change
data: {"message_id":"m-5678","from_state":"executing","to_state":"completed","at":"2026-02-08T11:00:30Z"}

event: agent_registered
data: {"agent_id":"patent-agent","capabilities":["patent-eligibility","prior-art-search"],"at":"2026-02-08T11:01:00Z"}

event: agent_expired
data: {"agent_id":"market-analyst","at":"2026-02-08T12:00:00Z"}
```

**Event types:**
- `message` — New message sent
- `ack` — Message acknowledged
- `progress` — Progress update on a request
- `state_change` — Message state transition
- `agent_registered` — Agent joined the bus
- `agent_expired` — Agent's registration expired
- `human_injection` — Human sent a message (see below)

---

### POST /v1/inject

Human injection endpoint. Allows authorized humans to send messages into
conversations. Messages appear in the observation stream with
`from: "human:<identity>"`.

**Request:**
```json
{
  "identity": "joel",
  "conversation_id": "disclosure-2026-003",
  "to": "market-analyst",
  "body": "Also consider the automotive sector — there's a large OEM interested."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| identity | string | yes | Human identifier (must be in allowlist) |
| conversation_id | string | no | Target conversation |
| to | string | no | Target agent (if directed); omit for broadcast to conversation |
| body | string | yes | Message content |

**Response:**
```json
{
  "ok": true,
  "message_id": "m-9999"
}
```

---

### GET /v1/conversations/:id/messages

Retrieve the full message history for a conversation.

**Request:**
```
GET /v1/conversations/disclosure-2026-003/messages
GET /v1/conversations/disclosure-2026-003/messages?cursor=abc&limit=50
```

**Response:**
```json
{
  "conversation_id": "disclosure-2026-003",
  "messages": [
    {
      "message_id": "m-5678",
      "type": "request",
      "from": "tdg-assistant",
      "to": "market-analyst",
      "body": "Analyze the market potential...",
      "state": "completed",
      "created_at": "2026-02-08T11:00:00Z"
    },
    {
      "message_id": "m-5679",
      "type": "response",
      "from": "market-analyst",
      "to": "tdg-assistant",
      "in_reply_to": "m-5678",
      "body": "Estimated licensing revenue: $2M-8M/year...",
      "created_at": "2026-02-08T11:00:30Z"
    }
  ],
  "cursor": "xyz"
}
```

---

## Message State Machine

```
┌─────────┐
│ pending │
└────┬────┘
     │ Bus delivers to agent
     ▼
┌─────────┐  ack_timeout (10s)   ┌─────────┐
│ waiting │ ──────────────────▶  │  error  │
│  (ack)  │                      │(timeout)│
└────┬────┘                      └─────────┘
     │ Agent sends ack
     ▼
┌──────────┐  status=rejected    ┌─────────┐
│  acked   │ ──────────────────▶ │rejected │
└────┬─────┘                     └─────────┘
     │ status=accepted
     ▼
┌───────────┐
│ executing │◀─────────────┐
└─────┬─────┘              │
      │ progress event     │
      └────────────────────┘
      │
      │ final event        ttl expires
      ▼                    ▼
┌───────────┐        ┌─────────┐
│ completed │        │  error  │
└───────────┘        │(timeout)│
                     └─────────┘
      │ error event
      ▼
┌─────────┐
│  error  │
└─────────┘
```

This state machine applies only to `request` type messages. `inform` messages
have no state (fire-and-forget). `response` messages are terminal.

---

## Error Taxonomy

| Code | Transient | Description |
|------|-----------|-------------|
| validation | no | Invalid request format |
| unauthorized | no | Agent not registered or auth failed |
| not_found | no | Target agent not registered |
| rejected | no | Agent rejected the request |
| rate_limited | yes | Too many requests |
| unavailable | yes | Agent temporarily unavailable |
| timeout | yes | Ack or TTL timeout |
| internal | yes | Bus internal error |

**Error response shape:**
```json
{
  "ok": false,
  "error": {
    "code": "timeout",
    "message": "Agent did not acknowledge within 10s",
    "transient": true,
    "retry_after": 5
  }
}
```

**Retry policy:**
- Bus provides at-least-once delivery for inbox (cursor + ack mechanism)
- Bus does NOT automatically re-send requests
- Caller retry policy: retry only on `transient=true`
- Idempotency: same `request_id` = same request (bus deduplicates)

---

## Authentication

- Per-agent shared secret configured at registration
- HMAC-SHA256 signature in `X-Bus-Signature` header
- Global allowlist of permitted agent_ids
- Human injection requires separate allowlist
- Per-conversation ACLs deferred to future version

---

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| ack_timeout | 10s | Time for agent to acknowledge receipt |
| ttl | 600s | Overall request lifetime |
| registration_ttl | 60s | Agent registration expiry |
| long_poll_max | 60s | Maximum inbox poll wait |

---

## Attachments

URL-based only. No direct file upload.

```json
{
  "attachments": [
    {
      "url": "https://storage.example.com/file.pdf",
      "name": "disclosure.pdf",
      "content_type": "application/pdf",
      "size": 102400,
      "sha256": "e3b0c44..."
    }
  ]
}
```

---

## Example: Invention Disclosure Workflow

This shows how tdg-assistant orchestrates a commercialization assessment
using two SME agents via the bus.

```
1. tdg-assistant creates conversation "disclosure-2026-003"

2. tdg-assistant → market-analyst (request):
   "Analyze market potential for nano-coating invention. See attached."

3. tdg-assistant → patent-agent (request, same conversation):
   "Assess patent eligibility and conduct prior art search. See attached."

   (Both agents work in parallel)

4. market-analyst → tdg-assistant (response):
   "Three target markets identified. Estimated licensing revenue: $2M-8M/year.
    Strongest fit: automotive OEM coatings."

5. patent-agent → tdg-assistant (response):
   "Invention appears patent-eligible. Found 3 related patents but claims are
    differentiable. Recommend provisional filing."

6. tdg-assistant → conversation (inform):
   "Assessment complete. Recommendation: file provisional patent, prioritize
    automotive OEM licensing outreach."
```

An observer (web dashboard, Joel's CLI) sees all 6 messages
in real-time via `GET /v1/observe?conversation_id=disclosure-2026-003`.

---

## What This Spec Does NOT Define

- **Transport adapters**: How a web UI, CLI, or logging system consumes
  the observation stream is an implementation detail, not part of this protocol.
- **Agent internals**: What an agent does with a message (call an LLM, query
  a database, ask a human) is entirely up to the agent.
- **Orchestration logic**: Multi-step workflows (like the disclosure example)
  are the caller's responsibility. The bus is a communication layer, not a
  workflow engine.
- **Message content schemas**: The `body` field is freeform text. Domain-specific
  schemas (disclosure format, market report format) are conventions between
  agents, not bus concerns.
