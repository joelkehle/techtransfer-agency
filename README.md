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

For components that authenticate to the bus (`operator`, `patent-extractor`, `patent-screen`, `patent-pipeline`, `patent-team`), required secrets are loaded from environment variables with no defaults. See `.env.example` for the full variable list.

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

### Patent Screen Agent

Run the standalone patent eligibility screen agent:

```bash
export PATENT_SCREEN_AGENT_SECRET=replace-with-strong-secret
export ANTHROPIC_API_KEY=replace-with-api-key
go run ./cmd/patent-screen --bus-url http://localhost:8080 --agent-id patent-screen
```

For operator-driven submissions (`workflow=patent-screen`) with PDF attachments, run the extractor agent too:

```bash
export PATENT_EXTRACTOR_AGENT_SECRET=replace-with-strong-secret
go run ./cmd/patent-extractor --bus-url http://localhost:8080 --agent-id patent-extractor --next-agent-id patent-screen
```

Details: `docs/PATENT_ELIGIBILITY_SCREEN_SPEC.md`

### Prior Art Search Agent

Run the standalone prior art search agent:

```bash
export PRIOR_ART_AGENT_SECRET=replace-with-strong-secret
export ANTHROPIC_API_KEY=replace-with-api-key
export PATENTSVIEW_API_KEY=replace-with-api-key
go run ./cmd/prior-art-search --bus-url http://localhost:8080 --agent-id prior-art-search
```

For operator-driven submissions (`workflow=prior-art-search`) with PDF attachments, run the extractor route too:

```bash
export PRIOR_ART_EXTRACTOR_AGENT_SECRET=replace-with-strong-secret
go run ./cmd/patent-extractor --bus-url http://localhost:8080 --agent-id prior-art-extractor --capability prior-art-search --secret-env PRIOR_ART_EXTRACTOR_AGENT_SECRET --next-agent-id prior-art-search
```

Details: `docs/PRIOR_ART_SEARCH_SPEC_v3.2.md`

### Runtime Options

- `STORE_BACKEND`:
  - `persistent` (default): snapshot-backed store persisted to disk
  - `memory`: in-memory only
- `STATE_FILE`: path for persistent snapshot file (default `./data/state.json`)
- `AGENT_ALLOWLIST`: optional comma-separated allowed agent IDs
- `HUMAN_ALLOWLIST`: optional comma-separated allowed human injection identities

### Local Deploy Notes

- Prefer Docker Compose v2 (`docker compose`) over legacy `docker-compose` v1.
- If `docker compose` is missing, install the plugin for your user:

```bash
mkdir -p ~/.docker/cli-plugins
curl -fsSL https://github.com/docker/compose/releases/download/v2.27.0/docker-compose-linux-x86_64 \
  -o ~/.docker/cli-plugins/docker-compose
chmod +x ~/.docker/cli-plugins/docker-compose
docker compose version
```

- To redeploy patent-screen pipeline services without restarting `bus`:

```bash
make redeploy-patent-screen
```

This rebuilds and restarts only `operator`, `patent-extractor`, and `patent-screen`.

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
- `docs/PATENT_ELIGIBILITY_SCREEN_SPEC.md`
- `docs/PRIOR_ART_SEARCH_SPEC_v3.2.md`
- `docs/PRIOR_ART_SEARCH_IMPLEMENTATION_PROMPT.md` (implementation history)

## License

This project is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE).

You may view, use, and modify the source for any noncommercial purpose. This includes personal use, research, education, and use by charitable organizations, educational institutions, and government institutions.

Commercial use requires a separate license. Contact joel@techtransfer.agency for commercial licensing inquiries.

Agents that connect to the bus operate under their own licenses but must be open source or source-available per the store terms.
