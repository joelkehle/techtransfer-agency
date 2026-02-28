# Prior Art Search Agent — Spec v3.2

> Note: This is the reviewed implementation spec used to build `internal/priorartsearch/` and `cmd/prior-art-search/`. Keep this file versioned in `docs/` as the source of truth for this agent's behavior.

> v3.2: Final patches. FilingDate changed to *string (nullable). Query IDs now unique per API call (Q1_narrow_p1/p2/p3, Q1_narrow_all, Q1_broad). StrategyCount derived from Q# prefix. key_players prompt requires exact name match from assignee frequency.
>
> v3.1: CPC regex includes Y section. Deterministic first-3 phrases. Hard cap on stored patents. Order-preserving dedupe. 2-letter acronyms preserved. patent_variants excluded from tokens. Primary assignee counting. Dropped unused assignee_type.
>
> v3: Fixed field paths (assignees.*, inventors.*), dropped patent_num_claims, nested array flattening, error flag handling, CPC regex, broad query token explosion fix, title OR abstract, CPC OR arrays, degraded report layouts, accept+trim, ordering semantics, map-by-ID batch validation, patent_count override, blocking_patents membership, structured_results shapes, LLM model/temp config, assignee normalization, code-only determination fallback.

## What This Agent Does

The Prior Art Search agent takes an invention disclosure as input and produces a structured prior art landscape report. It queries the USPTO PatentsView PatentSearch API to find granted U.S. patents that are potentially relevant to the invention, then uses the LLM to assess relevance and map the results against the invention's novel elements.

This is a preliminary search, not a freedom-to-operate opinion or a professional patentability search. It tells a tech transfer officer what the patent landscape looks like around an invention so they can decide whether to engage outside patent counsel. It provides specific patent numbers, titles, and relevance assessments so the TTO analyst can follow up.

The agent is designed to consume the output of the Patent Eligibility Screen agent's Stage 6 (§102/§103 preliminary flags), which provides prior art search priority and novelty/non-obviousness concerns. When that data is available, the search is targeted. When it's not (standalone mode), the agent derives everything from the disclosure text directly.

### MVP Scope (Friday Demo)

This version searches granted U.S. patents only via `/api/v1/patent/`. It does NOT cover:
- Pre-grant publications (future: `/api/v1/publication/`)
- Patent claims text or brief summary text (beta endpoints, limited year coverage)
- CPC description lookup
- Foreign patents or non-patent literature
- Citation graph expansion (seed → cited-by / cites traversal)

These are all v2 enhancements.

## Architecture

Deterministic state machine in Go. Same pattern as the Patent Eligibility Screen and Market Analysis agents: registers on the bus, receives a disclosure via its inbox, processes it through a sequential pipeline, returns a structured report.

```
Disclosure In (+ optional prior_context from Patent Eligibility Screen)
    │
    ▼
┌─────────────────────────────────┐
│ Stage 1: Search Strategy        │  LLM reads disclosure, produces search plan
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────┐
│ Stage 2: Patent Search          │  Code executes queries against PatentsView API
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────┐
│ Stage 3: Relevance Assessment   │  LLM scores each result against the disclosure
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────┐
│ Stage 4: Landscape Analysis     │  LLM synthesizes findings into landscape view
└───────────┬─────────────────────┘
            │
            ▼
┌─────────────────────────────────┐
│ Stage 5: Report Assembly        │  Code only — no LLM. Assembles final report.
└─────────────────────────────────┘
    │
    ▼
Structured Report Out
```

All stages are sequential. No early exits — the agent always produces a report, even if no relevant prior art is found.

**Degraded modes:**
- **Stage 3 fails:** Report contains search coverage + raw patent list (title, abstract, assignee) without relevance scoring. Determination computed by code-only fallback heuristic.
- **Stage 4 fails:** Report contains search coverage + scored results from Stage 3, plus code-generated landscape statistics (top assignees, CPC histogram, novel element coverage). No LLM narrative. Determination computed by code-only fallback heuristic.
- **Stage 1 or Stage 2 total failure:** Pipeline errors. No report.

**Code-only determination fallback** (used in any degraded mode):
```
if count(HIGH) > 0          → BLOCKING_ART_FOUND
else if count(MEDIUM) >= 5   → CROWDED_FIELD
else if total_retrieved == 0  → INCONCLUSIVE
else                          → CLEAR_FIELD
```
If Stage 3 failed (no assessments), all counts are 0, so fallback produces INCONCLUSIVE.

## External Dependencies

### PatentsView PatentSearch API

Base URL: `https://search.patentsview.org`

All endpoint paths are absolute. Example full URL: `https://search.patentsview.org/api/v1/patent/`

Authentication: `X-Api-Key` header. Loaded from `PATENTSVIEW_API_KEY` env var. If not set, immediate error: "PATENTSVIEW_API_KEY not configured."

Rate limit: 45 requests/minute. Client-side token bucket at 40 req/min. On 429, respect `Retry-After` header if present; otherwise exponential backoff (1s, 2s, 4s), up to 3 retries.

Request format: Always POST with JSON body. Body keys: `q` (required), `f`, `s`, `o`.

#### Response Handling

Every response body is JSON with this structure:
```json
{
  "error": false,
  "count": 100,
  "total_hits": 4521,
  "patents": [ ... ]
}
```

