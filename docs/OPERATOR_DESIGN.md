# Operator Agent — Design Document

## Problem

Business Development Officers (BDOs) at UCLA's technology transfer office need
to submit invention disclosures and other documents for specialist analysis —
patent screening, prior art search, market analysis, commercialization strategy,
grant restriction lookup, and more. Not every BDO uses the tto-assistant. They
need a simple web interface: upload a document, pick what you want done, get a
report.

## Users

- ~10 BDOs at UCLA
- Internal tool, no public access
- No login system; access controlled at the network level (VPN / firewall)
- Each BDO only sees their own submissions

## Non-Goals

- User accounts, authentication, or authorization
- Persistent submission history (BDO saves the report themselves)
- Chat or conversational UI
- Knowledge of specific workflow internals

---

## Architecture

```
┌──────────────┐          ┌───────────────────────┐          ┌──────────────────┐
│   Browser    │──POST───►│    Operator Agent     │──bus────►│ Specialist Agents│
│              │◄─poll────│                        │◄─bus─────│ (patent, prior   │
│  Upload doc  │          │  Web server (browser)  │          │  art, market,    │
│  Pick workflow│          │  Bus client (agents)   │          │  grants, etc.)   │
│  Download report│        │                        │          │                  │
└──────────────┘          └───────────────────────┘          └──────────────────┘
```

The operator is a single process running two things:

1. **Web server** — serves the UI, accepts submissions, delivers reports
2. **Bus client** — registered agent that sends requests and receives results

### Two Paths Into the Bus

| Path | User | How It Works |
|------|------|-------------|
| tto-assistant | Joel | Agent-to-agent on the bus directly |
| Operator web UI | Other BDOs | Browser → operator → bus → specialists |

The operator is the second path. The tto-assistant already has the first.

---

## Submission Flow

```
1. BDO opens web page
2. Page calls GET /v1/agents on the bus (via operator)
   → displays available workflows grouped by capability
3. BDO uploads a document and selects one or more workflows
4. Operator creates a submission:
   - Generates a unique submission token
   - For each selected workflow:
     - Sends a "request" message to the entry-point agent on the bus
   - Returns the submission token to the browser
5. Browser polls GET /status/{token} with a spinner
6. As final reports arrive in the operator's bus inbox:
   - Operator stores them in memory keyed by submission token
   - Status endpoint returns completed reports
7. BDO downloads the report(s)
   - Report is self-contained: date-stamped, version-numbered
   - BDO is responsible for saving it in the appropriate place
```

### Submission Isolation

Each submission gets a unique token (UUID or similar). The token is the only way
to retrieve results. No token = no access. This provides per-submission isolation
without an auth system — similar to a share link.

---

## Operator Endpoints (Browser-Facing)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve the submission UI |
| GET | `/workflows` | List available workflows (proxies bus agent registry) |
| POST | `/submit` | Upload document + select workflows → returns `{token}` |
| GET | `/status/{token}` | Poll submission status → `{status, reports[]}` |
| GET | `/report/{token}/{workflow}` | Download a completed report |

### POST /submit

Request (multipart/form-data):
- `file` — the document (PDF, etc.)
- `workflows` — comma-separated workflow identifiers (capabilities)
- `case_id` — optional identifier for the submission

Response:
```json
{
  "token": "abc-123-def",
  "workflows": ["patent-screen", "prior-art"],
  "status": "submitted"
}
```

### GET /status/{token}

Response:
```json
{
  "token": "abc-123-def",
  "status": "partial",
  "workflows": {
    "patent-screen": {"status": "completed", "ready": true},
    "prior-art": {"status": "executing", "ready": false}
  }
}
```

Status values: `submitted` → `executing` → `completed` | `error`

The browser polls this endpoint on an interval. When all workflows are
`completed`, the BDO downloads the reports.

---

## Operator as a Bus Agent

The operator registers on the bus like any other agent:

```json
{
  "agent_id": "operator",
  "capabilities": ["submission-portal"],
  "mode": "pull",
  "ttl": 120,
  "secret": "<operator-secret>"
}
```

### Sending Requests

When a BDO submits a document for the "patent-screen" workflow, the operator:

1. Looks up agents with capability `patent-screen` via `GET /v1/agents?capability=patent-screen`
2. Sends a `request` message to the entry-point agent:
   ```json
   {
     "to": "intake",
     "from": "operator",
     "conversation_id": "submission-abc-123-def",
     "request_id": "sub-abc-123-patent",
     "type": "request",
     "body": "{\"task\": \"patent-screen\", \"case_id\": \"CASE-2026-042\"}",
     "attachments": [{"url": "file:///path/to/uploaded.pdf", "name": "disclosure.pdf", "content_type": "application/pdf"}]
   }
   ```
