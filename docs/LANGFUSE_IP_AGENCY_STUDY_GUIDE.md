# Langfuse + IP Agency Study Guide

## What You Can Study Today

This deployment now supports two telemetry layers:

1. Direct LLM spans from `market-analysis`:
   - Span name: `llm.anthropic.generate`
   - Includes token usage and estimated cost attributes.

2. Workflow lifecycle spans from `operator` for SME workflows:
   - Span names:
     - `workflow.submit`
     - `workflow.result`
   - Captures workflow, target agent, status, token, case id, and elapsed time.
   - This is what enables study of:
     - `patent-screen`
     - `prior-art-search`
     even when those external images do not emit internal OTEL spans.

## Access

- Tailscale short host: `http://beelink:3010`
- Tailscale FQDN fallback: `http://beelink.tail9063c0.ts.net:3010`

## Fast Start (Run + Study)

1. Generate a fresh workflow run:

```bash
SMOKE_WORKFLOW=patent-screen make langfuse-smoke
SMOKE_WORKFLOW=prior-art-search make langfuse-smoke
SMOKE_WORKFLOW=market-analysis make langfuse-smoke
```

2. Open Langfuse and inspect traces:
   - Filter by service:
     - `operator` to analyze SME workflow lifecycle
     - `market-analysis` to analyze LLM call internals
   - Filter by span name:
     - `workflow.result` for outcome/latency across SME workflows
     - `llm.anthropic.generate` for LLM-level details

3. Pull terminal baseline summaries:

```bash
make langfuse-sme-study
make langfuse-day1-baseline
```

## Recommended Views

### SME Operations View (Subject-Matter Agent Focus)

Use service `operator` + span `workflow.result`, then group or filter by:
- `workflow` (`patent-screen`, `prior-art-search`)
- `target_agent`
- `result_status`

Watch:
- run counts
- p50/p95 elapsed time
- error rate

### LLM Efficiency View (Market Analysis)

Use service `market-analysis` + span `llm.anthropic.generate`, inspect attributes:
- `llm.usage.input_tokens`
- `llm.usage.output_tokens`
- `llm.cost.estimated_usd`
- `llm.model`

Watch:
- cost per run
- token distribution
- latency distribution

## Important Limitation

Current `patent-screen` and `prior-art-search` images are sourced from `tdg-ip-agents` and do not currently emit detailed internal LLM spans in this deployment.

To get stage-level/cost-level internals for those two agents, add OTEL instrumentation in `tdg-ip-agents` and redeploy those images.