**Critical:** Check `error` field first. If `error` is `true`, treat as failure even if HTTP status is 200. Log the response body for debugging.

The data array key is `patents` for the `/api/v1/patent/` endpoint (matches endpoint name).

#### MVP Endpoint

| Endpoint | Purpose |
|----------|---------|
| `/api/v1/patent/` | Search granted U.S. patents by keyword, CPC, date, assignee |

#### Query Language

JSON-based. Key operators:
- `_text_any`: matches ANY space-separated word (OR). Input: `{"_text_any": {"patent_abstract": "federated distributed collaborative"}}`.
- `_text_all`: matches ALL space-separated words (AND). Input: `{"_text_all": {"patent_abstract": "federated learning privacy"}}`.
- `_text_phrase`: matches exact phrase. Input: `{"_text_phrase": {"patent_abstract": "federated learning"}}`.
- `_and`, `_or`, `_not`: boolean combinators.
- `_gte`, `_lte`: range filters.
- String equality / array match: `{"field": "value"}` or `{"field": ["val1", "val2"]}` (array = match any).

**Multi-word synonyms in `_text_any`:** The API tokenizes on whitespace. A synonym like "secure aggregation" becomes two independent OR tokens "secure" and "aggregation." This means multi-word synonyms MUST be handled differently — see Token Hygiene below.

#### Fields to Request

```json
[
  "patent_id",
  "patent_title",
  "patent_abstract",
  "patent_date",
  "application.filing_date",
  "assignees.assignee_organization",
  "cpc_at_issue.cpc_subclass_id",
  "inventors.inventor_name_first",
  "inventors.inventor_name_last"
]
```

**Nested array flattening rules (Go):**
- `assignees`: array of objects. Collect `assignee_organization` strings. Drop empty/null. Preserve order. First non-empty entry is the "primary assignee" for display.
- `inventors`: array of objects. For each, join `inventor_name_first` + " " + `inventor_name_last`. Handle null first or last (use whichever is present). Cap at 10 inventors per patent.
- `cpc_at_issue`: array of objects. Collect `cpc_subclass_id` strings. Deduplicate. Preserve order.

**Assignee name normalization:** Trim whitespace, collapse internal whitespace to single space, preserve original casing, drop empty strings. This must be deterministic so frequency counts are stable.

#### CPC Subclass Validation

Stage 1 outputs CPC subclass codes. Before using them in queries, validate with regex:

```
^[A-HY][0-9]{2}[A-Z]$
```

This matches valid CPC subclass IDs (e.g., `G06N`, `A61K`, `H04L`, `Y02E`). Sections A–H plus Y (climate/energy tagging). Uppercase input before matching. Drop any that don't match and log a warning. This prevents the LLM from emitting group-level codes (contain `/`) or malformed strings.

#### CPC OR Handling

When a strategy has multiple CPC subclasses, use array syntax for OR:
```json
{"cpc_at_issue.cpc_subclass_id": ["G06N", "H04L"]}
```
Arrays are "match any" per the API docs. Do NOT combine with `_or` — use the array form.

#### Sorting

Always include `s`:
```json
"s": [{"patent_date": "desc"}, {"patent_id": "asc"}]
```
Most recent first, stable tiebreaker. This biases toward recent art. Acknowledged tradeoff: may miss older foundational patents. Acceptable for MVP triage; v2 can add a second pass sorted by citation count.

#### Pagination

Use `o.size` (max 1000). For MVP: one page per query, `"o": {"size": 200}`. Use `o.after` for cursor pagination if needed in future.

#### Text Search: Always Query Title OR Abstract

For every text search criterion (`_text_any`, `_text_all`, `_text_phrase`), query BOTH `patent_title` and `patent_abstract` using `_or`:

```json
{"_or": [
  {"_text_phrase": {"patent_title": "federated learning"}},
  {"_text_phrase": {"patent_abstract": "federated learning"}}
]}
```

This is a low-effort, high-recall win. Some patents have the key concept in the title but not the abstract, or vice versa.

### Anthropic API

`ANTHROPIC_API_KEY` env var. Model specified by `PRIOR_ART_LLM_MODEL` env var (default: `claude-sonnet-4-20250514`). Temperature: 0.0 for all stages (we want deterministic, conservative output).

## Bus Message Contracts

### Request Envelope (inbound)

```json
{
  "case_id": "string (required, non-empty)",
  "disclosure_text": "string (required, non-empty, >= 100 chars)",
  "metadata": {
    "source_filename": "string (optional)",
    "extraction_method": "string (optional)",
    "truncated": "boolean (optional)"
  },
  "prior_context": {
    "stage6_output": {
      "novelty_concerns": ["string"],
      "non_obviousness_concerns": ["string"],
      "prior_art_search_priority": "HIGH | MEDIUM | LOW",
      "reasoning": "string"
    },
    "stage1_output": {
      "invention_title": "string",
      "technology_area": "string",
      "novel_elements": ["string"],
      "invention_description": "string"
    }
  }
}
```

Validation:
- `case_id`: non-empty string.
- `disclosure_text`: >= 100 chars. If < 100, immediate error. If > 100,000, truncate and set `input_truncated` flag.
- `prior_context`: optional.

