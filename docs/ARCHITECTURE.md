# IP Agency Architecture

Visual diagram: [architecture.html](/architecture.html) (served at techtransfer.agency/architecture.html)

Tracking:
- Decisions + backlog: `docs/DECISIONS_AND_BACKLOG.md`
- Migration target: `docs/AGENT_BUS_EXTRACTION_CHECKLIST.md`
- UCLA promotion target: `docs/UCLA_REPO_PROMOTION_PLAN.md`

## Repositories

| Repo | Owner | Purpose |
|------|-------|---------|
| `techtransfer-agency` | Infrastructure | HTTP bus, operator, utility agents, shared client library |
| `tdg-ip-agents` | UCLA TDG | Patent eligibility screen, prior art search |

## Target Split After Bus Extraction

Planned target architecture:

- `pinakes` - reusable bus/runtime/client/discovery substrate
- `tdg-ip-agents` - UCLA product repo: operator, PDF utilities, UCLA agents, deploy packaging
- `techtransfer-agency` - transition repo while code moves; later either explicit small infra repo or sunset candidate

The rest of this document describes the current mixed deployment shape before that split is finished.

## Layers

The system is organized top-to-bottom in four layers:

### 1. Operator

**Operator** (`cmd/operator`)
Web UI agent — accepts invention disclosure PDFs from inventors/TTO staff,
dispatches work via the bus, and renders reports back to the user.
The Operator is an agent: it registers with the bus and communicates
through it like any other agent. It never talks directly to downstream agents.

### 2. HTTP Bus (Infrastructure)

**HTTP Bus** (`cmd/techtransfer-agency`)
Core message router. Agents register with a capability and HMAC secret,
poll an inbox, ack messages, and post responses. The bus has no knowledge
of what agents do — it routes by capability name.

**Bus Client** (`pinakes/pkg/busclient`)
Shared Go module for agent-to-bus communication: registration, heartbeat,
inbox polling, message ack, response posting. Imported by all agent repos
via `github.com/joelkehle/pinakes/pkg/busclient`.

### 3. Utility Agents

**PDF Extractor** (`cmd/patent-extractor`, `internal/pdfextractor`)
Extracts text from uploaded PDFs and forwards to downstream agents.
One binary, deployed as N container instances configured by flags
(`-next-agent-id`, `-capability`). No domain logic — deploy-time plumbing.

**Report Renderer** (`cmd/render-patent-report`)
Converts agent markdown reports to branded PDF output. Shared
infrastructure — agents produce markdown content, the renderer handles
visual presentation so all reports have a consistent look.

### 4. Subject Matter Expert Agents

SME agents can be contributed by different teams. Each only needs
`pkg/busclient` and a capability name to participate.

**UCLA TDG** (`tdg-ip-agents` repo):

- **Patent Screen** (`cmd/patent-screen`) — Multi-stage LLM analysis
  (Alice/Mayo, novelty, non-obviousness, utility, enablement), produces
  PI-readable eligibility report.
- **Prior Art Search** (`cmd/prior-art-search`) — Queries PatentsView API,
  runs LLM relevance assessment, produces prior art report.

**Market Analysis** (`cmd/market-analysis`, stays in `techtransfer-agency` for now):

- LLM analysis producing auditable market report. Separate contributor
  grouping from TDG agents.

## Data Flow

```
Inventor / TTO Staff
        |
        | uploads PDF
        v
   +-----------+
   |  Operator  |  (web UI agent)
   +-----------+
        ↕
   HTTP / HMAC-SHA256
        ↕
   +-----------+
   |  HTTP Bus  |  (routes by capability)
   +-----------+
        ↕
   HTTP / HMAC-SHA256
        ↕
  +--- UTILITY AGENTS ------+
  | PDF Extractor            |
  | Report Renderer          |
  +--------------------------+
        |              ↑
        | extracted     | markdown
        | text          | reports
        v              |
  +--- SME AGENTS ---------------------+
  |                                     |
  | [UCLA TDG]         [Market]         |
  |  Patent Screen      Market Analysis |
  |  Prior Art Search                   |
  |                                     |
  +-------------------------------------+
```

All arrows go through the bus — the diagram above shows the logical
data flow for readability, not the physical routing.

## Communication

- **Protocol**: HTTP, HMAC-SHA256 signed requests
- **Agent lifecycle**: register → heartbeat → poll inbox → ack → process → respond
- **Capability routing**: bus routes messages to agents by capability name
- **No direct agent-to-agent calls**: all communication goes through the bus

## Auth

Each agent authenticates with its own HMAC-SHA256 secret — the bus and
agent share a key that signs every request, proving identity without
transmitting passwords. HMAC verifies who sent a message and that it
wasn't tampered with. It does not encrypt content — agent-to-bus traffic
runs on a private Docker network. Remote deployment would add TLS.

## Deployment

All components run as Docker containers on a shared network (`agentnet`).
The bus repo's `docker-compose.yml` defines bus, operator, and utility
agent services. SME agent services reference pre-built images from their
respective repos.

Operator is exposed on port 3000, proxied via Cloudflare tunnel
(`techtransfer.agency`).
