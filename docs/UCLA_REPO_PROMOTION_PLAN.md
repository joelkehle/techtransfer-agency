---
summary: Plan for promoting tdg-ip-agents from a UCLA SME-agent repo into the UCLA customer/product repo that consumes the upstream pinakes repo.
read_when:
  - deciding where UCLA-specific app code should live
  - promoting tdg-ip-agents into the UCLA repo
  - deciding between separate-process and embedded bus packaging
---

# UCLA Repo Promotion Plan

Last updated: 2026-03-17

Related plan:

- `docs/AGENT_BUS_EXTRACTION_CHECKLIST.md` - cross-repo PR sequence
- `docs/BUS_HTTP_CONTRACT.md` - current bus contract to preserve during extraction

## Goal

Make `tdg-ip-agents` the UCLA-owned product repo.

That repo should package:

- UCLA-specific app/runtime code
- operator/web UI
- PDF intake/extraction/rendering
- UCLA-specific config + deployment defaults
- UCLA agents
- pinned dependency on upstream `pinakes`

It should not become the bus source of truth.

## Why `tdg-ip-agents`

- ownership boundary already right: UCLA-specific domain agents live there
- cleaner than turning `techtransfer-agency` into the UCLA repo
- keeps bus evolution independent from UCLA app cadence
- avoids copy-paste bus drift

## Target Repo Split

### Upstream: `pinakes`

Owns:

- core bus runtime
- HTTP transport
- client SDK
- auth/signing helpers if generalized
- reference standalone server
- protocol docs + contract tests

### UCLA Customer Repo: `tdg-ip-agents`

Owns:

- UCLA product README + runbooks
- UCLA runtime/deploy manifests
- UCLA workflow/app composition
- operator/web UI
- PDF intake/extract/render utilities
- `patent-screen`
- `prior-art-search`
- UCLA-specific integration/tests
- optional `market-analysis` if UCLA wants all three agents in one product repo

### Mixed Infra Repo: `techtransfer-agency`

After migration, should retain only what is intentionally infra/shared and not UCLA product-specific.
If little remains after the move, it should be treated as a sunset candidate, not a zombie repo.

## Recommended Packaging Path

### First landing: one product repo, separate bus process

Run UCLA as:

- `pinakes` process/container from upstream repo
- UCLA operator, PDF utilities, agents, app code from `tdg-ip-agents`

Why first:

- lower risk
- cleaner rollback
- less API design pressure on embed surface

### Later option: one product repo, in-process bus library

Run UCLA as a single assembled binary/service that imports `pinakes` packages.

Do this only after:

- upstream bus API is stable
- UCLA packaging needs justify the tighter coupling

## Proposed `tdg-ip-agents` Shape

Possible destination shape:

- `cmd/operator/` or renamed UCLA product entrypoint
- `cmd/patent-extractor/`
- `cmd/patent-screen/`
- `cmd/prior-art-search/`
- `cmd/render-patent-report/`
- `cmd/ucla-ip-agency/` or similar assembled product entrypoint
- `internal/operator/`
- `internal/patentscreen/`
- `internal/pdfextractor/`
- `internal/priorartsearch/`
- `internal/uclaapp/` - UCLA-specific product wiring
- `internal/config/` - UCLA defaults/profile loading
- `deploy/` - compose/manifests
- `docs/` - UCLA product docs/runbooks
- `web/` - operator static assets

Exact names can stay flexible. Boundary matters more than names.

## What Moves In

- UCLA-specific runtime shell
- UCLA-specific workflow/orchestration code
- operator/web UI
- `web/` operator assets
- PDF intake/extraction/rendering
- UCLA deployment defaults
- UCLA operator/product docs
- third UCLA agent, if UCLA wants a single deliverable repo

## What Stays Out

- generic bus protocol ownership
- generic bus server ownership
- bus client source of truth
- low-level bus store/runtime internals
- any assumption that PDF handling is part of the bus

## Architecture Biases

- operator is UCLA product code
- current PDF utilities are UCLA product code
- `pinakes` should work for agentic app composition and agent/resource discovery without assuming either of those pieces exist
- `market-analysis` is still a conscious decision point; if UCLA wants one packaged repo with three agents, bias toward moving it there

## Migration Phases

### Phase A - Repo identity

- update README/docs to describe repo as UCLA product repo, not only agent repo
- add product-level architecture note
- point bus dependency at upstream `pinakes`

### Phase B - Product shell

- add UCLA app/config/deploy skeleton
- define where composed runtime entrypoint lives
- move operator and PDF utility code under UCLA repo ownership
- add repo-level smoke path for UCLA workflow

### Phase C - Third agent decision

- decide whether `market-analysis` joins this repo now
- if yes, migrate it with tests/docs
- if no, document why and keep interface boundary explicit

### Phase D - Packaging choice

- ship separate-process path first
- revisit embedded/library mode after review and runtime validation

## Versioning And Test Contract

- `pinakes` owns semver and protocol compatibility policy
- UCLA repo pins released versions/tags
- breaking bus API or protocol changes require major version bump upstream
- canonical protocol contract tests live in `pinakes`
- UCLA repo keeps product integration/smoke tests, not duplicate protocol suites

## Review Questions

These are the right review checkpoints before implementation:

- should UCLA ship all three agents from one repo now, or stage `market-analysis` later?
- does UCLA need a single binary/service, or is one repo with two containers enough at first?
- what, if anything, should remain in `techtransfer-agency` after operator + PDF utilities move?

## Recommended Near-Term Outcome

After review, implementation should aim for:

- `pinakes` created and released upstream
- `tdg-ip-agents` documented as UCLA product repo
- operator + current PDF utilities clearly owned by UCLA repo
- UCLA runtime able to consume pinned bus releases
- embed mode deferred until post-migration stability
