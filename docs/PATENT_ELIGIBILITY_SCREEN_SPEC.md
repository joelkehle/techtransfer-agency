# Patent Eligibility Screen Agent — Spec v2

## What This Agent Does

The Patent Eligibility Screen agent takes an invention disclosure as input and produces a preliminary patent eligibility assessment grounded in the USPTO's MPEP § 2106 framework. It follows the Alice/Mayo two-step test as a deterministic pipeline: each step of the legal framework is a stage in the Go code, executed in order. The LLM provides judgment within stages. The Go code controls flow, validates outputs, computes the final determination, and assembles the report.

This is a screen, not a legal opinion. It tells a tech transfer officer whether the invention is worth spending money on a patent attorney. It cites chapter and verse from the MPEP so the TTO analyst can follow the reasoning.

## Architecture

The agent is a deterministic state machine implemented in Go. It follows the same pattern as the existing patent-team agents: it registers on the bus, receives a disclosure via its inbox, processes it through the pipeline, and returns a structured report.

The pipeline has two tracks that converge at Stage 7:

```
Disclosure In
    │
    ▼
┌─────────────────────────────────┐
│ Stage 1: Structured Extraction  │  LLM reads disclosure, code validates schema
└───────────┬─────────────────────┘
            │
     ┌──────┴──────┐
     │             │
     ▼             ▼
 ELIGIBILITY    ADVISORY
   TRACK         TRACK
     │             │
     ▼             ▼
┌──────────┐  ┌──────────┐
│ Stage 2  │  │ Stage 6  │  §102/§103 flags (always runs after Stage 1)
├──────────┤  └────┬─────┘
│ Stage 3  │       │
│  ↓ or →7 │       │
├──────────┤       │
│ Stage 4  │       │
│  ↓ or →7 │       │
├──────────┤       │
│ Stage 5  │       │
└────┬─────┘       │
     │             │
     └──────┬──────┘
            ▼
┌─────────────────────────────────┐
│ Stage 7: Report Assembly        │  Code only — no LLM. Assembles both tracks.
└─────────────────────────────────┘
    │
    ▼
Structured Report Out
```

**Eligibility track** (Stages 2→3→4→5): Sequential, with early exits. Each stage must complete before the next begins.

**Advisory track** (Stage 6): Runs whenever Stage 1 succeeds, independent of eligibility flow. May run concurrently with the eligibility track or sequentially — implementation choice, but must complete before Stage 7.

If any stage fails validation after retries, the pipeline errors — it does not skip or produce a partial report.

## Bus Message Contracts

### Request Envelope (inbound to this agent)

The agent receives a bus message with this body schema:

```json
{
  "case_id": "string (required, non-empty — unique identifier for this disclosure)",
  "disclosure_text": "string (required, non-empty — plain text extracted by upstream pdf-extractor)",
  "metadata": {
    "source_filename": "string (optional — original PDF filename)",
    "extraction_method": "string (optional — how text was extracted: pdftotext, ocr, etc.)",
    "truncated": "boolean (optional — whether the extraction was truncated)"
  }
}
```

Validation (code, before pipeline starts):
- `case_id` must be a non-empty string.
- `disclosure_text` must be present and >= 100 characters. If < 100 chars, immediately error: "Disclosure text is insufficient for analysis."
- `disclosure_text` must be <= 100,000 characters. If exceeded, truncate to 100,000 and set an internal `input_truncated` flag. This flag feeds into confidence scoring.
- `metadata` is optional. Missing fields default to empty string / false.

### Response Envelope (outbound from this agent)

The agent sends a bus response message with this body schema:

