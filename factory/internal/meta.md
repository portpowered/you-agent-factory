# Repository Maintainer Meta

Local theory-of-mind for the repository maintainer workflow (gitignored).

## System architecture (stable)

- **Event-first factory runtime**: workers emit outputs that re-enter as events;
  canonical state is derived from the event history, not direct mutation.
- **FactoryService** coordinates APIs, CLI, sessions, persistence, and runtime
  construction; per-session state lives in the session runtime.
- **Public vocabulary** (`Factory`, `Factory Session`, `Work`, `Work Request`,
  `Provider Session`) hides internal Petri-net terms in customer surfaces.
- **OpenAPI** is authored in `api/components/`, bundled to `api/openapi.yaml`,
  and drives generated Go/TS clients.
- **Frontend** consumes generated types + event streams; backend remains
  authoritative.

## What changed recently (manual-fixes-0 arc)

- Graph editor and save path now carry **layout topology** (nodes, edges,
  waypoints) through the event stream and current-factory PUT contract.
- Layout boundary validation treats **`size` as optional** on nodes again so
  legacy `factory.json` without sizes can load; editor may still emit sizes on
  save.
- Bundled factory supporting files gained explicit **`id`** fields in API
  projection.
- Maintainer factory shed the resource-token `cleaner` cron pattern in favor of
  `though-retrigger` thoughts loopback plus explicit review work type.

## Operational notes

- `factory/internal/**` and `factory/inputs/**` (except checked-in `.gitkeep`
  stubs) are **gitignored** — maintain them locally, never commit.
- On branches with config validation changes, prefer **`./bin/you`** from
  `make build` over the global install.
- Default cleanup dispatch: **one `idea` per cycle** via `you submit
  --session '~default'`; use `you submit batch` only for ordered/mixed work.
- Test cleanup preference: delete or rewrite **meta tests** (file layout, barrel
  inventories, struct field lists, route/command catalogs); keep behavioral
  runtime, API, CLI, UI, and emitted-event assertions.

## Progressive tracks

### Meta-test retirement wave

Open PRs already cover petri transition guard, world-view contract guard, and
several UI public-barrel / inventory tests. New dispatch this cycle:
**exhaustion-rule AST guard** in `pkg/config/exhaustiontests/` — behavioral
coverage already exists via OpenAPI boundary, mapping, runtime, and replay tests.

### Dynamic Workflows v0

Batch 001 contract kernel is merged (PRs #767, #771–#773, #776). Operator plan
recommends **contract-repair** before Batch 002 fake-session skeleton. Do not
schedule skeleton handler/CLI/MCP/UI wiring until B1–B12 gaps in the plan doc
are closed.

### manual-fixes-0 integration risk

PR #783 bundles graph-editor fixes with interface and test updates. Cleanup
ideas should avoid overlapping files until that PR lands or scope is explicitly
non-overlapping (backend-only guard retirement, etc.).
