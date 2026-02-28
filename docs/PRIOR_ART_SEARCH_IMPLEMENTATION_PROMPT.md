# Codex Task: Build Prior Art Search Agent

> Note: This prompt is retained as implementation history for the prior-art-search build. Active behavior/spec authority is `docs/PRIOR_ART_SEARCH_SPEC_v3.2.md`.

## Context

You are building a new agent for the TechTransfer Agency system. The codebase is at `joelkehle/techtransfer-agency`. Two sibling agents already exist and are working:

- **Patent Eligibility Screen** agent (`internal/patentscreen/`)
- **Market Analysis** agent (`internal/marketanalysis/`)

Both follow the same architecture: deterministic Go pipeline, LLM provides judgment within defined stages, Go code controls flow and validates outputs. The bus client is in `internal/busclient/`.

You are building the third agent: **Prior Art Search**.

## Spec

The complete specification is in `PRIOR_ART_SEARCH_SPEC_v3.2.md` (attached). This spec has been through 4 review cycles (internal + external) and all API field paths have been verified against the PatentsView Swagger docs. Implement it as written. If something seems ambiguous, check the spec again — it's probably addressed. If it genuinely isn't, match the pattern from `internal/patentscreen/`.

## What to build

1. `internal/priorartsearch/` — the agent package, following the file structure in the spec:
   - `agent.go` — bus registration, inbox polling, message handling
   - `pipeline.go` — 5-stage pipeline orchestrator with degraded mode support
   - `stages.go` — Stages 1, 3, 4 (LLM stages)
   - `search.go` — Stage 2 (PatentsView API client, query builder, token hygiene, rate limiter)
   - `report.go` — Stage 5 (report assembly: normal + 2 degraded layouts)
   - `llm.go` — Anthropic API client wrapper
   - `types.go` — all types, schemas, enums, Disclaimer constant, stopwords list
   - `*_test.go` — tests for each file as specified

2. `cmd/prior-art-search/main.go` — binary entry point

## Critical implementation rules

- **Do NOT import from** `internal/patentteam/`, `internal/patentscreen/`, or `internal/marketanalysis/`. Only import from `internal/busclient/`.
- **Do follow the existing patterns** in `internal/patentscreen/` for: bus registration, inbox polling, LLM retry logic, progress events, error event posting. Read that code first to understand the conventions.
- **PatentsView API:** Always POST. Always include `X-Api-Key` header. Always include `s` sort parameter. Always check `error` field in response body. Field paths are in the spec and have been verified — use them exactly.
- **Token hygiene** is specified in detail in Stage 2. Implement the normalizer exactly: order-preserving dedupe, preserve 2-letter acronyms from the acronyms list, exclude patent_variants, stopword list is in the spec.
- **Query IDs** must be unique per API call: `Q1_narrow_p1`, `Q1_narrow_p2`, `Q1_broad`, etc. These are used in `matched_queries`, `total_hits_by_query`, and StrategyCount derivation.
- **Degraded mode:** If Stage 3 fails, produce a report with raw results + code-only determination. If Stage 4 fails, produce a report with scored results + code-generated statistics + code-only determination. The degraded determination heuristic is in the spec.
- **Stage 4:** `patent_count` is NOT in the LLM output schema. Go attaches it by matching `key_players[].name` against `assignee_frequency`. `blocking_patents` must be membership-validated against the Stage 3 assessed set.
- **LLM prompts:** Include them verbatim from the spec. All prompts start with "Return valid JSON only. No markdown fences, no commentary."
- **Accept+trim policy for Stage 1:** String length overages are truncated, not retried. Only structural failures (missing fields, wrong types) trigger retries.
- **Stage 3 with 0 patents:** Skip the LLM call entirely. Return empty assessments. No tokens spent.

## Environment variables

```
PATENTSVIEW_API_KEY     (required — Joel will provide before demo)
ANTHROPIC_API_KEY       (required)
PRIOR_ART_LLM_MODEL    (default: claude-sonnet-4-20250514)
PRIOR_ART_MAX_PATENTS   (default: 300)
PRIOR_ART_MAX_ASSESS    (default: 75)
PRIOR_ART_BATCH_SIZE    (default: 15)
PRIOR_ART_RATE_LIMIT    (default: 40)
PRIOR_ART_AGENT_SECRET  (required)
```

## Test fixtures

Create 2 test disclosures:
1. Software/AI invention (CPC G06N / G06F)
2. Biotech/pharma invention (CPC A61K / C12N)

Mock PatentsView responses with real patent IDs and abstracts from public patents. Store as JSON files in `internal/priorartsearch/testdata/`.

## Definition of done

- `go build ./cmd/prior-art-search/` succeeds
- `go test ./internal/priorartsearch/...` passes
- `go vet ./...` clean
- Agent registers on the bus, receives a disclosure, runs the 5-stage pipeline, and returns a structured report with markdown
- All 3 report layouts work (normal, Stage 3 degraded, Stage 4 degraded)
- Rate limiter respects 40 req/min
- Code-only determination fallback works when LLM stages fail
