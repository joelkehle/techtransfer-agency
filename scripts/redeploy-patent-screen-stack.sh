#!/usr/bin/env bash
set -euo pipefail

# Rebuild/redeploy only operator + patent-extractor + patent-screen.
# Intentionally does NOT touch bus, to avoid state resets.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  echo "No compose command found (expected 'docker compose' or 'docker-compose')." >&2
  exit 1
fi

if [[ -f .env ]]; then
  set -a
  # shellcheck source=/dev/null
  source .env
  set +a
fi

# Compose files in this repo use required interpolation for all services.
# When deploying a subset, ensure unrelated required vars are still set.
: "${ANTHROPIC_API_KEY:=placeholder}"
: "${MARKET_ANALYSIS_AGENT_SECRET:=placeholder}"
: "${MARKET_EXTRACTOR_AGENT_SECRET:=placeholder}"
: "${OPERATOR_AGENT_SECRET:=placeholder}"
: "${PATENTSVIEW_API_KEY:=placeholder}"
: "${PATENT_EXTRACTOR_AGENT_SECRET:=placeholder}"
: "${PATENT_SCREEN_AGENT_SECRET:=placeholder}"
: "${PRIOR_ART_AGENT_SECRET:=placeholder}"
: "${PRIOR_ART_EXTRACTOR_AGENT_SECRET:=placeholder}"
export ANTHROPIC_API_KEY MARKET_ANALYSIS_AGENT_SECRET MARKET_EXTRACTOR_AGENT_SECRET OPERATOR_AGENT_SECRET
export PATENTSVIEW_API_KEY PATENT_EXTRACTOR_AGENT_SECRET PATENT_SCREEN_AGENT_SECRET PRIOR_ART_AGENT_SECRET
export PRIOR_ART_EXTRACTOR_AGENT_SECRET

SERVICES=(operator patent-extractor patent-screen)

echo "Using compose: ${COMPOSE[*]}"
echo "Redeploying services: ${SERVICES[*]}"

# Remove only target service containers to avoid legacy recreate edge cases.
"${COMPOSE[@]}" rm -sf "${SERVICES[@]}" >/dev/null 2>&1 || true
"${COMPOSE[@]}" up -d --no-deps --build "${SERVICES[@]}"

echo
echo "Status:"
docker ps --format 'table {{.Names}}\t{{.Status}}' | rg 'NAMES|techtransfer-agency(-|_)(operator|patent-extractor|patent-screen|bus)(-|_)1' -n || true

echo
./scripts/check-production-url.sh "https://techtransfer.agency/"
