#!/usr/bin/env bash
set -euo pipefail

clickhouse_container="${LANGFUSE_CLICKHOUSE_CONTAINER:-techtransfer-agency-clickhouse-1}"
hours="${LANGFUSE_BASELINE_HOURS:-24}"

if ! [[ "${hours}" =~ ^[0-9]+$ ]]; then
  echo "LANGFUSE_BASELINE_HOURS must be an integer" >&2
  exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -qx "${clickhouse_container}"; then
  echo "ClickHouse container ${clickhouse_container} is not running" >&2
  exit 1
fi

run_query() {
  local title="$1"
  local sql="$2"
  echo
  echo "== ${title} =="
  docker exec "${clickhouse_container}" clickhouse-client --query "${sql}"
}

run_query "Cost Per Workflow (USD, last ${hours}h)" "
WITH
  JSONExtractString(metadata['resourceAttributes'], 'service.name') AS workflow,
  toFloat64OrZero(JSONExtractString(metadata['attributes'], 'llm.cost.estimated_usd')) AS estimated_cost_usd
SELECT
  workflow,
  round(sum(estimated_cost_usd), 6) AS total_estimated_cost_usd,
  count() AS llm_calls
FROM observations
WHERE name = 'llm.anthropic.generate'
  AND start_time >= now() - INTERVAL ${hours} HOUR
GROUP BY workflow
ORDER BY total_estimated_cost_usd DESC
FORMAT PrettyCompactMonoBlock
"

run_query "Tokens Per Workflow (last ${hours}h)" "
WITH
  JSONExtractString(metadata['resourceAttributes'], 'service.name') AS workflow,
  toInt64OrZero(extract(JSONExtractString(metadata['attributes'], 'llm.usage.input_tokens'), '([0-9]+)')) AS input_tok,
  toInt64OrZero(extract(JSONExtractString(metadata['attributes'], 'llm.usage.output_tokens'), '([0-9]+)')) AS output_tok,
  toInt64OrZero(extract(JSONExtractString(metadata['attributes'], 'llm.usage.cache_creation_input_tokens'), '([0-9]+)')) AS cache_write_tok,
  toInt64OrZero(extract(JSONExtractString(metadata['attributes'], 'llm.usage.cache_read_input_tokens'), '([0-9]+)')) AS cache_read_tok
SELECT
  workflow,
  sum(input_tok) AS input_tokens,
  sum(output_tok) AS output_tokens,
  sum(cache_write_tok) AS cache_write_input_tokens,
  sum(cache_read_tok) AS cache_read_input_tokens,
  sum(input_tok + cache_write_tok + cache_read_tok) AS total_input_tokens
FROM observations
WHERE name = 'llm.anthropic.generate'
  AND start_time >= now() - INTERVAL ${hours} HOUR
GROUP BY workflow
ORDER BY total_input_tokens DESC
FORMAT PrettyCompactMonoBlock
"

run_query "p50/p95 LLM Latency By Agent (ms, last ${hours}h)" "
WITH
  JSONExtractString(metadata['resourceAttributes'], 'service.name') AS workflow,
  dateDiff('millisecond', start_time, ifNull(end_time, now64(3))) AS latency_ms
SELECT
  workflow,
  round(quantileTDigest(0.5)(latency_ms), 1) AS p50_ms,
  round(quantileTDigest(0.95)(latency_ms), 1) AS p95_ms,
  count() AS llm_calls
FROM observations
WHERE name = 'llm.anthropic.generate'
  AND start_time >= now() - INTERVAL ${hours} HOUR
GROUP BY workflow
ORDER BY p95_ms DESC
FORMAT PrettyCompactMonoBlock
"

run_query "Retry/Error Rate By Stage (best-effort, last ${hours}h)" "
WITH spans AS (
  SELECT
    trace_id,
    coalesce(
      nullIf(JSONExtractString(metadata['attributes'], 'workflow.stage'), ''),
      nullIf(JSONExtractString(metadata['attributes'], 'stage'), ''),
      'unknown'
    ) AS stage,
    if(level != 'DEFAULT' OR status_message IS NOT NULL, 1, 0) AS is_error
  FROM observations
  WHERE name = 'llm.anthropic.generate'
    AND start_time >= now() - INTERVAL ${hours} HOUR
),
attempts AS (
  SELECT
    trace_id,
    stage,
    count() AS attempts,
    max(is_error) AS had_error
  FROM spans
  GROUP BY trace_id, stage
)
SELECT
  stage,
  count() AS stage_runs,
  countIf(attempts > 1) AS retried_runs,
  round(if(count() = 0, 0, countIf(attempts > 1) * 100.0 / count()), 2) AS retry_rate_pct,
  countIf(had_error = 1) AS errored_runs,
  round(if(count() = 0, 0, countIf(had_error = 1) * 100.0 / count()), 2) AS error_rate_pct
FROM attempts
GROUP BY stage
ORDER BY stage_runs DESC
FORMAT PrettyCompactMonoBlock
"
