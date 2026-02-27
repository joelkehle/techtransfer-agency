# Report Audit Principle

## Core Rule

Every agent produces a report. The report is the agent's work product. A domain expert — trained in the art but unfamiliar with the system — must be able to check the agent's work by reading the report alone.

```
f(x) = y

x = input (disclosure, document, data)
y = report (the auditable output)
```

The expert does not have access to the code, the prompts, the logs, or the JSON. They have `y`. Everything they need to verify, challenge, or build upon the agent's conclusions must be in `y`.

## Who Checks What

Each agent has a target reviewer:

| Agent | Reviewer | What they verify |
|-------|----------|-----------------|
| Patent Eligibility Screen | Patent attorney | Legal reasoning follows MPEP framework, each step of Alice/Mayo is sound, MPEP cites are correct, confidence flags match their read of the disclosure |
| Market Analysis | Business school professor / TTO officer | Commercialization logic is sound, assumptions are traceable, financial model inputs are defensible, recommendation follows from evidence |
| (Future agents) | Identify the domain expert at design time | Define what "checking the work" means for that domain |

## What the Report Must Show

### 1. What the agent understood (input interpretation)

The expert's first question is: "Did the agent read this correctly?"

- Show extracted fields, not just that extraction happened
- Show what was missing and why — nullable fields with reasons are more valuable than omitting them
- If the input was ambiguous, say so

### 2. What the agent concluded at each step (stage-by-stage reasoning)

The expert's second question is: "How did you get from the input to this conclusion?"

For every analytical stage:
- **The conclusion** — what was decided (determination, score, classification, estimate)
- **The reasoning** — the argument that supports the conclusion, in enough detail that the expert can agree or disagree
- **The evidence** — what from the input (or domain knowledge) supports the reasoning
- **The framework** — what standard, methodology, or legal test was applied (MPEP section, financial model, scoring rubric)
- **The confidence** — how certain the agent is and why, including what information was insufficient

### 3. What assumptions were made (provenance tracking)

The expert's third question is: "Where did this number / claim come from?"

Every assumption, estimate, or judgment must be tagged with its source:
- **DISCLOSURE_DERIVED** — directly stated in or calculable from the input
- **INFERRED** — logically follows from disclosed information but not explicitly stated
- **ESTIMATED** — educated guess based on domain knowledge, not input-specific
- **DOMAIN_DEFAULT** — standard value for this sector/domain, not adjusted for this case
- **ADJUSTED** — domain default modified based on input-specific evidence (show both the default and the adjustment reasoning)

### 4. What was uncertain (confidence and gaps)

The expert's fourth question is: "What should I double-check?"

- Confidence scores alone are not useful — the reason behind the confidence is what matters
- Flag insufficient information explicitly — "I could not assess X because the disclosure did not mention Y"
- Unknown key factors should be listed, not buried
- If the agent short-circuited or degraded, explain what was skipped and why

### 5. What the agent recommends (decision + next steps)

The expert's final question is: "What should I do with this?"

- The recommendation tier / determination with the logic that produced it
- Specific next steps tied to the findings
- Diligence questions ranked by impact — what would change the conclusion if answered differently
- Model limitations — what this analysis cannot tell you

## Report Structure Pattern

Every agent report should follow this general structure:

```
1. Header (case ID, invention title, date, mode, disclaimer)
2. Executive Summary (2-3 sentences, the bottom line)
3. Recommendation / Determination (the answer, with confidence)
4. Stage-by-Stage Analysis (the full reasoning chain)
   - For each stage:
     - What was analyzed
     - What was concluded
     - Why (reasoning + evidence + framework reference)
     - Confidence + gaps
5. Key Uncertainties (what the expert should focus on)
6. Recommended Next Steps (specific, actionable)
7. Assumptions Audit Trail (every assumption with source tag)
8. Appendix (full structured data for programmatic consumption)
```

Sections 1-3 let a busy expert get the answer in 30 seconds. Section 4 is where they do the real checking. Sections 5-7 tell them what to do next. Section 8 is for downstream systems.

## Anti-Patterns

- **Summary without reasoning**: "Recommendation: GO" without showing why is useless to a reviewer
- **Numbers without provenance**: "$5M TAM" without showing the assumptions and sources is uncheckable
- **Confidence without explanation**: "Confidence: 0.72" means nothing — "Confidence: 0.72 — disclosure describes a working prototype but does not identify target buyers" is auditable
- **Hidden short-circuits**: If the pipeline exited early, the expert needs to know what was skipped and why, not just the final answer
- **JSON appendix as substitute for narrative**: The appendix is for machines. The narrative sections are for humans. Never rely on the appendix to communicate reasoning.
- **Omitting what's missing**: Silence about missing information looks like the agent didn't notice. Explicit "not found in disclosure" is a finding, not a gap.

## Applying This to New Agents

When designing a new agent:

1. **Identify the reviewer** — who is the domain expert that will check this work?
2. **Define "checking the work"** — what specific questions will they ask at each stage?
3. **Map stage outputs to report sections** — every structured field the pipeline produces should surface in the narrative, not just the appendix
4. **Test with a real expert** — give them a report and ask: "Can you tell me whether this agent got it right?" If they need the JSON appendix to answer, the narrative is incomplete.
