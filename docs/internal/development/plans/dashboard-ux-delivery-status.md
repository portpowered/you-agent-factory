---
author: Agent Factory Team
last-modified: 2026-08-15
doc-id: agent-factory/plans/dashboard-ux-delivery-status
---

# Dashboard UX Delivery Status

## Audit baseline

This is a read-only delivery audit of the thirty historical dashboard UX
probes. The audited merged tree is `origin/main` at
`a6f52a5650ae1480a81a9dc4cbaf574ead2de581`, audited on 2026-08-15. At the
start of this audit `HEAD` matched that commit exactly and the product tree
had no diff against it. The audit does not treat the historical probe labels
as current status; the probe rows and rollups will be added against this
named baseline.

The audit scope is deliberately limited to evidence and documentation. It
does not change dashboard behavior, tests, generated artifacts, or runtime
code. The final implementation diff is allowed to contain this document and
the dated pointers in the two historical UX plans.

## Evidence order and status rules

Evidence is considered in this order:

1. An existing automated assertion that exercises the cited behavior on the
   named baseline and actually executes.
2. Demonstrable current source whose exact file and line range establishes the
   customer-visible contract when no behavior-specific assertion exists.
3. A reasoned source read naming every inspected file and the limitation that
   prevents stronger evidence.

Compilation, typechecking, generated types, PR titles, reliability-only
`thr-*` changes, file/symbol presence, and unrelated green suites are not
product-delivery evidence. A test that cannot start, or whose cited assertion
does not execute because setup/rendering fails, is not a pass. Any probe that
depends on such a blocked suite remains `OPEN` until another permitted form of
evidence proves the behavior.

The final matrix uses exactly these statuses:

- `FIXED`: every checkable sub-finding in the probe has passing evidence.
- `PARTIAL`: at least one sub-finding has passing evidence and at least one
  concrete sub-finding remains unresolved.
- `OPEN`: no sufficient delivery evidence exists, or a required behavior test
  is blocked before the cited assertion executes.
- `SUPERSEDED`: the historical finding no longer describes the current
  product contract, with the replacement contract named in the row.

## Executability evidence

The following commands were run from this checkout after installing the locked
UI dependencies with `bun install --frozen-lockfile`. Installation only
prepared the environment and is not product evidence.

| Command | Outcome | What executed and what it proves |
| --- | --- | --- |
| `bun run --cwd ui/packages/components check:declaration-runtime` | Pass: 68 declarations and 68 runtime modules; every declaration reference resolved. | The current bundled components artifact has a runtime counterpart for the graph entrypoint. |
| `bunx --no-install vitest run --config vitest.config.ts src/package-declaration-runtime.test.ts` from `ui/packages/components` | Pass: 1 file, 6 tests. | The behavior-specific declaration/runtime guard executed both the missing `./graph-edge` diagnostic and the aligned runtime-counterpart assertions in `ui/packages/components/src/package-declaration-runtime.test.ts:68-153`. |
| `bun run --cwd ui/packages/factory-graph check:components-runtime-resolution` | Pass: 27 focused Bun tests, 114 expectations. | The package-local source aliases were checked and the graph-editor control assertions actually ran under the factory-graph package configuration. React emitted non-fatal `act(...)` warnings; no assertion was skipped. |
| `bun run --cwd ui/packages/factory-graph test` | Pass: 7 files, 130 tests. | The package-owned semantic graph test assertions executed. |
| `bun test src/features/current-selection/work-selection/lib/selected-work-relationship-graph.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.bun.unit.test.ts src/features/trace-drilldown/lib/trace-relation-factory-graph.bun.unit.test.ts` from `ui` | Pass: 5 files, 16 tests, 42 expectations. | The selected-Work and trace relation graph projection assertions executed, including empty/error states, repeated relations, endpoint identity, and localized edge labels. |
| `bun test src/api/worker-sessions/api.bun.unit.test.ts` from `ui` | Pass: 1 file, 5 tests, 8 expectations. | The provider-neutral Worker Session URL, reconnect cursor, typed source failure, list shape, and malformed-frame assertions executed. |
| `go test ./pkg/services/worker_sessions/...` | Pass on the retry: all five Worker Session packages passed. The first attempt hit a transient missing Go build-cache artifact before compiling any package. | Worker Session contract, lifecycle, observation, transport, and wire tests executed on the named tree. |
| `bun run --cwd ui typecheck` | Pass. | TypeScript typechecking completed; this is a quality gate only and is not counted as product-delivery evidence. |
| `bun run --cwd ui/packages/factory-visualizers test` | Blocked: 7 files started, 28 tests passed, 3 failed. | The three graph-rendering assertions did not reach their intended renderer assertions because the mocked `@xyflow/react` module has no `useStore` export. The exact blocked tests are `src/factory-topology-replay.semantics.test.tsx` / “renders a guarded logical move with its public control details” and `src/factory-recording-topology-replay.states.test.tsx` / “invalidates cached ticks when same-identity recording evidence changes” plus “contains a controlled projection failure and removes stale ready content”. These failures are present on the unchanged audited tree; dependent probe claims will not be credited from this suite. |

The first runtime-resolution attempt, before dependency installation, stopped at
the preload with `happy-dom` missing (`0 pass, 1 error`). It was environment
setup failure, not a product result. After installation and package builds, the
same focused command reached and reported its 27 assertions above.

## Confirmed starting points

### Worker Sessions

The current Worker Session contract is provider-neutral at the canonical
identity boundary. `pkg/services/worker_sessions/contracts.go:85-114` exposes
observation, transcript, and retained/live stream operations by Worker Session
ID, including sessions with no Provider Session reference.
`pkg/services/worker_sessions/observation.go:118-227` defines the identity-only
requests and cursors; the cursor carries Worker Session ID, stream generation,
and position rather than falling back to provider identity.

Behavior-specific tests confirm the starting point rather than merely naming
symbols: `pkg/services/worker_sessions/internal/service/control_production_boundary_test.go:546-551`
reads a transcript by Worker Session ID, `:590-611` checks chronological
attempt ordering and exact dispatch/turn/provider correlation, and `:842-868`
checks replay-only records, terminal replay, summary, and closure. The UI
entry point is similarly observable in
`ui/src/api/worker-sessions/api.bun.unit.test.ts:10-63`, where provider-neutral
scoping, reconnect cursors, typed source failure, and observation shape are
asserted and executed by the command above.

### Factory graph visual plan

The separate graph visual plan starts from the canonical semantic graph used by
the activity dashboard, observe/edit modes, and package replay surfaces
(`docs/internal/development/plans/factory-graph-visual-ux.md:73-78`). It assigns
the shared visual contract, semantic state, Work-volume rules, node primitives,
group-region presentation, and package fixtures to `ui/packages/factory-graph`
(`:265-271`), while dashboard-owned state remains pending layout/history,
React Flow interaction wiring, save/reload, and timeline destinations
(`:273-280`). Its separate visual rollup will therefore attribute graph
presentation work to Stories 1-7 without double-counting dashboard Stories
9-10 or other probe owners.

## Classification matrix

The thirty probe rows and explicit status totals are added in the subsequent
audit stories after their owner-specific evidence is checked. No historical
`Fail` label is promoted to a current status by this baseline work.

