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

echo "== SME Workflow Outcomes (operator bridge spans, last ${hours}h) =="
docker exec "${clickhouse_container}" clickhouse-client --query "
WITH
  JSONExtractString(metadata['resourceAttributes'], 'service.name') AS service_name,
  JSONExtractString(metadata['attributes'], 'workflow') AS workflow,
  JSONExtractString(metadata['attributes'], 'target_agent') AS target_agent,
  JSONExtractString(metadata['attributes'], 'result_status') AS result_status,
  toFloat64OrZero(JSONExtractString(metadata['attributes'], 'workflow.elapsed_ms')) AS elapsed_ms
SELECT
  workflow,
  target_agent,
  count() AS runs,
  countIf(result_status = 'error') AS errors,
  round(if(runs = 0, 0, errors * 100.0 / runs), 2) AS error_rate_pct,
  round(quantileTDigest(0.5)(elapsed_ms), 1) AS p50_ms,
  round(quantileTDigest(0.95)(elapsed_ms), 1) AS p95_ms
FROM observations
WHERE name = 'workflow.result'
  AND service_name = 'operator'
  AND start_time >= now() - INTERVAL ${hours} HOUR
GROUP BY workflow, target_agent
ORDER BY runs DESC, workflow
FORMAT PrettyCompactMonoBlock
"

echo
echo "== Recent SME Workflow Runs =="
docker exec "${clickhouse_container}" clickhouse-client --query "
WITH
  JSONExtractString(metadata['resourceAttributes'], 'service.name') AS service_name,
  JSONExtractString(metadata['attributes'], 'workflow') AS workflow,
  JSONExtractString(metadata['attributes'], 'target_agent') AS target_agent,
  JSONExtractString(metadata['attributes'], 'result_status') AS result_status,
  JSONExtractString(metadata['attributes'], 'token') AS token,
  JSONExtractString(metadata['attributes'], 'case_id') AS case_id,
  toFloat64OrZero(JSONExtractString(metadata['attributes'], 'workflow.elapsed_ms')) AS elapsed_ms_f
SELECT
  start_time,
  workflow,
  target_agent,
  result_status,
  round(elapsed_ms_f, 1) AS elapsed_ms,
  token,
  case_id
FROM observations
WHERE name = 'workflow.result'
  AND service_name = 'operator'
  AND start_time >= now() - INTERVAL ${hours} HOUR
ORDER BY start_time DESC
LIMIT 30
FORMAT PrettyCompactMonoBlock
"
