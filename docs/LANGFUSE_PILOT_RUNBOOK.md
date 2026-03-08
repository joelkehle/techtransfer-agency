# Langfuse Pilot Runbook

## Prerequisites
1. Existing stack running so external network exists:
   - `docker compose up -d`
2. `.env` populated with strong values for:
   - `LANGFUSE_ENCRYPTION_KEY` (64 hex chars)
   - `LANGFUSE_NEXTAUTH_SECRET`
   - `LANGFUSE_SALT`
   - `LANGFUSE_POSTGRES_PASSWORD`
   - `LANGFUSE_CLICKHOUSE_PASSWORD`
   - `LANGFUSE_MINIO_ROOT_PASSWORD`
   - `LANGFUSE_REDIS_AUTH`
   - `LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY`
   - `LANGFUSE_S3_MEDIA_UPLOAD_SECRET_ACCESS_KEY`
   - `LANGFUSE_S3_BATCH_EXPORT_SECRET_ACCESS_KEY`
3. For MinIO-backed local storage, keep these aligned:
   - `LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY == LANGFUSE_MINIO_ROOT_PASSWORD`
   - `LANGFUSE_S3_MEDIA_UPLOAD_SECRET_ACCESS_KEY == LANGFUSE_MINIO_ROOT_PASSWORD`
   - `LANGFUSE_S3_BATCH_EXPORT_SECRET_ACCESS_KEY == LANGFUSE_MINIO_ROOT_PASSWORD`

## Start Langfuse
1. `docker compose -f docker-compose.langfuse.yml up -d`
2. Open `http://localhost:3010`
3. Create org/project and generate project API keys.

## Configure OTLP Export From Agents
1. Set OTLP endpoint (trace-specific):
   - `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://langfuse-web:3000/api/public/otel/v1/traces`
   - If running outside docker network, use host URL: `http://localhost:3010/api/public/otel/v1/traces`
2. Set OTLP auth header:
   - `AUTH_B64=$(printf '%s:%s' "$LANGFUSE_PUBLIC_KEY" "$LANGFUSE_SECRET_KEY" | base64 | tr -d '\n')`
   - `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic%20${AUTH_B64}`
   - `OTEL_EXPORTER_OTLP_TRACES_HEADERS=Authorization=Basic%20${AUTH_B64}`
3. Optional sampling:
   - `OTEL_TRACE_SAMPLING_RATIO=1.0`

## Smoke Checks
1. Langfuse web health:
   - `curl -fsS http://localhost:3010 >/dev/null`
2. Service health:
   - `docker compose -f docker-compose.langfuse.yml ps`
3. Telemetry path:
   - Run one `market-analysis` or `patent-pipeline` workflow.
   - Verify traces appear in Langfuse within a few seconds.

## Day-1 Dashboard Baseline
1. Cost per workflow (USD)
2. Tokens per workflow (input/output)
3. p50/p95 LLM latency by agent
4. Retry/error rate by stage

Generate baseline snapshots from local ClickHouse:

```bash
make langfuse-day1-baseline
```

Run a one-command submit/ingest smoke:

```bash
make langfuse-smoke
```

## Stop
1. `docker compose -f docker-compose.langfuse.yml down`
2. To keep data, do not remove named volumes.
