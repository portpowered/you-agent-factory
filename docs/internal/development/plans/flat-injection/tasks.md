# Flat-injection task packets

Status: planned; no implementation or test results are claimed. Revised by the blind validation review ([validation-review.md](validation-review.md)): packets T27–T31 were added by splitting T10, T15, T18, T20 and T22, and every packet gained a "Validation-review coverage and scope" block naming existing tests, characterization prerequisites and focused commands. Parent: [plan.md](plan.md). Impact: [inventory.md](inventory.md). Core before/after contracts: [contracts.md](contracts.md). Enforcement: [lint.md](lint.md). Each packet is self-contained and corresponds to a behavior or bounded shared capability.

v1.1 delivery rule (AM14/AM15) applies to every packet: narrow changed-package tests/lint precede early push/open PR; required CI owns broad test-functional/test-full/verify-pr. Historical local broad-suite push instructions are superseded. Author handoff is not independent validation, terminal CI or merge.

Provider registrations and regeneration are small deltas inside each lane PR; the composition steward owns `pkg/wire/wire.go` and generated `wire_gen.go`, never a final batch integration. Parallel development uses stable existing peer contracts. Each lane lands missing characterization before restructuring.

## T01 — Selected process time controls default facts and scheduling

**Parent behavior:** F15/F16; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/edges/definition.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Selected process time controls default facts and scheduling, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F15/F16.

**Actor and trigger:** CLI operator/API client; given selected controllable process source; no specialized override, drive admission/artifact/recording/chat timestamps and scheduling; given explicit specialized legacy clock and replay clock, execute their owning operations.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/edges/definition.go` / `Clock fields`, `pkg/platform/clock/clock.go` `TimerSource.After` (shared seam), all associated inventory entries assigned to T01, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Add ProcessScheduler; specialized override wins, then explicit scheduler, then Clock timer capability. A legacy Now-only source controls timestamps while an explicitly documented wall scheduler controls deadlines; never pretend timers are deterministic. Publish additive seams immediately.

Capability gap. `platformclock.TimerSource` (`pkg/platform/clock/clock.go:30`) offers only `Now` and `NewTimer`, and `Deterministic` (`:65`) has no `After`. Several consumers need more than that:
- `Now`+`After`: `SyncWaitScheduler` (`factory_sessions/internal/execution/service.go:84`), `pkg/platform/process/command.go:24`, the Linear hosted-source clock (`hosted_sources/internal/linear/contracts.go:29`), and the webhook clock (`webhooks/wire/wire.go:21-24`).
- A full `clockwork.Clock`: Automations (`automations/internal/service.go:274`).
- `AfterFunc`: the watcher `debounceClock`.

**Operator decision (2026-10-02): one shared seam.** This lane adds `After(time.Duration) <-chan time.Time` to `platformclock.TimerSource` (`Real` already has it) and implements it on `Deterministic` so it fires on logical tick advance. The `Now`+`After` consumers above use that seam directly; no per-owner `After` adapters over `NewTimer`. Consumers that need `AfterFunc` or a full clockwork clock (Automations, the watcher `debounceClock`) keep the rule: the owning lane replaces the requirement with its smallest owner-local interface over `TimerSource`. A unit test advances a `Deterministic` source and asserts `After` fires; no implementation may silently use host timers.

Also in scope (inventory additions): the three independent process-clock re-defaults at `pkg/wire/profiles.go:472-478`, `models_runtime.go:173-176` and `session_runtime_providers.go:234-239`.

Replay risk. `provideFactoryRuntimeClock` never returns nil (`wire_gen.go:141`). The `if clock == nil && artifact != nil` branch in `factory_sessions/internal/service/construction.go:382` may therefore never select the replay-artifact clock on the composed path. Before claiming F16, characterize which clock replay actually uses.

**Amendment v1.1:** AM15: retain `fi-t01-process-time-20261002` at `b8960a415c4e63ce5bd5e5bd001068b5ca4692d7`. FI-SHARED-QUALITY remains owned by Factory Reliability/shared quality maintenance; primary-lane coverage, fixture isolation and real lane-audit release are outstanding. This docs revision neither repairs nor exempts that gate. Author handoff is separate from review terminal CI/merge.

**Contract and configuration excerpts:**

Authored source: `pkg/services/edges/definition.go` / `Clock fields`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
// Relevant existing Edges field:
	Clock                            platformclock.Source
```

Proposed:

```go
// Relevant proposed Edges fields:
Clock platformclock.Source
ProcessScheduler platformclock.TimerSource

// pkg/platform/clock/clock.go (one shared seam; see contracts.md "T01"):
type TimerSource interface {
	Source
	NewTimer(time.Duration) Timer
	After(time.Duration) <-chan time.Time // added; Deterministic fires on tick advance
}
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `TestMergeUsesExplicitReplacementsAndPreservesDefaults` and `TestMergeUsesCallerOwnedAgyPTYClock` (`pkg/services/edges/definition_test.go`).
- Wire: `TestFactoryRuntimeEffectProvidersSelectExactProcessEdges` (`pkg/wire/runtime_inputs_test.go:601`) and `TestFactoryRuntimeClockResolverPreservesOverrideAndSelectsPlatformDefault` (`pkg/wire/boundary_test.go:80`).
- Functional: `TestAPIResponseEventSessionExpiryReturnsTypedGone` and the cron cases in `tests/functional/automations/cron_root_composition_test.go`.

No existing test proves that a replaced `Edges.Clock` changes event, artifact or recording timestamps; F15/F16 are new. `pkg/root` contains only `TestMain`, so `go test ./pkg/root` proves nothing today.

**Characterization prerequisites:**

1. `TestFactoryRuntimeMetricsClockSelectsTimerCapableEdgeOrReal`. Layer: unit. Path: `pkg/wire/runtime_inputs_test.go`.
   - Given, in turn: a Now-only `Edges.Clock`, a timer-capable fake, and no clock.
   - When `provideFactoryRuntimeMetricsClock` runs.
   - Then it returns, respectively: Real, the exact fake, and Real.
   - This pins the silent fallback at `profiles.go:481-485` before it is changed.
2. `TestReplayUsesRecordedArtifactClockThroughComposedProcess`. Layer: functional. Path: `tests/functional/factory/replay_contracts/`.
   - Given a recording with known tick timestamps.
   - When it is replayed through `Process.Execute`.
   - Then record which timestamps the replayed facts carry, as characterization. This settles the `construction.go:382` question before F16 is claimed.

**Fake-clock waiter-count audit (required before cutover).** These functional tests count waiters with `BlockUntilContext(ctx, n)`:
- `tests/functional/work/submission/legacy_unary_test.go:209`
- `tests/functional/workstations/cron/helpers_test.go:91`
- `tests/functional/workstations/poller/poller_test.go:240`
- `tests/functional/factory/packaged/loop/invocation_test.go:177`

Moving extra timers onto `Edges.Clock` changes those counts. List every waiter the lane adds or moves. Update each count in the same PR, with the reason in the PR body.

**Focused commands:**
- `go test ./pkg/platform/clock ./pkg/services/edges ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/events/response_events`, `tests/functional/automations`, `tests/functional/work/submission`, `tests/functional/workstations/cron`, `tests/functional/workstations/poller`, `tests/functional/factory/packaged/loop`, `tests/functional/factory/replay_contracts`

**Acceptance criteria:**

- [ ] Given Selected controllable process source; no specialized override, when drive admission/artifact/recording/chat timestamps and scheduling, then public/edge-observed facts follow selected source and scheduling progression.
- [ ] Given Explicit specialized legacy clock and replay clock, when execute their owning operations, then specialized override wins; replay facts follow ticks; OS cleanup remains governed by scheduler.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.
- [ ] Given a `Deterministic` source, when `After(d)` is called and the source advances past `d`, then the channel fires, and it does not fire before; `Real` and `Deterministic` both satisfy the extended `TimerSource`.

**Verification:**

- Behavioral witness: F15/F16, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/platform/clock ./pkg/root ./pkg/wire`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T02 — Diagnostics share a backend and retain local output policy

**Parent behavior:** F14; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/edges/definition.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Diagnostics share a backend and retain local output policy, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F14.

**Actor and trigger:** CLI operator/API client; given concurrent quiet, normal, verbose/debug invocations, run/read through CLI with controlled logs.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/edges/definition.go` / `ProcessLogger field`, all associated inventory entries assigned to T02, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Add optional process Zap backend override; default selected once. Replace two Models NewNop injections with backend-derived logging/diagnostic adapters. Keep invocation terminal level/sink policy local and session .With/.Named correlation.

**Contract and configuration excerpts:**

Authored source: `pkg/services/edges/definition.go` / `ProcessLogger field`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
// ProcessLogger is not present.
```

Proposed:

```go
// Add to Edges; nil is an omitted caller override only.
ProcessLogger *zap.Logger
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `TestResolve_QuietWinsOverVerboseAndDebug` (`pkg/transports/cli/terminalpolicy/policy_test.go`) and `TestModelsListCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr` (`pkg/transports/cli/root_models_docs_coverage_test.go:993`).
- Wire: `TestProvideOperatorSettingsServiceLogsThroughTheCanonicalWireLogger`.
- Functional:
  - `TestSuccessfulInvocationOutputModes` (`tests/functional/cli/factory_run/output/output_test.go`). Each subtest builds its own process, so this is not a concurrency witness.
  - `TestProcessModelsInvokeFailureKeepsStreamsSafeAndReleasesCapacity` (`tests/functional/models/model_invoke/cli_test.go:226`).

**Current behavior and migration risk (AM01).** `logging.NewDefaultLogger` delegates to `BuildTerminalMutedLogger` (`pkg/platform/logging/logger.go:38-52`): warn-level records go to `io.Discard`; default terminal diagnostics do not write OS stderr. Models host diagnostics currently use `zap.NewNop()`. Characterize capture/redaction and per-invocation terminal/file routing before changing either origin; never infer a leak from the obsolete OS-stderr description.

**Characterization prerequisites:**

1. `TestConcurrentQuietAndVerboseInvocationsKeepOwnFraming`. Layer: functional. Path: `tests/functional/transport/cli/output/`.
   - Given one shared process with separate quiet, JSON, response-stream NDJSON, normal and verbose/debug invocations running concurrently with scenario-owned profiles/sessions.
   - When both complete.
   - Then each successful invocation preserves its selected framing: quiet emits no response output, JSON remains parseable, response-stream NDJSON has ordered frames, and verbose/debug diagnostics remain local. Separately assert quiet+JSON or quiet+explicit-output returns `INVOCATION_OUTPUT_CONFLICT`, with no dispatch.
2. `TestProcessModelsInvokeQuietKeepsHostDiagnosticsOffStdout`. Layer: functional. Path: `tests/functional/models/model_invoke/`.
   - Given a controlled host failure in separate quiet and JSON invocations (never the rejected quiet+JSON combination).
   - When the model is invoked.
   - Then failure leaves stdout empty, writes the existing typed model-failure stderr, produces no success artifact, and releases capacity for recovery (`tests/functional/models/model_invoke/cli_test.go:269-281`).

**Focused commands:**
- `go test ./pkg/platform/logging ./pkg/transports/cli/terminalpolicy ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/cli/factory_run/output`, `tests/functional/transport/cli/output`, `tests/functional/models/model_invoke`, `tests/functional/models/model_list`, `tests/functional/transport/acp/stdio`

**Acceptance criteria:**

- [ ] Given Concurrent quiet, normal, verbose/debug invocations, when run/read through CLI with controlled logs, then correct stdout/stderr/NDJSON framing; one invocation cannot change peer output policy.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F14, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/platform/logging ./pkg/wire`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T03 — Recording stop export and replay preserve history

**Parent behavior:** F07/F08; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/recordings/wire/wire.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Recording stop export and replay preserve history, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F07/F08.

**Actor and trigger:** CLI operator/API client; given selected recording path and injected flush failure, stop after Work completion or cancellation; given recorded history and representative legacy fixture, export/read/replay/resume through public surface.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/recordings/wire/wire.go` / `NewService`, all associated inventory entries assigned to T03, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Independently inject canonical ledger, projection, lifecycle, artifact export and replay owners. Public root forwards operations. Require one RecordingClock; remove variadic clock/zero timestamp and logger fallbacks; preserve flush errors and recording handles.

**Amendment v1.1:** AM11: preserve `fi-t03-recordings-history-20261002` at `5725072a5179e7d5e28c869584a2fdb3a8635e29`; corrected terminal-metadata characterization precedes restructuring. F07/F08 remain mandatory.

**Contract and configuration excerpts:**

Authored source: `pkg/services/recordings/wire/wire.go` / `NewService`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func NewService(
	ledger recordings.Ledger,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error)
```

Proposed:

```go
// Removed: composite NewService in recordings/wire.
// Authored destination: recordings/internal/core.go.
// Internal root only combines the already constructed owner contracts.
func NewCombinedService(
 ledger recordings.Ledger,
 projection recordings.ProjectionService,
 lifecycle recordinglifecycle.Service,
 artifacts artifactsexport.Service,
 replay recordingsreplay.Service,
 canonical canonicalledger.Service,
 historical historicalquery.Service,
 clock recordings.RecordingClock,
 logger logging.Logger,
) recordings.Service
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: the flush-retry and Finish flush-failure tests in `pkg/services/recordings/lifecycle_capability_test.go`, and `TestReplayArtifactLoaderReadsLegacyArtifactAndClockThroughRecordingWire`.
- Wire: `pkg/wire/recordings_*_composition_test.go`.
- Functional:
  - `TestRecordingFlushBeforeProcessExecuteReturns` (`tests/functional/recordings/process`).
  - `TestRecordReplayLifecycleActivatesThroughRootBuildProcessAfterLifecycle` (`tests/functional/recordings/root_composition`).
  - `TestLegacyReplayFailureInspection` (`tests/functional/factory/replay_contracts`).
- HTTP: `pkg/transports/http/recordings/handlers_history_test.go`.

**Characterization prerequisite:**

- `TestCombinedServiceAbandonedScopeFinishesAtInjectedClock`. Layer: unit. Path: `pkg/services/recordings/internal/`.
  - Given an already started recording, a fixed injected UTC clock, post-start cancellation and a writer that fails the final flush (not the begin write).
  - When abandonment finishes the recording through the lifecycle owner.
  - Then joined cancellation/final-flush causes remain inspectable and terminal metadata uses the injected UTC time.
  - With an absent clock, finalization returns `ErrInvalidRecordingTerminalMetadata` and leaves `FinalizedAt` unset; a zero clock fallback is not successful finalization (`core.go:449-454`; lifecycle `service.go:360-384`).

**Shared-row ownership:** this lane owns the Recordings fallback rows that inventory previously assigned to T23 (variadic clocks, `firstRecordingClock`, `EnsureLogger` at `core.go:390`). It also owns the flush-ticker host-scheduling addition.

**Focused commands:**
- `go test ./pkg/services/recordings/... ./pkg/transports/http/recordings ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/recordings`, `tests/functional/replay_contracts`, `tests/functional/factory/replay_contracts`, `tests/functional/sessions/resume_from_recording`
- Long-only F08 cases: `go test -tags=functionallong ./tests/functional/replay_contracts/...` (or `make test-functional-long`).

**Acceptance criteria:**

- [ ] Given Selected recording path and injected flush failure, when stop after Work completion or cancellation, then durable history readable on successful flush; flush failure is surfaced safely.
- [ ] Given Recorded history and representative legacy fixture, when export/read/replay/resume through public surface, then same admitted facts, history, logical identity and typed legacy failure inspection.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F07/F08, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/recordings/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T04 — Factory load validation and persistence retain authored behavior

**Parent behavior:** F02; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/factory_definitions/internal/services/validation/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Factory load validation and persistence retain authored behavior, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F02.

**Actor and trigger:** CLI operator/API client; given valid and invalid factory/work inputs, operator loads/saves/submits.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/factory_definitions/internal/services/validation/service.go` / `Dependencies`, all associated inventory entries assigned to T04, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Independently inject catalog, validation, compilation, authoring, portability and distribution. Extract runtime snapshot query authority before removing attachment. Remove all owned bags and constructor guards; preserve Factory validation and layout semantics.

**Contract and configuration excerpts:**

Authored source: `pkg/services/factory_definitions/internal/services/validation/service.go` / `Dependencies`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type Dependencies struct {
	Operations            factorydefinitions.DefinitionValidationOperation
	Effective             factorydefinitions.EffectiveDefinitionValidationOperation
	LoadCanonical         factorydefinitions.CanonicalFactoryJSONLoader
	RequiredToolChecker   factorydefinitions.RequiredToolChecker
	OrchestratorValidator factorydefinitions.OrchestratorDefinitionValidator
}
```

Proposed:

```go
// Removed: validation.Dependencies.
func NewService(
 operations factorydefinitions.DefinitionValidationOperation,
 effective factorydefinitions.EffectiveDefinitionValidationOperation,
 loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
 requiredToolChecker factorydefinitions.RequiredToolChecker,
 orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) validationservice.Service
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage (strong):**
- Functional CLI: `TestCLIFactoryRejectedAuthoredSourcesFailBeforeRuntimeExecution` and `TestCLIFactoryValidateDoesNotMutateOnFailure` (`tests/functional/factory_definitions/transports`), and `TestExportImportSmoke_*` (`tests/functional/bootstrap_portability`).
- Functional HTTP: `TestCurrentFactoryPUT_*` (`tests/functional/runtime_api/factory_transformation`).
- Wire: `pkg/wire/factory_definition_packaged_installation_test.go`.

No blocking gap.

**Also in scope (inventory additions):** the System Initialization duplicate packaged installer in `pkg/wire/profiles.go:380-403`. This is the only lane that touches System Initialization. Keep `tests/functional/product/init_setup` green.

**Focused commands:**
- `go test ./pkg/services/factory_definitions/... ./pkg/services/system_initialization/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/factory_definitions`, `tests/functional/bootstrap_portability`, `tests/functional/runtime_api/factory_transformation`, `tests/functional/factory/packaged`, `tests/functional/product/init_setup`

**Acceptance criteria:**