### Response Envelope (outbound)

```json
{
  "case_id": "string",
  "agent": "prior-art-search",
  "version": "3.2.0",
  "determination": "CLEAR_FIELD | CROWDED_FIELD | BLOCKING_ART_FOUND | INCONCLUSIVE",
  "report_markdown": "string",
  "structured_results": {
    "search_strategy": "Stage 1 output object (full schema)",
    "patents_found": {
      "patents": "Stage 2 patents array",
      "queries_executed": 0,
      "queries_failed": 0,
      "queries_skipped": 0,
      "total_api_calls": 0,
      "total_hits_by_query": {}
    },
    "assessments": "Stage 3 merged assessments array, or null if Stage 3 failed",
    "landscape": "Stage 4 output object, or null if Stage 4 failed"
  },
  "metadata": {
    "stages_executed": ["string"],
    "stages_failed": ["string"],
    "degraded": false,
    "degraded_reason": "string or null",
    "total_patents_retrieved": 0,
    "total_patents_assessed": 0,
    "abstracts_missing": 0,
    "assessed_none": 0,
    "assessment_truncated": false,
    "api_calls_made": 0,
    "duration_ms": 0,
    "model": "string",
    "temperature": 0.0,
    "input_truncated": false
  }
}
```

## Pipeline Stages

### Stage 1: Search Strategy Generation

Purpose: Analyze the disclosure. Extract invention title, novel elements, and technology domains. Produce a search plan with term families.

Input: `disclosure_text` + `prior_context` (if available).

LLM prompt context (include verbatim):
```
You are a patent search strategist. Given an invention disclosure, produce
a search plan for the USPTO PatentsView API and extract key invention
metadata.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble. Your entire response must be a single JSON object matching the
schema below.

PART 1 — INVENTION EXTRACTION

Extract from the disclosure:
- A concise title for the invention (10-200 chars)
- A one-paragraph summary (50-500 chars)
- The 3-10 novel elements that distinguish this invention from prior work.
  Assign each a stable ID (NE1, NE2, ... NE10).
- The broad technology domains (1-5)

PART 2 — SEARCH PLAN

Cover the invention from multiple angles:
1. DIRECT MATCH: Terms describing the core mechanism or method.
2. COMPONENT MATCH: Terms for individual components/subsystems.
3. PROBLEM-SOLUTION MATCH: Terms for the problem and general solution
   category.

For each query strategy, provide TERM FAMILIES. Patent literature uses
different vocabulary than disclosures. For each core concept, provide:
- The canonical term (as used in the disclosure)
- Synonyms and near-synonyms (how patents describe the same concept)
- Abbreviations and acronyms
- Common patent-ese variants

IMPORTANT RULES FOR TERM FAMILIES:
- Multi-word terms (2+ words) should be listed as PHRASES, not as
  entries in the synonyms list. The search system tokenizes on whitespace,
  so "secure aggregation" in the synonyms list becomes two separate
  words "secure" and "aggregation." Put multi-word terms in the phrases
  list instead.
- Single-word synonyms go in the synonyms list.
- Keep synonym lists to genuinely relevant technical terms. Don't pad.

For CPC codes: provide SUBCLASS-level codes only (4 characters, e.g.,
"G06N", "A61K", "H04L"). Do NOT provide group-level codes (which
contain slashes like "G06N3/08").

ORDERING: List items most-important-first within all arrays:
- term_families: most central concept first
- phrases: most specific/discriminating phrase first
- cpc_subclasses: most relevant subclass first

Generate between 3 and 5 query strategies.
```

If `prior_context.stage6_output` is available, append:
```
The Patent Eligibility Screen agent identified these concerns:

Novelty concerns: [insert novelty_concerns]
Non-obviousness concerns: [insert non_obviousness_concerns]
Search priority: [insert prior_art_search_priority]

Use these to sharpen your search. Novelty concerns indicate where prior
art is most likely.
```

If `prior_context.stage1_output` is available, append:
```
The Patent Eligibility Screen agent extracted:

Title: [insert invention_title]
Technology area: [insert technology_area]
Novel elements: [insert novel_elements]
Description: [insert invention_description]

Use this to inform your term families and CPC classification. You may
adopt the title and novel elements directly or refine them.
```

Required output schema:
```json
{
  "invention_title": "string (10-200 chars)",
  "invention_summary": "string (50-500 chars)",
  "novel_elements": [
    {
      "id": "string (NE1, NE2, ... NE10)",
      "description": "string (20-300 chars)"
    }
  ],
  "technology_domains": ["string (1-5 entries)"],
  "query_strategies": [
    {
      "id": "string (Q1, Q2, ...)",
      "description": "string (20-200 chars)",
      "term_families": [
        {
          "canonical": "string (single word or short compound noun)",
          "synonyms": ["string (single-word synonyms only, 0-8 entries)"],
          "acronyms": ["string (0-4 entries)"],
          "patent_variants": ["string (0-4 entries)"]
        }
      ],
      "phrases": ["string (multi-word exact phrases, 0-5 entries)"],
      "cpc_subclasses": ["string (0-5 entries, e.g. 'G06N')"],
      "priority": "PRIMARY | SECONDARY | TERTIARY"
    }
  ],
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)"
}
```

