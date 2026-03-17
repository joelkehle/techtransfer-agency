---
summary: PR-by-PR migration checklist for extracting the bus into an authoritative upstream repo while promoting tdg-ip-agents into the UCLA customer app repo.
read_when:
  - splitting the bus out of techtransfer-agency
  - planning the pinakes repo
  - planning how tdg-ip-agents becomes the UCLA app repo
---

# Agent Bus Extraction Checklist

Last updated: 2026-03-17

This checklist updates the March 16, 2026 extraction plan for the clarified target:

- `pinakes` becomes the bus source of truth, owned upstream by Joel
- `tdg-ip-agents` grows into the UCLA customer/product repo
- `techtransfer-agency` stops being the long-term home of the bus
- operator and current PDF utilities are UCLA product code, not bus dependencies

Related plan:

- `docs/UCLA_REPO_PROMOTION_PLAN.md` - how `tdg-ip-agents` becomes the UCLA assembled app repo
- `docs/BUS_HTTP_CONTRACT.md` - frozen HTTP contract + config surface for PR 1

## Goal

Split bus ownership cleanly:

- upstream repo: reusable bus core, HTTP transport, client SDK, reference server, discovery/runtime substrate
- UCLA repo: UCLA-specific app wiring, operator, PDF utilities, UCLA agents, deployment packaging, pinned bus dependency

## Recommended Shape

### Phase 1: Upstream Extraction First

Create `pinakes` with:

- `cmd/pinakes/` - reference standalone server
- `pkg/bus/` - core bus domain + storage/runtime primitives
- `pkg/httpapi/` - HTTP transport + handlers
- `pkg/busclient/` - shared Go client
- protocol docs + contract tests

### Phase 2: UCLA Repo Promotion

Promote `tdg-ip-agents` from "agents only" into the UCLA product repo that owns:

- UCLA-specific workflow/app composition
- UCLA deployment/runtime defaults
- operator/web app
- PDF intake/extract/report utilities
- patent-screen
- prior-art-search
- market-analysis if UCLA wants that agent in the same product repo

### Phase 3: Optional In-Process Embedding

Only after Phase 1 and Phase 2 are stable:

- expose a small embeddable bus API
- keep standalone HTTP server as canonical/runtime reference path

## Non-Goals

- no protocol redesign
- no capability rename sweep
- no operator UX rewrite as part of extraction
- no forced in-process embed in the first landing
- no "copy-paste fork" of the bus as the default UCLA path
- no assumption that the bus requires PDF handling or a built-in operator

## Current Seams

Bus seams in `techtransfer-agency` today:

- [cmd/techtransfer-agency/main.go](/home/joelkehle/Projects/techtransfer-agency/cmd/techtransfer-agency/main.go)
- [internal/bus/store.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store.go)
- [internal/httpapi/server.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/server.go)
- [pkg/busclient/client.go](/home/joelkehle/Projects/techtransfer-agency/pkg/busclient/client.go)
- [internal/httpapi/contract_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/contract_test.go)
- [tests/e2e_test.go](/home/joelkehle/Projects/techtransfer-agency/tests/e2e_test.go)

UCLA repo seams today:

- [README.md](/home/joelkehle/Projects/tdg-ip-agents/README.md)
- [cmd/patent-screen/main.go](/home/joelkehle/Projects/tdg-ip-agents/cmd/patent-screen/main.go)
- [cmd/prior-art-search/main.go](/home/joelkehle/Projects/tdg-ip-agents/cmd/prior-art-search/main.go)
- [internal/patentscreen/agent.go](/home/joelkehle/Projects/tdg-ip-agents/internal/patentscreen/agent.go)
- [internal/priorartsearch/agent.go](/home/joelkehle/Projects/tdg-ip-agents/internal/priorartsearch/agent.go)

## Guardrails

- bus protocol stays HTTP-first during extraction
- `pkg/busclient` becomes bus-owned API surface
- UCLA repo consumes released bus versions; no silent source drift
- PRs stay rollback-safe
- `pinakes` ships top-level `GET /health` + `GET /metrics`
- embedded mode stays additive, not the first migration target
- operator and PDF utilities do not block `pinakes` API design

## Target End State

### `pinakes`

Owns:

- reusable bus runtime
- HTTP transport
- client SDK
- protocol/contract tests
- release/version policy

Explicitly does not own:

- UCLA operator
- UCLA PDF intake/extraction/rendering
- UCLA workflow labels or reports

### `tdg-ip-agents`

Owns:

- UCLA operator/app shell
- UCLA PDF utilities
- UCLA agents
- UCLA deploy/runtime packaging
- UCLA integration and smoke coverage

### `techtransfer-agency`

Expected role after migration:

- temporary transition repo while code moves
- either slimmed to a small infra/shared repo with explicit purpose
- or retired if little meaningful code remains

## PR Sequence

### PR 1 - Freeze bus contract in `techtransfer-agency`

Purpose: lock current behavior before code moves.

Scope:

- inventory bus endpoints and env vars
- tighten/extend contract coverage for:
  - registration
  - list agents
  - conversation creation
  - send/receive/ack
  - event/progress posting
  - observe stream
  - health/status
- document startup/config surface

Verification:

- `go test ./internal/httpapi ./tests`
- `make gate`

Stop point:

- extraction success can be judged against tests, not memory

### PR 2 - Create `pinakes` repo with public packages

Purpose: establish the upstream source-of-truth repo.

Scope in new repo:

