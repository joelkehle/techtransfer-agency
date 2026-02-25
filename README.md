# TechTransfer Agency

Reference implementation of the TechTransfer Agency Protocol v2 for university tech transfer workflows.

## Goals
- Decoupled bus service for agent-to-agent communication
- HTTP + SSE interface, no frontend coupling
- Works with TDG Assistant (or any client) over HTTP
- Persistent state backend with in-memory alternative
- Push-mode callback delivery with retry/backoff

## Endpoints
- `POST /v1/agents/register`
- `GET /v1/agents`
- `POST /v1/conversations`
- `GET /v1/conversations`
- `POST /v1/messages`
- `GET /v1/inbox`
- `POST /v1/acks`
- `POST /v1/events`
- `GET /v1/observe`
- `POST /v1/inject`
- `GET /v1/conversations/{id}/messages`
- `GET /v1/health`
- `GET /v1/system/status`

## Run

```bash
go run ./cmd/techtransfer-agency
```

Server listens on `:8080` (or `PORT`).

For components that authenticate to the bus (`operator`, `patent-pipeline`, `patent-team`), required secrets are loaded from environment variables with no defaults. See `.env.example` for the full variable list.

### Patent Team Demo (End-to-End Use Case)

Run a concrete multi-agent workflow for PDF invention screening:

```bash
export PATENT_TEAM_COORDINATOR_SECRET=replace-with-strong-secret
export PATENT_TEAM_INTAKE_SECRET=replace-with-strong-secret
export PATENT_TEAM_EXTRACTOR_SECRET=replace-with-strong-secret
export PATENT_TEAM_PATENT_AGENT_SECRET=replace-with-strong-secret
export PATENT_TEAM_REPORTER_SECRET=replace-with-strong-secret
go run ./cmd/patent-team --pdf /absolute/path/to/disclosure.pdf --case-id CASE-2026-001
```

This command starts a local in-process bus by default and runs:

- `coordinator -> intake -> pdf-extractor -> patent-agent -> reporter -> coordinator`

Details: `docs/PATENT_TEAM.md`

### Runtime Options

- `STORE_BACKEND`:
  - `persistent` (default): snapshot-backed store persisted to disk
  - `memory`: in-memory only
- `STATE_FILE`: path for persistent snapshot file (default `./data/state.json`)
- `AGENT_ALLOWLIST`: optional comma-separated allowed agent IDs
- `HUMAN_ALLOWLIST`: optional comma-separated allowed human injection identities

### Authentication

- Register with `secret` in `POST /v1/agents/register`.
- Signed endpoints require `X-Bus-Signature` using HMAC-SHA256(secret, payload):
  - Header accepts raw hex digest or `sha256=<hex>` format.
  - `POST /v1/messages` (payload = raw request body)
  - `GET /v1/inbox` (payload = raw query string)
  - `POST /v1/acks` (payload = raw request body)
  - `POST /v1/events` (payload = raw request body, plus `X-Agent-ID`)

## Test

```bash
go test ./...
```

Full quality gate:

```bash
make tools   # one-time: installs staticcheck + govulncheck
make gate
```

## Spec
- `docs/PROTOCOL_SPEC_v2.md`
- `docs/NORMATIVE_CLARIFICATIONS.md`

## License

This project is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE).

You may view, use, and modify the source for any noncommercial purpose. This includes personal use, research, education, and use by charitable organizations, educational institutions, and government institutions.

Commercial use requires a separate license. Contact joel@techtransfer.agency for commercial licensing inquiries.

Agents that connect to the bus operate under their own licenses but must be open source or source-available per the store terms.
