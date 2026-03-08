# Langfuse Self-Hosted Rollout Plan (TechTransfer Agency)

## Goal
Deploy self-hosted Langfuse alongside the existing Docker-based TechTransfer Agency stack, instrument the highest-value LLM paths first, and establish week-1 operating metrics to decide whether to continue rollout.

## Recommendation
Run a 1-week pilot first. Keep this scoped to observability and measurement before making any broader platform changes.

## Target Architecture
1. Keep the current stack unchanged (`bus`, `operator`, extractors, and workflow agents).
2. Add a Langfuse stack in Docker:
   - `langfuse-web`
   - `langfuse-worker`
   - `postgres`
   - `clickhouse`
   - `redis`
   - `minio` (or external S3-compatible storage)
   - `otel-collector` (recommended by default for buffering/retries)
3. Data flow:
   - TTA Go services emit OpenTelemetry spans.
   - Spans go to `otel-collector` or directly to Langfuse OTLP endpoint.
   - Langfuse web receives events and queues jobs.
   - Langfuse worker persists/processes into ClickHouse/Postgres and blob storage.
4. Networking and ports:
   - Join Langfuse services to `tta-agentnet`.
   - Keep `operator` on host port `3000`.
   - Expose Langfuse UI on a different host port (example: `3010`) to avoid conflict.

## Deployment Details (Compose)
1. Use a separate compose file (`docker-compose.langfuse.yml`) and attach services to the existing bus network:
   - Define `agentnet` as an external network with name `tta-agentnet`.
2. Define named volumes explicitly:
   - `langfuse-pg-data`
   - `langfuse-ch-data`
   - `langfuse-minio-data`
3. Add service healthchecks (Langfuse web, Postgres, ClickHouse, Redis, MinIO).
4. Startup order:
   - Start the main TTA stack first (to create `tta-agentnet`), then start Langfuse stack.

## What To Run
1. Add a dedicated compose file:
   - `docker-compose.langfuse.yml` with the Langfuse services above.
2. Bring up Langfuse:
   - `docker compose -f docker-compose.langfuse.yml up -d`
   - If your host uses Compose v1: `docker-compose -f docker-compose.langfuse.yml up -d`
3. Initialize Langfuse:
   - Create org/project in UI.
   - Create API keys for OTLP ingestion.
4. Configure telemetry env in TTA services:
   - `OTEL_EXPORTER_OTLP_ENDPOINT`
   - `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`
   - `OTEL_SERVICE_NAME`
   - `OTEL_TRACE_SAMPLING_RATIO`
5. If sending directly to Langfuse OTLP:
   - Auth header: `Authorization: Basic <base64(public_key:secret_key)>`
   - Trace endpoint: `http://<langfuse-host>/api/public/otel/v1/traces`

## Go Instrumentation Implementation Notes
1. Add a shared package (example: `internal/telemetry`) to initialize OpenTelemetry once per service.
2. Use Go OTEL SDK and OTLP HTTP exporter:
   - `go.opentelemetry.io/otel`
   - `go.opentelemetry.io/otel/sdk/trace`
   - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
3. First instrumentation targets in this repo:
   - `internal/marketanalysis/llm.go`
   - `internal/patentscreen/llm.go` (if this local implementation is used in deployment)
   - `internal/patentteam/evaluator_llm.go`
4. Emit generation spans around each Anthropic API call with:
   - model
   - token usage
   - estimated cost
   - latency
   - workflow identifiers (`workflow`, `agent_id`, `conversation_id`, `message_id`)

## Instrumentation Order (Week 1)
Week of March 9-13, 2026.

1. Day 1: Platform bring-up
   - Bring up Langfuse stack.
   - Verify UI/API health.
   - Verify persistent volumes and restart behavior.

2. Day 2: First high-value instrumentation
   - Instrument `market-analysis` LLM call path.
   - Emit one root span per workflow run (`conversation_id`, `workflow`, `agent_id`).
   - Emit generation spans with model, usage, latency, and cost details.

3. Day 3: Expand local LLM instrumentation
   - Instrument `patentteam` evaluator LLM path.
   - Instrument local `patentscreen` path where code ownership exists.
   - Note: production `patent-screen` and `prior-art-search` currently run as external images; full instrumentation for those requires updates in the `tdg-ip-agents` repository.
   - Add stage spans for retries and stage failures.

4. Day 4: Cross-service linkage
   - Propagate trace context through bus metadata where feasible.
   - At minimum, ensure `conversation_id` is attached to trace attributes everywhere.
   - Instrument one bus send/receive/ack path end-to-end.

5. Day 5: Dashboards and review
   - Build baseline dashboards (cost, tokens, latency, retries, errors).
   - Run a go/no-go review for production rollout scope.

## Resource, Retention, and Backup Baseline
1. Capacity baseline for pilot host:
   - Reserve enough headroom for existing TTA services plus Langfuse stack.
   - Langfuse deployment docs recommend at least 4 CPU cores, 16 GiB RAM, and 100 GiB storage for VM-style deployment.
2. Data retention:
   - Define pilot retention horizon (example: 14-30 days of traces).
   - Add ClickHouse TTL policy before scaling ingest volume.
3. Backups:
   - Postgres backup (scheduled dump/snapshot).
   - MinIO/S3 object retention policy.
   - Snapshot/backup plan for ClickHouse data.

## What To Measure In Week 1
1. Coverage
   - Percent of workflow runs with a Langfuse trace.
   - Percent of LLM calls with `usage_details` and `cost_details`.
2. Reliability
   - Telemetry export error rate.
   - Missing root-span rate.
3. Cost visibility
   - Cost per workflow.
   - Cost per agent.
   - Cost per model.
   - Input/output token distribution.
4. Performance
   - p50/p95 LLM latency.
   - p50/p95 end-to-end workflow latency.
5. Quality proxies
   - Stage retry rates.
   - Stage failure rates.
   - Needs-review/deferral rates relative to spend.

## Decision Gate (End of Week 1)
Continue rollout if all of the following are true:
1. Trace coverage is high enough to trust dashboards (target >= 90% of intended workflows).
2. Export reliability is stable (low dropped/error rates).
3. Cost and latency dashboards clearly identify optimization opportunities.
4. Operational overhead is acceptable for the team maintaining Docker infrastructure.

## Key Tradeoffs
1. Pros
   - Faster prompt/system-message iteration with better observability.
   - Better multi-agent traceability than ad hoc logs.
   - Built-in pathways to evaluation datasets and prompt experiments.
2. Cons
   - More operational components and backup/monitoring work.
   - Docker Compose deployment model is low-scale and not HA by default.
   - Some admin features are cloud-only (example: spend alerts are not self-hosted).

## Source References
- https://langfuse.com/self-hosting/deployment/docker-compose
- https://langfuse.com/self-hosting
- https://langfuse.com/self-hosting/deployment/local
- https://langfuse.com/self-hosting/deployment/infrastructure/containers
- https://langfuse.com/self-hosting/deployment/infrastructure/blobstorage
- https://langfuse.com/integrations/native/opentelemetry
- https://langfuse.com/docs/observability/features/token-and-cost-tracking
- https://langfuse.com/docs/prompt-management/features/a-b-testing
- https://langfuse.com/docs/evaluation/experiments/datasets
- https://langfuse.com/docs/administration/spend-alerts