- [ ] Given Valid and invalid Factory/Work inputs, when operator loads/saves/submits, then same public result or typed validation failure; no invalid dispatch.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F02, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_definitions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T05 — Work preparation and admission use injected behavior

**Parent behavior:** F02; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/work/internal/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Work preparation and admission use injected behavior, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F02.

**Actor and trigger:** CLI operator/API client; given valid and invalid factory/work inputs, operator loads/saves/submits.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/work/internal/service.go` / `applicationService`, all associated inventory entries assigned to T05, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject RuntimeSessionResolver, stateaccess, request/content preparation and invocation-input preparation as distinct completed roles (AM02; contracts.md T05). Stop constructing preparation on each operation. Require explicit completed-flush reader and content capabilities; admission-only paths use a nonnil implementation returning existing content configuration errors.

**Amendment v1.1:** AM02: both preparation roles are direct Work dependencies. Construct private invocation-input policy and completed public mapping adapter once in owner Wire, separately from RequestPreparationService/ContentPreparation. Keep CompletedFlushSequenceReader required under stateaccess; preserve cancellation/error mapping and admission-only configuration errors. Native pairs and legal private destinations are in contracts.md T05.

**Contract and configuration excerpts:**

Authored source: `pkg/services/work/internal/service.go` / `applicationService`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type applicationService struct {
	runtimes             work.RuntimeResolver
	readSubmittedFile    work.SubmittedFileReader
	inspectSubmittedFile work.SubmittedFilePathInspector
	contentStaging       work.ContentStagingService
	contentMaterializer  work.ContentMaterializer
	stateAccess          stateaccess.Service
}
```

Proposed:

```go
func NewService(
 runtimes work.RuntimeResolver,
 readSubmittedFile work.SubmittedFileReader,
 inspectSubmittedFile work.SubmittedFilePathInspector,
 contentStaging work.ContentStagingService,
 contentMaterializer work.ContentMaterializer,
 stateAccess stateaccess.Service,
 preparation work.RequestPreparationService,
 invocationPreparation work.InvocationInputPreparation,
) work.FileSubmissionService
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `TestNewServiceContentSliceRequiresInjectedDependencies` and the `state_access` read-confirmation tests.
- Wire: `TestProvideWorkServiceConstructsThroughWorkWireBridge`.
- Functional: `TestCLISubmitBatchInvalidJSONFailsBeforeUpsert` and `TestWorkEffectsRemainInertThroughRootBuildProcessConstruction` (`tests/functional/work/`), and `tests/functional/runtime_api/api_work_service_application_slices_test.go`.

**Characterization prerequisite:**

- `TestWorkListConfirmsStateAfterRecordingFlush`. Layer: functional. Path: `tests/functional/work/root_composition/`.
  - Given a recorded session with completed Work.
  - When the recording flushes and Work is listed over HTTP and CLI.
  - Then flushed items report `confirmationState=CONFIRMED` and unflushed items report UNCONFIRMED.
  - Why: the completed-flush reader (`work/internal/service.go:39`, wired at `pkg/wire/runtime_inputs.go:441`) becomes required. If it is mis-wired, every item would stay UNCONFIRMED and no test would notice.

**Also in scope:** the `NewAdapterFromRoles` root synthesis (`work/transports/http/adapter.go:28-30`).

**Focused commands:**
- `go test ./pkg/services/work/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/work/...`
- `go run ./cmd/functionallane -root ./tests/functional/runtime_api/...`

**Acceptance criteria:**

- [ ] Given Valid and invalid Factory/Work inputs, when operator loads/saves/submits, then same public result or typed validation failure; no invalid dispatch.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F02, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/work/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T06 — Provider Session inspection uses fixed readers

**Parent behavior:** F11; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/provider_sessions/internal/service/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Provider Session inspection uses fixed readers, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F11.

**Actor and trigger:** CLI operator/API client; given fixture codex/cursor transcripts and unavailable storage, cLI inspects Provider Sessions.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/provider_sessions/internal/service/service.go` / `inspectionService`, all associated inventory entries assigned to T06, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Construct Codex and Cursor readers separately with direct walker/filesystem/database effects. Inject readers into inspection root; storage roots remain data. Remove reader Dependencies and hidden default selection; preserve safe unavailable-storage outcomes.

**Amendment v1.1:** AM14/AM15: retain `fi-t06-reader-characterization-20261003` at `784df0844baf076e589d386f53ba78570c06c8c2`. Author owns characterization and pushed-head/open-PR/CI-start/feedback handoff; independent validation and terminal quality/merge belong to review. FI-SHARED-QUALITY primary-lane and fixture-isolation release remains outstanding, never an author-produced validator PASS or docs waiver.

**Contract and configuration excerpts:**

Authored source: `pkg/services/provider_sessions/internal/service/service.go` / `inspectionService`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type inspectionService struct {
	codex        codexreader.Service
	cursorReader cursorreader.Service
	files        providersessionsinternal.FileSystem
}
```

Proposed:

```go
func NewInspectionService(
 codex codexreader.Service,
 cursor cursorreader.Service,
 files providersessionsinternal.FileSystem,
) providersessions.Service
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `TestNewForRootsRejectsMissingProcessEdges` and the `TestDefaultAgentStorageRoot_*` cases.
- Functional:
  - The missing, corrupt and unavailable transcript cases in `tests/functional/provider_sessions/details`.
  - `TestAPIProviderSessionRejectsRawFilesystemPathInput`.
  - `TestProviderSessionsRemainInertThroughRootBuildProcessConstruction`.

**Characterization prerequisite:**

- `TestProviderSessionsHomeResolutionFailureOutcome`. Layer: functional. Path: `tests/functional/provider_sessions/`.
  - Given `ProviderSessionResolveHomeDirectory` returns an error.
  - When BuildProcess runs and provider-session details are requested.
  - Then the current outcome is pinned exactly, whether that is a construction error or a safe read error.
  - Why: home resolution moves out of `New` (`internal/service/service.go:45-52`).

**Also in scope:** the eight redundant guards at `provider_sessions/internal/service/service.go:95-122` and the transport guards.

**Focused commands:**
- `go test ./pkg/services/provider_sessions/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/provider_sessions/...`

**Acceptance criteria:**

- [ ] Given Fixture Codex/Cursor transcripts and unavailable storage, when cLI inspects Provider Sessions, then same discovery/transcript result or safe error.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F11, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/provider_sessions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T07 — Settings preserve unknown fields and resolution

**Parent behavior:** F12; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/operator_settings/wire/service_from_config_document.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Settings preserve unknown fields and resolution, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F12.

**Actor and trigger:** CLI operator/API client; given operator settings with unknown fields and unavailable prerequisite, read/write/resolve settings.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/operator_settings/wire/service_from_config_document.go` / `NewServiceFromConfigDocument`, all associated inventory entries assigned to T07, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Replace active owner-or-ports ConfigDocument construction with injected document and resolution owners. Preserve unknown-field decoder/preserver policy. Remove test-only global ctor registry/HomePorts paths after caller checks; preserve exported compatibility if real consumers exist.

**Contract and configuration excerpts:**

Authored source: `pkg/services/operator_settings/wire/service_from_config_document.go` / `NewServiceFromConfigDocument`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func NewServiceFromConfigDocument(
	service operatorsettings.ConfigDocumentService,
	providersRoot providers.Service,
	idGenerator operatorsettings.IDGenerator,
	logger logging.Logger,
) (operatorsettings.Service, error)
```

Proposed:

```go
// Removed: NewServiceFromConfigDocument owner-or-ports construction.
// Authored destination: operator_settings/internal/service/service.go.
// Settings owner constructor uses the independently constructed leaves.
func New(
 document settingsdocument.Service,
 resolution resolution.Service,
 files operatorsettings.FileSystem,
 createTemp operatorsettings.CreateTemporaryFile,
 decoder operatorsettings.ConfigDecoder,
 encoder operatorsettings.ConfigEncoder,
 idGenerator operatorsettings.IDGenerator,
 logger logging.Logger,
 diagnostics operatorsettings.ConfigDiagnosticsDecoder,
) (operatorsettings.Service, error)
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Caller-check result.** The global Providers constructor registry is dead in production:
- `RegisterProvidersRootConstructor` has no callers.
- `operator_settings/internal/construct` is imported only by tests.
- `RegisterDefaultsResolutionFromHome` is already a no-op.

Live callers that must migrate first:
- `pkg/wire/profiles.go:316` (`NewServiceFromConfigDocument`, the production path).
- `tests/functional/internal/support/acp_agent_profile.go:33` `SeedACPAgentProfile`, used by the ACP stdio, realclient and chat_sessions functional tests.
- `pkg/wire/session_runtime_providers_test.go:277`.

**Canonical reachability (AM03).** No canonical HTTP/MCP Settings load/update routes exist. The misleading root-composition tests construct dormant adapters via `NewServiceFromHomePorts`; they do not prove canonical activation. Review all authored OpenAPI fragments and production import/binding reachability before retiring compatibility code. Do not add routes.

**Characterization prerequisite:**

- `TestCLISettingsUnknownFieldsPrecedenceAndSafeFailure`. Layer: functional. Path: `tests/functional/operator_settings/root_composition/`.
  - Given a customer config with unknown fields, explicit/default values and a controlled unavailable prerequisite.
  - When existing canonical CLI read/write/resolve commands run through `support.BuildProcess` and `Process.Execute`.
  - Then unknown fields round-trip, precedence holds and current safe failures remain visible (F12).
  - Port dormant HTTP/MCP adapter behavior to owner component unit coverage with direct fakes. Those tests prove adapter behavior only, not canonical transport activation.

**Also in scope:** the per-operation guards in `config_document.go` and `backend_scope.go` (inventory additions), and the Settings function bag in System Initialization.

**Focused commands:**
- `go test ./pkg/services/operator_settings/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/operator_settings`, `tests/functional/transport/acp`, `tests/functional/sessions/chat_sessions`

**Acceptance criteria:**

- [ ] Given Operator settings with unknown fields and unavailable prerequisite, when read/write/resolve settings, then preserve unknown fields/default precedence; same safe failure.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F12, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/operator_settings/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T08 — Provider attempts use fixed execution collaborators

**Parent behavior:** F05; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/providers/wire/wire.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Provider attempts use fixed execution collaborators, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F05.

**Actor and trigger:** CLI operator/API client; given controlled provider success, denial, timeout, and cancellation, invoke Worker/direct child/Factory Work and cancel one.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/providers/wire/wire.go` / `wireOptions`, all associated inventory entries assigned to T08, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject catalog, ACP, built-in Codex/Claude/AGY adapters, registrations and normalized execution. Remove behavioral effects from options, preserve pure registration/config values. Inject clocks/loggers directly, retire args ...any constructor after callers; keep request override selection.

**Contract and configuration excerpts:**

Authored source: `pkg/services/providers/wire/wire.go` / `wireOptions`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type wireOptions struct {
	catalog           []catalogwire.Option
	commandRunner     providerservice.CommandRunner
	agyCommandRunner  providerservice.CommandRunner
	agyCommandClock   platformclock.Source
	agyPTYPlatform    AgyPTYPlatformDependencies
	acpIntegrations   []providers.ACPIntegration
	commandFactory    platformprocess.CommandFactory
	executableLocator platformprocess.ExecutableLocator
	registrations     ProviderRegistrations
	logger            logging.Logger
}
```

Proposed:

```go
// Removed: wireOptions as a behavioral effect container.
// Authored destination: providers/internal/service/service.go.
// Pure catalog/registration/integration values stay on their owning adapters.
func NewWithACP(
 catalogService catalog.Service,
 executionService execution.Service,
 acpService acp.Service,
 packagedACP []providers.ACPIntegration,
 logger logging.Logger,
 lifecycle providerLifecycle,
) (*Service, error)
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green (verified 2026-10-02):** unit timeout/cancel classification in `pkg/services/providers/internal/services/execution/internal/service/cancellation_test.go`, `terminal_result_test.go` and `pkg/services/providers/execute_characterization_test.go`; PTY `TestPTYEffect*`; wire `TestProvideProvidersServicePrefersAgyCommandRunnerWithInjectedPTYHost` (`pkg/wire/agy_pty_test.go`) and `TestAgyLegacyPTYCompatibilityThroughProvidersWire`; HTTP `transports/http/execute_error_mapping_test.go`; MCP `transports/mcp/execute_test.go`; functional `TestAgyTimeoutFailureThroughRootBuildProcess`, `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical` (`tests/functional/providers/agy`), and the ACP restart cases in `tests/functional/providers/acp`.

**Characterization prerequisite (lands in its own commit before restructuring):**

- `TestAgyCanonicalCommandRunnerExecutesWithZeroPTYEffects`. Layer: functional. Path: `tests/functional/providers/agy/pty_selection_test.go`.
  - Given a supplied controlled `ProviderCommandRunner` and fake `AgyPTYHost` observers.
  - When AGY Work executes through canonical `BuildProcess` / `Process.Execute`.
  - Then the command runner observes the expected argv/workspace, Work completes, cancellation/cleanup preserves F05, and PTY launches remain zero (`session_runtime_providers.go:202-214`; `runtime_provider_bindings.go:26-39`).
  - Separately characterize absent override/default command composition and inert construction without launching a real provider. Retain owner-unit legacy PTY execution/cleanup compatibility; do not change canonical runner selection (AM04).

Do not count `tests/functional/providers/{contract,script,mock_workers,observability}` as evidence. They contain only `doc.go` and are quarantined.

**Focused commands:**
- `go test ./pkg/services/providers/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/providers/...`
- `go run ./cmd/functionallane -root ./tests/functional/workers/inference/...`

**Shared-row ownership:** this lane owns every provider-internal fallback/clock row (inventory T23 rows for `providers/internal/service/service.go` and `providers/wire/wire.go`, and the T21 row for `execution/wire/wire.go` built-in effects). T23/T21 do not edit these functions.

**Acceptance criteria:**

- [ ] Each distinct selected subcase F05a–d in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Controlled provider success, denial, timeout, and cancellation, when invoke Worker/direct child/Factory Work and cancel one, then same terminal classification; released worktree/attempt; peer session survives.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F05a–d plus  F05, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/providers/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T09 — Worker execution retains attempt cleanup and strategy policy

**Parent behavior:** F05; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/workers/wire/wire.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Worker execution retains attempt cleanup and strategy policy, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F05.

**Actor and trigger:** CLI operator/API client; given controlled provider success, denial, timeout, and cancellation, invoke Worker/direct child/Factory Work and cancel one.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/workers/wire/wire.go` / `ScriptDependencies`, all associated inventory entries assigned to T09, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject agent/script/inference/mock strategies, registry, harness, prompting/worktree behavior and Execute. Direct invocation uses the same Execute owner. Remove runner bags and factories; requests/worktrees remain scoped. Disabled/mock behavior is explicitly selected.

**Amendment v1.1:** AM05: preserve `RetainWorktree` compatibility (`execute.go:438-441`; Runtime `invoke_worker.go:409-414,479`). T09 releases attempt resources and nonretained attempt checkouts; Runtime T15 owns retained/reused session-generation checkout lifetime. Characterize retained, nonretained and reuse cases without unconditional deletion or weakening F05a. Any retained-checkout lifetime policy change requires operator authority.

**Contract and configuration excerpts:**

Authored source: `pkg/services/workers/wire/wire.go` / `ScriptDependencies`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type ScriptDependencies struct {
	CommandRunner platformprocess.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}
```

Proposed:

```go
// Removed: ScriptDependencies. Focused worker runner provider:
func NewScriptRunner(
 config ScriptConfig,
 commandRunner platformprocess.CommandRunner,
 factoryDocs workers.FactoryDocsLoader,
 now func() time.Time,
 publish workers.ProgressPublisher,
 record workers.ScriptEventRecorder,
) workers.Runner
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `runners/wire` registry tests, including `TestNewProductionRegistryInferenceFallsBackThroughAgentRunner` (a strategy rule flattening must keep); script and inference runner suites; `durable_provider_test.go`; `workers/wire/command_bridge_test.go`.
- Functional:
  - `TestCLIRunTimeoutRecoversOnSameProcess` and `TestCLIRunCancellationRecoversOnSameProcess` (`tests/functional/workers/transports/cli/run/modes/adverse_modes_test.go`)
  - `TestFactoryRuntimeMixedDirectAndFactoryChildDispatchesStayIsolatedThroughCLI` (`tests/functional/factory_runtime/root_composition/dispatch_worker_sessions_concurrency_activation_test.go`)

**Caller-check result:** `workers/internal/invocation.go` `newInvocation` has no production caller. Its only producer, `provideConductorInvocationWithProgressFactory` (`pkg/wire/session_runtime_providers.go:1105`), is in `servicesSet` but no generated injector calls it. The callers are tests only (`pkg/wire/session_runtime_providers_test.go`, `workers/wire/runtime_bridge_canonical_test.go`). Disposition: delete, with no characterization needed. Coordinate the deletion with T17, which owns the durable/conductor factories in session_runtime_providers. `NewProviderFromCommandRunner` is live through `durable_provider.go:17` (T17).

**Characterization prerequisite (shared with T16; whichever lane starts first lands it):**

- `TestTwoFactorySessionsCancelOneInFlightProviderAttemptPeerCompletes`. Layer: functional. Path: `tests/functional/workers/concurrency/cross_session_cancel_test.go`.
  - Given two explicit Factory Sessions in one `BuildProcess`, each with an attempt held at a barrier in a controlled `ProviderCommandRunner`.
  - When session A's attempt is cancelled through the public control.
  - Then A emits a cancelled terminal event and releases its worktree/attempt; B is released and completes with only its own events.
  - Why: existing cancellation tests use one session (`TestDWROS8ManagerInterruptsOnlyOneRemoteWorker`) or a JavaScript loop (`TestCrossCancelGatedJavaScriptSessionPreservesAnotherSession`).

**Focused commands:**
- `go test ./pkg/services/workers/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/workers/...`
- `go run ./cmd/functionallane -root ./tests/functional/factory_runtime/root_composition/...`

**Size:** about 2,400 LOC across 8 files. If one focused review pass cannot cover it, escalate so the dead-path deletion can move to its own task. The project lead MAY also pre-split along these lines at admission (operator decision, 2026-10-02); otherwise split only if review requires it.

**Acceptance criteria:**

- [ ] Each distinct selected subcase F05a–d in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Controlled provider success, denial, timeout, and cancellation, when invoke Worker/direct child/Factory Work and cancel one, then same terminal classification; released worktree/attempt; peer session survives.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F05a–d plus  F05, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/workers/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T10 — Model catalog preparation and host facts use fixed owners

**Parent behavior:** F09; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/models/wire/wire.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Model catalog preparation and host facts use fixed owners, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F09.

**Actor and trigger:** CLI operator/API client; given two model scopes and controlled inference success/failure/capacity, invoke/repeat/cancel in either scope.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/models/wire/wire.go` / `modelsServiceComponents`, all associated inventory entries assigned to T10, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject scopes/assets/catalog/host/inference independently. Extract slot-facts and capacity/idle state authority before host/leases; remove both adapter.host assignment and BindCoordinator, as specified concretely in contracts.md, plus component assembly. Require process effects/logger/revision resolver and remove variadic defaults.

