#!/usr/bin/env bash
set -euo pipefail

URL="${1:-https://techtransfer.agency/}"
TIMEOUT_SEC="${TIMEOUT_SEC:-20}"
EXPECTED_MARKER="${EXPECTED_MARKER:-Patent Eligibility Screen}"
CF_ACCESS_CLIENT_ID="${CF_ACCESS_CLIENT_ID:-}"
CF_ACCESS_CLIENT_SECRET="${CF_ACCESS_CLIENT_SECRET:-}"

tmp_headers="$(mktemp)"
tmp_body="$(mktemp)"
trap 'rm -f "$tmp_headers" "$tmp_body"' EXIT

echo "Checking production URL: $URL"

curl_args=(
  --silent
  --show-error
  --location
  --max-time "$TIMEOUT_SEC"
  --retry 2
  --retry-delay 1
  --retry-connrefused
  --dump-header "$tmp_headers"
  --output "$tmp_body"
)

if [[ -n "$CF_ACCESS_CLIENT_ID" && -n "$CF_ACCESS_CLIENT_SECRET" ]]; then
  curl_args+=(-H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID")
  curl_args+=(-H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET")
fi

curl "${curl_args[@]}" "$URL"

status="$(awk 'toupper($1) ~ /^HTTP\// {code=$2} END{print code}' "$tmp_headers")"
if [[ -z "${status:-}" ]]; then
  echo "Smoke check failed: could not determine HTTP status." >&2
  exit 1
fi
if (( status < 200 || status >= 400 )); then
  echo "Smoke check failed: HTTP status $status from $URL" >&2
  exit 1
fi

if grep -qiE "cloudflare access|get a login code emailed to you|/cdn-cgi/access/" "$tmp_body"; then
  echo "Smoke check failed: Cloudflare Access/login interstitial returned for $URL." >&2
  exit 1
fi

if ! grep -qi "$EXPECTED_MARKER" "$tmp_body"; then
  echo "Smoke check failed: response body does not contain expected marker '$EXPECTED_MARKER'." >&2
  exit 1
fi

echo "Smoke check passed: HTTP $status and expected app marker '$EXPECTED_MARKER' present."
