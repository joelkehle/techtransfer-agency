# Langfuse Pilot Status (As of March 7, 2026 PST / March 8, 2026 UTC)

## Scope
This file captures the validated runtime status of the self-hosted Langfuse pilot in this repository after bring-up, wiring, and smoke checks.

## Current Status
- Langfuse stack is up and healthy:
  - `langfuse-web`
  - `langfuse-worker`
  - `postgres`
  - `clickhouse`
  - `redis`
  - `minio`
- Langfuse init records exist in Postgres:
  - organization: `tta-org`
  - project: `tta-pilot`
  - API key row exists for `tta-pilot`
- OTLP ingestion is working end-to-end.
- Operator smoke workflows completed successfully for:
  - `market-analysis`
  - `prior-art-search`
- Langfuse ClickHouse contains ingested telemetry:
  - `traces`: `25`
  - `observations`: `25`

## Required Working OTLP Configuration
- Use trace-specific endpoint:
  - `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://langfuse-web:3000/api/public/otel/v1/traces`
- Keep auth header in URL-encoded form for env parsing:
  - `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic%20<base64(public_key:secret_key)>`
  - `OTEL_EXPORTER_OTLP_TRACES_HEADERS=Authorization=Basic%20<base64(public_key:secret_key)>`
- Sampling:
  - `OTEL_TRACE_SAMPLING_RATIO=1.0`

## Critical Fix Applied
Langfuse OTLP returned `500 Failed to upload JSON to S3` until S3 secret values were aligned with the MinIO credentials.

Set these to the same value as `LANGFUSE_MINIO_ROOT_PASSWORD`:
- `LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY`
- `LANGFUSE_S3_MEDIA_UPLOAD_SECRET_ACCESS_KEY`
- `LANGFUSE_S3_BATCH_EXPORT_SECRET_ACCESS_KEY`

## Verification Commands
Check service health:

```bash
docker ps --format '{{.Names}}\t{{.Status}}' | rg 'langfuse|clickhouse|minio|redis|postgres|operator|bus|market-analysis|patent-screen|prior-art-search'
```

Check Langfuse trace counts:

```bash
docker exec techtransfer-agency-clickhouse-1 clickhouse-client --query "SELECT count() FROM traces; SELECT count() FROM observations;"
```

Check recent trace names:

```bash
docker exec techtransfer-agency-clickhouse-1 clickhouse-client --query "SELECT name, count() FROM traces GROUP BY name ORDER BY count() DESC LIMIT 10;"
```

Run one-command telemetry smoke:

```bash
make langfuse-smoke
```

Print Day-1 baseline metrics:

```bash
make langfuse-day1-baseline
```

## Known Caveat
`docker compose up -d` for some app services may fail with a network label mismatch on `tta-agentnet` in this environment. Manual container recreation was used to apply updated OTLP env vars for affected services.