- create module `github.com/joelkehle/pinakes`
- add:
  - `cmd/pinakes/`
  - `pkg/bus/`
  - `pkg/httpapi/`
  - `pkg/busclient/`
- move/adapt bus code from `techtransfer-agency`
- remove any dependency on `techtransfer-agency/internal/*`

Design rule:

- public packages first
- thin server composition layer
- no UCLA/app-specific names in exported API

Verification:

- `go test ./...`
- contract tests pass in new repo

Stop point:

- `pinakes` builds/tests green; no consumers switched yet

### PR 3 - Make `pinakes` releasable and operable

Purpose: make upstream bus consumable by customer repos.

Scope:

- add `GET /health`
- add `GET /metrics`
- keep `/v1/health` for compatibility
- add CI, image build, release workflow
- publish first tagged release + container image
- document versioning contract:
  - semver
  - consumer repos pin tags
  - breaking protocol/API changes require major version bump

Verification:

- `go test ./...`
- release workflow passes
- `/health` and `/metrics` work locally

Stop point:

- UCLA repo can pin a real bus version

### PR 4 - Move bus client dependency to `pinakes`

Purpose: re-home the shared client before UCLA app composition work.

Scope:

- switch `techtransfer-agency` imports from
  `github.com/joelkehle/techtransfer-agency/pkg/busclient`
  to `github.com/joelkehle/pinakes/pkg/busclient`
- switch `tdg-ip-agents` imports to the same upstream package
- keep runtime behavior unchanged

Verification:

- `go test ./...` in both repos
- existing local runs still work

Stop point:

- all bus consumers depend on upstream client package

### PR 5 - Promote `tdg-ip-agents` into the UCLA product repo

Purpose: turn UCLA's agent repo into UCLA's assembled app repo.

Recommended split:

- PR 5a: move operator + PDF utilities + `web/` assets
- PR 5b: add product shell, compose/deploy packaging, config/docs cleanup

Scope:

- add UCLA app/runtime shell in `tdg-ip-agents`
- add product-level docs, config, compose/deploy packaging
- move UCLA-specific operator code into that repo
- move `web/` static assets served by operator into that repo
- move current UCLA PDF utilities into that repo:
  - extract
  - render
- keep patent-screen + prior-art-search in-place
- define location for UCLA-specific workflow/orchestration code
- verify `render-patent-report` coupling and move it in the same slice as `internal/patentscreen` if needed
- decide whether market-analysis joins this repo now or later

Related detail:

- see `docs/UCLA_REPO_PROMOTION_PLAN.md`

Verification:

- repo can build current agents plus new product shell
- docs explain how UCLA consumes pinned `pinakes`

Stop point:

- UCLA repo boundary is explicit, including operator + PDF utilities, even if bus still runs as separate process

### PR 6 - Switch UCLA runtime wiring to upstream `pinakes`

Purpose: make UCLA run against the extracted bus, not the mixed infra repo.

Scope:

- update UCLA compose/deploy manifests to run:
  - UCLA app/services from `tdg-ip-agents`
  - bus from `pinakes`
- keep service URLs/contracts stable where possible
- validate operator/agent workflow against the externalized bus

Recommended first landing:

- same repo family, separate process/binary
- not in-process embed yet

Verification:

- compose config validates
- end-to-end UCLA workflow smoke passes

Stop point:

- UCLA product works with upstream bus release

### PR 7 - Clean ownership boundaries

Purpose: remove stale bus ownership from the wrong repo.

Scope:

- remove bus implementation from `techtransfer-agency`
- remove bus-specific tests/docs that now belong upstream
- remove or clearly archive legacy pipeline code if still unused:
  - `cmd/patent-team/`
  - `internal/patentteam/`
- update docs so:
  - `pinakes` = bus source of truth
  - `tdg-ip-agents` = UCLA customer/product repo
  - `techtransfer-agency` = explicit small infra repo, or sunset candidate

Verification:

- no lingering imports of old bus package paths
- both repos build/tests green

Stop point:

- repo boundaries match the intended architecture

### PR 8 - Optional: add embeddable library surface to `pinakes`

Purpose: support single-process UCLA packaging only after boundaries are stable.

Scope:

- define a small stable embed API
- keep HTTP server path canonical
- add example host wiring for library mode

Verification:

- standalone server unchanged
- embed example/test passes

Stop point:

- UCLA can choose between separate-process or in-process packaging without forking the bus

## Order Constraints

1. freeze contract before moving code
2. release `pinakes` before repointing consumers
3. switch `busclient` imports before deleting old package
4. promote `tdg-ip-agents` before final cleanup
5. treat embed mode as follow-on, not day-one requirement

## Contract Test Ownership

- `pinakes` owns canonical protocol contract tests long-term
- UCLA repo owns integration/smoke tests against a bus instance
- do not duplicate protocol assertions across both repos

## Rollback Points

- after PR 1: docs/tests only
- after PR 3: upstream repo can exist unused
- after PR 4: consumers can still run old deployment shape
- after PR 6: rollback by repinning UCLA deployment to prior bus/runtime shape
- after PR 7: rollback gets more expensive; avoid until UCLA runtime is proven

## Risks To Watch

- accidental protocol drift during package cleanup
- release/version mismatch across three repos
- UCLA product-shell scope creep before bus release is stable
- market-analysis placement ambiguity delaying the repo promotion
- trying to design perfect embed APIs too early

## Suggested First Slice

Start with PR 1 only:

- freeze contract
- document config/env surface
- make extraction pass/fail measurable

Then pause for review before creating `pinakes`.
