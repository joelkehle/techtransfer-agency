---
summary: Exact move set and bootstrap notes for creating the initial pinakes repo from techtransfer-agency.
read_when:
  - starting PR 2 of the bus extraction
  - creating the initial pinakes repo
  - deciding what moves now vs later
---

# Agent Bus PR 2 Bootstrap

Last updated: 2026-03-17

Purpose: remove guesswork from PR 2.

Use this doc when creating the first `pinakes` repo cut.

Related docs:

- `docs/AGENT_BUS_EXTRACTION_CHECKLIST.md`
- `docs/BUS_HTTP_CONTRACT.md`

## Move Now

Create the initial `pinakes` repo with these files as the seed set:

### Server entrypoint

- [main.go](/home/joelkehle/Projects/techtransfer-agency/cmd/techtransfer-agency/main.go)

Target:

- `cmd/pinakes/main.go`

### Bus core

- [api.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/api.go)
- [errors.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/errors.go)
- [persistent.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/persistent.go)
- [sqlite.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/sqlite.go)
- [store.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store.go)
- [types.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/types.go)

Target:

- `pkg/bus/...`

### HTTP transport

- [server.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/server.go)

Target:

- `pkg/httpapi/server.go`

### Client SDK

- [client.go](/home/joelkehle/Projects/techtransfer-agency/pkg/busclient/client.go)

Target:

- `pkg/busclient/client.go`

### Canonical tests to move upstream

- [contract_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/contract_test.go)
- [server_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/server_test.go)
- [persistent_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/persistent_test.go)
- [sqlite_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/sqlite_test.go)
- [store_benchmark_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store_benchmark_test.go)
- [store_fuzz_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store_fuzz_test.go)
- [store_observe_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store_observe_test.go)
- [store_test.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store_test.go)

Target:

- `pkg/bus/...`
- `pkg/httpapi/...`

## Do Not Move In PR 2

These are not bus-owned:

- operator
- `web/`
- extractors / PDF utilities
- `render-patent-report`
- `patent-screen`
- `prior-art-search`
- `market-analysis`
- legacy `cmd/patent-team` / `internal/patentteam`

## New Repo Minimal Dependency Set

From the current code, the initial `pinakes` repo only needs:

- stdlib
- `github.com/jmoiron/sqlx`
- `modernc.org/sqlite`

It does not need:

- Anthropic SDK
- chromedp
- OpenTelemetry
- goldmark

## Current Public Constructors / Surfaces To Preserve

### Bus

- [api.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/api.go)
  - `type API interface`
- [store.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store.go)
  - `func NewStore(cfg Config) *Store`
- [persistent.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/persistent.go)
  - `func NewPersistentStore(path string, cfg Config) (*PersistentStore, error)`
- [sqlite.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/sqlite.go)
  - `func NewSQLiteStore(dbPath string, cfg Config) (*SQLiteStore, error)`

### HTTP

- [server.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/server.go)
  - `func NewServer(store bus.API) http.Handler`

### Client SDK

- [client.go](/home/joelkehle/Projects/techtransfer-agency/pkg/busclient/client.go)
  - `type Client`
  - `func NewClient(baseURL string) *Client`
  - `func Sign(secret string, payload []byte) string`

## Internal Dependency Picture

Current package graph:

- `cmd/techtransfer-agency` -> `internal/bus`, `internal/httpapi`
- `internal/httpapi` -> `internal/bus`
- `pkg/busclient` -> no repo-internal packages
- `internal/bus` -> no repo-internal packages

Meaning:

- `internal/bus` is already close to standalone
- `pkg/busclient` is already standalone
- only `internal/httpapi` needs import rewrites from `internal/bus` to `pkg/bus`
- `cmd/techtransfer-agency` becomes a thin composition layer in the new repo

## Consumer Repoint Inventory

After `pinakes` exists, these consumers need repointing from
`github.com/joelkehle/techtransfer-agency/pkg/busclient` to
`github.com/joelkehle/pinakes/pkg/busclient`:

- [client.go](/home/joelkehle/Projects/techtransfer-agency/internal/patentscreen/client.go)
- [client.go](/home/joelkehle/Projects/techtransfer-agency/internal/pdfextractor/client.go)
- [bridge.go](/home/joelkehle/Projects/techtransfer-agency/internal/operator/bridge.go)
- [server.go](/home/joelkehle/Projects/techtransfer-agency/internal/operator/server.go)
- [client.go](/home/joelkehle/Projects/techtransfer-agency/internal/marketanalysis/client.go)
- [client.go](/home/joelkehle/Projects/techtransfer-agency/internal/patentteam/client.go)
- [main.go](/home/joelkehle/Projects/techtransfer-agency/cmd/dummy-agent/main.go)
- [e2e_test.go](/home/joelkehle/Projects/techtransfer-agency/tests/e2e_test.go)

Also note the deprecated shim:

- [client.go](/home/joelkehle/Projects/techtransfer-agency/internal/busclient/client.go)

## Bus-Specific Leftovers In This Repo

Once PR 2 lands upstream, these remain in `techtransfer-agency` only as transition debt:

- [main.go](/home/joelkehle/Projects/techtransfer-agency/cmd/techtransfer-agency/main.go)
- [server.go](/home/joelkehle/Projects/techtransfer-agency/internal/httpapi/server.go)
- [store.go](/home/joelkehle/Projects/techtransfer-agency/internal/bus/store.go)
- [client.go](/home/joelkehle/Projects/techtransfer-agency/pkg/busclient/client.go)

PR 4 and PR 7 are what remove those references and ownership.

## First Bootstrap Checks In `pinakes`

Before switching any consumers:

1. `go test ./...`
2. contract tests green in new repo
3. `cmd/pinakes` runs locally
4. `/v1/health` and `/v1/system/status` match current contract
5. module path is `github.com/joelkehle/pinakes`

## Open Questions For PR 2

- whether to place tests under `pkg/bus` / `pkg/httpapi` immediately, or keep initial file layout closer to source then tidy in a follow-up
- whether to add `GET /health` and `GET /metrics` in PR 2 or PR 3

My bias:

- keep PR 2 close to source move
- add top-level observability/release plumbing in PR 3, not while bootstrapping the repo
