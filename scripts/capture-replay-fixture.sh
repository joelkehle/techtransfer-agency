#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "Usage: $0 <token> <workflow> [output-path]" >&2
  echo "Example: $0 abc123 prior-art-search" >&2
  echo "Example: $0 abc123 patent-screen web/fixtures/patent-screen-replay.json" >&2
  exit 1
fi

token="$1"
workflow="$2"
output_path="${3:-web/fixtures/${workflow}-replay.json}"
operator_base_url="${OPERATOR_BASE_URL:-http://localhost:3000}"

case "$workflow" in
  patent-screen|prior-art-search)
    ;;
  *)
    echo "Unsupported workflow '$workflow'. Expected: patent-screen or prior-art-search" >&2
    exit 1
    ;;
esac

url="${operator_base_url%/}/report/${token}/${workflow}"

echo "Capturing fixture from: $url"
mkdir -p "$(dirname "$output_path")"
curl -fsS "$url" -o "$output_path"

echo "Saved: $output_path"
echo "Tip: if demo mode is on, refresh the browser before submitting to load the new fixture."