Validation (code):
- `invention_title`: 10-200 chars. If > 200, **truncate to 200 and log**. If < 10, retry.
- `invention_summary`: 50-500 chars. If > 500, **truncate to 500 and log**. If < 50, retry.
- `novel_elements`: 3-10 entries. Each: `id` matches `NE[1-9][0-9]?`, `description` 20-300 chars (truncate if over, retry if under 20). IDs must be unique and sequential.
- `technology_domains`: 1-5 entries, each non-empty.
- `query_strategies`: 3-5 entries.
- Each strategy: `id` non-empty, `description` 20-200 chars (truncate if over), `term_families` 1-8 entries, `phrases` 0-5 entries, `cpc_subclasses` 0-5 entries.
- Each term family: `canonical` non-empty, `synonyms` 0-8, `acronyms` 0-4, `patent_variants` 0-4.
- CPC subclass validation: apply regex `^[A-HY][0-9]{2}[A-Z]$`. Uppercase input before matching. Drop non-matching entries silently, log warning.
- `priority`: one of `PRIMARY`, `SECONDARY`, `TERTIARY`. Case-insensitive match, normalize to uppercase.
- At least one strategy must have priority `PRIMARY`.

**Accept+trim policy for Stage 1:** Since Stage 1 failure aborts the entire pipeline, all string length overages are handled by truncation (not failure). Only structural problems (missing fields, wrong types, too few novel elements) trigger retries.

### Stage 2: Patent Search Execution

Pure code — no LLM call.

Input: Stage 1 output.

#### Token Hygiene

Before building any `_text_any` or `_text_all` query string, apply this normalizer:

1. Collect terms from: `canonical` + `synonyms` + `acronyms` from all term families in the strategy. **Do NOT include `patent_variants`** — they exist for analyst interpretability in the report, not for search (they're too generic and overlap with stopwords).
2. Split on whitespace and hyphens.
3. Lowercase all tokens.
4. Drop tokens < 3 characters, **EXCEPT** tokens that originated from `term_families[].acronyms` (preserve 2-letter acronyms like AI, ML, VR, AR, RF).
5. Drop stopwords: `system`, `method`, `apparatus`, `device`, `process`, `using`, `based`, `for`, `and`, `the`, `of`, `via`, `includes`, `comprising`, `network`, `means`, `said`, `wherein`, `thereof`, `therein`, `step`, `steps`.
6. Deduplicate while **preserving first-seen order** (use a slice + seen-set, not a map iteration). This matters because the "first 30" cap relies on stable ordering from most-important-first term families.
7. Cap at 30 tokens max. Keep the first 30.

#### Query Construction

For each query strategy, build TWO queries:

**Narrow query (high precision):**
- Use `_text_phrase` for the first entry in `phrases` (most discriminating phrase).
- Search title OR abstract (wrapped in `_or`).
- If CPC codes available, add as `_and` clause using array syntax.

```json
{
  "q": {
    "_and": [
      {"_or": [
        {"_text_phrase": {"patent_title": "federated learning"}},
        {"_text_phrase": {"patent_abstract": "federated learning"}}
      ]},
      {"cpc_at_issue.cpc_subclass_id": ["G06N"]}
    ]
  },
  "f": ["patent_id", "patent_title", "patent_abstract", "patent_date",
        "application.filing_date",
        "assignees.assignee_organization",
        "cpc_at_issue.cpc_subclass_id",
        "inventors.inventor_name_first",
        "inventors.inventor_name_last"],
  "s": [{"patent_date": "desc"}, {"patent_id": "asc"}],
  "o": {"size": 200}
}
```

- If strategy has no phrases: use `_text_all` with the canonical terms from the first 2 term families (again, title OR abstract).
- If strategy has no phrases AND no term families with usable canonical terms: skip narrow query for this strategy, log warning.

**Broad query (higher recall):**
- Build token list from ALL single-word terms across all term families (after token hygiene).
- Use `_text_any` with the cleaned token string, searching title OR abstract.
- **If CPC codes are present:** add as `_and` clause. This bounds the explosion.
- **If CPC codes are NOT present:** do NOT run an unbounded `_text_any`. Instead, use `_text_all` with the canonical terms from the top 3 term families. This sacrifices recall but prevents "query returns the universe."

```json
{
  "q": {
    "_and": [
      {"_or": [
        {"_text_any": {"patent_title": "federated distributed collaborative privacy differential"}},
        {"_text_any": {"patent_abstract": "federated distributed collaborative privacy differential"}}
      ]},
      {"cpc_at_issue.cpc_subclass_id": ["G06N", "H04L"]}
    ]
  },
  "f": [ ... ],
  "s": [{"patent_date": "desc"}, {"patent_id": "asc"}],
  "o": {"size": 200}
}
```

**Multiple phrases:** If a strategy has 2+ phrases, run narrow queries for `phrases[0]` through `phrases[min(len(phrases), 3) - 1]` (i.e., the first 3 phrases, most-discriminating-first per Stage 1 ordering). Each gets its own narrow query (title OR abstract, plus CPC if available).

#### Execution Rules

1. Execute in priority order: PRIMARY first, then SECONDARY, then TERTIARY. Within a priority level, narrow before broad.

2. Rate limiting: 40 req/min client-side token bucket. On 429: respect `Retry-After` header, else backoff (1s, 2s, 4s), 3 retries.

3. Deduplication: Track `patent_id`. If duplicate, keep first occurrence and append the **query ID** to `matched_queries`.

**Query ID naming convention** (one unique ID per API call):
- Phrase narrow queries: `{strategy_id}_narrow_p1`, `{strategy_id}_narrow_p2`, `{strategy_id}_narrow_p3` (one per phrase)
- Non-phrase narrow (`_text_all` fallback): `{strategy_id}_narrow_all`
- Broad queries: `{strategy_id}_broad`

Examples: `Q1_narrow_p1`, `Q1_narrow_p2`, `Q1_broad`, `Q2_narrow_all`, `Q2_broad`.

4. **"Multiple strategies" counting:** `StrategyCount` = count of distinct **strategy ID prefixes** (the `Q#` portion) across `matched_queries`. A patent matching `Q1_narrow_p1` and `Q1_broad` has StrategyCount = 1. A patent matching `Q1_narrow_p1` and `Q2_broad` has StrategyCount = 2. Extract the prefix by splitting on the first `_`.

5. Result cap: Hard cap at `PRIOR_ART_MAX_PATENTS` (default 300). Once the stored unique patent count reaches the cap, ignore additional new patent_ids from remaining query results (still finish the current API response to avoid partial parsing, but don't store new patents). After the current strategy's queries complete, stop executing further strategies.

6. Log `total_hits` per query. If > 10,000 on broad: log warning. If 0 on narrow: log info (expected, not error).

7. Error handling:
   - `error: true` in response body (any HTTP status): treat as failure. Log response body.
   - 403: hard-fail pipeline. "PatentsView API authentication failed. Check PATENTSVIEW_API_KEY."
   - 429: respect `Retry-After`, retry up to 3 times.
   - 400: log exact query JSON and response. Skip query. If 3+ queries return 400, hard-fail: "Multiple PatentsView query failures — likely a query builder bug. Check logs."
   - 5xx: retry 3 times with backoff, then skip and log.
   - Timeout (30s): retry once, then skip and log.
   - ALL queries fail: pipeline error "PatentsView API unavailable."

8. For each retrieved patent, store:

```go
type PatentResult struct {
    PatentID       string   // patent_id
    Title          string   // patent_title
    Abstract       string   // patent_abstract (may be empty)
    GrantDate      string   // patent_date (YYYY-MM-DD)
    FilingDate     *string  // application.filing_date (YYYY-MM-DD) or nil if missing
    Assignees      []string // flattened from assignees[].assignee_organization
    CPCSubclasses  []string // flattened + deduped from cpc_at_issue[].cpc_subclass_id
    Inventors      []string // "First Last" from inventors[].inventor_name_first/last
    MatchedQueries []string // e.g. ["Q1_narrow_p1", "Q2_broad"]
    StrategyCount  int      // count of distinct strategy IDs (computed)
}
```

Output:
```json
{
  "patents": [ /* PatentResult array */ ],
  "queries_executed": 0,
  "queries_failed": 0,
  "queries_skipped": 0,
  "total_api_calls": 0,
  "total_hits_by_query": {"Q1_narrow_p1": 42, "Q1_broad": 3891, "Q2_narrow_all": 7}
}
```

### Stage 3: Relevance Assessment

Input: Stage 1 output (invention summary, novel elements with IDs) + Stage 2 output (patents).

**If Stage 2 returned 0 patents:** Skip LLM call entirely. Return empty assessments array. This is code-only — no model call, no tokens spent. Pipeline continues to Stage 4 (which will see 0 assessed patents and produce CLEAR_FIELD or INCONCLUSIVE).

**Pre-filtering (code):**
1. Remove patents with empty/null/whitespace-only abstracts. Count as `abstracts_missing` in metadata.
2. Sort by `StrategyCount` descending, then `GrantDate` descending.
3. Take top 75. If more, set `assessment_truncated = true` in metadata.

**Batching:** 15 patents per batch. Each batch is a separate LLM call.

LLM prompt (include verbatim per batch):
```
You are a patent analyst assessing prior art relevance.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble. Your entire response must be a single JSON object.

INVENTION SUMMARY:
[insert invention_summary from Stage 1]

NOVEL ELEMENTS:
[for each, format as: "NE1: description", "NE2: description", ...]

PATENTS TO ASSESS (assess each one):
[for each patent in batch:
  INDEX: [0-based]
  PATENT_ID: [patent_id]
  TITLE: [patent_title]
  ABSTRACT: [first 600 chars of patent_abstract]
  GRANT_DATE: [patent_date]
  ASSIGNEE: [first assignee or "Unknown"]
]

For each patent, provide:
1. RELEVANCE:
   - HIGH: Same problem, same/very similar approach. Potential §102/§103.
   - MEDIUM: Related problem or overlapping techniques. Possible §103
     when combined. Worth reviewing.
   - LOW: Same domain, different problem/approach. Unlikely cited.
   - NONE: Not relevant.
   Default MEDIUM if unsure HIGH/MEDIUM. Default LOW if unsure MEDIUM/LOW.

2. OVERLAP DESCRIPTION: 1-3 sentences on what overlaps (or doesn't).

3. NOVEL ELEMENTS COVERED: Which elements does this patent address?
   Use IDs (NE1, NE2, ...). Empty array if none.

Return one assessment per patent, in the same order as input.
```

Output schema per batch:
```json
{
  "assessments": [
    {
      "patent_id": "string",
      "relevance": "HIGH | MEDIUM | LOW | NONE",
      "overlap_description": "string (20-500 chars)",
      "novel_elements_covered": ["string (NE1, NE2, ...)"],
      "confidence_score": "float (0.0-1.0)"
    }
  ]
}
```

Validation (code):
- **Map by `patent_id`, not by position.** Match each assessment to its input patent by ID. If an assessment has an unrecognized patent_id, drop it and log.
- **Missing patents:** If the model didn't return an assessment for a patent in the batch, retry. On 3rd failure, fill missing with `{relevance: "NONE", overlap_description: "Assessment not returned by model", novel_elements_covered: [], confidence_score: 0.0}` and log warning.
- **Extra assessments:** Drop and log.
- `relevance`: one of four enums. Case-insensitive, normalize to uppercase.
- `overlap_description`: if > 500 chars, truncate. If < 20 chars, accept but log.
- `novel_elements_covered`: filter to only IDs present in Stage 1 novel_elements. Drop invalid IDs silently, log.
- `confidence_score`: 0.0-1.0. Clamp if outside range.

**Post-processing (code):**
1. Merge all batch results.
2. Sort: HIGH > MEDIUM > LOW, then by count of `novel_elements_covered` descending, then by `GrantDate` descending.
3. Separate NONE results: count as `assessed_none` in metadata, exclude from report.

### Stage 4: Landscape Analysis

Input: Stage 1 + Stage 3 (HIGH and MEDIUM assessments only).

**Pre-computation (code — pass to LLM):**

1. `assignee_frequency`: Count patents per **primary assignee** (first non-empty normalized assignee name) across HIGH + MEDIUM results. One patent increments exactly one assignee. Sort descending. Top 10.
2. `cpc_histogram`: Count patents per CPC subclass across ALL retrieved patents (from Stage 2). Sort descending. Top 10.
3. `novel_element_coverage`: For each NE ID, count HIGH + MEDIUM patents covering it.
4. `high_count`, `medium_count`: totals.

LLM prompt (include verbatim):
```
You are a patent landscape analyst preparing a summary for a tech transfer
officer.

IMPORTANT: Return valid JSON only. No markdown fences, no commentary, no
preamble.

INVENTION SUMMARY:
[insert invention_summary]

NOVEL ELEMENTS:
[insert NE IDs + descriptions]

ASSIGNEE FREQUENCY (computed, accurate):
[e.g. "Google LLC: 8 patents, MIT: 5 patents, Samsung: 3 patents, ..."]

CPC DISTRIBUTION (computed, accurate):
[e.g. "G06N: 32 patents, H04L: 18 patents, ..."]

NOVEL ELEMENT COVERAGE (computed, accurate):
[e.g. "NE1: 6 patents, NE2: 2 patents, NE3: 0 patents, ..."]

SEARCH STATISTICS:
Total patents retrieved: [count]
Total assessed: [count]
HIGH relevance: [high_count]
MEDIUM relevance: [medium_count]

TOP PRIOR ART (up to 20, HIGH and MEDIUM only):
[for each:
  PATENT_ID: [id]
  TITLE: [title]
  ABSTRACT: [first 200 chars]
  GRANT_DATE: [date]
  ASSIGNEE: [primary assignee]
  RELEVANCE: [HIGH/MEDIUM]
  OVERLAP: [overlap_description]
  ELEMENTS COVERED: [NE IDs]
]

Analyze:

1. LANDSCAPE DENSITY: Crowded, moderate, or sparse?

2. KEY PLAYERS: Using the assignee frequency above, identify top 3-5
   and explain why they matter. Do NOT invent patent counts — use the
   numbers provided above. When you output key_players[].name, copy the
   assignee name EXACTLY as it appears in ASSIGNEE FREQUENCY (e.g.,
   "Google LLC" not "Google").

3. BLOCKING RISK: Any patents that directly anticipate (§102) or render
   obvious (§103) the core invention? Cite specific patent IDs from the
   TOP PRIOR ART list above. Only cite patents that appear in that list.

4. DESIGN-AROUND POTENTIAL: Are existing patents narrow or broad?

5. WHITE SPACE: Using novel element coverage, which aspects have least
   coverage? These are the strongest claim candidates.

6. DETERMINATION: CLEAR_FIELD, CROWDED_FIELD, BLOCKING_ART_FOUND, or
   INCONCLUSIVE.
```

Output schema:
```json
{
  "landscape_density": "SPARSE | MODERATE | DENSE",
  "landscape_density_reasoning": "string (50-500 chars)",
  "key_players": [
    {
      "name": "string",
      "relevance_note": "string (20-200 chars)"
    }
  ],
  "blocking_risk": {
    "level": "HIGH | MEDIUM | LOW | NONE",
    "blocking_patents": ["string (patent IDs, 0-5)"],
    "reasoning": "string (50-1000 chars)"
  },
  "design_around_potential": {
    "level": "EASY | MODERATE | DIFFICULT",
    "reasoning": "string (50-500 chars)"
  },
  "white_space": ["string (1-5 entries, 20-200 chars each)"],
  "determination": "CLEAR_FIELD | CROWDED_FIELD | BLOCKING_ART_FOUND | INCONCLUSIVE",
  "determination_reasoning": "string (50-1000 chars)",
  "confidence_score": "float (0.0-1.0)",
  "confidence_reason": "string (min 10 chars)"
}
```

Validation (code):
- All enum fields: case-insensitive, normalize to uppercase.
- `key_players`: 0-5 entries. **`patent_count` is NOT in the LLM output schema.** Go attaches `patent_count` from `assignee_frequency` by matching `name` (case-insensitive, trimmed). If a key_player name doesn't match any assignee, set count to 0 and log warning.
- `blocking_risk.level`: if `HIGH`, `blocking_patents` must have >= 1 entry. If `NONE`, must be empty.
- **`blocking_patents` membership validation:** Drop any patent ID not present in the Stage 3 assessed set (HIGH or MEDIUM). Log dropped IDs. If all are dropped and level was HIGH, downgrade level to MEDIUM and log.
- `white_space`: 1-5 entries. Truncate strings if over 200 chars.
- All reasoning fields: truncate if over limit, accept if under (don't retry for length).

**Consistency enforcement (code overrides):**
- `blocking_risk.level` HIGH + `determination` not BLOCKING_ART_FOUND → override determination. Log.
- `blocking_risk.level` NONE + `landscape_density` SPARSE + `determination` not CLEAR_FIELD → log warning only, do NOT override.

### Stage 5: Report Assembly

Pure code. No LLM.

Input: All stage outputs + metadata.

#### Disclaimer

```
DISCLAIMER: This is an automated preliminary prior art search generated by
an AI system. It is NOT a substitute for a professional patentability
search or freedom-to-operate analysis conducted by a qualified patent
attorney or search firm. This search covers only granted U.S. patents in
the USPTO PatentsView database. It does not cover pre-grant publications,
foreign patents, non-patent literature, trade publications, or unpublished
applications. The absence of relevant results does not mean no prior art
exists. Any patenting decision should be made in consultation with patent
counsel.
```

#### Normal Report Layout

1. **Header**: Case ID, invention title, date, disclaimer.

2. **Executive Summary**: Determination + determination_reasoning from Stage 4.

3. **Search Coverage** (audit trail):
   - Strategy count, descriptions, term families
   - Queries executed / failed / skipped
   - Total patents retrieved, total assessed
   - `total_hits_by_query` table
   - Warnings (broad queries, missing abstracts, truncated assessment)

4. **Top Results**: All HIGH + top MEDIUM, up to 15 total:
   - Patent number as link: `https://patents.google.com/patent/US{patent_id}`
   - Title, grant date, filing date, assignee
   - Relevance, overlap description
   - Novel elements covered (ID + description)

5. **Landscape Analysis**: Density, key players (with code-computed counts), blocking risk, design-around, white space.

6. **Novel Element Coverage Matrix**: Table: NE ID | Description | HIGH count | MEDIUM count | Total. Zero-coverage elements highlighted.

7. **Full Results Table**: All assessed (HIGH + MEDIUM + LOW), sorted by tier then date. Compact: patent link, title, relevance, matched strategies, grant date.

8. **Metadata**: Runtime, API calls, model, stages executed/failed.

#### Degraded Report: Stage 3 Failed

Sections included: Header, Executive Summary ("INCONCLUSIVE — relevance assessment failed"), Search Coverage, Raw Results List (patent link, title, abstract first 200 chars, assignee, grant date — no relevance scoring), Metadata.

Sections omitted: Top Results, Landscape Analysis, Novel Element Coverage Matrix, Full Results Table.

#### Degraded Report: Stage 4 Failed

Sections included: Header, Executive Summary (determination from code-only fallback heuristic), Search Coverage, Top Results (from Stage 3), Code-Generated Landscape Statistics (top assignees table, CPC histogram, novel element coverage matrix — all computed in Go, no narrative), Full Results Table, Metadata.

Sections omitted: LLM-generated Landscape Analysis narrative.

## Bus Integration

Same as Patent Eligibility Screen:
- Registers with `agent-id=prior-art-search`.
- Polls inbox. Validates envelope. Runs pipeline. Posts progress events per stage. Sends response.
- Progress: `type=progress`, stage name, status, timing.
- Final: `type=final`, determination.

## Error Handling Summary

| Condition | Action |
|-----------|--------|
| `PATENTSVIEW_API_KEY` not set | Immediate error |
| `ANTHROPIC_API_KEY` not set | Immediate error |
| `disclosure_text` < 100 chars | Immediate error |
| Stage 1 fails (3 retries) | Pipeline error, no report |
| Stage 2: 403 from API | Pipeline error: "auth failed" |
| Stage 2: 3+ queries return 400 | Pipeline error: "query builder bug" |
| Stage 2: all queries fail | Pipeline error: "API unavailable" |
| Stage 2: some queries fail | Continue, note in report |
| Stage 2: 0 patents returned | Continue, Stage 3 skips LLM, report notes it |
| Stage 3 fails (3 retries/batch) | Degraded report (no assessments) |
| Stage 4 fails (3 retries) | Degraded report (no landscape narrative) |

## File Structure

```
internal/priorartsearch/
    agent.go          // Bus registration, inbox polling, message handling
    pipeline.go       // Pipeline orchestrator (5 stages, degraded modes)
    stages.go         // Stages 1, 3, 4 (LLM)
    stages_test.go    // Unit tests (mock LLM)
    search.go         // Stage 2: API client, query builder, token hygiene, rate limiter
    search_test.go    // API client tests (mock HTTP)
    report.go         // Stage 5: report assembly (normal + 2 degraded layouts)
    report_test.go    // Report tests (all 3 layouts)
    types.go          // Types, schemas, enums, Disclaimer, stopwords list

cmd/prior-art-search/main.go
```

Standalone. No imports from `internal/patentteam/`, `internal/patentscreen/`, or `internal/marketanalysis/`. May import `internal/busclient/`.

## Testing Strategy

### Unit Tests (LLM stages, mocked)

Stages 1, 3, 4:
- Happy path: valid JSON, correct schema.
- Retry (parse): markdown fences around JSON → retry → valid.
- Retry (validation): wrong enum / missing field → retry → valid.
- Failure: 3 attempts fail → error.
- Accept+trim: Stage 1 returns 600-char summary → truncated to 500, no retry.
- Stage 3: model returns 14 of 15 assessments → retry → fills missing on 3rd fail.
- Stage 3: model returns extra assessments → dropped.
- Stage 3: invalid NE IDs (NE99) → dropped silently.
- Stage 4: blocking_patents contains ID not in assessed set → dropped.
- Stage 4: key_player name matched to assignee_frequency → count attached.

### Stage 2 Tests (mocked HTTP)

- Happy path: valid results, field extraction, flattening, dedup.
- Rate limiting: 429 + Retry-After header respected.
- Error flag: HTTP 200 but `error: true` in body → treated as failure.
- Auth failure: 403 → immediate pipeline error.
- Partial failure: some 400s → continue. 3+ 400s → hard fail.
- Total failure: all 500 → pipeline error.
- Dedup: same patent_id in Q1_narrow and Q2_broad → single entry, StrategyCount = 2.
- Result cap: > 300 unique → current strategy completes, then stops.
- 0 results: pipeline continues, Stage 3 skips LLM.
- Missing abstracts: null abstract → filtered before Stage 3.
- CPC validation: "G06N3/08" from Stage 1 → dropped by regex, logged.
- Token hygiene: multi-word synonym "secure aggregation" → split → individual tokens.

### Pipeline Integration Tests

- Full normal pipeline.
- With prior_context / without.
- 0 results → CLEAR_FIELD or INCONCLUSIVE.
- Degraded: Stage 3 fails → report has raw results, INCONCLUSIVE.
- Degraded: Stage 4 fails → report has scored results + code stats, code-only determination.
- Consistency: blocking_risk HIGH + determination CROWDED_FIELD → overridden.

### Fixtures

2 disclosures:
1. Software/AI (CPC G06N / G06F).
2. Biotech/pharma (CPC A61K / C12N).

Mocked PatentsView responses with real patent IDs/abstracts. JSON files alongside fixtures.

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PATENTSVIEW_API_KEY` | Yes | — | PatentsView API key |
| `ANTHROPIC_API_KEY` | Yes | — | Anthropic API key |
| `PRIOR_ART_LLM_MODEL` | No | `claude-sonnet-4-20250514` | Model for LLM stages |
| `PRIOR_ART_MAX_PATENTS` | No | 300 | Max unique patents to retrieve |
| `PRIOR_ART_MAX_ASSESS` | No | 75 | Max patents for LLM assessment |
| `PRIOR_ART_BATCH_SIZE` | No | 15 | Patents per assessment batch |
| `PRIOR_ART_RATE_LIMIT` | No | 40 | PatentsView requests/minute |
| `PRIOR_ART_AGENT_SECRET` | Yes | — | Bus auth secret |

## Future Enhancements (v2+)

- Pre-grant publications (`/api/v1/publication/` — different field names: `document_number`, `publication_title`, `publication_abstract`, `publication_date`)
- Citation graph expansion (seed patents → cited-by / cites endpoints)
- Claims text for top results (pending broader year coverage on beta endpoints)
- CPC group-level filtering + human-readable descriptions
- Adaptive strategy: total_hits-driven query broadening/narrowing
- Second sort pass by citation count to catch foundational patents
- Non-patent literature (IEEE, PubMed, arXiv)
- Foreign patents (EPO, WIPO)

## References

- PatentsView PatentSearch API Reference: https://search.patentsview.org/docs/docs/Search%20API/SearchAPIReference/
- PatentsView Endpoint Dictionary: https://search.patentsview.org/docs/docs/Search%20API/EndpointDictionary/
- PatentsView Swagger: https://search.patentsview.org/swagger-ui/
- PatentsView Long Text Status: https://search.patentsview.org/docs/docs/Search%20API/TextEndpointStatus
- MPEP § 904: How to Search
- MPEP § 2141: Examination Guidelines for Determining Obviousness
- CPC Classification: https://www.cooperativepatentclassification.org/
