# BTRC P6 secondary-graph deletion register

This register reconciles the P6 candidate-residue table in
`docs/internal/packaged-service-structure/build-time-runtime-composition-plan.md`
against the current tree. It exists because the plan's candidate table was
authored before P3, P4, and the three P5 lanes merged, so several rows it names
for deletion were already retired by those packets and survive only in plan
prose.

Every row below is classified as **already deleted** (retired by an earlier
packet), **retained with caller** (a live non-test caller is named, so P6 may
not delete it), or **deleted in this packet**. No row is left unaddressed.

Reconciliation base: `469c3bbcb`, the merge-base of this branch and
`origin/main`.

Method: symbol audits use word-boundary matches across **all** file types, not
just `.go`. Substring matching produces false positives on this tree —
`ForRuntime` matches `WaitForRuntimeAvailability`, and `ExecutionService`
matches the canonical, unrelated `DurableExecutionService`,
`OwnedExecutionService`, and `TargetExecutionService`. Package rows use
import-path audits and distinguish importers inside the package's own directory
from external ones.

## P6 candidate-residue reconciliation

| Plan row | Status | Evidence |
| --- | --- | --- |
| HTTP application binding and central mapping | **Already deleted** | `pkg/transports/http/application` and `pkg/transports/mapping/composition` were both removed in `d6fd68c33` "refactor(http): retire residual composition forwarding", merged as part of P5B (#2042, `b2ad0f7d6`); `92bff1790` "refactor(http): compose owner adapters in Wire" had already moved routes onto Wire-built owner handlers. `Handler.Bind`, `BindDurableExecution`, and `HTTPBinder` have zero matches repo-wide. |
| Opened HTTP service bags and Sessions application roles | **Retained with caller** | `RuntimeHTTPServices` has zero matches repo-wide and is already deleted. `factory_sessions/internal/roles` and `factory_sessions/wire/application_graph.go` are **not** residue: `pkg/wire` imports `factory_sessions/wire` (`pkg/wire/wire.go:18`, generated `pkg/wire/wire_gen.go:21`) and binds its runtime-opening contracts (`wire.go:402-404`, `wire_gen.go:1121`). `roles` has ~58 importers across Sessions. Canonical, load-bearing; no deletion. |
| Sessions runtime compatibility | **Already deleted** | `ForRuntime` (word-boundary) and `BindWorkerInvoker` have zero matches repo-wide. Bare `ExecutionService` has no production declaration; the only matches are synthetic checker fixtures in `cmd/ownershipboundarycheck/main_test.go` and `owner_inventory_test.go`. |
| Runtime legacy owner surface (`APIFactory`) | **Retained with caller** | `factoryruntime.APIFactory` is declared at `pkg/services/factory_runtime/interfaces.go:24`. Its sole non-test production reference is the interface embed at `pkg/services/factory_runtime/internal/host/engine.go:17`. Its two methods are still reached by the projection fallbacks in the next row, so it is retained until those callers move. |
| Cross-owner projection fallbacks | **Retained with caller** | Both plan-named sites are live: `factory_visualization/internal/service/runtime_source.go:37` casts `runtime.Factory` to an anonymous `SubscribeFactoryEvents` interface (carrying a now-stale `TODO(P5B)`), and `work/internal/live_session_runtime.go:40` casts to `runtimeWorkSubmitter`. See the root-cause section below — the shape is wider than these two files. |
| Workers persistent execution graph | **Already deleted**, plus one row **deleted in this packet** and one **retained with caller** | `pkg/services/workers/internal/services/runtime_assembly` does not exist. `BuildRuntime`, `BuildRuntimeExecutors`, and `AssembledRuntimeBinding` match **only** in three `docs/**/*.md` plan files and have zero Go references. `WorkstationPool` has zero matches repo-wide. What remained was the provider-backed direct-invocation constructor family; see the Workers section below. |

## Sessions runtime-opening family is canonical, not a second graph

The opening packages are reached from `pkg/wire.InjectBundle`, so they are the
Sessions-side implementation of the canonical path rather than a parallel
activation graph. Each has a live external importer and is therefore
**retained with caller**:

| Package | Go files | External importer |
| --- | --- | --- |
| `internal/runtimeopening` | 25 | `factory_sessions/wire/application_graph.go`, `internal/services/invocation/wire/wire.go` (plus sibling opening packages) |
| `internal/applicationopening` | 3 | `factory_sessions/wire/application_graph.go` |
| `internal/executionopening` | 9 | `factory_sessions/wire/application_graph.go` |
| `internal/ondemandtarget` | 4 | `factory_sessions/wire/on_demand_factory_target.go` |

The plan's own deletion-register instruction for `internal/runtimeopening` is
migrate-and-reduce, not wholesale deletion: resolve Definitions values, call
`Runtime.Activate`, retain only the returned opaque binding, and route cleanup
through the Runtime operation, with the private `instance_host` permitted to
remain. Nothing in this family is presently deletable.

## Root cause of the remaining projection-fallback shape

`factoryruntime.Service` does not declare `SubmitWorkRequest` or
`SubscribeFactoryEvents`, but `factoryruntime.APIFactory` declares exactly
those two. `LiveRuntime.Factory` is typed `factory.Service`
(`factory_sessions/types.go:40`, whose comment records that it "remains
populated as a compatibility fallback while callers migrate off hosted runtime
products"), so every holder of a `Service` must type-assert to reach those two
methods.

The shape therefore has **17 production sites across 8 files**, wider than the
two files the plan names. Any lane closing this row must sweep siblings by
signature rather than fixing the two named files:

- `pkg/services/factory_runtime/internal/root.go:497,511,671,683`
- `pkg/services/factory_sessions/internal/sessionservice/runtime_factory_state.go:33,53,206`
- `pkg/services/factory_sessions/internal/sessionservice/runtime_sessions.go:58,98`
- `pkg/services/factory_sessions/internal/sessionservice/runtime_gateway.go:55`
- `pkg/services/factory_sessions/internal/sessionservice/work_runtime_adapter.go:43`
- `pkg/services/factory_sessions/internal/runtimeopening/runtime_activation.go:78,82`
- `pkg/services/work/internal/live_session_runtime.go:40` and `work/internal/service.go:293`
- `pkg/services/factory_visualization/internal/service/runtime_source.go:37`
- `pkg/transports/mapping/runtime_api.go:45`

Two observations matter for whoever closes this row. First,
`factory_runtime/internal/root.go` already implements `SubmitWorkRequest`
(line 493) and `SubscribeFactoryEvents` (line 509) canonically, so migrating
callers needs an explicitly declared and injected owner port rather than new
behavior. Second, `root.go:489-492` justifies its own bridge as retained "for
the legacy HTTP mapping until that representation migrates" — that legacy HTTP
mapping is exactly what P5B deleted (row 1 above), so the stated justification
is stale and the row is ready to migrate.

An existing characterization test already pins the legacy cast:
`TestRuntimeSubscribeUsesMigrationOnlyAPIFactoryCast` at
`factory_visualization/runtime_observation_boundary_test.go:134`. It must be
updated or retired together with the migration.

## Workers direct-invocation constructor family

The plan's headline Workers targets were already retired (see the last table
row above). What remained behind three `TODO(P6-C)` markers was the
provider-backed direct-invocation constructor family:

| Candidate | Status | Evidence |
| --- | --- | --- |
| `workers/wire.NewInvocation` | **Deleted in this packet** | Zero callers. Every reference was its own declaration plus the exported wrapper's self-call. |
| `workers/wire.NewInvocationWithProgress` | **Deleted in this packet** | Zero callers. |
| `workers/internal.NewInvocation` | **Deleted in this packet** | Its only caller was the `workers/wire` wrapper above. |
| `NewConductorInvocationWithProgress` | **Retained with caller** | One production caller: `pkg/wire/session_runtime_providers.go:1072`. Successor: `workers.Service.Execute`. |
| `workers/internal.NewInvocationWithProgress` | **Retained with caller** | Reached from `NewConductorInvocationWithProgress` (`conductor_invocation.go:28`). |

The retained rows are held deliberately. The plan states that P6 "is not
permission to delete a compatibility method whose caller has not moved," and
that a retained compatibility contract "must list its remaining caller and
retirement slice." The `TODO(P6-C)` markers were therefore rescoped to name
that exact caller and `workers.Service.Execute` as the successor boundary,
rather than removed outright.

## Static-gate state at the reconciliation base

`make deadcode` is **already failing on `origin/main`** and is not caused by
this packet. Measured in a disposable detached worktree at the merge-base:

| Tree | Findings | Baseline |
| --- | --- | --- |
| `469c3bbcb` (`origin/main`) | 548 | 342 |
| this packet's head | 545 | 342 |

The ~200-finding gap between the baseline and main predates this branch and is
spread across the whole codebase; none of the deleted Workers symbols appear in
`bin/deadcode-current.txt`. Regenerating `deadcode-baseline.txt` here would
mask those unrelated findings, so the baseline is intentionally left alone and
the drift is recorded rather than absorbed. `make pkg-boundary`,
`make pkg-structure`, `make pkg-file-count`, `make pkg-maint`,
`make backend-size`, and `make vet` all pass at this packet's head.

No baseline or manifest row required editing for the Workers deletion: only
functions were removed, no package or file, so every package-keyed baseline
(`backend-package-file-count.json`, both coverage minimums,
`ownership-inventory.json`, `package-target-manifest.json`,
`backend-exemption-budget.json`) stays valid and entry-complete.
