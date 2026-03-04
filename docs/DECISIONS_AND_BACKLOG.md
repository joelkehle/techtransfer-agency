# Decisions And Backlog

Last updated: 2026-03-04

This document tracks architecture/ops decisions and follow-up work discussed during migration + demo hardening.

## Confirmed Decisions

### Repo Ownership

- `tdg-ip-agents` is the UCLA SME agent repo.
- `techtransfer-agency` remains infrastructure + shared utilities.
- `pkg/busclient` stays in `techtransfer-agency` and is consumed as a Go module from there (no extra repo).
- `render-patent-report` stays in `techtransfer-agency` as shared utility infrastructure.
- Extractors stay infra-side (`patent-extractor`, `prior-art-extractor`, `market-extractor`).

### What Moves To `tdg-ip-agents`

- `cmd/patent-screen/` + `internal/patentscreen/`
- `cmd/prior-art-search/` + `internal/priorartsearch/`

### What Stays In `techtransfer-agency`

- Bus, Operator, `pkg/busclient`
- Utility agents:
  - `cmd/patent-extractor/` + `internal/pdfextractor/`
  - `cmd/render-patent-report/`
- `cmd/market-analysis/` + `internal/marketanalysis/`

### Compose / Runtime

- Host currently uses Docker Compose v1 (`docker-compose`), so v1-compatible commands are required.
- GHCR org for SME images: `ghcr.io/joelkehle/tdg-ip-agents/...`
- `tdg-ip-agents` and image visibility were made public for unauthenticated pulls.
- `uploads` volume on SME agents should be re-verified before removal; current behavior can run from extracted text payloads.
- Stack isolation now uses configurable network + namespaced agent IDs via compose env vars:
  - `STACK_NETWORK_NAME`
  - `*_AGENT_ID` for operator, extractors, and SME agents
  - operator preferred targets via `WORKFLOW_TARGET_*` envs

### Demo / Development Mode

- UI now uses single-select services (radio).
- UI has explicit `Demo mode (use canned outputs)` toggle + banner.
- Demo mode replays fixtures for:
  - `patent-screen`
  - `prior-art-search`
- Capture helper added:
  - `scripts/capture-replay-fixture.sh`

### Secret Source

- Local `.env` is preferred over Infisical for this stack at this time.
- Infisical remains optional for future central management.

### Demo Inputs (No OCR Required)

Recommended disclosures for reliable demo runs without OCR:

- `uploads/2026-124 INVENTION DISCLOSURE (SEMWAL).pdf` (strong patent-screen input)
- `uploads/2026-087 SOFTWARE DISCLOSURE (BAKER).pdf` (strong prior-art-search input)

## Open Follow-Ups

### P0 (Operational Stability)

1. Agent ID collision hardening.
   - Problem: stale containers with the same `agent_id` can interfere on the same bus.
   - Implemented baseline:
     - compose-level namespaced `*_AGENT_ID` defaults (`tta-*`)
     - isolated stack network (`STACK_NETWORK_NAME`)
     - strict `AGENT_ALLOWLIST` default aligned to namespaced IDs
     - operator preferred routing via `WORKFLOW_TARGET_*` env vars
   - Remaining architecture fix: add bus-side lease/ownership semantics for `agent_id` (reject conflicting re-registration even without allowlist).
2. Prior-art agent response robustness in Stage 4.
   - Current issue: strict JSON schema unmarshal failures from LLM output can fail the run.
   - Needed: tolerant parsing / normalization before strict decode, plus retry shaping.
3. Prior-art runtime hygiene.
   - Ensure only one active `prior-art-search` + `prior-art-extractor` pair on the target bus.
   - Add startup validation that logs hard errors on duplicate/conflicting registrations.

### P1 (Product/Demo Improvements)

1. `render-patent-report` decoupling.
   - Remove `internal/patentscreen` coupling; accept generic markdown/report envelope.
2. OCR fallback for image-only PDFs.
   - Add optional OCR pipeline (`ocrmypdf`/`tesseract`) when text extraction fails.
3. Cost observability.
   - Add Langfuse self-hosted deployment plan (Docker) for token/cost tracing.

### P2 (Ops and Governance)

1. Formal environment profile support.
   - Explicit `demo`, `dev`, `prod` profiles with isolated bus/agent IDs.
2. Optional Infisical profile.
   - Keep local `.env` default; provide optional Infisical-backed run path.

## Guardrails For Live Demo Runs

1. Verify service health before submitting:
   - `curl -fsS http://localhost:8080/v1/health`
   - `docker ps` for bus/operator/extractors/SME agents
2. Ensure required keys are present in `.env`:
   - `ANTHROPIC_API_KEY`
   - `PATENTSVIEW_API_KEY` (required for prior-art-search)
3. For cost-safe rehearsal:
   - keep UI demo mode enabled and use fixture replay
4. For real capture:
   - disable demo mode, run one workflow at a time, capture fixture, re-enable demo mode
