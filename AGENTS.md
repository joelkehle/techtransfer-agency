Public repository note: local/private agent instructions are intentionally not included here.

For contributor guidance, see `CONTRIBUTING.md`.

## Local Ops Notes (Copied from TDG Agent Guidance)
- Secrets workflow: prefer Infisical-backed execution for commands that require secrets.
- Infisical login flow:
  - `infisical login --domain https://app.infisical.com/api`
  - Select US region.
  - At `Paste your browser token here:`, paste the full browser token (base64 blob; no whitespace).
  - Success message expected: `Browser login successful`.
- Infisical run wrapper pattern (when available in repo):
  - `scripts/infisical-run.sh --env=dev --path=/mock|/local -- <cmd>`
  - For non-dev: `--env=staging|prod`.
- Oracle usage guidance:
  - Prefer API mode over browser mode for reliability in automated runs.
  - Canonical wrapper pattern (when available in repo):
    - `scripts/oracle-run.sh --profile mock -- -p "..."`
  - If wrapper is unavailable, run `npx -y @steipete/oracle --help` first, then invoke with explicit `--engine api`.
  - Use longer timeouts for deep reviews (e.g., `--timeout 3600`).