**Amendment v1.1:** AM06: fixed leaf provider extraction is this stage. ProcessDependencies declaration/forwarding/consumption remains temporary compatibility, with final removal assigned exclusively to T11 after T10/T30 merge. T30 exclusively owns host slot/facts/leases/coordinator; current-main terminal rows are preserved. Temporary aggregate retention is not final S01 satisfaction.

**Contract and configuration excerpts:**

Authored source: `pkg/services/models/wire/wire.go` / `modelsServiceComponents`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type modelsServiceComponents struct {
	runtimeScopes runtimescopes.Service
	assets        scopedassets.Service
	catalog       catalog.Service
	runtimeHost   runtimehost.Service
	inference     inference.Service
}
```

Proposed:

```go
// Removed: modelsServiceComponents as a dependency/construction container.
// Inject fixed behavior roles individually; retain scoped state/resources
// under the owners and operation contracts specified in contracts.md.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Lease unit tests: `TestAcquireModelLeaseRejectsCapacityExhaustion`, `...RejectsContendedCapacity`, `TestReleaseModelLeaseNotifiesCapacityCoordinator`, and `leases/internal/service/concurrency_test.go`.
- Host and lease tests in `runtime_host/internal/service/unload_test.go`: `TestIdleUnloadStopsRuntimeAfterLeaseRelease`, `TestResourcePressureEvictsIdleRuntime`, `TestCloseRuntimeScopeStopsOnlyThatScopesSupervisedRuntimes`, `TestCloseRuntimeScopeRevokesLeasesOwnedByStoppedHost`.

**Characterization prerequisite:**

- `TestResourcePressureDoesNotEvictPeerScopeActiveLeaseHolder`. Layer: unit. Path: `pkg/services/models/internal/services/runtime_host/internal/service/unload_test.go`.
  - Given `MaxLoadedRuntimes=1` and scope A holding an active lease.
  - When scope B calls `EnsureModelHost`.
  - Then A is not stopped and B receives the existing capacity classification; after A releases, B succeeds.
  - Why: eviction moves into the new coordinator, and no test covers a peer scope's active holder.

**Split (validation review):** this lane is now only the leaf providers replacing modelsServiceComponents; ProcessDependencies remains temporary compatibility until T11 final cutover (AM06).

The `SlotState` / capacity coordinator cycle cut, which removes `adapter.host` and `BindCoordinator` (inventory `slot_facts.go` row and the Additional coordination finding), moves to **T30**. T11 depends on both this lane and T30. Land the prerequisite above in T30 if T30 starts first.

**Shared-row ownership:** this lane owns the Models fallback rows that inventory assigns to T23 in `models/wire/wire.go` (variadic and symlink resolvers, `VideoAudioRunner`). The `runtime_factory.go` rows belong to T11. T02 owns only the two `NewNop` arguments in `pkg/wire/models_runtime.go` and coordinates that edit with this lane.

**Focused commands:**
- `go test ./pkg/services/models/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/models/...`
- `go run ./cmd/functionallane -root ./tests/functional/factory/packaged/tts/...`

**Acceptance criteria:**

- [ ] Each distinct selected subcase F09a–d in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Two model scopes and controlled inference success/failure/capacity, when invoke/repeat/cancel in either scope, then same result/error, no cross-scope config leak; capacity released for next invocation.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F09a–d plus  F09, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/models/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T11 — Model scopes isolate configuration and release capacity

**Parent behavior:** F09; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/models/internal/service/runtime_factory.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Model scopes isolate configuration and release capacity, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F09.

**Actor and trigger:** CLI operator/API client; given two model scopes and controlled inference success/failure/capacity, invoke/repeat/cancel in either scope.

**Dependencies:** T10, T30.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/models/internal/service/runtime_factory.go` / `Root`, all associated inventory entries assigned to T11, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Replace runtimeByScope service cache with scope configuration/asset facts plus host/lease handles owned by existing scoped owners. Fixed behavior operates on models.RuntimeScopeRef. Remove lazy host/gateway/executor graphs and runner fallback; preserve readiness/join cleanup.

**Amendment v1.1:** AM06: after T10/T30 merge, migrate Models Root and both scoped execution callers, then remove ProcessDependencies declaration, forwarding and consumption. T30 host/lease/coordinator work is consumed, not repeated; T29 requires final zero unresolved findings.

**Contract and configuration excerpts:**

Authored source: `pkg/services/models/internal/service/runtime_factory.go` / `Root`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
	runtimeByScope             map[models.RuntimeScopeRef]models.Service
```

Proposed:

```go
// Removed: map of scoped models.Service graphs.
// Scoped state belongs to runtimescopes; host/lease resources remain keyed.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage is insufficient.** `TestRootInvokeLocalUsesBoundRuntimeAndReleasesLease` (`local_execution_test.go:69`) pre-fills `runtimeByScope`. `TestRootCloseRuntimeScopePreventsConcurrentLazyRuntimeReinsertion` (`runtime_factory_inference_test.go:818`) calls `scopedRuntimeWithBuilder` directly. Both are deleted with the cache, which would leave the close-vs-invoke race with no witness.

No test runs `InvokeLocal` or `PullModelForScope` against two scopes in one Root. `TestModelsDirectTTSKeepsConcurrentScenariosIsolatedThroughRootBuildProcess` deliberately uses separate processes.

**Characterization prerequisites (land before restructuring, against the current cache):**

1. `TestRootInvokeLocalTwoScopesUseOwnConfigAndReleaseCapacity`. Layer: unit. Path: `pkg/services/models/internal/service/scoped_execution_characterization_test.go`.
   - Given two scopes opened with distinct `RuntimeConfig` on a Root built through its wire constructor, with no pre-filled map.
   - When each runs `InvokeLocal` twice, interleaved.
   - Then each result reflects its own config, each call releases its lease, and closing A leaves B invocable.
2. `TestRootCloseRuntimeScopeRejectsConcurrentInvokeLocal`. Same file.
   - Given an invoke blocked in scope resolution.
   - When the scope closes.
   - Then the invoke returns `ErrRuntimeScopeClosed` and no runtime invocation occurs.
   - Uses public `Open`, `InvokeLocal` and `CloseRuntimeScope` only.
3. `TestModelWorkersInTwoFactorySessionsKeepScopedConfigInOneProcess`. Layer: functional. Path: `tests/functional/models/root_composition/two_scope_test.go`.
   - Given one `BuildProcess` and two explicit sessions whose factories declare distinct local model resources.
   - When Work runs in both and one session is cancelled mid-invoke.
   - Then outputs follow each session's config, the cancelled host is released, and the peer then succeeds.

**Also in scope:**
- the `legacyhost/lease_policy.go:71` `time.AfterFunc` (from the inventory additions);
- the `runtime_factory.go:108-126` process-bag and resolver fallbacks (inventory rows previously assigned to T23).

**Focused commands:**
- `go test ./pkg/services/models/...`
- `go run ./cmd/functionallane -root ./tests/functional/models/...`
- `go run ./cmd/functionallane -root ./tests/functional/factory/invocation/...`

**Acceptance criteria:**

- [ ] Each distinct selected subcase F09a–d in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Two model scopes and controlled inference success/failure/capacity, when invoke/repeat/cancel in either scope, then same result/error, no cross-scope config leak; capacity released for next invocation.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F09a–d plus  F09, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/models/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T12 — Automation sources admit Work and stop independently

**Parent behavior:** F10; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/automations/internal/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Automation sources admit Work and stop independently, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F10.

**Actor and trigger:** CLI operator/API client; given controlled source scheduler and cursor store, trigger cron/script/hosted/watch event; stop/restart; fault cursor write.

**Dependencies:** Merged AM17 evidence amendment before successor planning; AM16 is already merged, not a live Work dependency. Continue from retained #2683/head `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`. No T20/WSV wait.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/automations/internal/service.go` / `Service`, all associated inventory entries assigned to T12, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**AM16 / T12 — approved private behavior-owner decomposition (binding 2026-10-03T10:20Z).** Authority: `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decision 2026-10-03T10:20Z; this supersedes the 09:05Z deferral only. Retain draft [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) at `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; its stories 002/003 and full F10/S01 remain incomplete. The separately authorized `fi-plan-amendment-am16-t12-20261003` docs lane depends on merged v1.1 #2685 and owns only this six-companion packet. AM16 is now merged historical authority; the retained T12 successor consumes merged AM17 before planning, with no historical AM16 Work dependency or T20/WSV wait. T12 owns implementation, same-PR stale script_pollers baseline deletion and obsolete-helper removal. T20 owns unchanged one-interface enforcement and measurement; deadcode allowance remains 0. No public API, configuration, event, persistence-policy or acceptance change is authorized. Native capability/provider/caller/removal pairs C01–C16 are canonical in [contracts.md](contracts.md#t12--automations-runtimesource-isolation). The docs author handoff requires final head pushed, open PR, CI started and blocking feedback addressed; independent AM16-DOC-VAL/REVIEW owns clean-room loopback, terminal CI and current-main merge. The successor still requires exact-head T12-F10 (F10a–m), T12-S01 and T12-G01/G02, independent review and merge. Historical partial evidence never substitutes for those gates.

**Concrete decomposition:** Follow C01–C16: recovery → script; cron/watch/hosted → lifecycle → reconciliation → owner → Root. SourceLifecycle and CursorScopes each have one private service root/interface and focused provider; runtime controls/scoped reads inline on existing Services. Lifecycle owns schedulerSources and runtime registration, never a parent-Service pointer. Retain keyed source/runtime cancellation/cursor state. Preserve durable-cursor-error-to-memory behavior; flattening alone preserves it. An explicit-failure policy is a separate follow-up outside this program (operator decision, 2026-10-02).

**Contract and configuration excerpts:**

Authored source: `pkg/services/automations/internal/service.go; pkg/services/automations/wire/wire.go`. Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7. Owner: T12. Primary C09 pair follows; contracts.md C01–C16 binds all capability/provider/caller/removal shapes. Public config/REST/CLI/events remain unchanged.

Current:

```go
func newService(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorFileSystem scriptpollerswire.CursorPersistenceFileSystem,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
	pollers scriptpollers.Service,
	cursors scriptpollers.CursorScopes,
) *Service

func NewRoot(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hosted HostedSourceInputs,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Root, error)

func NewService(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Service, error)
```

Proposed:

```go
// automations/internal/service.go
func newService(logger *zap.Logger, clock Clock,
	lifecycle sourcelifecycle.SourceLifecycle,
	reconciler reconciliation.Service, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers) *Service
func New(logger *zap.Logger, clock Clock,
	lifecycle sourcelifecycle.SourceLifecycle,
	reconciler reconciliation.Service, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers) *Service

// automations/wire/wire.go
type Owner = automationinternal.Service
type Reconciliation = reconciliation.Service
type ScriptPollers = scriptpollers.Service
type Cron = cron.Service
type FilesystemWatchers = filesystemwatchers.Service
type Clock = automationinternal.Clock
func NewService(logger *zap.Logger, clock Clock, lifecycle SourceLifecycle,
	reconciler Reconciliation, scriptPollers ScriptPollers, cronService Cron,
	filesystemWatchers FilesystemWatchers,
	hostedPollers automations.HostedPollers) *Owner
func NewRoot(service *Owner) automations.Root
```

Generated outputs and consumers: focused owner providers/fixtures and canonical `pkg/wire/wire_gen.go` via `make generate-wire`; no public clients/migration. Root configures lifecycle facts before runtime-keyed Start/Wait; Stop/Wait joins before release with old-instance guards (C15). T12 deletes obsolete constructors/helpers C10 and stale baseline C11 in the same PR.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage is insufficient.**
- `TestRuntimeLifecycle_IsolatesOwnersAndClassifiesDuplicates` asserts distinct owner pointers. It is internal and will break.
- `TestRuntimeLifecycle_StartsAndStopsSchedulerOwnership` asserts no admission.
- `TestNewRootRoutesActivatedRuntimeCursorToFactoryLocalRecorder` (`automations/wire/wire_test.go:484`) uses a new Root, not deactivate/re-activate.
- `TestWatcherExecutionFollowsCurrentFactorySwitch` (`tests/functional/workstations/watcher/files_test.go:511`) is long-only.
- `getCursorFromActiveRuntime` (`runtime_lifecycle.go:166`) returns the first per-runtime owner in map order. Removing per-runtime owners changes that lookup.

**Characterization prerequisites:**

1. `TestRuntimeLifecycle_ReactivationStopsPriorAdmissionAndResumesCursor`. Layer: unit. Path: `pkg/services/automations/internal/runtime_lifecycle_test.go`.
   - Given a fake clock, a recording submitter, a script poller with a durable cursor filesystem, and runtimes A and B active.
   - When A is deactivated, the clock advances, and A is re-activated with the same ID.
   - Then A admits nothing while stopped, `GetCursor` returns A's last committed cursor after re-activation, and B keeps submitting throughout.
2. `TestGetCursorWithTwoRuntimesSharingPollerInstanceID`. Same file.
   - Given two active runtimes whose pollers share an instance ID.
   - When `GetCursor` is called.
   - Then record the current result exactly (characterization). Any change to it is a delta plan, not a silent fix.

**Durable-cursor fallback (`service.go:147-153`):** `NewDurableCursorRecorder` fails only on a nil filesystem or an empty directory, and both are already guarded. The memory fallback is unreachable today. Record that instead of writing a test, and preserve current behavior. Making durable-cursor failure explicit is a separate follow-up outside this program (operator decision, 2026-10-02); it is not a task here and this lane must not change it.

**Shared-row ownership:** this lane owns the Automations rows that inventory assigns to T21 (`supervisorClock`) and T23 (cursor selection, logger fallback, script-poller getters, watcher/debounce clock fallbacks). It also owns the host-timer additions `cron_watcher.go:286` and `watcher.go:620`. Where Automations needs `AfterFunc` or clockwork behavior beyond the shared `TimerSource` (`Now`/`NewTimer`/`After`, added by T01), this lane uses the existing selected clockwork.Clock view through owner Wire (C06), without adding Scheduler as a second interface in script_pollers; T01 publishes no per-owner adapters.

### AM17 — T12 evidence boundaries

Binding authority: operator mailbox `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decisions 2026-10-03T12:01Z A and 12:57Z. AM17 corrects evidence classification under flat-injection-v1.1; it changes no endpoint, service signature, configuration, event, persistence, policy or acceptance criterion. AM16 is merged historical authority, not a live Work dependency. The retained T12 successor consumes merged AM17 before planning and continues from [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) / `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; stories 002/003 and full F10/S01 remain unproven. No T20/WSV wait or queue mutation is required.

