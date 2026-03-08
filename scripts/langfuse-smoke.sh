#!/usr/bin/env bash
set -euo pipefail

operator_url="${OPERATOR_BASE_URL:-http://localhost:3000}"
workflow="${SMOKE_WORKFLOW:-market-analysis}"
poll_seconds="${SMOKE_POLL_SECONDS:-2}"
timeout_seconds="${SMOKE_TIMEOUT_SECONDS:-600}"
ingest_wait_seconds="${SMOKE_INGEST_WAIT_SECONDS:-8}"
clickhouse_container="${LANGFUSE_CLICKHOUSE_CONTAINER:-techtransfer-agency-clickhouse-1}"
input_file="${SMOKE_INPUT_FILE:-}"
case_id="${SMOKE_CASE_ID:-LF-SMOKE-$(date +%s)}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -qx "${clickhouse_container}"; then
  echo "ClickHouse container ${clickhouse_container} is not running" >&2
  exit 1
fi

if ! [[ "${poll_seconds}" =~ ^[0-9]+$ && "${timeout_seconds}" =~ ^[0-9]+$ && "${ingest_wait_seconds}" =~ ^[0-9]+$ ]]; then
  echo "SMOKE_POLL_SECONDS, SMOKE_TIMEOUT_SECONDS, and SMOKE_INGEST_WAIT_SECONDS must be integers" >&2
  exit 1
fi

cleanup_input=0
if [[ -z "${input_file}" ]]; then
  input_file="$(mktemp /tmp/langfuse-smoke-XXXXXX.txt)"
  cleanup_input=1
  cat > "${input_file}" <<'TXT'
Invention Title: Adaptive Tissue Regeneration Patch

Summary:
Bioactive patch with micro-reservoir release and pH-triggered delivery to support
chronic wound healing in outpatient and hospital settings.
TXT
fi

cleanup() {
  if [[ "${cleanup_input}" -eq 1 ]]; then
    rm -f "${input_file}"
  fi
}
trap cleanup EXIT

trace_count() {
  docker exec "${clickhouse_container}" clickhouse-client --query "SELECT count() FROM traces FORMAT TSV"
}

before_count="$(trace_count)"
echo "Traces before smoke run: ${before_count}"

submit_response="$(curl -fsS -X POST "${operator_url%/}/submit" \
  -F "workflows=${workflow}" \
  -F "case_number=${case_id}" \
  -F "file=@${input_file};type=text/plain")"

token="$(printf '%s' "${submit_response}" | jq -r '.token')"
if [[ -z "${token}" || "${token}" == "null" ]]; then
  echo "Submission failed: ${submit_response}" >&2
  exit 1
fi

echo "Submitted token=${token} workflow=${workflow}"

max_polls=$(( timeout_seconds / poll_seconds ))
for (( i=1; i<=max_polls; i++ )); do
  status_json="$(curl -fsS "${operator_url%/}/status/${token}")"
  workflow_status="$(printf '%s' "${status_json}" | jq -r --arg wf "${workflow}" '.workflows[$wf].status // empty')"
  ready="$(printf '%s' "${status_json}" | jq -r --arg wf "${workflow}" '.workflows[$wf].ready // false')"
  overall="$(printf '%s' "${status_json}" | jq -r '.status // "unknown"')"

  echo "poll=${i} overall=${overall} workflow_status=${workflow_status:-missing} ready=${ready}"

  if [[ "${workflow_status}" == "completed" && "${ready}" == "true" ]]; then
    break
  fi

  if [[ "${workflow_status}" == "error" || "${overall}" == "error" ]]; then
    echo "Workflow ended in error: ${status_json}" >&2
    exit 2
  fi

  if [[ "${i}" -eq "${max_polls}" ]]; then
    echo "Timed out waiting for workflow completion (token=${token})" >&2
    exit 3
  fi

  sleep "${poll_seconds}"
done

echo "Workflow completed; waiting ${ingest_wait_seconds}s for telemetry ingestion"
sleep "${ingest_wait_seconds}"

after_count="$(trace_count)"
echo "Traces after smoke run: ${after_count}"

if (( after_count > before_count )); then
  echo "Smoke test passed: traces increased by $((after_count - before_count))"
  echo "Token: ${token}"
  exit 0
fi

echo "Smoke test failed: traces did not increase" >&2
echo "Token: ${token}" >&2
exit 4