```json
{
  "case_id": "string",
  "determination": "LIKELY_ELIGIBLE | LIKELY_NOT_ELIGIBLE | NEEDS_FURTHER_REVIEW",
  "pathway": "string (e.g., 'B1 — no judicial exception')",
  "report_markdown": "string (human-readable report in markdown)",
  "stage_outputs": "map[string]object — keys are stage names (\"stage_1\"..\"stage_6\"), values are that stage's validated output. Only stages that executed are present; skipped stages have no key.",
  "pipeline_metadata": {
    "stages_executed": ["string (stage names that ran)"],
    "stages_skipped": ["string (stage names skipped via early exit)"],
    "total_llm_calls": "integer",
    "total_retries": "integer",
    "started_at": "RFC3339 timestamp",
    "completed_at": "RFC3339 timestamp",
    "input_truncated": "boolean",
    "needs_review_reasons": ["string (why NEEDS_FURTHER_REVIEW, if applicable)"]
  },
  "disclaimer": "string (required, constant)"
}
```

The `stage_outputs` object contains the validated JSON output from each stage that ran. Stages that were skipped via early exit are omitted (their keys are absent, not null).

## Disclaimer Constant

Define once in `types.go`, use everywhere:

```go
const Disclaimer = "This is a preliminary automated screen, not a legal opinion. " +
    "It is not intended for patent filing, prosecution, or as legal advice. " +
    "Consult qualified patent counsel for formal evaluation."
```

This text must appear in both the markdown report header and the machine-readable response envelope.

## Stage Definitions

### Common: Per-Stage Confidence Fields

Every LLM-powered stage (Stages 1–6) must include these fields in its output schema, in addition to the stage-specific fields:

```json
{
  "confidence_score": "float (0.0–1.0, required — LLM's self-assessed confidence in its analysis)",
  "confidence_reason": "string (required, non-empty — why the confidence is at this level)",
  "insufficient_information": "boolean (required — true if the disclosure lacks enough detail for this stage)"
}
```

Validation (code):
- `confidence_score` must be a float in [0.0, 1.0]. Values outside this range fail validation.
- `confidence_reason` must be a non-empty string, minimum 10 characters.
- `insufficient_information` must be a boolean.

These fields are used by the determination logic to trigger NEEDS_FURTHER_REVIEW. The LLM never sees or produces the final determination.

### Stage 1: Structured Extraction

Purpose: Extract the key elements of the invention from the disclosure text.

Input: Raw disclosure text from the request envelope.

LLM task: Read the disclosure and extract the following fields into a JSON object.

Required output schema:
```json
{
  "invention_title": "string (required, 5–200 chars)",
  "abstract": "string (required, 20–500 chars — 1-3 sentence summary)",
  "problem_solved": "string (required, 20–1000 chars)",
  "invention_description": "string (required, 50–5000 chars — how the invention works, key technical details)",
  "novel_elements": ["string (required, 1–20 entries, each 10–500 chars)"],
  "technology_area": "string (required, 5–100 chars — e.g., semiconductor fabrication, biotech, software, mechanical)",
  "claims_present": "boolean (required)",
  "claims_summary": "string or null (10–2000 chars if present)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- All required string fields must be non-empty and within the specified length ranges.
- `novel_elements` must have 1–20 entries, each non-empty.
- `claims_present=true` => `claims_summary` must be a non-empty string (min 10 chars).
- `claims_present=false` => `claims_summary` must be null.
- Confidence fields validated per common rules above.

### Stage 2: Statutory Category Classification

Purpose: Determine whether the invention falls within one of the four statutory categories of 35 U.S.C. § 101. Per MPEP § 2106.03.

Input: Stage 1 output (`invention_description`, `novel_elements`, `claims_summary` if present).

LLM prompt context (include verbatim in the prompt):
```
Under 35 U.S.C. § 101, patentable subject matter must fall within one of four
statutory categories:

1. PROCESS: An act or series of acts or steps performed on a subject matter
   to produce a useful, concrete result. (MPEP § 2106.03(a))

2. MACHINE: A concrete thing, consisting of parts or certain devices and
   combination of devices. (MPEP § 2106.03(b))

3. MANUFACTURE: An article produced from raw or prepared materials by giving
   these materials new forms, qualities, properties, or combinations, whether
   by hand labor or by machinery. (MPEP § 2106.03(c))

