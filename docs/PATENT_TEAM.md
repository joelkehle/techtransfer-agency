# Patent Team Workflow (MVP)

This repository now includes a reference multi-agent workflow that runs over Agent Bus v2 for one concrete use case:

- input: invention disclosure PDF
- output: preliminary patent-eligibility screening report

## Why this structure

The bus remains transport-only. Humans submit files to an edge/intake step, and agents exchange normalized message payloads + attachment references.

For this MVP, attachment URLs use `file://` paths so you can run locally.

## Agent Roles

1. `coordinator`
- Starts the run and waits for final report.

2. `intake`
- Validates inbound payload and forwards to extraction.

3. `pdf-extractor`
- Extracts text from PDF using TDG-style fallback order:
  - `doc-cache get <file>`
  - `pdftotext -layout <file> -`
  - byte-level printable-text fallback

4. `patent-agent`
- Produces a structured eligibility assessment (`likely_eligible`, `needs_more_info`, `likely_not_eligible`).
- Prompt shape and counsel framing are aligned with TDG patent-agent prompt style.

5. `reporter`
- Renders final human-readable report and sends it back to `coordinator`.

## Run

```bash
go run ./cmd/patent-team --pdf /absolute/path/to/disclosure.pdf --case-id CASE-2026-001
```

Behavior:
- Starts a local in-process bus by default.
- Registers all team agents.
- Runs full chain through bus endpoints (`messages`, `acks`, `events`, `inbox`).
- Prints final report.

To use an existing bus server:

```bash
go run ./cmd/patent-team --bus-url http://127.0.0.1:8080 --pdf /absolute/path/to/disclosure.pdf --case-id CASE-2026-001
```

## Scope notes

- This is an engineering validation workflow, not legal advice.
- The eligibility output is heuristic and should be reviewed by patent counsel.
- The implementation is intentionally transparent so you can inspect bus state transitions end-to-end.