Follow [the AM17 resolution](plan.md#authorized-amendment-flat-injection-v11) to [the complete F10 requirements ledger](tasks.md#am17--t12-evidence-boundaries). T12 owns both proofs below; independent VAL01 confirms composed behavior. The existing `automations.Root.GetCursor` delegates the service operation (`pkg/services/automations/contracts.go:215–222,457–474`). Durable reads validate `ExpectedCursor` and return detached opaque facts (`pkg/services/automations/internal/services/script_pollers/internal/service/durable_cursor.go:74–108`). The cursor HTTP adapter (`pkg/services/automations/transports/http/convergence_operations.go:52–97`) is unbound in canonical Wire/HTTP; adapter tests are component evidence. Neither that adapter nor `AutomationsRootFromEdges` is a composed functional entry point. Canonical construction/execution remains `pkg/root/process.go:17` / `pkg/initializer/application/process.go:185`.

| Obligation | Component/service-contract evidence — T12-F10-COMPONENT | Composed evidence — T12-F10 / VAL01 | Owner |
| --- | --- | --- | --- |
| F10b | Existing GetCursor returns exact committed opaque cursor/checkpoint facts | Existing customer activation admits public Work; resumed command receives the same exact committed environment facts | T12; both proofs required |
| F10g | After failed replacement, GetCursor returns prior exact committed opaque cursor/checkpoint facts | Admitted public Work remains; existing error/diagnostic is visible; next command resumes prior-commit environment facts; peer progresses | T12; both proofs required |
| F10j | GetCursor with stale ExpectedCursor returns the existing typed conflict; exact committed cursor/checkpoint facts remain unchanged; no mutation, extra command or Work | No composed public cursor endpoint is required or invented; admission/restart/diagnostic/isolation obligations remain in F10b/g and the other F10 rows | T12 component/service-contract proof |

All F10a–m success, failure and recovery obligations remain required on the successor head. C01–C16 native shapes, exactly one named interface per private service root, same-PR obsolete-helper/stale-baseline deletion, deletion-only baselines, deadcode allowance 0, T12-S01/G01/G02 and FI-A1–A8 remain unchanged. This amendment yields documentation/source/static evidence only, never runtime PASS.

AM17-DOC-TRACE/INVARIANTS/LINT belong to the docs author. Author delivery stops at final head pushed, PR open, required CI started and blocking feedback addressed. Independent ordinary review owns AM17-DOC-VAL: a fresh isolated candidate checkout, the [validation loopback report](../../../../../factory/docs/standards/validation-loopback-template.md) with AM17-1..6, AM17-DELIVERY and FI-A1–A8 individually PASS/FAIL/BLOCKED, and a smallest delta request for findings without silent repair. AM17-DOC-REVIEW owns terminal passing own-head CI, conflicts, current-main rebase, immediate premerge `make lint pkg-file-count`, and merge. `make generate-wire` plus clean diff applies only to a separately authorized graph change. Merge releases corrected successor planning; runtime gates and aggregate Project acceptance remain later work.

**Retained complete T12-F10 requirements ledger (component and composed proof required on successor head):**

Use reusable inert `root.BuildProcess` and public `Process.Execute`, explicit scenario-owned Factory Sessions, isolated profiles/directories/routes/events, and command-runner effects through `edges.Edges`. Independent hosted/session scenarios run in parallel; the Current Factory watcher cohort retains only its required local serialization. Observe public Work/events/status and controlled effects, never owner pointers. Cursor reads/conflicts use separate component/service-contract proof at existing automations.GetCursor; do not expose internal owners or build a second functional graph. No built CLI in functional cells; VCS/OS proof belongs to a separate prebuilt integration witness.

| ID | Given | When | Then |
| --- | --- | --- | --- |
| F10a | Valid inert process and configured cron source with controlled scheduler | Activate an explicit session and advance one due tick | Exactly the characterized scheduled Work identity/payload/state appears; no execution/admission before activation |
| F10b | Script source with controlled command output containing Work and advancing cursor/checkpoint | Activate through the existing customer entry point, complete one command, and resume; separately call existing automations.GetCursor at the owning component/service-contract boundary | Composed public Work is admitted and the resumed command receives exact committed opaque cursor/checkpoint env facts; component/service-contract reads return the same exact committed opaque facts. Both proofs are required |
| F10c | Hosted source with controlled HTTP success/secret/checkpoint edges | Activate and return one representative external item | Normalized Work/result belongs to that session and configured source; no real remote request occurs |
| F10d | Owned watcher root with one valid input | Preseed/start watch then deliver a new valid input event | Work reaches the authored customer state with preserved channel/execution correlation |
| F10e | A and B live with independently gated triggers, repeated for cron/script/hosted/watch because each has a distinct shutdown edge | Stop A, observe acknowledgement and joined terminal state, send stopped A and live B triggers; reactivate A and trigger again | No A admission after stop/join; B completes its next Work; reactivated A admits its next eligible Work under existing identity/cursor semantics; cancellation is attributable to A only |
| F10f | A/B active and script A has a committed cursor/checkpoint | Deactivate A, advance time, reactivate the same runtime ID and Factory directory | A resumes exact last committed facts with no stopped-interval admission; B continues |
| F10g | A script Work has been admitted and a prior cursor is committed | Fault A cursor replacement, observe the existing composed error/diagnostic, then allow the next controlled attempt; separately read GetCursor at the owning component/service-contract boundary | Admitted Work remains; composed error/diagnostic, next-command prior-commit env facts and peer progress remain observable; component/service-contract cursor read returns the prior exact committed opaque facts. Both proofs are required |
| F10h | Active script/hosted sources return representative empty output | Complete a poll cycle | No new Work appears and no cursor is advanced without the existing advancement preconditions |
| F10i | Hosted source fails one request then succeeds and peer is live | Advance selected retry schedule and release the successful response | Existing redacted failure/retry observation, one logical admitted Work under existing identity policy, peer progresses |
| F10j | A committed cursor is available | Call existing automations.GetCursor with stale ExpectedCursor at the owning component/service-contract boundary | Existing typed conflict is returned; exact committed cursor/checkpoint facts remain unchanged; the read causes no extra command or Work. This is component/service-contract evidence, not a composed public cursor endpoint or functional second graph |
| F10k | A watcher already admitted a file identity | Deliver its repeated observation; stop/restart only as supported by characterized watcher facts | No duplicate logical Work for the retained identity, existing watcher cursor/handled-state recovery holds |
| F10l | A local watcher and customer-selected Current Factory | Switch Current Factory using the existing public journey | Subsequent file Work executes in the new Current Factory; prior generation stops; retain TestWatcherExecutionFollowsCurrentFactorySwitch with -short=false |
| F10m | Hosted source has a prior checkpoint and newly admitted external Work; peer source remains live | Fault checkpoint Save after admission, observe its existing error/restart, then release the next controlled poll | Admitted Work remains; no checkpoint success is claimed; next hosted query resumes prior checkpoint under existing identity/retry policy; redacted error and peer progress remain observable |

**Focused commands:**
- `go test ./pkg/services/automations/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/automations/...`
- `go run ./cmd/functionallane -root ./tests/functional/workstations/... -short=false`

**Size:** about 1,700 LOC plus subservices. If one focused review pass cannot cover it, escalate with a split: (a) the `SourceLifecycle` driver replacing the reconciler parent callbacks; (b) removing the per-runtime owner. The project lead MAY also pre-split along these lines at admission (operator decision, 2026-10-02); otherwise split only if review requires it.

**Acceptance criteria:**

- [ ] Given Controlled source scheduler and cursor store, when trigger cron/script/hosted/watch event; stop/restart; fault cursor write, then expected Work admission/cursor behavior; no post-stop admission or peer cancellation.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: complete F10a–m; F10b/g/j cursor facts and conflicts at existing GetCursor component/service-contract boundary (T12-F10-COMPONENT), plus composed public Work/restart/diagnostics/isolation through supported customer/effect observations (T12-F10). For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: component/unit/service-contract, controlled; T12-F10-COMPONENT covers F10b/g/j exact opaque commits, failed-commit retention and stale-read typed conflict/no mutation/no extra command or Work; `go test ./pkg/services/automations/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), publish after narrow owner checks; required CI owns broad verification, with no local `make verify-pr`, `make test-functional` or `make test-full` push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; narrow local lint and required own-head CI; broad PR verification belongs to CI. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T13 — Session directory and streams survive start failure

**Parent behavior:** F03/F06; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/factory_sessions/internal/sessionservice/assembly.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Session directory and streams survive start failure, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F03/F06.

**Actor and trigger:** CLI operator/API client; given resource opening fails once, start then retry explicit session; given four explicit sessions with owned inputs/routes/streams, overlap Work, reads, and one cancellation.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/factory_sessions/internal/sessionservice/assembly.go` / `Assembly`, all associated inventory entries assigned to T13, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject session directory/admission state, response registry/stream manager, runtime-session/invocation and durable owners. Resolve Definitions/Work queries using independent session-state authority; remove concrete Assembly binding and nil error swallowing.

**Amendment v1.1:** AM07: independent sessionruntime authority -> scope activation/control -> durable owner -> invocation -> gateway -> Assembly bridge. Authority/control never depend on final Root/gateway/invoker; timeout cancellation calls scope control. Inject durable directly into gateway and remove BindProcessDurable/AttachSessionGateway/history/root binders in this lane, with only the minimal T17 NewRootFromAssembly caller bridge. Keep selected clock in Assembly and preserve failed-start rollback, identity, locks, durable history and response sequencing. Concrete pairs are in contracts.md T13; T17 removes Complete/legacy opening later.

**Contract and configuration excerpts:**

Authored source: `pkg/services/factory_sessions/internal/sessionservice/assembly.go` / `Assembly`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type Assembly struct {
	roles.SessionGateway
	registry                     sessionregistry.Service
	state                        *sessionruntime.Service
	streams                      streamManager
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	liveChangeCoordinator        factorysessioncontracts.LiveChangeCoordinator
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	recordedSessionInventory     recordings.RecordedSessionInventory
	directoryInspection          roles.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
	responseStreams              responsestreamservice.Service
	workAdmissionsMu             sync.Mutex
	workAdmissions               map[string][]*workAdmissionProjection
	// beforeWorkAdmissionProjectionRegistration is only populated by the
	// same-package replacement-window regression. It makes the otherwise
	// scheduler-dependent capture/registration gap deterministic without
	// changing the production dependency graph.
	beforeWorkAdmissionProjectionRegistration func()
}
```

Proposed:

```go
// Remove Assembly's registry/state/stream construction and peer lookup.
// A forwarding assembly facade can remain while callers migrate.
// Exact injected NewAssembly destination signature is in contracts.md.
// All retained registries/projections are explicit owned state, not services.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage (partial):**
- `TestHandleStartFailureUnregistersFailedBatchSession` (`factory_sessions/internal/runtimebinding/binding_test.go:627`). Unit only.
- `TestRootBuildProcessIsInertAndReusableAcrossFactorySessions` (`tests/functional/sessions/root_composition/process_reuse_inert_test.go:36`). It injects a start failure but never retries.
- `TestConcurrentFactorySessionResponseEventStreamsStayIsolatedAndResumeFromCursor` (`tests/functional/events/response_events/concurrent_session_isolation_test.go:44`). Two sessions, no cancellation.

F03 and F06 have no functional witness.

**Characterization prerequisites (shared with T15/T17; whichever lane starts first lands them, and the others reuse them):**

1. `TestRootProcessStartFailureThenRetrySucceedsWithoutLiveSession`. Layer: functional. Path: `tests/functional/sessions/root_composition/process_start_retry_test.go`. Reuse the `router.setFailure` fixture.
   - Given the shared root process with an `APIServerStarter` that fails once.
   - When `you run --with-server` runs, the failure clears, and the command runs again.
   - Then the first run returns the injected error and leaves no listed session, the second run succeeds, and the first listener is cancelled.
2. `TestFourExplicitSessionsIsolateOneCancellation`. Layer: functional. Path: `tests/functional/events/response_events/`.
   - Given one composed API server, four explicitly opened sessions, and a per-prompt gated provider runner.
   - When Work is submitted to all four, one session is cancelled through the API, and the others are released.
   - Then three sessions end TERMINAL and one ends cancelled. Each SSE stream carries only its own session ID, in ascending sequence. The cancelled cursor still reads its retained history.

**Ownership boundary with T17 (resolved here):**
- T13 adds leaf providers: registry, streams, identity, invocation and response state. It removes the setters and cycle inside `sessionservice/assembly.go`, including `AttachSessionGateway` and the bind helpers (see the inventory additions).
- T17 owns:
  - final NewRootFromAssembly removal; T13 removes BindProcessDurable replacement with a minimal caller bridge;
  - `RuntimeModelInvokerConfig` (reassigned from T13, consistent with contracts.md);
  - every durable/conductor factory in `pkg/wire/session_runtime_providers.go` and `profiles.go:441-470`;
  - the dead `provideConductorInvocationWithProgressFactory` (no production caller; see T09).
- AM07 corrects the NewAssembly proposal to retain clock explicitly; no clock omission or replacement binding remains authorized.

**Focused commands:**
- `go test ./pkg/services/factory_sessions/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/sessions`, `tests/functional/events/response_events`, `tests/functional/transport/acp/stdio`, `tests/functional/transport/mcp`

**Size:** about 3.4k LOC in scope. If one focused review pass cannot cover it, escalate with a split: (a) assembly/registry/streams; (b) invocation and model-invoker callbacks. The project lead MAY also pre-split along these lines at admission (operator decision, 2026-10-02); otherwise split only if review requires it.

**Acceptance criteria:**

- [ ] Given Resource opening fails once, when start then retry explicit session, then failed start is not live; retry succeeds; owned resources released.
- [ ] Given Four explicit sessions with owned inputs/routes/streams, when overlap Work, reads, and one cancellation, then outputs/events/log correlation and cursors remain attributable to each session.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F03/F06, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_sessions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T14 — Runtime controls use one keyed lifecycle authority

**Parent behavior:** F04/F06; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/factory_runtime/internal/root.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Runtime controls use one keyed lifecycle authority, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F04/F06.

**Actor and trigger:** CLI operator/API client; pause/resume/cancel controls on explicit sessions (F04 replacement is proven by T15/T17, not this lane); given four explicit sessions with owned inputs/routes/streams, overlap Work, reads, and one cancellation.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/factory_runtime/internal/root.go` / `NewRoot`, all associated inventory entries assigned to T14, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject orchestration, instance host and dispatch planning into Root; activation uses the same keyed lifecycle authority. Preserve active/failed/activating/deactivating locks/maps. Validate handle identity before consolidating the two instance-host paths; keep cancellation scoped.

**Amendment v1.1:** AM08: inject replay-sensitive factoryruntime.Clock and distinct process platformclock.TimerSource into instance host and lifecycle owner; host receives completed lifecycle. Forward scheduler only through readiness callers/owner Wire and the legacy Assembly host-injection bridge. Preserve one-second readiness ceiling and 10ms polling with cancellable NewTimer loops, no real-clock fallback or replay-driven OS wait; native pairs are in contracts.md T14. Add controlled scheduler readiness/cancel unit proof before cutover.

**Contract and configuration excerpts:**

Authored source: `pkg/services/factory_runtime/internal/root.go` / `NewRoot`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func NewRoot(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowRuntime factoryruntime.JavaScriptWorkflowRuntime,
	clock factoryruntime.Clock,
	workersPublisher dispatchplanning.WorkersPublisher,
	workersCanceler dispatchplanning.WorkersCanceler,
) (*Root, error)
```

Proposed:

```go
func NewRoot(
 orchestration orchestration.Service,
 instanceHost instancehost.Service,
 dispatchPlan dispatchplanning.Service,
) (*Root, error)
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- `TestFactoryRuntimeControlObservationAndDispatchPlanActivateThroughRootBuildProcessAfterLifecycle` (`tests/functional/factory_runtime/root_composition/lifecycle_activation_test.go:31`).
- `TestAPIPauseResumeCancelAndTerminateFactorySession` (`tests/functional/sessions/controls/pause_resume_test.go:322`).
- `TestFactoryRuntimeDispatchPlanningCancellationReachesPublishedWorkerThroughPublicDurableControl`.

**Hidden edit.** Today there are two instance hosts: `internal/root.go:81` and `internal/assembly.go:245`. Making one handle authority requires a minimal edit to `assembly.go`, which is otherwise T31/T15's file. Keep that edit to the host injection only.

**Also in scope (inventory additions):**
- The triple orchestration construction (`root.go:86`, `orchestration_owner.go:14,23`).
- The placeholder collaborators in `provideFactoryRuntimeRoot` (`pkg/wire/factory_definition_services.go:495-510`).
- The `host/lifecycle.go:171` ticker.

**Characterization prerequisite:**

- `TestRootControlReachesHandleRegisteredByActivation`. Layer: unit. Path: `pkg/services/factory_runtime/internal/root_handle_authority_test.go`.
  - Given Root and one activation that registers a handle.
  - When Root pauses and then cancels that runtime ID.
  - Then the activation's handle observes both, and an unknown ID returns the typed not-found error.

**Witness correction.** This lane cannot prove F04 (replacement) alone. Its functional witnesses are F06 and the controls tests above; F04 belongs to T15/T17.

**Focused commands:**
- `go test ./pkg/services/factory_runtime/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/factory_runtime`, `tests/functional/sessions/controls`, `tests/functional/sessions/root_composition`

**Acceptance criteria:**

- [ ] Given an explicit running session, when it is paused, resumed and cancelled through the public API, then `TestAPIPauseResumeCancelAndTerminateFactorySession` and `TestRootControlReachesHandleRegisteredByActivation` pass, and an unknown runtime ID returns the existing typed not-found error.
- [ ] Given Four explicit sessions with owned inputs/routes/streams, when overlap Work, reads, and one cancellation, then outputs/events/log correlation and cursors remain attributable to each session.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F06 plus the public controls tests (F04 is proven by T15/T17), observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_runtime/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T15 — Runtime activation replacement and replay preserve cleanup

**Parent behavior:** F03/F04/F08; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/factory_runtime/internal/assembly.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Runtime activation replacement and replay preserve cleanup, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F03/F04/F08.

**Actor and trigger:** CLI operator/API client; given resource opening fails once, start then retry explicit session; given active session and retained response cursor, replace successfully, then fault a later replacement; given recorded history and representative legacy fixture, export/read/replay/resume through public surface.

**Dependencies:** T14, T16, T31.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/factory_runtime/internal/assembly.go` / `Assembly`, all associated inventory entries assigned to T15, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject compilation/engine/dispatch/artifacts/log/metrics/sidecar behavior. Replace Assembly/RuntimeBuild/Factory/Bundle graphs with scoped net/marking/buffers/attempts/resources and activation views. Preserve SetTick/checkpoints, atomic publication, precommit survival and failed cleanup retry.

**Amendment v1.1:** AM05/AM07/AM09: consume T16 shared supervisor after T14/T16/T31 merge. Preserve RetainWorktree/reused checkout lifetime under existing policy. Own final opaque RuntimeBinding/keyed lifecycle replacement of RuntimeRecord service/logger getters; T13 scope registration is interim, not S01 proof. T16 owns all WorkerSessionsFactory deletion; this task owns activation/replacement/replay behavior.

**Contract and configuration excerpts:**

Authored source: `pkg/services/factory_runtime/internal/assembly.go` / `Assembly`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type Assembly struct {
	runtimeFactory        *RuntimeFactory
	workerSessionsFactory factoryruntime.WorkerSessionsFactory
	workerService         workers.Service
	metricsClock          platformclock.TimerSource
}
```

Proposed:

```go
// Removed: Assembly as a dependency/construction container.
// Inject fixed behavior roles individually; retain scoped state/resources
// under the owners and operation contracts specified in contracts.md.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit:
  - `TestRuntimeRootActivationUnwindsFailedStartAndCanRetry` and `TestRuntimeRootFailedCleanupRemainsExplicitlyRetryable` (`factory_runtime/wire/runtime_activation_test.go:256,294`).
  - `host/replacement_test.go`.
  - `TestBuild_FinalizesRecordingBeforeClosingRuntimeSinksOnPartialFailure`.
- Functional:
  - F04 success: `TestFactoryResponseEventSequenceSurvivesSessionRuntimeReplacement` (`tests/functional/events/response_events/session_runtime_replace_test.go:54`).
  - F08: `TestLegacyReplayFailureInspection` and `TestComposedRecordReplayUsesRootBuildProcessAndExecute` (`tests/functional/factory/replay_contracts`).
  - Restart: `tests/functional/sessions/restart/logical_identity_test.go`.

**Characterization prerequisite (failed later replacement has no witness):**

- `TestReplaceStartFailureKeepsPriorGenerationAndRestoresSidecars`. Layer: unit. Path: `pkg/services/factory_sessions/internal/runtimebinding/replace_failure_test.go`.
  - Given a live session handle in service mode and a replacement whose `WaitForStart` or sidecar start fails.
  - When `Replace` runs.
  - Then the error wraps "start replacement runtime", the session still resolves to the prior handle, the prior sidecars restart, and nothing is retired.
  - Add a functional witness only if the lane finds a deterministic way to fault the replacement.

The F03 prerequisite (`TestRootProcessStartFailureThenRetrySucceedsWithoutLiveSession`) is defined in T13. Land it here if T13 has not.

**Coverage-loss rule.** Twelve test files build `host.Bundle{...}` literals, and `instance_host/build/service_test.go` / `service_behavior_test.go` test code this lane deletes. Port their assertions onto the activation operation; deleting them without porting is a blocker.

**Split (validation review).** The instance-host build leaves, `BundleBuilder` / `instance_host/build/service.go` and `WorkstationRequestExecutorConfig`, move to **T31**, which this lane depends on. This lane folds `RuntimeFactory.Build` and `runtime_build` into the activation operation. It also changes `Root.Activate` and its Sessions callers.

**Ownership with T16.** T16 adds the keyed attempt opener. T16 owns declaration, `provideWorkerSessionsFactoryWithRecorder`, and call-site removal together after its separately merged keyed-attempt contract PR and characterization. T15 consumes shared supervision and does not delete that factory (AM09).

**Unlisted Sessions edit.** The `Root.Activate` signature change forces an edit to `factory_sessions/internal/service/runtime_activation.go:286` (961 lines). It is in scope here.

**Also in scope (inventory additions):**
- `attachInvocationScheduleFactory`;
- the engine and executor setters (`SetMockWorkersConfig`, `SetPromptSourceReader`, `SetProgressPublisher`, `SetRuntimeLogger`, `SetRuntimeConfig`).

**Focused commands:**
- `go test ./pkg/services/factory_runtime/... ./pkg/services/factory_sessions/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/events/response_events`, `tests/functional/factory/replay_contracts`, `tests/functional/sessions/restart`, `tests/functional/runtime_api/factory_transformation`, `tests/functional/factory_runtime`
- Long F08 cases: `go test -tags=functionallong ./tests/functional/replay_contracts/...`

**Acceptance criteria:**

- [ ] Given Resource opening fails once, when start then retry explicit session, then failed start is not live; retry succeeds; owned resources released.
- [ ] Given Active session and retained response cursor, when replace successfully, then fault a later replacement, then cursor/event history survive; failed precommit replacement keeps prior generation usable.
- [ ] Given Recorded history and representative legacy fixture, when export/read/replay/resume through public surface, then same admitted facts, history, logical identity and typed legacy failure inspection.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F03/F04/F08, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_runtime/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T16 — Worker Session supervision isolates keyed attempts

**Parent behavior:** F05/F06; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/worker_sessions/wire/wire.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Worker Session supervision isolates keyed attempts, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F05/F06.

**Actor and trigger:** CLI operator/API client; given controlled provider success, denial, timeout, and cancellation, invoke Worker/direct child/Factory Work and cancel one; given four explicit sessions with owned inputs/routes/streams, overlap Work, reads, and one cancellation.

**Dependencies:** None. Within the lane, the contract PR below is the first required step and merges before any structural PR.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover. Worker-session visibility project (`docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`): this lane owns `worker_sessions` construction shape; visibility tasks that add Worker Sessions constructor dependencies wait for this lane to merge, and this lane does not wait for visibility (operator decision, 2026-10-02).

**Scope:**

- In: first, the contracts.md current/proposed Go pair for the keyed attempt request (own PR); then `pkg/services/worker_sessions/wire/wire.go` / `NewService`, all associated inventory entries assigned to T16, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Construct supervision once; remove per-runtime factory. Key cancellation/publication/retention by runtime/session/attempt identity. Recording adapter routes via explicit association. Preserve provider session links and source-native Events ownership.

**Amendment v1.1:** AM09: first merge the independently reviewed keyed-attempt Current/Proposed contract PR. Then characterize overrides, replay runner, runtime/session/recording identity, clock and retention before removing WorkerSessionsFactory declaration, focused provider and all call sites together in this lane. T15 depends on this completed cutover. This amendment does not author or approve the pending keyed-attempt request design.

**Contract and configuration excerpts:**

Authored source: `pkg/services/worker_sessions/wire/wire.go` / `NewService`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func NewService(
	execution workers.Service,
	eventsAppender EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
	providerSessions providersessions.Service,
	recording recordings.WorkerSessionRecordingService,
) (workersessions.Service, error)
```

Proposed:

```go
func NewService(
	execution workers.Service,
	eventsAppender EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
	providerSessions providersessions.Service,
	recording recordings.WorkerSessionRecordingService,
) (workersessions.Service, error)
// Construct once through canonical Wire; remove per-runtime factory.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Scope correction (verified):** the per-runtime factory is called at `factory_runtime/internal/build.go:400` with a per-runtime `workers.Service`. That service carries the session's `ProviderOverride`, `CommandRunnerOverride`, `ReplayCommandRunner`, session/runtime/recording IDs (`runtime_build.go:477-505`) and a per-runtime clock with `publisher.Bind` (`:621-631`).

Worker Sessions state is keyed by bare dispatch ID (`dispatchOwners`, `latestRuntimeDispatchIDs`, `invoke_session.go:294-310`). One shared supervisor must therefore key attempts by (runtime ID, dispatch ID). Each attempt must carry its runtime's execution handle and clock through the attempt request. The proposed `NewService` signature above is unchanged from current and does not yet show this.

**First required step (operator decision, 2026-10-02):** before any structural change, write the current/proposed Go contract for keying Worker Session attempts by (runtime ID, dispatch ID) into contracts.md (section "T16 — Worker Sessions keyed attempt supervision") and merge it in its own PR. The pair shows how each attempt request carries its runtime's execution handle, overrides and clock. If keying is not feasible, the lane stops and returns a delta plan; this is the plan §2 "scoped state that cannot be safely keyed" replanning trigger.

This lane edits `factory_runtime/internal/{build.go,runtime_build.go}` and `composition_contracts.go:84`. Coordinate with T15, which merges after this lane.

**Characterization prerequisites:**

1. `TestRuntimeAttemptsFromTwoRuntimesWithEqualDispatchIDsRemainIsolated`. Layer: unit. Path: `pkg/services/worker_sessions/internal/service/runtime_attempt_keying_test.go`.
   - Given two runtime contexts.
   - When each begins an attempt with the same `DispatchID` and one is cancelled.
   - Then there is no owner conflict, only the cancelled attempt terminates, and each publishes to its own session.
   - Write it against today's per-runtime factory first.
2. `TestTwoFactorySessionsCancelOneInFlightProviderAttemptPeerCompletes`. Layer: functional. Shared with T09; see T09 for the path and Given/When/Then.

**Also in scope (inventory additions):**
- the `WorkerSessionsObservationForSession` locator sites;
- `newSupervisionDeadlineTimer` (host-timer fallback);
- `ProviderSessionObservationPublisher.Bind`.

**Focused commands:**
- `go test ./pkg/services/worker_sessions/... ./pkg/services/factory_runtime/... ./pkg/wire`
- `go run ./cmd/functionallane -root ./tests/functional/workers/...`
- `go run ./cmd/functionallane -root ./tests/functional/events/response_events/...`
- `go run ./cmd/functionallane -root ./tests/functional/factory_runtime/root_composition/...`
- `go run ./cmd/functionallane -root ./tests/functional/provider_sessions/...`

**Acceptance criteria:**

- [ ] The (runtime ID, dispatch ID) attempt-request contract pair is merged into contracts.md in its own PR before any structural PR, and the delivered code matches it; or the lane stopped with a delta plan.
- [ ] Each distinct selected subcase F05a–d in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Controlled provider success, denial, timeout, and cancellation, when invoke Worker/direct child/Factory Work and cancel one, then same terminal classification; released worktree/attempt; peer session survives.
- [ ] Given Four explicit sessions with owned inputs/routes/streams, when overlap Work, reads, and one cancellation, then outputs/events/log correlation and cursors remain attributable to each session.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F05a–d plus  F05/F06, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/worker_sessions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T17 — Session opening activates roles without service products

**Parent behavior:** F03/F04/F06; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/factory_sessions/internal/service/factory.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Session opening activates roles without service products, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F03/F04/F06.

**Actor and trigger:** CLI operator/API client; given resource opening fails once, start then retry explicit session; given active session and retained response cursor, replace successfully, then fault a later replacement; given four explicit sessions with owned inputs/routes/streams, overlap Work, reads, and one cancellation.

**Dependencies:** T13, T15.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/factory_sessions/internal/service/factory.go` / `ProviderSessionsPorts`, all associated inventory entries assigned to T17, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Decompose normalization, definition/replay selection, durable execution, activation and observation/presentation. Remove eleven bags/factory-of-factory/runtimeProducts and SessionState inherited peer references. Opening passes IDs/model scope/replay facts/cleanup handles only.

**Amendment v1.1:** AM07/AM13: after T13/T15 merge, remove legacy Complete/opening/NewSessionRuntime/products/factories. T13 already removed BindProcessDurable and gateway replacement with the minimal bridge; do not reintroduce late binding. Gateway/durable receive independent authority, never final Root as a resolver. Replacement webhook readiness is a separately held production prerequisite; structural opening edits cannot claim F18a release.

**Contract and configuration excerpts:**

Authored source: `pkg/services/factory_sessions/internal/service/factory.go` / `ProviderSessionsPorts`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type ProviderSessionsPorts struct {
	Service providersessions.Service
}
```

Proposed:

```go
// Removed: ProviderSessionsPorts as a dependency/construction container.
// Inject fixed behavior roles individually; retain scoped state/resources
// under the owners and operation contracts specified in contracts.md.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage:**
- Everything listed for T13 and T15.
- `TestBuildProcessRoutesLiveOpenListControlAndCloseThroughFactorySessionsRoot` (`tests/functional/sessions/live_runtime_build_process_test.go:20`).
- `tests/functional/sessions/execution/fscp03_*`.
- ACP, which consumes `factorysessions.Service` directly (`pkg/wire/acp_transport.go:181`):
  - `tests/functional/sessions/chat_sessions/root_composition/acp_prompt_delegation_test.go`
  - `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go`

**Characterization prerequisites:**

1. The F03 and F06 tests defined in T13. Land them here if T13 has not.
2. `TestMockWorkersPassthroughUnmatchedReachesProviderEdge`. Layer: functional. Path: `tests/functional/workers/mock/`.
   - Given a mock config with passthrough enabled and an unmatched dispatch.
   - When `you run --with-mock-workers` runs.
   - Then the controlled `ProviderCommandRunner` is called exactly once.
   - Covers `durable_provider.go`.
3. `TestConcurrentSessionsKeepOwnDurableJavaScriptCollaborators`. Layer: functional. Path: `tests/functional/orchestration/javascript/composition/`.
   - Given two explicit JavaScript-workflow sessions in one process with distinct controlled workers.
   - When both dispatch child Work concurrently.
   - Then each child runs on its own session's worker and emits progress to its own stream.
   - Why: this pins the shared durable-runtime setters (inventory additions) before they are removed.

**Ownership received from T13:** `NewRootFromAssembly` / `BindProcessDurable`, `RuntimeModelInvokerConfig`, all durable/conductor factories (`session_runtime_providers.go`, `profiles.go:441-470`), and `historical_replay_runtime.go:177` (a consumer of `DurableExecutionFactory`). The `startFactoryWebhookSubscription` edit in `open.go:638` comes from T26; accept it.

**Focused commands:**
- `go test ./pkg/services/factory_sessions/... ./pkg/transports/acp/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/sessions`, `tests/functional/events/response_events`, `tests/functional/transport/acp`, `tests/functional/orchestration/javascript`, `tests/functional/workers/mock`

**Size:** about 3.3k LOC. If one focused review pass cannot cover it, escalate with a split: (a) the mechanical change from Ports bags to parameters; (b) the `open.go` / `runtimeProducts` decomposition. The project lead MAY also pre-split along these lines at admission (operator decision, 2026-10-02); otherwise split only if review requires it.

**Acceptance criteria:**

- [ ] Given Resource opening fails once, when start then retry explicit session, then failed start is not live; retry succeeds; owned resources released.
- [ ] Given Active session and retained response cursor, when replace successfully, then fault a later replacement, then cursor/event history survive; failed precommit replacement keeps prior generation usable.
- [ ] Given Four explicit sessions with owned inputs/routes/streams, when overlap Work, reads, and one cancellation, then outputs/events/log correlation and cursors remain attributable to each session.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F03/F04/F06, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_sessions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T18 — HTTP MCP and visualization use direct owner contracts

**Parent behavior:** F13; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/wire/http_runtime_binding.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** HTTP MCP and visualization use direct owner contracts, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F13.

**Actor and trigger:** CLI operator/API client; given explicit live session or expired/missing target, retained/live sse events and gap, hTTP reads/actions/SSE, MCP/ACP invocation.

**Dependencies:** None. (Sessions-getter removal split to T28, which depends on T17; dead MCP wrappers moved to T24.)

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover. Worker-session visibility project (`docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`): this lane (with T28) owns Worker Sessions HTTP handler wiring; visibility tasks that add handler wiring wait for T18 and T28 to merge, and this lane does not wait for visibility (operator decision, 2026-10-02).

**Scope:**

- In: `pkg/wire/http_runtime_binding.go` / `httpRuntimeBinding`, all associated inventory entries assigned to T18, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Construct adapters/tool dispatch/projection once; remove transport service bags and root getter lookup. Binder takes session/host selection and cancellation only. Preserve request mapping/missing/gone/gap, retained/live events, ACP ownership and close behavior.

**Amendment v1.1:** AM12: retain `fi-t18-direct-transport-adapters-20261002` and its uncommitted `tests/functional/transport/mcp/stdio/start_sync_tool_test.go` witness. start_sync completes, but read_events for its returned durable session yields `factory_session.session.not_found`, including isolated rerun. FI-PREREQ-MCP-DISCOVERY is owned by FI Sessions/Recordings/MCP, shared with WSV as consumer; diagnose identity/recording lookup and deliver an independently reviewed behavior-preserving correction. Any policy/public-contract correction requires operator decision. Adapter development is independent; integrated M01/F13 requires both completion and ordered same-session events after prerequisite merge, never a mock substitute. Whole-lane admission must separate development from gated integrated acceptance.

**Contract and configuration excerpts:**

Authored source: `pkg/wire/http_runtime_binding.go` / `httpRuntimeBinding`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type httpRuntimeBinding func(*factorysessionwire.Root, string, initializer.InvocationCancellation) (http.Handler, error)
```

Proposed:

```go
type httpRuntimeBinding func(string, initializer.InvocationCancellation) (http.Handler, error)
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Split (validation review).** This packet is now **adapter-only and has no dependency**. Sessions-getter removal (the `httpRuntimeBinding` signature, `SessionPresentation` shrink, `run_session_runtime.go:300`, `sessionInspectionForHTTP`) moved to **T28**, which depends on T17. The five MCP `RootDependencies` wrappers with no production importer (factory_runtime, recordings, work, providers, factory_visualization; production uses `factorysessionmcp.BindToolOperation` at `profiles.go:617`) moved to **T24** for deletion.

In scope here:
- the handler `Dependencies` bags;
- `mcpserver.Options`;
- the MCP builder closure in `profiles.go`;
- the Factory Visualization composite `NewRoot` and stub recordings peer;
- `newHTTPWorkerSessionsHandler` / `workerSessionObservationSources` (`http_runtime_binding.go:257-326`), which this lane changes to accept injected adapters while the binding signature is still unchanged;
- `BindSessionObserver`;
- the visualization `time.After`.

The "Current/Proposed" `httpRuntimeBinding` excerpt below is T28's target. This lane must not change that signature.

**Existing coverage to keep green:**
- SSE typed errors: `TestAPIResponseEventSessionExpiryReturnsTypedGone`, `TestAPIResponseEventCursorGapEmitsStreamGap`, `TestAPIResponseEventSSEStreamsRetainedThenLiveEvents`, `TestAPIResponseEventStreamClosesAtDocumentedBoundary` (`tests/functional/events/response_events/stream_test.go`).
- `TestAPIFactorySessionNotFoundUsesTypedError`.
- Visualization: `tests/functional/factory_visualization/*`. These set `edges.FactoryVisualizationSink`, so sink selection is not covered.
- MCP: `TestMCPMissingFactorySessionReturnsCanonicalNotFound`, `TestMCPResumePackage_PublicCatalogAndSessionReadUseFlattenedRuntime`, and `tests/functional/transport/mcp/stdio/discovery_test.go`.
- ACP is not touched by this lane.

**Characterization prerequisite:**

- `TestMCPStartSyncRunsFactorySessionThroughComposedProcess`. Layer: functional. Path: `tests/functional/transport/mcp/stdio/start_sync_tool_test.go`.
  - Given a scaffolded factory and a controlled `ProviderCommandRunner` passed through `support.BuildProcessWithContext`.
  - When `you server mcp` receives `start_sync` and then `read_events` for that session.
  - Then the result is COMPLETED with the provider output, the events carry that session ID, and closing stdin exits cleanly.
  - Why: the only `start` tool test (`pkg/transports/cli/mcp/serve_smoke_test.go:45`) builds the server by hand.

**Focused commands:**
- `go test ./pkg/services/factory_sessions/transports/... ./pkg/services/factory_definitions/transports/http/... ./pkg/transports/http/... ./pkg/transports/mcp/... ./pkg/services/factory_visualization/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/events/response_events`, `tests/functional/transport/mcp`, `tests/functional/transport/http`, `tests/functional/factory_visualization`, `tests/functional/workers/transports/http`, `tests/functional/sessions/lifecycle`

**Acceptance criteria:**

- [ ] Given Explicit live session or expired/missing target, retained/live SSE events and gap, when hTTP reads/actions/SSE, MCP/ACP invocation, then existing request/result/error, typed gone/gap, ordering and close boundaries.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F13, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/factory_sessions/transports/... ./pkg/transports/http/... ./pkg/transports/mcp/... ./pkg/services/factory_visualization/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T19 — CLI placement and framing use direct roles

**Parent behavior:** F14; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/transports/cli/commandregistry/representative_handlers.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** CLI placement and framing use direct roles, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F14.

**Actor and trigger:** CLI operator/API client; given concurrent quiet, normal, verbose/debug invocations, run/read through CLI with controlled logs.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/transports/cli/commandregistry/representative_handlers.go` / `SessionResolvedServices`, all associated inventory entries assigned to T19, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Replace SessionResolvedServices and FactoryConfigInitServices with directly injected handler operations and local/remote placement selection. Keep target/request data, inject timing/output roles, preserve stdout/stderr/NDJSON and concurrent verbosity policies.

**Contract and configuration excerpts:**

Authored source: `pkg/transports/cli/commandregistry/representative_handlers.go` / `SessionResolvedServices`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
type SessionResolvedServices struct {
	LocalSessions  sessioncli.Service
	RemoteSessions sessioncli.Service
	PrepareList    func(context.Context, *sessioncli.ListConfig) error
	Diagnostics    func(*cobra.Command) io.Writer
}
```

Proposed:

```go
// Removed: SessionResolvedServices service bag.
func BindSessionResolvedHandlers(
 local sessioncli.Service,
 remote sessioncli.Service,
 prepareList func(context.Context, *sessioncli.ListConfig) error,
 diagnostics func(*cobra.Command) io.Writer,
) SessionResolvedHandlers
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- `TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition` (`tests/functional/sessions/cli/resolved_session_family_test.go:48`).
- `tests/functional/factory_definitions/transports/cli/*`.
- `tests/functional/transport/cli/commands/factory_wiring_test.go`.
- `tests/functional/product/init_setup`.
- Output framing: `tests/functional/transport/cli/output/{ndjson_stream,json_result,text_stream}_test.go`.

This lane does not change logger policy. The F14 concurrency witness belongs to T02, whose prerequisite is `TestConcurrentQuietAndVerboseInvocationsKeepOwnFraming`.

**Also in scope (inventory additions):** the CLI effect and wait fallbacks in `work/transports/cli/watch_stream.go:450,481` and `worker_sessions/transports/cli/{invoke,interrupt,continue}.go`.

**Focused commands:**
- `go test ./pkg/transports/cli/... ./pkg/services/work/transports/cli/... ./pkg/services/worker_sessions/transports/cli/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/transport/cli`, `tests/functional/sessions/cli`, `tests/functional/factory_definitions/transports/cli`, `tests/functional/product/init_setup`

**Acceptance criteria:**

- [ ] Given Concurrent quiet, normal, verbose/debug invocations, when run/read through CLI with controlled logs, then correct stdout/stderr/NDJSON framing; one invocation cannot change peer output policy.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F14, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/transports/cli/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then `make test-functional` before opening the PR. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR `make lint` and appropriate `make verify-fast`/`make verify-pr`. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T20 — Existing lint rejects construction graphs and fallbacks

**Parent behavior:** S01; plan sections 4/10.

**Problem:** The existing AST checkers cannot detect secondary construction, dependency bags, getter locators or dependency fallbacks, so migrated owners can silently regress.

**Outcome:** Existing lint rejects construction graphs and fallbacks, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; S01.

**Actor and trigger:** Operator/client runs the public case S01; shared enablers establish a construction/checking capability used across several journeys and therefore cannot be owned by one service behavior slice.

**Dependencies:** None. (Final repository-scope enforcement split to T29.)

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `cmd/pkgboundarycheck/service_scanners.go` / `lint rules`, all associated inventory entries assigned to T20, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Implement lint.md qualified constructor/type registry and strict parameter-to-field mapping in existing AST checkers. Add fixtures first, then enforce migrated named capability sets. No broad baseline or file exceptions; unresolved alias classification is explicit coverage debt. Move topology assertions out of functional tests.

**Contract and configuration excerpts:** No runtime interface/config delta. T20 changes checker semantics/fixtures in lint.md; T22 changes evidence procedures. No generated public consumers.

**Validation-review coverage and scope (2026-10-02):**

**Split (validation review).** This packet covers checker infrastructure, fixtures, report mode, and enabling each named capability set as soon as its owner lane has merged with zero findings. Removing the final repository-scope restriction and the allowances moved to **T29**, which depends on every owner lane and on T24.

`pkg/wire/wire.go` integration is not owned by this lane. The composition steward handles it inside each lane PR (plan §9); inventory row 98 is corrected.

**Main-safety rule.** Do not enable a capability set while an open lane PR is still modifying it. Every enablement PR follows the program merge-safety rule: rebase onto current origin/main and rerun `make lint pkg-file-count` (plus `make generate-wire` and a diff check when Wire changed) on the rebased head immediately before merge. A per-site ratchet that passes on two separate PRs can still break main after both merge.

**Acceptance criteria:**

- [ ] Given the lint.md §5 fixture matrix, when `go test ./cmd/pkgboundarycheck ./cmd/durableruntimeconstructioncheck ./cmd/loggingboundarycheck ./cmd/pkgmaintcheck ./cmd/servicecyclecheck` runs, then every prohibited-pattern fixture (secondary constructor call in an operation, dependency bag, service getter used as locator, recursive provider, nil-to-default substitution, redundant requiredness guard, hidden clock/logger default) is reported with its qualified symbol, and every listed exclusion fixture (scoped state allocation, legitimate derived logger, replay clock, domain validation) is not reported.
- [ ] Given a capability set whose owner lane has merged with zero findings, when the set is enabled and `make lint` runs on the rebased head, then it passes; adding one prohibited fixture-shaped line to that owner package makes it fail with a symbol/path-only diagnostic.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: S01, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./cmd/pkgboundarycheck ./cmd/loggingboundarycheck ./cmd/durableruntimeconstructioncheck ./cmd/servicecyclecheck ./cmd/pkgmaintcheck`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T21 — Artifact chat and protocol facts honor selected process effects

**Parent behavior:** F15/F16; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/wire/runtime_inputs.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Artifact chat and protocol facts honor selected process effects, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F15/F16.

**Actor and trigger:** CLI operator/API client; given selected controllable process source; no specialized override, drive admission/artifact/recording/chat timestamps and scheduling; given explicit specialized legacy clock and replay clock, execute their owning operations.

**Dependencies:** T01, T02.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/wire/runtime_inputs.go` / `provideRuntimeArtifactClock`, all associated inventory entries assigned to T21, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Adopt normalized effects in Chat Sessions/artifacts/recording planner/CLI timing/sync waits/staging/metrics and explicit PTY/subprocess defaults. Owner lanes handle their service providers; coordinate named functions, preserve specialized override precedence and replay separation.

**Contract and configuration excerpts:**

Authored source: `pkg/wire/runtime_inputs.go` / `provideRuntimeArtifactClock`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func provideRuntimeArtifactClock() runtimeArtifactClock             { return time.Now }
```

Proposed:

```go
func provideRuntimeArtifactClock(source platformclock.Source) runtimeArtifactClock {
 return source.Now
}
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage.** Only the "specialized override wins" cases are covered:
- `FactoryWebhookClock` (`tests/functional/events/factory_events/webhooks_test.go:112`)
- `HostedClock` (`tests/functional/workstations/poller/hosted_characterization_test.go:200`)
- `TestProvideFactoryDefinitionClock...`
- `AgyPTYClock`
- `TestCLIHTTPProfilesPreserveCommandTimeouts` (`pkg/wire/cli_http_test.go:19`)

No test pins the default sites to `Edges.Clock`.

**Characterization prerequisite (new F15 witness):**

- `TestSelectedProcessClockStampsChatTurnsAndRuntimeArtifacts`. Layer: functional. Path: `tests/functional/sessions/chat_sessions/root_composition/`.
  - Given a fixed `Edges.Clock` and no specialized override.
  - When an ACP prompt runs.
  - Then the chat turn timestamps and the runtime-log/transcript timestamps equal the fixed time.
  - Write it first with the expected default-path values recorded as current behavior (host time), then flip the assertion in the cutover commit.

**Waiter-count audit.** Moving sync waits (`session_runtime_providers.go:804`) or metrics timers onto a fake clock changes `BlockUntilContext` counts in the tests listed in T01. Update each count explicitly.

**Scope narrowed (validation review).** This lane owns only the `pkg/wire` default-adoption rows.

The service-internal rows move to their owner lanes:
- `automations/internal/service.go` `supervisorClock` → T12;
- `providers/.../execution/wire/wire.go` built-in effects → T08.

This lane removes the `pkg/wire/hosted_sources.go:78` webhook default; T26 removes the owner default.

**Focused commands:**
- `go test ./pkg/wire ./pkg/services/chat_sessions/...`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/sessions/chat_sessions`, `tests/functional/transport/acp/stdio`, `tests/functional/recordings`, `tests/functional/events/factory_events`, `tests/functional/workstations`, `tests/functional/work/submission`, `tests/functional/factory/packaged/loop`, `tests/functional/models/root_composition`

**Acceptance criteria:**

- [ ] Given Selected controllable process source; no specialized override, when drive admission/artifact/recording/chat timestamps and scheduling, then public/edge-observed facts follow selected source and scheduling progression.
- [ ] Given Explicit specialized legacy clock and replay clock, when execute their owning operations, then specialized override wins; replay facts follow ticks; OS cleanup remains governed by scheduler.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F15/F16, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/wire ./pkg/services/chat_sessions/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T22 — Integrated lifecycle evidence meets declared bounds

**Parent behavior:** P01/I01; plan sections 4/10.

**Problem:** Per-lane evidence does not show whether the integrated flattening regressed lifecycle latency, leaked handles across repeated sessions, or broke delivered-CLI shutdown/flush.

**Outcome:** Integrated lifecycle evidence meets declared bounds, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; P01/I01.

**Actor and trigger:** Operator/client runs the public case P01/I01; shared enablers establish a construction/checking capability used across several journeys and therefore cannot be owned by one service behavior slice.

**Dependencies:** T27 (baseline harness), and completed T01–T19, T21, T23–T26, T28, T30, T31.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `docs/internal/development/plans/flat-injection/plan.md` / `evidence procedure`, all associated inventory entries assigned to T22, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Compare 10 valid baseline/final construction and start/stop samples with median/p95/range. Run dedicated 100-cycle retention stress with 10-minute limit and prebuilt CLI shutdown/flush smoke. Report BLOCKED if baseline cannot be reproduced; no invented performance result.

**Amendment v1.1:** AM10: final P01 comparison consumes T27 exact pinned baseline and the independently reviewed FI-PREREQ-BASELINE-OBS runbook. Count the same private owners/checkpoints with matching methodology; no public session-count substitution, changed pin or invented measurement.

**Contract and configuration excerpts:** No runtime interface/config delta. T20 changes checker semantics/fixtures in lint.md; T22 changes evidence procedures. No generated public consumers.

**Validation-review coverage and scope (2026-10-02):**

**Split (validation review).** The baseline is captured by **T27** at the start of the project, not by this lane. This lane runs the final P01 comparison against T27's recorded baseline. It re-measures the pinned base commit on the same host when the environment differs. It also runs I01.

**P01 command:** `go test ./tests/stress -run TestLifecycleProfile -count=1 -timeout 10m`, using the harness T27 adds. `make test-stress` passes `-short` and skips the profile.

**I01 procedure.**
1. `make test-release` builds the prebuilt artifact and exports `INFINITE_YOU_RELEASE_PREBUILT_PATH`.
2. Add a subtest to `TestRootProcessCompiledBinaryModeMatrix` (`tests/release/root_process_smoke_test.go:39`) that does `run --with-server --record <path>`, interrupts, and asserts that the recording parses and contains the run's events. The existing cases use `--no-record`, so they prove no flush.
3. Do not use `tests/integration/transport/server_binding`, because it builds its own binary.

**Acceptance criteria:**

- [ ] Given T27's recorded baseline (10 inert-construction and 10 explicit-session start/stop samples at the pinned base commit), when the same harness runs 10 samples on the final integrated head on the same host, then the report lists median, p95 and full range for both, and the final median and p95 are each no more than 10% above baseline. Otherwise the report says FAIL with the measured numbers; it never omits slow valid samples.
- [ ] Given 100 sequential start/replace/stop cycles, when the stress profile completes within 10 minutes, then goroutine count and owned handle/lease counts return to the pre-loop value, within the harness's documented tolerance.
- [ ] Given the prebuilt artifact from `make test-release`, when `run --with-server --record <path>` is interrupted, then the process exits gracefully and the recording parses and contains the run's events.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: P01/I01, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/wire`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T23 — Caller construction validates once and constructors assume real effects

**Parent behavior:** U03/F01; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/root/process.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Caller construction validates once and constructors assume real effects, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; U03/F01.

**Actor and trigger:** CLI operator/API client; given controlled effect observers and valid process input, buildProcess constructs the process.

**Dependencies:** None; coordinate additive seams with T01/T02.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/root/process.go` / `validateConstructionOverrides`, all associated inventory entries assigned to T23, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Preflight present caller overrides/required capabilities before constructor execution: typed-nil interfaces and nil functions are invalid while omitted optional Edges remain valid. Canonical providers choose missing optional defaults once. Each lane removes its own fallback/guards/variadics; this lane owns boundary policy and preflight only.

**Contract and configuration excerpts:**

Authored source: `pkg/root/process.go` / `validateConstructionOverrides`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
// validateConstructionOverrides is not present.
```

Proposed:

```go
// Private public-boundary helper called before Wire execution.
func validateConstructionOverrides(edges serviceedges.Edges) error
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Scope narrowed (validation review).** This lane owns only:
- boundary normalization and preflight in `pkg/root`;
- centralizing typed-nil validation (the Models `isNilDependency` pattern);
- System Initialization guards;
- the ACP `pkg/wire/acp_transport.go` logger, variadic and environment-read rows;
- the explicit disabled-implementation policy.

Per-owner fallback rows that inventory previously listed under T23 move to their owner lanes, because those lanes rewrite the same functions:
- Providers → T08
- Models `wire.go` → T10
- Models `runtime_factory.go` / inference → T11
- Automations → T12
- Recordings → T03

**Existing coverage:**
- `TestModelsCompositionRejectsTypedNilHostEdges` (`pkg/wire/model_invocation_edges_test.go:271`)
- `TestNewRootRequiresCursorPersistenceEffect` (`automations/wire/wire_test.go`)

`pkg/root` has only `TestMain`.

**Characterization prerequisites:**

1. `TestBuildProcessRejectsTypedNilEdgeWithoutInvokingCollaborators`. Layer: unit. Path: `pkg/root/root_test.go`.
   - Given each `Edges` override in turn set to a typed-nil value.
   - When `BuildProcess` runs.
   - Then it returns the actionable validation error, and no controlled collaborator observes a call.
2. `TestProviderAttemptLogsRespectQuietPolicy`. Layer: functional. Path: `tests/functional/providers/`.
   - Given a controlled provider command and `--quiet`.
   - When Work runs.
   - Then stdout and stderr match the pre-change baseline.
   - Why: production never passes `WithLogger` (`pkg/wire/session_runtime_providers.go:169-213`), so Providers logs go to a noop logger today. Once Wire supplies the real logger, attempt `Info` logs reach the process logger. Hand this test to T08 if T08 starts first.
3. `TestNewRootWithoutFactoryDirKeepsCursorInMemory`. Layer: unit. Path: `pkg/services/automations/wire/wire_test.go`.
   - Given `defaultFactoryDir=""` and a recording cursor filesystem.
   - When a cursor is set and read.
   - Then it round-trips with zero filesystem writes.
   - This is the intentional memory branch. T12 owns the surrounding code.

**Focused commands:**
- `go test ./pkg/root ./pkg/services/edges ./pkg/services/system_initialization/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/providers`, `tests/functional/product/init_setup`, `tests/functional/transport/acp`

**Acceptance criteria:**

- [ ] Given Controlled effect observers and valid process input, when buildProcess constructs the process, then no runtime/sidecar/provider execution occurs before selected lifecycle.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: U03/F01, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/root ./pkg/services/edges ./pkg/wire`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T24 — Platform helpers and unused compatibility graphs retire explicitly

**Parent behavior:** U03/S01; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/platform/logging/logger.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Platform helpers and unused compatibility graphs retire explicitly, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; U03/S01.

**Actor and trigger:** Operator/client runs the public case U03/S01; shared enablers establish a construction/checking capability used across several journeys and therefore cannot be owned by one service behavior slice.

**Dependencies:** T03, T08, T09, T10, T11, T12, T23, T26 (their `EnsureLogger`/`clock.Ensure`/compatibility callers must be migrated first; a factory DEPENDS_ON gates the whole lane, so this is explicit). This lane migrates the remaining unowned `EnsureLogger` sites (Events, ACP stdio, Runtime subsystems) itself.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/platform/logging/logger.go` / `EnsureLogger`, all associated inventory entries assigned to T24, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Remove EnsureLogger/clock.Ensure selectors after owned callers use direct effects. Retain explicit NoopLogger and scoped resources. Retire unused global/args ...any/variadic compatibility graphs after reference checks; real external consumers get a documented compatibility boundary. managedchild process acquisition/context normalization is legitimate leaf/resource behavior.

**Contract and configuration excerpts:**

Authored source: `pkg/platform/logging/logger.go` / `EnsureLogger`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func EnsureLogger(l Logger) Logger {
	if l == nil {
		return NoopLogger{}
	}
	return l
}
```

Proposed:

```go
// Removed: EnsureLogger; retain explicit NoopLogger selected in Wire.
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Caller-check results (verified):**
- `AutomationsRootFromEdges`: only `TestAutomationsRootFromEdgesComposesPublishedRoot` uses it.
- `providers/wire` `newRoot(args ...any)`: only `providers/wire/wire_test.go:597` uses it.
- `clock.Ensure`: test-only callers.
- `EnsureLogger`: about 30 production call sites.

**Moved here from T18:** delete the five MCP `RootDependencies` wrappers that have no production importer (`factory_runtime`, `recordings`, `work`, `providers`, `factory_visualization` `transports/mcp`). No functional work is needed.

**Additional inventory rows** (see the validation-review additions): the recordings `NewServiceWithProjection*` noop-logger constructors, `supervised_subprocess.go` `Real{}`, `provideWorkerSessionsFactory`, `RuntimeLedgerFactory`, the `os.TempDir` fallback, the platform leaf-timer classification, and the `pkg/initializer` caller-checks.

**Required audit before deleting `EnsureLogger`.** For each production site, record whether production passes nil today. Where it does (as Providers does), switching from noop to the process logger is a visible output change. Each such site needs a quiet-policy characterization like T23 prerequisite 2 before removal.

**Dependencies note:** deleting `EnsureLogger` and `clock.Ensure` waits until T03, T08, T10–T12 and T23 have migrated their call sites. Land the platform-leaf classification and the dead-wrapper deletions first.

**Focused commands:**
- `go test ./pkg/platform/... ./pkg/services/providers/wire ./pkg/services/operator_settings/... ./pkg/wire`
- `go run ./cmd/functionallane -root <dir>/...` for each of `tests/functional/automations`, `tests/functional/providers`, `tests/functional/operator_settings`

**Acceptance criteria:**

- [ ] Given valid public inputs, when U03/S01 runs, then the plan matrix outcome is preserved.
- [ ] Given the named failure/cancellation/isolation case, when it runs, then the existing typed outcome, resource cleanup and peer-session survival are preserved.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: U03/S01, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/platform/clock ./pkg/platform/logging ./pkg/services/operator_settings/... ./pkg/services/providers/wire`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T25 — Costs reports preserve scoped valuation and diagnostics

**Parent behavior:** F17; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/costs/internal/service/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Costs reports preserve scoped valuation and diagnostics, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F17.

**Actor and trigger:** CLI operator/API client; given controlled prices, settings and canonical usage; unavailable price/metrics edge, query Costs through CLI/HTTP.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/costs/internal/service/service.go` / `New`, all associated inventory entries assigned to T25, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject valid pricing, Settings, canonical metrics and logger directly; remove internal constructor/query dependency requiredness checks. Keep request.Validate, cancellation, price policy and query-timeout configuration. The HTTP adapter adopts valid direct query/logger parameters without nil fallback.

**Amendment v1.1:** AM14: preserve #2676 at `26cdf0749d99219b281e56afbb2fd22b2e0cb364` and its owned rows. Implementation supplies author evidence and handoff only. Independent VAL25/review owns M01/M06/M07/M08/L25-7, exact-head L25-6, real Linux/artifact proof, terminal applicable CI and merge. Keep all obligations; author cannot mark its story incomplete solely to await an independent validator report.

**Contract and configuration excerpts:**

Authored source: `pkg/services/costs/internal/service/service.go` / `New`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func New(
	pricing costs.PriceTableReader,
	settings operatorsettings.Service,
	metrics factoryvisualization.RuntimeMetricsQuery,
	logger logging.Logger,
) (costs.CostsQuery, error) {
	switch {
	case pricing == nil:
		return nil, errors.New("construct Costs query: price-table reader is required")
	case settings == nil:
		return nil, errors.New("construct Costs query: Operator Settings reader is required")
	case metrics == nil:
		return nil, errors.New("construct Costs query: runtime metrics query is required")
	}
	service := &Service{
		pricing:  pricing,
		settings: settings,
		metrics:  metrics,
		logger:   logging.EnsureLogger(logger),
	}
	return service.QueryCosts, nil
}
```

Proposed:

```go
func New(
	pricing costs.PriceTableReader,
	settings operatorsettings.Service,
	metrics factoryvisualization.RuntimeMetricsQuery,
	logger logging.Logger,
) (costs.CostsQuery, error) {
	service := &Service{
		pricing:  pricing,
		settings: settings,
		metrics:  metrics,
		logger:   logger,
	}
	return service.QueryCosts, nil
}
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage to keep green:**
- Unit: `TestQueryErrorsAreTypedAndLogsSafeTerminalOutcome`, `TestQueryRejectsInvalidOrUnreadableOperatorPriceTableBeforeMetrics`, `TestQueryCancellationDoesNotReadDependencies`, `TestNewAdapterRejectsMissingQuery`, and `TestHandlerMapsCanceledCostsQueryToRequestTimeout`.
- CLI: `TestCostsCommand*`.
- Functional: `TestRuntimeCostsEndToEndFromProviderCompletion` and `TestRuntimeCostsNoUsageThroughPublicCLI` (`tests/functional/factory/visualization/runtime_metrics/end_to_end_costs_test.go`), and `TestReplayPricedUsageReachesPublicCosts`.

No characterization gap was found.

**Focused commands:**
- `go test ./pkg/services/costs/...`
- `go run ./cmd/functionallane -root ./tests/functional/factory/visualization/runtime_metrics/...`

**Acceptance criteria:**

- [ ] Given Controlled prices, Settings and canonical usage; unavailable price/metrics edge, when query Costs through CLI/HTTP, then same deterministic scoped report or typed failure; redacted attributed diagnostics.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: F17, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/costs/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.
## T26 — Webhook subscriptions use injected effects and isolate delivery

**Parent behavior:** F18; plan sections 4/10.

**Problem:** Existing construction/default-selection paths in `pkg/services/webhooks/internal/service/service.go` obscure direct dependencies or lack integrated evidence.

**Outcome:** Webhook subscriptions use injected effects and isolate delivery, with the scope and witness below.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F18.

**Actor and trigger:** CLI operator/API client; given two explicit recorded sessions and controlled webhook success/retry/outage/secret/store failure, activate subscriptions, admit Work, stop one.

**Dependencies:** None.

**Parallel and shared-surface ownership:** All unrelated lanes run concurrently. This lane owns named provider functions and owner code. Composition steward integrates/regenerates this lane delta in its PR. T01 owns additive time fields; T02 coordinates logger fields with T01; T20 owns checker rules. Dependent lanes develop against proposed contracts before cutover.

**Scope:**

- In: `pkg/services/webhooks/internal/service/service.go` / `NewWithDeadLetterAppender`, all associated inventory entries assigned to T26, focused providers, owned tests and removal.
- Out: public schema/format/config/policy changes, unrelated owners, UI, paid endpoints.

**Implementation constraints:** Preserve event-first ownership, Go internal access, root peer contracts and isolated scoped state. Owner wire exposes focused aliases/providers; no new service locator or broad bag. Internal constructors take real required dependencies and neither validate nil repeatedly nor substitute defaults; retain domain validation/resource failure handling.

**Concrete decomposition:** Inject canonical Recordings once and pass only scope/cursor/source/definitions/path in StartRequest; remove request.Events lookup and ctor nil/logger fallback. Inject required secret resolver/dead-letter effect; compatibility disabling is explicit at outer boundary. Preserve signatures, retry policy, redaction, per-session subscription resources and dead-letter errors.

**Amendment v1.1:** AM13: separate characterization, production prerequisite and future cutover. Retain `fi-t26-webhook-delivery-20261002` at `b82919b6133c4903a6ff30db9d5ad0b11904bd9e` and `fi-t26-scoped-activation-characterization-20261003` at `720f3d19cbb4fda41b287cf052c5fb3e0a7f94f9`; archived witness is `docs/temp/projects/flat-injection/reconciliation/wake-t26-activation-20261003/reproduction.json`. FI-PREREQ-WEBHOOK-READINESS belongs to FI Sessions/Runtime/Webhooks, with production authority held pending operator. Replacement has no readiness on current production; amendment merge grants no behavioral/policy change. F18a-f remain unchanged; explicit authorization and prerequisite merge precede replacement cutover.

**Contract and configuration excerpts:**

Authored source: `pkg/services/webhooks/internal/service/service.go` / `NewWithDeadLetterAppender`. Primary native delta below; core operation/activation/scope pairs are in contracts.md. Public config/REST/CLI/events remain unchanged.

Current:

```go
func NewWithDeadLetterAppender(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	}
```

Proposed:

```go
func NewWithDeadLetterAppender(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	}
```

Generated outputs and consumers: owning fixtures/provider aliases and canonical `pkg/wire/wire_gen.go` via `make generate-wire`. No public clients change.

Additional authored contract: `pkg/services/webhooks/contracts.go`.

Current:

```go
type StartRequest struct {
	Definitions      []factorydefinitions.FactoryWebhookConfig
	Events           recordings.Service
	Scope            recordings.CanonicalEventScope
	ActivationCursor *recordings.CanonicalEventCursor
	RuntimeSource    factorydefinitions.LoadedFactorySource
	DeadLetterPath   string
}
```

Proposed:

```go
type StartRequest struct {
	Definitions      []factorydefinitions.FactoryWebhookConfig
	Scope            recordings.CanonicalEventScope
	ActivationCursor *recordings.CanonicalEventCursor
	RuntimeSource    factorydefinitions.LoadedFactorySource
	DeadLetterPath   string
}
```

Add `events recordings.Service` to the internal Service receiver fields and owner wire constructor; no change to serialized webhook definitions/events. Existing peer Services are unchanged except the internal Go activation request removes service injection.

**Validation-review coverage and scope (2026-10-02):**

**Existing coverage:**
- Unit: 15 tests in `pkg/services/webhooks/internal/service`, covering signing, retry/Retry-After, exhaustion to dead letter, non-retryable, cancellation, overflow reconnect, and the secret resolver error.
- Functional: `TestFactoryWebhooksRunThroughRootProcess` (`tests/functional/events/factory_events/webhooks_test.go:41`), which uses one session.

**Characterization prerequisites (unit, `pkg/services/webhooks/internal/service/service_test.go`):**

1. `TestServiceSkipsEventsAtOrBeforeActivationCursor`
   - Given `ActivationCursor` at sequence 7.
   - When events 7 and 8 publish.
   - Then only 8 is delivered; an event from a different stream generation is delivered.
2. `TestServiceDeadLetterAppendFailureLogsWithoutRetryStorm` (F18e)
   - Given the appender returns an error.
   - When retries exhaust.
   - Then one append-failed diagnostic is written, there is no extra delivery attempt, and the peer endpoint still delivers.
3. `TestServiceClosingOneSubscriptionLeavesPeerDelivering` (F18f)
   - Given two `Start` calls on different scopes.
   - When the first subscription closes.
   - Then it returns after its worker joins, it sends nothing further, and the peer delivers the next event.

**Also in scope (inventory additions):**
- `NewWithDeadLetterAppender` returns nil (`service.go:62`);
- the owner clock default at `webhooks/wire/wire.go:28-29`.

The `StartRequest` caller `startFactoryWebhookSubscription` (`factory_sessions/internal/service/open.go:638`) is in T17's file. Change it in this lane with a minimal edit, and notify T17.

**Focused commands:**
- `go test ./pkg/services/webhooks/...`
- `go run ./cmd/functionallane -root ./tests/functional/events/factory_events/...`

**Acceptance criteria:**

- [ ] Each distinct selected subcase F18a–f in plan section 10 has its own observed result/state witness, not one bundled assertion.
- [ ] Given Two explicit recorded sessions and controlled webhook success/retry/outage/secret/store failure, when activate subscriptions, admit Work, stop one, then correct event signatures/delivery/retry/dead letters; peer delivery survives; no pre-activation history.
- [ ] Static/generation gates prove direct construction/effects in this lane and every assigned inventory entry has a terminal disposition.
- [ ] Valid internal construction requires no implicit fallback; boundary nil/typed-nil classification is preserved.

**Verification:**

- Behavioral witness: distinct selected subcases F18a–f plus  F18, observed public result/events/replay or supported effect calls. For S01 this is static evidence, not functional behavior.
- Executable-spine effect: preserve; shared effect/checker enablers extend fidelity.
- Required evidence: unit, controlled; `go test ./pkg/services/webhooks/...`. Proves owned transitions/validation/cleanup or checker fixtures; does not prove whole composition.
- Required evidence: functional, controlled; the focused `go run ./cmd/functionallane -root <dir>/...` runs named under "Validation-review coverage and scope" (the lane accepts `-root`, `-short`, `-count`, `-jobs`, `-timeout`; there is no `-run`), then required CI runs the broad functional gate after push/open PR; do not use `make test-functional`, `make test-full` or `make verify-pr` locally as a push gate on the shared host. Cases guarded by `SkipLongFunctional` need `-short=false`; `functionallong`-tagged cases need `make test-functional-long`. Proves named public behavior; does not prove OS signals or remote availability. T20 instead uses checker/static evidence; T22 instead runs dedicated P01/I01 procedures from plan section 7.
- Static/generation: `make pkg-boundary logging-boundary-check durable-runtime-construction-check service-cycle-check pkg-maint pkg-file-count`. `pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and owner packages are limited to 15 files. Add providers to existing files or to owner `wire` packages; a deleted file lowers its baseline entry in the same PR, and the lane must rebase and re-run immediately before merge because concurrent lanes edit the same JSON; changed graph also `make wire-smoke`; PR CI runs `make lint` and appropriate `make verify-fast`/`make verify-pr`; narrow changed-package tests/lint precede early push. Proves actual source/generation properties only.
- Highest feasible: controlled composed-process functional for service lanes; AST fixture/repository scan for T20; local real prebuilt integration and dedicated stress for T22.
- Unproven edges: integrated journey/ordering -> VAL01; OS signal/pipe/flush -> I01; performance/retention -> P01; paid availability outside scope.
- Test-layer design: results/events/effects as observers, shared inert BuildProcess where safe, explicit owned sessions before opening, isolated directories/routes/streams/fakes, Process.Execute by default. No built binary in functional tests, sleeps/global invocation locks or constructor counts. Unit input permutations stay local; I01 uses artifact supplied by build owner.

**Paid validation:** Not applicable; maximum calls/cost 0.

**Operational and rollout notes:** Add focused providers, cut over this behavior, delete its former path before completion. Preserve formats/identity/override precedence and cleanup telemetry. Revert lane changes plus generated Wire; no persisted migration. Stop on changed public behavior, isolation failure or undocumented fallback policy. Exact temporary checker allowances are removed by their lane.

**Escalation:** Return evidence, impact, safe work completed and smallest delta plan when required authority/prerequisites are absent or discovered behavior exceeds this outcome. Do not silently broaden scope; unrelated parallel work continues.

**Handoff artifacts:** Owned changes/characterization, generated Wire if affected, inventory dispositions, compatibility notes and artifact-specific PR evidence. Implementation stops after push/open PR/CI start and blocking feedback resolution; review owns terminal CI/conflicts/merge. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden. CI-run evidence is a PR comment, never committed.

## T27 — Lifecycle performance baseline is recorded before structural work

**Parent behavior:** P01 (plan §7 "Performance and scale"): baseline captured before affected structural work.

**Problem:** P01 requires a same-environment baseline before structural lanes merge. No lifecycle benchmark or profile exists: there are no `func Benchmark` in `pkg/root`, `pkg/wire`, Sessions or Runtime, and `tests/stress` has no lifecycle profile.

**Outcome:** A dedicated stress profile, plus a committed baseline record measured at the pinned project base commit.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 7 and 10 (P01).

**Actor and trigger:** Project lead or validation lane runs the profile on a named host.

This is a bounded enabling task. It must exist before any behavior lane changes lifecycle code, so it cannot live inside one behavior slice.

**Dependencies:** FI-PREREQ-BASELINE-OBS (read-only observation authority and demonstrated feasibility). Schedule that prerequisite first.

AM10: baseline stays pinned to `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`. Lifecycle driving uses public BuildProcess/Execute/session boundaries; private handle/lease/capacity observations require the independent read-only baseline observer runbook in inventory.md. Prove access, synchronization, nonmutation and matching timing before reporting P01. Tool/seam feasibility is unresolved at FI-PREREQ-BASELINE-OBS; unsupported access remains BLOCKED. No baseline production instrumentation or public session-count substitute is permitted.

**Parallel and shared-surface ownership:** This lane owns `tests/stress/lifecycle_profile_test.go` and the baseline record. It changes no production code.

**Scope:**
- In:
  - `tests/stress/lifecycle_profile_test.go`, reusing `startStressProcess` (`tests/stress/process_harness_test.go:73`, which calls `root.BuildProcess` at `:126`) and the median helpers in `tests/stress/query_latency_test.go`.
  - The baseline record `docs/internal/development/plans/flat-injection/p01-baseline.md`.
- Out: production code, CI job changes, thresholds other than plan §7.

**Implementation constraints:**
- Testing standard §6: the profile lives in `tests/stress`.
- Skip it under `-short`, so `make test-stress`, which passes `-short`, never runs it.
- No wall-clock assertion in ordinary suites.
- Report every valid sample.

**Amendment v1.1:** AM10: counted owners/checkpoints are in inventory.md. Observer feasibility is prerequisite evidence, not a design certified by this amendment. T22 compares only matching owner counts and timing at the original pin.

**Contract and configuration excerpts:** No interface or configuration change.

**Acceptance criteria:**
- [ ] Given a host with no co-tenant factory load, when `go test ./tests/stress -run TestLifecycleProfile -count=1 -timeout 10m` runs at the pinned base commit, then it emits 10 inert `BuildProcess`+Close samples and 10 explicit-session open/Work/close samples, each with median, p95 and full range, plus a 100-cycle open/replace/close run reporting goroutine count and owned-handle/lease counts before and after.
- [ ] Given that output, when the baseline record is committed, then it names the commit SHA, host, Go version, CPU and memory, and the exact command. Noisy samples are annotated, never dropped.
- [ ] Given `make test-stress`, when it runs, then the profile is skipped.

**Verification:**
- Behavioral witness: the baseline record.
- Required evidence: load/stress, local_real, the command above.
  - Proves: baseline numbers on one host.
  - Does not prove: fleet-scale throughput.
- Unproven edges: final comparison → T22.

**Paid validation:** Not applicable; 0 calls.

**Operational and rollout notes:** Test-only. Revert by deleting the test file and the record.

**Escalation:** If the harness cannot compile at the pinned base commit, record BLOCKED with the reason. Do not change the pin, modify baseline production or invent numbers. Record the exact access/build failure and smallest prerequisite/operator decision; observation and timing remain unproven.

**Handoff artifacts:** The harness, the baseline record, and the PR comment with the raw output. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden.

## T28 — HTTP and visualization stop locating services through Sessions getters

**Parent behavior:** F13; plan §6 (decomposition map, HTTP row).

**Problem:** `pkg/wire/http_runtime_binding.go` and `run_session_runtime.go` locate peer services through `*factorysessionwire.Root` getters and the `SessionPresentation` service container.

**Outcome:** The HTTP binding takes only session/host selection and cancellation. `SessionPresentation` carries facts only. The Sessions root peer-service getters are deleted.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11, 13; F13. Contract pair: contracts.md "T18 — Direct HTTP/MCP/visualization consumers" (this lane implements that pair).

**Actor and trigger:** HTTP/SSE client or dashboard reading an explicit live, expired or missing session.

**Dependencies:** T17 (Sessions opening no longer produces the service products these getters return), T18 (direct adapters already injected).

**Parallel and shared-surface ownership:** This lane owns `pkg/wire/http_runtime_binding.go`, `run_session_runtime.go:300-305`, `runtime_inputs.go:538-572` (`sessionInspectionForHTTP`, `ResolveFactorySessionRuntimeScope`), and the getters in `factory_sessions/internal/service/application_session.go:196-205`. The composition steward regenerates Wire in this PR. Worker-session visibility project (`docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`): this lane (with T18) owns Worker Sessions HTTP handler wiring and the `pkg/wire` binding; visibility tasks that add handler wiring or constructor dependencies wait for T18 and T28 to merge, and this lane does not wait for visibility (operator decision, 2026-10-02).

**Scope:**
- In:
  - The binding signature change.
  - Shrinking `SessionPresentation`.
  - Removing the `modelInvoker = root.WorkersService()` fallback (`http_runtime_binding.go:181-184`) and the `zap.NewNop` fallback (`run_session_runtime.go:305`).
  - Deleting the getters once no caller remains.
- Out: handler `Dependencies` bags (T18), MCP wrappers (T24), API/OpenAPI changes.

**Implementation constraints:**
- Missing/expired/gone/gap errors, SSE retained→live ordering, close boundaries and per-host cancellation stay unchanged.
- No new aggregate replaces the root.

**Contract and configuration excerpts:**

Current:

```go
type httpRuntimeBinding func(*factorysessionwire.Root, string, initializer.InvocationCancellation) (http.Handler, error)
```

Proposed:

```go
type httpRuntimeBinding func(string, initializer.InvocationCancellation) (http.Handler, error)
```

Current `SessionPresentation` (`application_session.go:128-140`) holds `FactoryRuntime`, `ModelInvoker`, `WorkerSessions`, `Logger`, `Reader`, `Projections`, `Clock` and `Recordings`. The proposed version holds only `ModelsScope`, `RuntimeID`, `GenerationID`, `MetricsRootDir` and `OperatorSettingsPath`, exactly as in contracts.md.

Generated: `pkg/wire/wire_gen.go` via `make generate-wire`.

**Acceptance criteria:**
- [ ] Given a live explicit session, when HTTP reads, actions and the SSE stream run, then `TestAPIResponseEventSSEStreamsRetainedThenLiveEvents` and `TestAPIResponseEventStreamClosesAtDocumentedBoundary` (`tests/functional/events/response_events/stream_test.go`) pass unchanged.
- [ ] Given an expired or missing session or a cursor gap, when it is read over HTTP, then `TestAPIResponseEventSessionExpiryReturnsTypedGone`, `TestAPIResponseEventCursorGapEmitsStreamGap` and `TestAPIFactorySessionNotFoundUsesTypedError` pass unchanged.
- [ ] Given `you run --with-server` with visualization, when the run completes, then `tests/functional/factory_visualization` and `tests/functional/factory/visualization/runtime_metrics` pass, and quiet-mode stderr matches `TestConcurrentQuietAndVerboseInvocationsKeepOwnFraming` (T02).
- [ ] Static: `rg` finds no production caller of the deleted getters, and `make pkg-boundary pkg-file-count wire-smoke` pass.

**Verification:**
- Required evidence:
  - Unit: `go test ./pkg/wire ./pkg/services/factory_sessions/... ./pkg/transports/http/...`.
  - Functional: `go run ./cmd/functionallane -root <dir>/...` for `tests/functional/events/response_events`, `tests/functional/factory_visualization`, `tests/functional/workers/transports/http` and `tests/functional/sessions/lifecycle`, then `make test-functional`.
  - PR: `make lint`.
- Unproven edges: OS signals → I01; integrated journey → VAL01.

**Paid validation:** Not applicable.

**Operational and rollout notes:** Revert this lane plus generated Wire. Stop on any SSE ordering or typed-error drift.

**Escalation:** Return evidence and the smallest delta plan if a getter has a non-HTTP production caller not listed here.

**Handoff artifacts:** Code, generated Wire, updated inventory dispositions for the T18 rows `application_session.go`, `http_runtime_binding.go`, and the `runtime_inputs.go` / `run_session_runtime.go` additions, and the PR evidence comment. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden.

## T29 — Final static enforcement covers the whole repository

**Parent behavior:** S01 (plan §13: zero unresolved inventory findings at completion).

**Problem:** T20 enables checks per capability set. Nothing removes the staged scope restrictions and migration allowances once every owner has migrated.

**Outcome:** The construction, fallback and getter rules run at repository scope with zero allowances. Any analysis limits that remain are recorded.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); section 13; lint.md §6 step 4.

**Actor and trigger:** `make lint` on every PR after this merges.

**Dependencies:** T01–T19, T21, T23–T26, T28, T30, T31.

**Parallel and shared-surface ownership:** This lane owns the `cmd/pkgboundarycheck` scope configuration and allowance lists. It changes no production code. A finding in owner code goes back to that owner as a delta.

**Scope:**
- In:
  - Remove the staged scope restriction and every temporary allowance.
  - Run the full static gates.
  - Record the unresolved analysis limits in lint.md §4.
- Out: fixing production findings, which belongs to the owning lane through a delta plan.

**Implementation constraints:** No blanket baseline. Unrelated existing baselines stay deletion-only.

**Contract and configuration excerpts:** No runtime interface change.

**Acceptance criteria:**
- [ ] Given the integrated head, when `make lint` runs with the restrictions removed, then it passes with zero allowances, and the allowance list is empty and deleted.
- [ ] Given each inventory.md row, including the validation-review additions, when the disposition audit runs, then every row has a terminal disposition (removed, direct injection, retained scoped state/resource, or retired compatibility code) linked to its PR.
- [ ] Given a new prohibited construction in any service package, when `make pkg-boundary` runs, then it fails with that symbol.

**Verification:**
- Static: `make lint` and `go test ./cmd/pkgboundarycheck ./cmd/durableruntimeconstructioncheck ./cmd/loggingboundarycheck ./cmd/pkgmaintcheck ./cmd/servicecyclecheck`.
- Proves source properties only. Runtime behavior is covered by VAL01.

**Paid validation:** Not applicable.

**Operational and rollout notes:** Rebase and re-run `make lint` immediately before merge. Revert restores the allowances.

**Escalation:** A remaining finding becomes a delta request to its owning lane. Do not add an allowance to get past it.

**Handoff artifacts:** Checker configuration change, the inventory disposition table, and the PR evidence comment. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden.

## T30 — Model slot state and capacity coordination without post-construction binding

**Parent behavior:** F09c/F09d; plan §6 "Dependency and mutation rules" (Models host/leases cycle).

**Problem:** `runtime_host/internal/service/slot_facts.go:28-43` builds leases and then patches `adapter.host = host`. `runtime_host/internal/service/service.go:102` installs the host as the lease capacity coordinator through `leaseswire.BindCoordinator` (`leases/wire/wire.go:32`). These are two hidden construction cycles.

**Outcome:** Keyed slot state and a capacity/idle coordinator are constructed independently and injected into both the host and the leases. `adapter.host`, `BindCoordinator` and `CoordinatorBindable` are deleted.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6 and 10 (F09c/F09d). Contract pairs: contracts.md "T10 slot state and lease coordination cycle cut" (the text sits under the T11 heading but belongs to this lane).

**Actor and trigger:** A model invocation acquires a slot while another scope holds capacity.

**Dependencies:** None.

**Parallel and shared-surface ownership:** This lane owns `pkg/services/models/internal/services/runtime_host/**` and `leases/**`. T10 owns `models/wire` leaf providers and `effects.go`. Coordinate the one `models/wire/wire.go` registration edit through the composition steward.

**Scope:**
- In: slot state owner, coordinator, the host and leases constructors, owner tests.
- Out: scoped runtime cache (T11), leaf provider split (T10), lease policy values.

**Implementation constraints:** Capacity classifications (`ErrHostCapacityExhausted`, `ErrHostCapacityContended`), idle-unload and eviction policy, and the single-release guarantee stay unchanged.

**Contract and configuration excerpts:** See the contracts.md pair cited above, including the exact current and proposed constructor shapes.

**Characterization prerequisite (land before restructuring):**

- `TestResourcePressureDoesNotEvictPeerScopeActiveLeaseHolder`. Layer: unit. Path: `pkg/services/models/internal/services/runtime_host/internal/service/unload_test.go`.
  - Given `MaxLoadedRuntimes=1` and scope A holding an active lease.
  - When scope B calls `EnsureModelHost`.
  - Then A is not stopped and B receives the existing capacity classification; after A releases, B succeeds.

Keep these green:
- `TestAcquireModelLeaseRejectsCapacityExhaustion`, `...RejectsContendedCapacity` and `TestReleaseModelLeaseNotifiesCapacityCoordinator`;
- `leases/internal/service/concurrency_test.go`;
- `TestIdleUnloadStopsRuntimeAfterLeaseRelease`, `TestResourcePressureEvictsIdleRuntime` and `TestCloseRuntimeScopeRevokesLeasesOwnedByStoppedHost`.

**Acceptance criteria:**
- [ ] Given an occupied slot, when the same slot is acquired, then the existing exhausted/contended classification is returned with no second allocation; after release, the next eligible attempt succeeds.
- [ ] Given resource pressure with a peer scope's active lease holder, when the other scope ensures a host, then the prerequisite test's outcome is unchanged.
- [ ] Given lease release, when the coordinator is notified, then idle-unload behaves as `TestIdleUnloadStopsRuntimeAfterLeaseRelease` asserts.
- [ ] Static: `rg 'adapter\.host|BindCoordinator|CoordinatorBindable' pkg/services/models` returns no production hit.

**Verification:**
- Required evidence:
  - Unit: `go test -race ./pkg/services/models/internal/services/runtime_host/...`.
  - Functional: `go run ./cmd/functionallane -root ./tests/functional/models/...`, then `make test-functional`.
  - Static: the plan's static/generation set including `pkg-file-count`, and `make wire-smoke`.
- Unproven edges: two-scope process behavior → T11.

**Paid validation:** Not applicable.

**Operational and rollout notes:** Revert this lane. Stop on any change in capacity classification.

**Escalation:** Return the smallest delta plan if the coordinator needs host state that cannot be keyed.

**Handoff artifacts:** Code, tests, inventory dispositions for the `slot_facts.go` and BindCoordinator rows, and the PR evidence comment. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden.

## T31 — Runtime instance-host build leaves are injected directly

**Parent behavior:** F03/F04 (preparation); plan §6 decomposition map, "Runtime Assembly/Build/Factory" row.

**Problem:** `factory_runtime/internal/services/instance_host/build/service.go` (`BundleBuilder` / `Service`) stores a deferred graph-construction closure. `orchestration/runtime/worker_session_control_targets.go` `WorkstationRequestExecutorConfig` mixes runtime values with many collaborators, including an `any` filesystem.

**Outcome:** The reusable activation/replacement behavior and the typed workstation-request collaborators are injected once. Request-resolution values become a separate value type. The nested builder is removed. T15 then folds `RuntimeFactory.Build` into the activation operation.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 6, 9–11; F03/F04. Inventory rows: `instance_host/build/service.go` and `worker_session_control_targets.go` (previously T15).

**Actor and trigger:** An explicit session starts or replaces its runtime.

**Dependencies:** T14 (single instance-host handle authority).

**Parallel and shared-surface ownership:** This lane owns `instance_host/build/**` and `worker_session_control_targets.go`. It must not change `Root.Activate`, `RuntimeFactory.Build` or `runtime_build.go`; those belong to T15.

**Scope:**
- In: the two inventory rows above, plus porting the assertions of `instance_host/build/service_test.go` and `service_behavior_test.go` onto the injected behavior.
- Out: activation signature, Sessions callers, Worker Sessions keying (T16).

**Implementation constraints:** Start failure unwinds, cleanup stays retryable, and replacement keeps the prior generation until commit. Assertions are ported, never dropped.

**Contract and configuration excerpts:** **First required step (operator decision, 2026-10-02):** before any structural change, add a current/proposed Go pair for `BundleBuilder` and `WorkstationRequestExecutorConfig` to contracts.md (section "T31 — Runtime instance-host build leaves"), per plan §5. Today contracts.md has no pair for these. If the pair cannot be written without changing dependency types or ownership beyond this outcome, stop and return a delta plan (plan §2 replanning trigger).

**Acceptance criteria:**
- [ ] The contracts.md current/proposed pair for `BundleBuilder` and `WorkstationRequestExecutorConfig` is merged before any structural change, and the delivered code matches it (or a delta plan was returned instead).
- [ ] Given a resource that fails once at start, when an explicit session starts and then retries, then `TestRuntimeRootActivationUnwindsFailedStartAndCanRetry` and `TestRuntimeRootFailedCleanupRemainsExplicitlyRetryable` (`factory_runtime/wire/runtime_activation_test.go:256,294`) pass unchanged, and so does the functional F03 test `TestRootProcessStartFailureThenRetrySucceedsWithoutLiveSession` (defined in T13).
- [ ] Given a successful replacement, when the session's response stream is read, then `TestFactoryResponseEventSequenceSurvivesSessionRuntimeReplacement` passes.
- [ ] Given the ported builder tests, when they run against the injected behavior, then every former assertion still holds (the PR lists old test → new test).

**Verification:**
- Required evidence:
  - Unit: `go test ./pkg/services/factory_runtime/... ./pkg/wire`.
  - Functional: `go run ./cmd/functionallane -root <dir>/...` for `tests/functional/events/response_events`, `tests/functional/factory_runtime` and `tests/functional/sessions/root_composition`, then `make test-functional`.
  - Static: the plan's static/generation set including `pkg-file-count`, and `make wire-smoke`.
- Unproven edges: activation fold → T15.

**Paid validation:** Not applicable.

**Operational and rollout notes:** Revert this lane plus generated Wire.

**Escalation:** Return the smallest delta if the contract pair cannot be authored, or if a builder closure captures per-session state that cannot move into the request value.

**Handoff artifacts:** Code, contracts.md pair, the test-port mapping, and the PR evidence comment. Merge safety (operator decision, 2026-10-02): immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden.

## VAL01 — Independent read-only validation loopback

**Parent behavior:** F01–F18 and project acceptance criteria.

**Problem:** Per-lane evidence does not establish integrated customer behavior.

**Outcome:** Independent artifact-specific PASS/FAIL/BLOCKED report.

**Plan reference:** `docs/internal/development/plans/flat-injection/plan.md` (repo-relative); sections 10/13.

**Actor and trigger:** Independent validator receives candidate prebuilt artifact, fresh owned profile and controlled fixtures. AM14: this is review/validation-owned evidence after implementation handoff, never a report the author must fabricate for an all-stories-pass gate. T25 VAL25 and T01/T06 FI-SHARED-QUALITY remain separate mandatory inputs; docs correction grants no release.

**Dependencies:** T01–T31 terminal criteria; P01/I01 evidence and candidate artifact.

**Parallel and shared-surface ownership:** Read-only validator; owners correct their findings independently. No silent implementation fixes.

**Scope:** In: clean customer spine, criterion/evidence review, failure/recovery/isolation. Out: code edits, paid calls, invented CI evidence.

**Implementation constraints:** Canonical graph/public entrypoints; report substitutions and uncertainty.

**Contract and configuration excerpts:** No interface/config change; report uses canonical validation-loopback-template.md.

**Acceptance criteria:**

- [ ] Every project criterion records PASS/FAIL/BLOCKED, evidence and unproven edge.
- [ ] Clean load/start/Work/observe/cancel/close/record/replay journey preserves outcomes.
- [ ] FAIL/BLOCKED requests the smallest delta plan and affected retest scope.

**Verification:**

- Behavioral witness: F01–F18 and integrated delivered journey.
- Executable-spine effect: increase_fidelity.
- Required evidence: end-to-end, controlled/local_real. Procedure: fresh owned profile/directory, prebuilt CLI and controlled provider/network fixtures; load Factory, start explicit session, submit Work, inspect ordered result/events, cancel peer, close/flush, replay history. Capture exact commands/outputs/artifact IDs. Proves cross-task integration; does not prove paid remote availability.
- Highest feasible: delivered local entrypoint with substituted provider effects.
- Unproven edges: real remote availability -> existing release gate outside scope.
- Test-layer design: candidate supplied by build owner, explicit owned sessions/resources, bounded observable waits; review static/unit evidence separately.

**Paid validation:** Calls/cost 0.

**Operational and rollout notes:** Failure blocks affected acceptance. Validator never repairs defects silently.

**Escalation:** Reproduction, expected/actual outcome, artifact, uncertainty and smallest correction.

**Handoff artifacts:** Report headings: Environment and artifact; Project criteria; Customer journey; Cross-task integration and usability; Findings; Verdict; Delta-plan request for FAIL/BLOCKED. UI accessibility is not applicable because there is no UI change. CI evidence stays in PR comments.