4. COMPOSITION_OF_MATTER: All compositions of two or more substances and
   all composite articles, whether they are results of chemical union or of
   mechanical mixture, or whether they are gases, fluids, powders, or solids.
   (MPEP § 2106.03(d))

A claim may fall into more than one category.
```

LLM task: Classify the invention into one or more of the four categories. Provide a brief explanation citing the relevant MPEP subsection.

Required output schema:
```json
{
  "categories": ["PROCESS | MACHINE | MANUFACTURE | COMPOSITION_OF_MATTER (0–4 entries, conditional on passes_step_1)"],
  "explanation": "string (required, 20–2000 chars)",
  "passes_step_1": "boolean (required)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- `passes_step_1` must be a boolean.
- If `passes_step_1` is true: `categories` must contain 1–4 entries. Each must be one of: `PROCESS`, `MACHINE`, `MANUFACTURE`, `COMPOSITION_OF_MATTER`. Exact uppercase match. No duplicates.
- If `passes_step_1` is false: `categories` must be an empty array.
- `explanation` must be non-empty, 20–2000 chars.

Flow control (code):
- If `passes_step_1` is false → pipeline exits eligibility track. Determination: `LIKELY_NOT_ELIGIBLE`, Pathway A.

### Stage 3: Alice/Mayo Step 2A, Prong 1 — Judicial Exception

Purpose: Determine whether the invention is directed to a judicial exception: an abstract idea, a law of nature, or a natural phenomenon. Per MPEP § 2106.04.

Input: Stage 1 output + Stage 2 output.

LLM prompt context (include verbatim in the prompt):
```
Under Step 2A, Prong One (MPEP § 2106.04), determine whether the claim
recites a judicial exception.

The three categories of judicial exceptions are:

1. ABSTRACT IDEAS, which include:
   a. Mathematical concepts — mathematical relationships, formulas, equations,
      calculations. (MPEP § 2106.04(a)(2), Group I)
   b. Certain methods of organizing human activity — fundamental economic
      practices, commercial interactions, managing personal behavior or
      relationships, legal interactions. (MPEP § 2106.04(a)(2), Group II)
   c. Mental processes — concepts performed in the human mind including
      observation, evaluation, judgment, opinion. A claim recites a mental
      process when it can practically be performed in the human mind.
      (MPEP § 2106.04(a)(2), Group III)

2. LAWS OF NATURE — naturally occurring principles or relationships.
   E.g., E=mc², the relationship between blood metabolite levels and drug
   dosage (Mayo v. Prometheus).

3. NATURAL PHENOMENA / PRODUCTS OF NATURE — naturally occurring things.
   E.g., naturally isolated DNA sequences (Myriad), naturally occurring
   minerals.

Important: A claim that merely involves or is based on a judicial exception
is different from a claim that recites a judicial exception. Only claims that
recite (set forth or describe) a judicial exception require further analysis.
(MPEP § 2106.04)
```

LLM task: Analyze the invention and determine if it recites a judicial exception. If yes, identify which type and which subcategory. Provide specific reasoning tied to the invention's elements.

Required output schema:
```json
{
  "recites_exception": "boolean (required)",
  "exception_type": "ABSTRACT_IDEA | LAW_OF_NATURE | NATURAL_PHENOMENON | null",
  "abstract_idea_subcategory": "MATHEMATICAL_CONCEPT | ORGANIZING_HUMAN_ACTIVITY | MENTAL_PROCESS | null",
  "reasoning": "string (required, 50–3000 chars — cite specific elements)",
  "mpep_reference": "string (required, 10–200 chars — specific MPEP section cited)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- If `recites_exception` is false: `exception_type` and `abstract_idea_subcategory` must be null.
- If `recites_exception` is true: `exception_type` must be one of: `ABSTRACT_IDEA`, `LAW_OF_NATURE`, `NATURAL_PHENOMENON`.
- If `exception_type` is `ABSTRACT_IDEA`: `abstract_idea_subcategory` must be one of: `MATHEMATICAL_CONCEPT`, `ORGANIZING_HUMAN_ACTIVITY`, `MENTAL_PROCESS`.
- If `exception_type` is not `ABSTRACT_IDEA`: `abstract_idea_subcategory` must be null.
- `reasoning` must be 50–3000 chars.
- `mpep_reference` must be 10–200 chars.

Flow control (code):
- If `recites_exception` is false → pipeline exits eligibility track. Determination: `LIKELY_ELIGIBLE`, Pathway B1.
- If `recites_exception` is true → pipeline continues to Stage 4.

### Stage 4: Alice/Mayo Step 2A, Prong 2 — Practical Application

Purpose: If the invention recites a judicial exception, determine whether additional elements integrate the exception into a practical application. Per MPEP § 2106.04(d).

Input: Stage 1 output + Stage 3 output.

LLM prompt context (include verbatim in the prompt):
```
Under Step 2A, Prong Two (MPEP § 2106.04(d)), evaluate whether the claim
as a whole integrates the judicial exception into a practical application.

A claim integrates the judicial exception into a practical application when
the additional elements (beyond the exception itself) apply, rely on, or use
the exception in a manner that imposes a meaningful limit on the exception.

Considerations indicating integration (MPEP § 2106.04(d)(1)):
- Improvement to the functioning of a computer or other technology
  (MPEP § 2106.05(a))
- Application of the exception with a particular machine
  (MPEP § 2106.05(b))
- Transformation of a particular article to a different state or thing
  (MPEP § 2106.05(c))
- Application of the exception in some other meaningful way beyond generally
  linking the use to a particular technological environment
  (MPEP § 2106.05(e))

Considerations indicating NO integration (MPEP § 2106.04(d)(1)):
- Adding the words "apply it" or equivalent with no meaningful limit
  (MPEP § 2106.05(f))
- Adding insignificant extra-solution activity (e.g., mere data gathering)
  (MPEP § 2106.05(g))
- Generally linking use to a particular technological environment or field
  (MPEP § 2106.05(h))

Evaluate the claim as a whole, not individual elements in isolation.
```

LLM task: Identify the additional elements beyond the judicial exception. Evaluate whether they integrate the exception into a practical application. Cite specific MPEP considerations.

Required output schema:
```json
{
  "additional_elements": ["string (required, 1–10 entries, each 10–500 chars)"],
  "integrates_practical_application": "boolean (required)",
  "considerations_for": ["string (0–10 entries, each 10–500 chars)"],
  "considerations_against": ["string (0–10 entries, each 10–500 chars)"],
  "reasoning": "string (required, 50–3000 chars)",
  "mpep_reference": "string (required, 10–200 chars)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- `additional_elements` must have 1–10 entries, each non-empty (even if the entry is "none identified").
- `integrates_practical_application` must be a boolean.
- `considerations_for` and `considerations_against`: 0–10 entries each.
- `reasoning` must be 50–3000 chars.
- `mpep_reference` must be 10–200 chars.

Flow control (code):
- If `integrates_practical_application` is true → pipeline exits eligibility track. Determination: `LIKELY_ELIGIBLE`, Pathway B2.
- If `integrates_practical_application` is false → pipeline continues to Stage 5.

### Stage 5: Alice/Mayo Step 2B — Inventive Concept

Purpose: Determine whether the claim elements, individually or in combination, amount to significantly more than the judicial exception. Per MPEP § 2106.05.

Input: Stage 1 output + Stage 3 output + Stage 4 output.

LLM prompt context (include verbatim in the prompt):
```
Under Step 2B (MPEP § 2106.05), determine whether the additional elements,
individually and in combination, provide an inventive concept — i.e., amount
to significantly more than the judicial exception itself.

Considerations indicating significantly more:
- Adds a specific limitation beyond what is well-understood, routine, and
  conventional in the field (MPEP § 2106.05(d))
- Adds unconventional steps that confine the claim to a particular useful
  application (MPEP § 2106.05(e))

Considerations indicating NOT significantly more:
- Adding well-understood, routine, conventional activities previously known
  in the industry (MPEP § 2106.05(d), Berkheimer Memo)
- Simply appending well-known conventional steps specified at a high level
  of generality

Per the Berkheimer Memo (April 2018), a finding that additional elements are
well-understood, routine, and conventional must be supported by evidence
(citation to publications, patents, court decisions, or official notice with
justification).
```

LLM task: Evaluate whether the additional elements (from Stage 4) provide an inventive concept. Apply the Berkheimer evidentiary standard where claiming elements are well-understood, routine, and conventional.

Required output schema:
```json
{
  "has_inventive_concept": "boolean (required)",
  "reasoning": "string (required, 50–3000 chars)",
  "berkheimer_considerations": "string (required, 20–2000 chars — evidence or reasoning for the well-understood/routine/conventional determination)",
  "mpep_reference": "string (required, 10–200 chars)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- `has_inventive_concept` must be a boolean.
- `reasoning` must be 50–3000 chars.
- `berkheimer_considerations` must be 20–2000 chars.
- `mpep_reference` must be 10–200 chars.

Flow control (code):
- If `has_inventive_concept` is true → `LIKELY_ELIGIBLE`, Pathway C.
- If `has_inventive_concept` is false → `LIKELY_NOT_ELIGIBLE`, Pathway D.

### Stage 6: §102/§103 Preliminary Flags

Purpose: Based solely on the disclosure text (not external prior art databases), flag any obvious novelty or non-obviousness concerns. This is NOT a prior art search — it's a smell test based on what the inventor disclosed. The Prior Art Search agent handles the real search.

**This stage always runs when Stage 1 succeeds.** It is independent of the eligibility track (Stages 2–5). It does not affect the eligibility determination. It produces advisory output for the report and informs the priority of the Prior Art Search agent.

Input: Stage 1 output (`invention_description`, `novel_elements`, `technology_area`).

LLM task: Based on the disclosure, assess:
- Does the inventor clearly articulate what is new about this invention vs. existing approaches?
- Are there any statements in the disclosure that suggest the individual elements are already known?
- Does the combination appear non-trivial, or does it seem like an obvious combination of known techniques?

Required output schema:
```json
{
  "novelty_concerns": ["string (0–10 entries, each 10–500 chars)"],
  "non_obviousness_concerns": ["string (0–10 entries, each 10–500 chars)"],
  "prior_art_search_priority": "HIGH | MEDIUM | LOW",
  "reasoning": "string (required, 50–3000 chars)",
  "confidence_score": "float (0.0–1.0)",
  "confidence_reason": "string (min 10 chars)",
  "insufficient_information": "boolean"
}
```

Validation (code):
- `prior_art_search_priority` must be one of: `HIGH`, `MEDIUM`, `LOW`. Exact uppercase match.
- `novelty_concerns` and `non_obviousness_concerns`: 0–10 entries each.
- `reasoning` must be 50–3000 chars.

### Stage 7: Report Assembly

Purpose: Assemble the final structured report from all stage outputs. This stage is pure code — no LLM call.

Input: Outputs from all completed stages + the pipeline flow path (which stages were executed, which were skipped via early exit) + pipeline metadata (timings, retry counts).

The markdown report must contain:

1. **Header**: Case ID, invention title, date, disclaimer (the `Disclaimer` constant).

2. **Executive Summary**: One paragraph stating the overall determination and the pathway taken:
   - `LIKELY_ELIGIBLE` (Pathway B1/B2/C) with one-sentence reason
   - `LIKELY_NOT_ELIGIBLE` (Pathway A/D) with one-sentence reason
   - `NEEDS_FURTHER_REVIEW` with one-sentence reason listing flagged stages

3. **Determination**: One of three values: `LIKELY_ELIGIBLE`, `LIKELY_NOT_ELIGIBLE`, `NEEDS_FURTHER_REVIEW`.

4. **Eligibility Analysis**: Each stage that was executed, presented in order, with:
   - Stage name and MPEP reference
   - The determination at that stage
   - The reasoning (from the LLM)
   - Confidence score and reason
   - The flow decision (continued / exited early)

5. **§102/§103 Flags**: The preliminary novelty and non-obviousness concerns from Stage 6, if any. Prior art search priority recommendation.

6. **Recommended Next Steps**: Generated by code logic, not LLM:
   - If `LIKELY_ELIGIBLE`: "Proceed to prior art search and patentability opinion."
   - If `LIKELY_NOT_ELIGIBLE`: "Review the § 101 concerns with patent counsel before investing in prior art search. The eligibility issues identified may be addressable through claim drafting."
   - If `NEEDS_FURTHER_REVIEW`: "The automated screen could not make a confident determination. Recommend human review of the specific stages flagged."

7. **Appendix**: Full structured data from each stage (the JSON outputs) for machine consumption.

Output: The response envelope defined in "Bus Message Contracts" above. The `report_markdown` field contains the human-readable report. The `stage_outputs` field contains the machine-readable stage data.

## Determination Logic

The overall determination is computed by Go code, not by the LLM. Two factors feed in: the eligibility track result and the confidence check.

### Step 1: Eligibility Track Result

```
IF NOT passes_step_1 (Stage 2):
    base_determination = LIKELY_NOT_ELIGIBLE
    pathway = "A — not a statutory category"

ELSE IF NOT recites_exception (Stage 3):
    base_determination = LIKELY_ELIGIBLE
    pathway = "B1 — no judicial exception"

ELSE IF integrates_practical_application (Stage 4):
    base_determination = LIKELY_ELIGIBLE
    pathway = "B2 — judicial exception integrated into practical application"

ELSE IF has_inventive_concept (Stage 5):
    base_determination = LIKELY_ELIGIBLE
    pathway = "C — inventive concept provides significantly more"

ELSE:
    base_determination = LIKELY_NOT_ELIGIBLE
    pathway = "D — no inventive concept beyond judicial exception"
```

### Step 2: Confidence Override

After computing the base determination, apply the confidence check. This can only **downgrade** a result to `NEEDS_FURTHER_REVIEW` — it never upgrades.

```
needs_review_reasons = []

FOR EACH executed stage WITH confidence fields:
    IF confidence_score < 0.65:
        append "Stage {N}: low confidence ({score}) — {confidence_reason}"
    IF insufficient_information == true:
        append "Stage {N}: insufficient information — {confidence_reason}"

IF input_truncated (from request envelope):
    append "Input disclosure was truncated to 100,000 characters"

IF any stage required a content retry (JSON parse error or schema validation error):
    append "Stage {N}: required {content_retries} content retries (borderline quality)"
    // Note: transport retries (timeout, 429, 5xx) do NOT trigger this rule.
    // They indicate infra issues, not borderline LLM output quality.

IF len(needs_review_reasons) > 0:
    final_determination = NEEDS_FURTHER_REVIEW
    // pathway unchanged — report shows both the base pathway and the override reasons
ELSE:
    final_determination = base_determination
```

The `needs_review_reasons` list is included in the response envelope's `pipeline_metadata` and in the report's executive summary.

## Pathway Reference

| Pathway | Exit Point | Determination | Meaning |
|---------|-----------|---------------|---------|
| A | Stage 2 | LIKELY_NOT_ELIGIBLE | Not a statutory category |
| B1 | Stage 3 | LIKELY_ELIGIBLE | No judicial exception recited |
| B2 | Stage 4 | LIKELY_ELIGIBLE | Judicial exception integrated into practical application |
| C | Stage 5 | LIKELY_ELIGIBLE | Inventive concept provides significantly more |
| D | Stage 5 | LIKELY_NOT_ELIGIBLE | No inventive concept beyond judicial exception |

Any pathway can be overridden to `NEEDS_FURTHER_REVIEW` by the confidence check.

## LLM Integration

Model: Claude (via Anthropic API, using `ANTHROPIC_API_KEY` from environment).

All LLM calls use the same pattern:
1. System prompt establishes role: "You are a patent examiner conducting a preliminary eligibility screen under 35 U.S.C. § 101, following the USPTO's MPEP § 2106 framework."
2. The stage-specific MPEP context (provided verbatim in each stage definition above) is included in the user message.
3. The invention details from prior stages are included.
4. The required output schema is specified with: "Respond with only valid JSON matching this schema. Do not include any text outside the JSON object."
5. Temperature: 0 (deterministic output).
6. Response is parsed as JSON. If parsing fails, retry per the policy below.
7. Parsed JSON is validated against the stage's schema (including field-level constraints). If validation fails, retry with the validation error message appended to the prompt.
8. Maximum 3 total attempts per stage (1 initial + 2 retries). After 3 failures, the stage errors.

### Retry Policy

| Failure Class | Retryable | Max Attempts | Backoff | Prompt Modification |
|---|---|---|---|---|
| JSON parse error | Yes | 3 total | None (immediate) | Append: "Your previous response was not valid JSON. Respond with only valid JSON." |
| Schema validation error | Yes | 3 total | None (immediate) | Append validation errors: "Your response failed validation: {errors}. Fix these issues." |
| HTTP timeout | Yes | 3 total | Exponential: 1s, 2s | None (same request) |
| HTTP 429 (rate limit) | Yes | 3 total | Exponential: 1s, 2s | None (same request) |
| HTTP 5xx (server error) | Yes | 3 total | Exponential: 1s, 2s | None (same request) |
| HTTP 4xx (not 429) | No | 1 | — | — |
| Empty response body | Yes | 3 total | None (immediate) | Append: "Your previous response was empty. Respond with valid JSON." |

The attempt counter is shared across all failure types within a single stage invocation. If a stage has used 2 retries across any combination of failures, the third failure is terminal.

Each stage tracks two counters in pipeline metadata:
- `attempts`: total attempts (all failure types). Used for error reporting.
- `content_retries`: retries caused by JSON parse errors, schema validation errors, or empty responses. Used by the confidence override — only content retries indicate borderline LLM output quality. Transport retries (timeout, 429, 5xx) do not affect determination.

## Bus Integration

The agent follows the standard bus protocol:
- Registers with capability: `patent-eligibility-screen`
- Receives requests matching the request envelope schema above
- Posts progress events at each stage transition: `"Stage 1: Extracting invention details..."`, `"Stage 2: Classifying statutory category..."`, etc.
- Posts a final event with `type=final` and the determination
- Sends a response message back to the sender with the response envelope

The agent does not handle PDF extraction — that is the upstream agent's job. It receives plain text.

## Error Handling

- If any stage fails after all retries: the agent posts an error event to the bus with the stage name, failure reason, and attempt count. It does NOT produce a partial report.
- If `disclosure_text` is missing or < 100 characters: the agent immediately errors with "Disclosure text is insufficient for analysis."
- If `disclosure_text` > 100,000 characters: truncate to 100,000 and set `input_truncated=true` (not an error, but feeds into NEEDS_FURTHER_REVIEW via confidence override).
- If `case_id` is missing or empty: the agent immediately errors with "case_id is required."
- If `ANTHROPIC_API_KEY` is not set: the agent immediately errors with "ANTHROPIC_API_KEY not configured."

## File Structure

Follow the existing pattern in `internal/patentteam/`:

```
internal/patentscreen/
    agent.go          // Bus registration, inbox polling, message handling
    pipeline.go       // The dual-track pipeline orchestrator
    stages.go         // Individual stage implementations (Stages 1-6)
    stages_test.go    // Unit tests for each stage (mock LLM responses)
    llm.go            // Anthropic API client wrapper, JSON parsing, retry logic
    llm_test.go       // LLM client tests with mock HTTP server
    report.go         // Stage 7 report assembly
    report_test.go    // Report assembly tests
    types.go          // All type definitions, schemas, enums, Disclaimer constant
```

`cmd/patent-screen/main.go`: Binary entry point. Registers on bus, starts polling.

The agent must be standalone. It must not import from `internal/patentteam/`. It may import from `internal/busclient/`.

## Testing Strategy

### Unit Tests (per stage, mocked LLM)

Each stage must have tests covering:
- **Happy path**: Valid JSON, correct schema, all fields within constraints.
- **Retry path (parse)**: First response is invalid JSON, second is valid.
- **Retry path (validation)**: First response has wrong enum / missing field, second is valid.
- **Failure path**: All 3 attempts fail, stage returns error.
- **Confidence fields**: Verify confidence_score, confidence_reason, insufficient_information are validated.
- **Constraint enforcement**: String too short, string too long, array too many entries, wrong enum casing.

### Pipeline Integration Tests (mocked LLM)

- **Full pipeline, all stages**: Mocked LLM returns valid responses for every stage. Verify final report structure.
- **Early exit B1**: Stage 3 returns `recites_exception=false`. Pipeline skips Stages 4–5, Stage 6 still runs, report assembled.
- **Early exit B2**: Stage 4 returns `integrates_practical_application=true`. Pipeline skips Stage 5, Stage 6 still runs.
- **Pathway A**: Stage 2 returns `passes_step_1=false`. Pipeline skips Stages 3–5, Stage 6 still runs.
- **Pathway D**: All stages run, `has_inventive_concept=false`. Determination is LIKELY_NOT_ELIGIBLE.
- **NEEDS_FURTHER_REVIEW override**: Pipeline produces LIKELY_ELIGIBLE but a stage has `confidence_score=0.4`. Final determination is NEEDS_FURTHER_REVIEW.
- **Error propagation**: Stage 3 fails after retries. Pipeline does not proceed to Stage 4. Error event posted.
- **Input truncation**: disclosure_text > 100,000 chars. Verify truncation, `input_truncated=true`, NEEDS_FURTHER_REVIEW triggered.

### Golden Test Fixtures

Include 3–5 representative disclosure texts as test fixtures in `internal/patentscreen/testdata/`:

| Fixture | Domain | Expected Pathway | Purpose |
|---------|--------|-----------------|---------|
| `software_eligible.txt` | Software/algorithm | B1 or B2 | Clear technical implementation, no abstract idea issues |
| `biotech_eligible.txt` | Biotech/pharma | C | Recites law of nature but has inventive concept |
| `hardware_eligible.txt` | Mechanical/electrical | B1 | Clearly statutory, no judicial exception |
| `abstract_idea_borderline.txt` | Software/business method | NEEDS_FURTHER_REVIEW | Borderline abstract idea, tests confidence thresholds |
| `business_method_ineligible.txt` | Pure business method | D | Abstract idea, no practical application, no inventive concept |

These fixtures are used in integration tests with mocked LLM responses. The mocked responses should be realistic but deterministic — stored as JSON files alongside the fixtures.

## References

- MPEP § 2106: Patent Subject Matter Eligibility (https://www.uspto.gov/web/offices/pac/mpep/s2106.html)
- MPEP § 2106.03: Statutory Categories (Step 1)
- MPEP § 2106.04: Judicial Exceptions (Step 2A)
- MPEP § 2106.04(a)(2): Abstract Idea Groupings
- MPEP § 2106.04(d): Practical Application (Step 2A Prong 2)
- MPEP § 2106.05: Inventive Concept (Step 2B)
- MPEP § 2106.05(d): Berkheimer Memo — Well-Understood, Routine, Conventional
- 2024 AI Subject Matter Eligibility Update (effective July 17, 2024)
- Alice Corp. v. CLS Bank, 573 U.S. 208 (2014)
- Mayo Collaborative Services v. Prometheus Labs, 566 U.S. 66 (2012)
