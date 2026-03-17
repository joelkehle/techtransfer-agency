# Decisions And Backlog

Last updated: 2026-03-17

This document tracks architecture/ops decisions and follow-up work discussed during migration + demo hardening.

Related migration plan:

- `docs/AGENT_BUS_EXTRACTION_CHECKLIST.md` - PR-by-PR plan for extracting the bus into a dedicated repo
- `docs/UCLA_REPO_PROMOTION_PLAN.md` - plan for promoting `tdg-ip-agents` into the UCLA customer/product repo

## Confirmed Decisions

### Repo Ownership

- `tdg-ip-agents` is the target UCLA product repo, not just the SME-agent repo.
- `pinakes` is the target authoritative bus repo.
- `techtransfer-agency` is a transition repo during extraction; it should end either as a very small infra/shared repo or be retired.
- `pkg/busclient` stays in `techtransfer-agency` for the SME-agent split completed on 2026-03-04. That note is historical, not a veto on later extraction. For the later bus split, see `docs/AGENT_BUS_EXTRACTION_CHECKLIST.md` and `docs/UCLA_REPO_PROMOTION_PLAN.md`.
- operator is UCLA product code, not bus infrastructure.
- current PDF utilities are UCLA product code, not bus infrastructure.

### What Moves To `tdg-ip-agents`

- operator/web UI
- PDF utilities:
  - `cmd/patent-extractor/` + `internal/pdfextractor/`
  - `cmd/render-patent-report/`
- `cmd/patent-screen/` + `internal/patentscreen/`
- `cmd/prior-art-search/` + `internal/priorartsearch/`
- `cmd/market-analysis/` + `internal/marketanalysis/` if UCLA wants a single three-agent deliverable repo

### What Stays In `techtransfer-agency`

- short term during migration: existing mixed code until moves complete
- long term: only explicitly shared infra that still justifies a standalone repo
- if that list becomes trivial, sunset the repo

### Bus Ownership / Compatibility

- `pinakes` should assume no built-in operator and no built-in PDF pipeline.
- `pinakes` owns protocol contract tests and semantic versioning policy.
- consumer repos pin bus releases/tags.
- breaking protocol or public API changes require a major version bump.

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