3. Stores a mapping: `token + workflow → conversation_id + request_id`

### Receiving Results

The operator's poll loop watches its inbox for `response` messages. When one
arrives, it matches it back to the submission token and stores the report body
in memory. The next time the browser polls `/status/{token}`, it sees the
completed workflow.

### Workflow Discovery

The operator does not hardcode any workflow. It discovers available specialist
agents from the bus registry by querying capabilities. When someone adds a new
specialist agent to the bus, it appears in the operator UI automatically.

The UI groups capabilities into human-readable workflow names. This mapping
can be a simple config file:

```json
{
  "patent-screen": {"label": "Patent Eligibility Screen", "description": "Assess patentability of an invention disclosure"},
  "prior-art": {"label": "Prior Art Search", "description": "Search for existing prior art"},
  "market-analysis": {"label": "Market Analysis", "description": "Identify target markets and potential value"},
  "commercialization": {"label": "Commercialization Strategy", "description": "Recommend patent, open source, or other paths"},
  "grant-restrictions": {"label": "Grant Restriction Lookup", "description": "Check funding terms for IP restrictions"}
}
```

Capabilities not in this config are either hidden or shown with their raw
capability name. This keeps the operator decoupled — it doesn't need a code
change when a new workflow appears, just an optional config update for a
friendly label.

---

## Report Format

Every report returned by a specialist pipeline must be self-contained:

- **Date** — when the analysis was performed
- **Version** — version of the specialist agent/workflow
- **Case ID** — ties back to the submission
- **Content** — the analysis itself
- **Disclaimer** — standard disclaimer about automated analysis

The operator does not modify report content. It passes through whatever the
final agent in the pipeline produces.

---

## In-Memory State

The operator holds submission state in memory:

```
submissions map[token] → {
    created_at    time.Time
    case_id       string
    workflows     map[capability] → {
        status          string        // submitted | executing | completed | error
        conversation_id string
        request_id      string
        report          string        // final report body, empty until done
    }
}
```

This state is ephemeral. If the operator restarts, in-flight submissions are
lost. This is acceptable because:

- Submissions complete in seconds to minutes, not hours
- The BDO can resubmit if needed
- There is no persistent history requirement

---

## UI

Single-page web application. Minimal, functional.

### Submission View
- File upload (drag-and-drop or file picker)
- Optional case ID field
- Checkboxes for available workflows (populated from `/workflows`)
- Submit button → navigates to status view

### Status View (at `/status/{token}`)
- Shows each selected workflow with a spinner or checkmark
- When a report is ready, shows a download button
- Auto-refreshes via polling (every few seconds)
- No login, no history, no navigation to other submissions

Technology: vanilla HTML/CSS/JS. No framework. Served as static files by the
operator process.

---

## File Layout

```
cmd/
  operator/
    main.go              # Process entry point, starts web server + bus poll loop

internal/
  operator/
    server.go            # HTTP handlers: /, /workflows, /submit, /status, /report
    bridge.go            # Bus client: registration, heartbeat, inbox polling, routing
    submissions.go       # In-memory submission state management

web/
  index.html             # Submission + status UI
  style.css
  app.js
```

The operator reuses `internal/patentteam/client.go` for all bus communication
(or extracts it to a shared `internal/busclient/` package).

---

## Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | No auth system | Internal tool, ~10 users, network-level access control |
| 2 | Submission tokens for isolation | BDO only sees their own results without needing accounts |
| 3 | Polling, not SSE | Only the final report matters; progress is cosmetic; polling is simpler |
| 4 | Workflow-agnostic operator | New specialists can be added to the bus without touching the operator |
| 5 | No persistent submission history | Report is self-contained; BDO saves it; bus state file covers debug |
| 6 | In-memory state only | Submissions are short-lived; restart = resubmit; matches bus simplicity |
| 7 | Capability-based discovery | Operator finds workflows from bus agent registry, not hardcoded config |
| 8 | Vanilla JS UI | No build step, no dependencies, matches project's zero-dependency philosophy |

---

## Future Considerations (Not Now)

- **Auth**: if this moves beyond UCLA or onto the public internet, add an auth layer in front
- **Persistence**: if submissions need audit trails, persist to a database
- **Cancellation**: the bus protocol has no cancel primitive; add one if long-running workflows need it
- **SSE/WebSocket**: upgrade from polling if real-time progress becomes a user need
- **File storage**: currently attachment URLs are file paths; may need object storage for multi-node deployment
