#!/usr/bin/env bash
set -euo pipefail

network_name="${STACK_NETWORK_NAME:-tta-agentnet}"
compose_project="${COMPOSE_PROJECT_NAME:-techtransfer-agency}"
expected_network_label="${COMPOSE_NETWORK_LABEL:-agentnet}"
langfuse_compose_file="${LANGFUSE_COMPOSE_FILE:-docker-compose.langfuse.yml}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! docker network inspect "${network_name}" >/dev/null 2>&1; then
  echo "Network ${network_name} not found; creating with compose labels"
  docker network create \
    --label "com.docker.compose.project=${compose_project}" \
    --label "com.docker.compose.network=${expected_network_label}" \
    "${network_name}" >/dev/null
  exit 0
fi

current_network_label="$(docker network inspect "${network_name}" --format '{{index .Labels "com.docker.compose.network"}}')"
current_project_label="$(docker network inspect "${network_name}" --format '{{index .Labels "com.docker.compose.project"}}')"

if [[ "${current_network_label}" == "${expected_network_label}" && "${current_project_label}" == "${compose_project}" ]]; then
  echo "Network labels are already healthy for ${network_name}"
  exit 0
fi

echo "Repairing compose network drift on ${network_name}"
echo "Current labels: project=${current_project_label:-<none>} network=${current_network_label:-<none>}"
echo "Target labels:  project=${compose_project} network=${expected_network_label}"
echo "This performs a controlled stack restart (volumes are preserved)."

if [[ -f "${langfuse_compose_file}" ]]; then
  docker compose -f "${langfuse_compose_file}" down || true
fi
docker compose down || true

docker network rm "${network_name}" || true
docker network create \
  --label "com.docker.compose.project=${compose_project}" \
  --label "com.docker.compose.network=${expected_network_label}" \
  "${network_name}" >/dev/null

docker compose up -d
if [[ -f "${langfuse_compose_file}" ]]; then
  docker compose -f "${langfuse_compose_file}" up -d
fi

# Cleanup legacy underscore-named containers from prior compose/manual runs.
docker ps -a --format '{{.Names}}' \
  | grep -E '^techtransfer-agency_(bus|operator|patent-extractor|prior-art-extractor|market-extractor|market-analysis|patent-screen|prior-art-search)_1$' \
  | xargs -r docker rm -f >/dev/null 2>&1 || true

echo "Network ${network_name} repaired successfully"
