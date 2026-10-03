# Flat-injection plan: blind validation review

Reviewed 2026-10-02 against origin/main `933b8c691d`. Inputs: [plan.md](plan.md), [tasks.md](tasks.md), [contracts.md](contracts.md), [inventory.md](inventory.md) and [lint.md](lint.md). The review read source only. No tests ran and no code changed.

## Verdict

**READY-WITH-CHANGES.**

The inventory and contract excerpts are accurate, and the overall approach is sound. As submitted, however, the plan was not dispatchable:
- three dependency edges gated whole lanes on partial work;
- the performance baseline was scheduled last;
- one repository gate (`pkg-file-count`) would have failed the plan's own provider-file strategy;
- per-owner fallback rows were assigned to T23/T21 while the owner lanes rewrite the same functions;
- the packets named no existing tests and no focused functional commands.

The review fixed these in place. The operator resolved every item in [Residual open questions](#residual-open-questions) on 2026-10-02. Items 1–2 remain first required steps inside the T16/T31 packets and do not block other lanes.

## What was verified

- **Inventory rows.** 36 rows in "Construction graphs" and the coordination table, and all 43 rows in "Shared sources", checked against source. All are accurate; the only drift was one ±3-line shift (`execution/wire/wire.go` 136–137 → 133–134), now corrected. lint.md §2 (all 12 rows) and §8 (22 sampled rows) are also accurate, with one line drift (`agy/pty.go` 69 → 72).
- **Contract excerpts.** Every "Current:" excerpt for T13–T19 in contracts.md and tasks.md matches source verbatim, for example `assembly.go:33-55`, `factory.go:212-224`, `http_runtime_binding.go:87` and `application_session.go:128-140`.
- **Commands.** All Make targets named in the packets exist: `test-functional`, `pkg-boundary`, `logging-boundary-check`, `durable-runtime-construction-check`, `service-cycle-check`, `wire-smoke`, `lint`, `verify-fast`, `verify-pr`, `generate-wire`. All `go test` package paths and the `cmd/*check` directories exist. Caveats:
  - `pkg/root` contains only `TestMain` (`pkg/root/root_test.go`).
  - `cmd/functionallane` has no `-run` flag and defaults to `-short=true` (`cmd/functionallane/main.go:72-77`).
  - `make test-stress` passes `-short` (`Makefile:722`).

## Findings

| # | Severity | Finding | Evidence | Change made |
| --- | --- | --- | --- | --- |
| 1 | High | **The plan's provider-file strategy would fail `pkg-file-count`.** Plan §9 said "Give each lane a focused provider file" in `pkg/wire`, but that gate is exact and deletion-only. `pkg/wire` is pinned at 50 files, and unlisted packages are capped at 15. Concurrent lanes lowering the same JSON entry each pass on their own PR but break main once both merge. | `cmd/pkgfilecountcheck/main.go:18,133-160`; `docs/internal/baselines/backend-package-file-count.json` (`pkg/wire` 50); 50 files in `pkg/wire` | Plan §9 shared-surfaces row rewritten. `pkg-file-count pkg-maint` added to every packet's static line, with a rebase-before-merge rule. |
| 2 | High | **The P01 baseline was only measured at the end (T22).** No lifecycle benchmark or profile exists. | No `func Benchmark` under `pkg/root`, `pkg/wire`, Sessions, Runtime or `tests`. `tests/stress/process_harness_test.go:73,126` is reusable. | New **T27**: a `tests/stress` lifecycle profile measured at the pinned base commit, scheduled first. T22 now depends on T27. Plan §7, §10 and §11 updated. |
| 3 | High | **Partial dependencies gated whole lanes.** In the factory, DEPENDS_ON gates an entire lane, but three tasks depended on later work for only part of their scope:<br>• T18 needed T17 only for getter removal.<br>• T20 needed every lane for final enforcement.<br>• T22 combined the baseline with the final comparison. | plan.md §11 (original); tasks.md T18/T20/T22 | Split into separate tasks:<br>• T18 is now adapters only (no deps); **T28** removes the getters (T17, T18).<br>• T20 is now infrastructure and per-set enablement; **T29** does final enforcement.<br>• T22 does the final comparison; **T27** captures the baseline. |
| 4 | High | **Hidden T15↔T16 cycle.** T15 depends on T16, but `WorkerSessionsFactory` is consumed in T15's `wire2.NewAssembly`. In addition, T16's proposed `NewService` is identical to the current one: per-attempt execution, clock and overrides come from a per-runtime `workers.Service`, and state is keyed by bare dispatch ID. | `composition_contracts.go:84`; `wire_gen.go:242`; `runtime_build.go:477-505,621-631`; `invoke_session.go:294-310` | Ownership fixed: T16 adds the keyed opener, and T15 deletes the factory. T16 must add the attempt-request contract pair before structural work (note added to contracts.md and the T16 packet). |
| 5 | High | **T23/T21 rows duplicated owner lanes.** Inventory assigned to T23 the per-owner fallbacks in Models, Automations, Providers and Recordings, and assigned to T21 the Automations `supervisorClock` and the Providers built-in clocks. Those owner lanes rewrite the same functions, while the T23 packet itself says lanes own their fallbacks. | inventory rows (original) at `models/.../runtime_factory.go:108-126`, `automations/internal/service.go:147-153,253,271-278`, `providers/wire/wire.go:283,598`, `recordings/internal/core.go:390,432` | 17 rows moved from T23 to T03, T08, T10, T11 and T12, and 2 rows moved from T21 to T08 and T12. T23 and T21 are narrowed to boundary work and `pkg/wire` defaults. |
| 6 | High | **Contradictory Automations cursor policy.** Inventory row 129 said "propagate durable failure", but plan §6/§7 says to preserve the memory fallback. The error branch is in fact unreachable. | `automations/internal/service.go:147-153` (blank base and nil filesystem are guarded first) | Row 129 rewritten to "preserve; record unreachable". Any change in policy needs an operator decision (open question 4). |
| 7 | High | **About 35 missed sites.** These include post-construction setters on the shared durable `JavaScriptRuntimeService` during opening (a cross-session overwrite risk); the `AttachSessionGateway` cycle; orchestration constructed three times; Root built with placeholder collaborators; `attachInvocationScheduleFactory`; engine setters; `WorkerSessionsObservationForSession` used as a locator at 7 sites; the visualization composite `NewRoot`; System Initialization (never mentioned in the plan); webhook/provider_sessions/settings guards; and host timers in 12 owner files. | inventory.md "Validation-review additions" (file:line per row) | Every row added with an owning task. The T13–T19, T21 and T23–T24 packets name them. |
| 8 | High | **T01's scheduler type is too narrow.** `TimerSource` has only `Now`/`NewTimer`, but consumers need `After`, `AfterFunc` or a full clockwork clock. In addition, the replay-clock branch may be dead on the composed path. | `pkg/platform/clock/clock.go:30,65`; `execution/service.go:84`; `process/command.go:24`; `automations/internal/service.go:274`; `factory_sessions/internal/service/construction.go:382` with `wire_gen.go:141` | T01 adds `After` to `TimerSource` (resolved question 3) and characterizes which clock replay uses (prerequisite 2). |
| 9 | Medium | **Fake-clock waiter counts.** Several functional tests count fake-clock waiters with `BlockUntilContext(ctx, n)`. Moving timers onto `Edges.Clock` will hang them or make them fail in ways that look unrelated. | `tests/functional/work/submission/legacy_unary_test.go:209`, `workstations/cron/helpers_test.go:91`, `workstations/poller/poller_test.go:240`, `factory/packaged/loop/invocation_test.go:177` | Audit rule added to T01 and T21. |
| 10 | Medium | **Real process logger may leak into quiet runs.** Providers logs go to noop in production today because `WithLogger` is never passed. The Models host logs are `NewNop`. The base logger writes to OS stderr, not to the invocation's stderr. | `pkg/wire/session_runtime_providers.go:169-213`; `providers/internal/service/service.go:84`; `wire_gen.go:110`; `models_runtime.go:344,348` | Quiet-policy prerequisites added to T02 and T23. T24 must audit each `EnsureLogger` site before removing it. |
| 11 | Medium | **Several lanes are too big for one PR.** T10, T15, T13, T17, T12 and T09 are each 1.7k–4.5k LOC. T10 and T15 each contain two independently observable changes. | Agent `wc -l` of the in-scope files | **T30** split from T10 (Models slot state and coordinator); **T31** split from T15 (instance-host build leaves). T13, T17, T12 and T09 get an explicit escalation split. |
| 12 | Medium | **Ownership conflicts between T13 and T17.** `NewRootFromAssembly`, `RuntimeModelInvokerConfig` and the durable factories were assigned inconsistently between inventory and contracts. The contracts.md T13 proposal also drops `clock`. | inventory rows 24 and 36; contracts.md T13/T17 | All three assigned to T17. A note was added for the dropped `clock`. |
| 13 | Medium | **Dead seams presented as needing migration.** `newInvocation` has no production caller. Five MCP `RootDependencies` wrappers have no production importer. The global Settings→Providers registry has no callers. | `pkg/wire/session_runtime_providers.go:1105` (in `servicesSet`, never injected); `profiles.go:617`; `operator_settings/internal/construct` imported only by tests | Dispositions recorded: T09/T17 delete the dead invocation path; the MCP wrappers move to T24; T07 deletes the registry. |
| 14 | Medium | **Functional commands were not runnable as written.** Packets said "owned cases selected through the existing bounded lane", but the lane cannot select by test. Long and `functionallong` replay cases are skipped by default, and four `tests/functional/providers/*` directories are quarantined and empty. | `cmd/functionallane/main.go:72-77`; `tests/functional/internal/support/short.go:5` | Every packet now names `-root` directories plus the long-mode commands. Plan §10 updated. |
| 15 | Medium | **Template acceptance criteria could not be observed.** T20 and T22 said "plan matrix outcome preserved / peer-session survival" for lint and performance tasks. T14 claimed F04, which it cannot prove. | tasks.md T20/T22/T14 (original) | Rewritten as fixture-, threshold- and controls-based criteria. |
| 16 | Low | **Absolute Codex-worktree paths in all 27 packets.** | tasks.md "Plan reference" lines | Rewritten to the repo-relative `docs/internal/development/plans/flat-injection/plan.md`. |
| 17 | Low | **Inventory row 98 conflicts with plan §9.** It assigned `pkg/wire/wire.go` integration to T20, while plan §9 gives it to a per-lane steward. | inventory.md row 98 | Reassigned to "Steward (each lane)". |
| 18 | Info | **Packets over 9,000 characters.** T13 (≈11.8k), T01 (≈11.1k), T15 (≈10.8k), T12 (≈10.5k), and T18, T16, T17, T26 (≈9.8–10k). | `awk` section sizes | None needed: packets are cited by path, not inlined. |
| 19 | Info | **I01 has no flush witness.** `TestRootProcessCompiledBinaryModeMatrix` runs with `--no-record`, and the Windows-only flush test is not portable. | `tests/release/root_process_smoke_test.go:39,102,111` | T22 now specifies a `--record` subtest under `make test-release`. |

## Test-layer coverage matrix

Legend:
- **✓** an existing test exercises the moving behavior (named in the packet's "Validation-review coverage and scope" block).
- **GAP→P** no witness exists at this layer; prerequisite P was added to the packet and must land before restructuring.
- **–** the layer is not touched.
- **new** the plan's own new witness (F15/F16/F14 and so on), written in the lane.

Paths and Given/When/Then for every prerequisite are in tasks.md.

| Task | Unit (owner) | Wire / composition | Functional CLI | HTTP | MCP | ACP | Replay / recordings | Integration / stress |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| T01 time | ✓ `definition_test.go` | GAP→`TestFactoryRuntimeMetricsClockSelectsTimerCapableEdgeOrReal` | new F15; waiter-count audit | ✓ SSE expiry | – | – | GAP→`TestReplayUsesRecordedArtifactClockThroughComposedProcess` | – |
| T02 logging | ✓ terminalpolicy | ✓ settings logger | GAP→`TestConcurrentQuietAndVerboseInvocationsKeepOwnFraming`, `TestProcessModelsInvokeQuietKeepsHostDiagnosticsOffStdout` | – | – | ✓ `transport/acp/stdio` | – | – |
| T03 Recordings | GAP→`TestCombinedServiceAbandonedScopeFinishesAtInjectedClock` | ✓ `recordings_*_composition_test.go` | ✓ `TestRecordingFlushBeforeProcessExecuteReturns` | ✓ `handlers_history_test.go` | – | – | ✓ `replay_contracts` (long cases need `functionallong`) | – |
| T04 Definitions + SysInit | ✓ | ✓ packaged installation | ✓ `factory_definitions`, `bootstrap_portability`, `product/init_setup` | ✓ `TestCurrentFactoryPUT_*` | – | – | – | – |
| T05 Work | ✓ | ✓ | GAP→`TestWorkListConfirmsStateAfterRecordingFlush` | ✓ `runtime_api` | – | – | ✓ (flush confirmation via the prerequisite) | – |
| T06 Provider Sessions | ✓ | ✓ | GAP→`TestProviderSessionsHomeResolutionFailureOutcome` | ✓ raw-path rejection | – | – | – | – |
| T07 Settings | ✓ (dead registry tests deleted) | ✓ | ✓ configcore CLI | GAP→`TestHTTPSettingsLoadAndUpdateThroughBuildProcessPreservesUnknownFields` (existing "root composition" tests bypass the canonical graph) | same as HTTP | ✓ via `SeedACPAgentProfile` migration | – | – |
| T08 Providers | ✓ timeout/cancel/PTY | ✓ `agy_pty_test.go` | GAP→`TestAgyInjectedPTYHostExecutesWorkThroughRootBuildProcess` | ✓ error mapping | ✓ `execute_test.go` | ✓ `providers/acp` | – | – |
| T09 Workers | ✓ registry/runners | ✓ (dead `newInvocation` deleted) | GAP→`TestTwoFactorySessionsCancelOneInFlightProviderAttemptPeerCompletes` | ✓ `workers/transports/http` | – | – | – | – |
| T10 Models leaves | ✓ | ✓ typed-nil edges | ✓ `models/...` | – | – | – | – | – |
| T30 slot state | GAP→`TestResourcePressureDoesNotEvictPeerScopeActiveLeaseHolder` | – | ✓ `models/...` (F09c sentinels stay unit-level) | – | – | – | – | – |
| T11 Models scopes | GAP→`TestRootInvokeLocalTwoScopesUseOwnConfigAndReleaseCapacity`, `TestRootCloseRuntimeScopeRejectsConcurrentInvokeLocal` (existing tests pin the internal cache) | – | GAP→`TestModelWorkersInTwoFactorySessionsKeepScopedConfigInOneProcess` | – | – | – | – | – |
| T12 Automations | GAP→`TestRuntimeLifecycle_ReactivationStopsPriorAdmissionAndResumesCursor`, `TestGetCursorWithTwoRuntimesSharingPollerInstanceID` | ✓ cursor across a new Root | ✓ `automations`; watcher switch is long-only (`-short=false`) | – | – | – | – | – |
| T13 Sessions leaves | ✓ `binding_test.go:627` | ✓ | GAP→`TestRootProcessStartFailureThenRetrySucceedsWithoutLiveSession` (F03), `TestFourExplicitSessionsIsolateOneCancellation` (F06) | ✓ via F06 | ✓ `transport/mcp` | ✓ `transport/acp/stdio` | – | – |
| T14 Runtime authority | GAP→`TestRootControlReachesHandleRegisteredByActivation` | ✓ | ✓ `lifecycle_activation_test.go`, controls | ✓ `pause_resume_test.go` | – | – | – | – |
| T31 build leaves | ✓ builder tests (must be ported, not dropped) | ✓ | uses T13's F03 prerequisite | – | – | – | – | – |
| T15 activation | ✓ `runtime_activation_test.go:256,294`; GAP→`TestReplaceStartFailureKeepsPriorGenerationAndRestoresSidecars` | ✓ | ✓ F04 success; F03 via T13 | ✓ factory PUT replacement | – | – | ✓ `factory/replay_contracts` (+`functionallong`) | – |
| T16 Worker Sessions | GAP→`TestRuntimeAttemptsFromTwoRuntimesWithEqualDispatchIDsRemainIsolated` | ✓ | GAP→ the shared T09 F05d test | ✓ route matrix | – | – | – | – |
| T17 opening | ✓ | ✓ | GAP→`TestMockWorkersPassthroughUnmatchedReachesProviderEdge`, `TestConcurrentSessionsKeepOwnDurableJavaScriptCollaborators`; F03/F06 via T13 | ✓ | ✓ | ✓ `acp_prompt_delegation_test.go`, `cli_serve_acp_controls_test.go` | ✓ restart identity | – |
| T18 adapters | ✓ owner transports | GAP (no direct `http_runtime_binding` unit test; covered functionally) | – | ✓ SSE typed gone/gap/close | GAP→`TestMCPStartSyncRunsFactorySessionThroughComposedProcess` (the existing start test hand-builds its server) | – (untouched) | – | – |
| T28 getter removal | – | ✓ via `wire-smoke` | ✓ visualization functional (sink selection bypassed by fixtures) | ✓ same SSE set | – | – | – | – |
| T19 CLI | ✓ | ✓ | ✓ `transport/cli/output`, `sessions/cli` (F14 concurrency belongs to T02) | – | – | – | – | – |
| T20 / T29 lint | ✓ checker fixtures (`cmd/pkgboundarycheck` 17 test files) | – | – | – | – | – | – | – |
| T21 effects | ✓ chat fixedClock | ✓ override-wins cases | GAP→`TestSelectedProcessClockStampsChatTurnsAndRuntimeArtifacts`; waiter-count audit | – | – | ✓ via the prerequisite | ✓ `recordings` | – |
| T23 boundary | GAP→`TestBuildProcessRejectsTypedNilEdgeWithoutInvokingCollaborators` (`pkg/root` has no tests), `TestNewRootWithoutFactoryDirKeepsCursorInMemory` | ✓ typed-nil Models | GAP→`TestProviderAttemptLogsRespectQuietPolicy` | – | – | ✓ `transport/acp` | – | – |
| T24 retirement | ✓ (callers are test-only) | ✓ | per-site quiet audit before `EnsureLogger` removal | – | ✓ dead wrappers (no production caller) | – | – | – |
| T25 Costs | ✓ | ✓ | ✓ `runtime_metrics/end_to_end_costs_test.go` | ✓ handler | – | – | ✓ `TestReplayPricedUsageReachesPublicCosts` | – |
| T26 Webhooks | GAP→`TestServiceSkipsEventsAtOrBeforeActivationCursor`, `TestServiceDeadLetterAppendFailureLogsWithoutRetryStorm`, `TestServiceClosingOneSubscriptionLeavesPeerDelivering` | ✓ | ✓ `TestFactoryWebhooksRunThroughRootProcess` (one session) | – | – | – | – | – |
| T27 / T22 | – | – | – | – | – | – | – | new `tests/stress` lifecycle profile; I01 `--record` subtest in `tests/release` |

**The operator's specific concerns:**
- **MCP/ACP built through Sessions getters.** ACP is flat: `pkg/wire/acp_transport.go:181-213` injects services directly, and it is well covered functionally. MCP `start_sync` through the composed `you server mcp` had no witness; that prerequisite is now in T18.
- **Automations restart.** Confirmed: there was no same-process re-activation test. The prerequisite is now in T12.
- **Models cross-scope.** Confirmed: there was no two-scopes-in-one-Root test at either the unit or the functional layer, and the existing unit tests pin the cache being deleted. Prerequisites are now in T11.

## Residual open questions

All seven were decided by the operator on 2026-10-02 and applied to plan.md, tasks.md, contracts.md, inventory.md and lint.md.

1. **T16 attempt-request contract. RESOLVED.** T16's first required step, before any structural change, is to write the current/proposed Go contract for keying Worker Session attempts by (runtime ID, dispatch ID) into contracts.md, in its own PR. If keying is not feasible, the lane stops and returns a delta plan (plan §2 replanning trigger).
2. **T31 contract pair. RESOLVED.** Same rule: T31's first required step is adding the current/proposed pair for `BundleBuilder` / `WorkstationRequestExecutorConfig` to contracts.md (placeholder section added).
3. **T01 adapter set. RESOLVED.** `After` is added to the process `TimerSource` interface as one shared seam, with a deterministic implementation; no per-owner adapters over `NewTimer`. Consumers needing `AfterFunc` or full clockwork keep the "smallest owner-local interface" rule. T01 proposed shape updated in tasks.md and a new contracts.md "T01" pair.
4. **Automations durable cursor policy. RESOLVED (out of program).** Making durable-cursor failure explicit is a separate follow-up, not a task here. T12 preserves current behavior; inventory row 129 and lint.md §2 no longer say "propagate".
5. **Main-safety for ratchet/baseline gates. RESOLVED.** Every lane's delivery criterion requires that, immediately before merge, review rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a diff check when Wire changed) on the rebased head; a stale-green merge is forbidden. Enabling GitHub "require branches up to date" is a repository-settings decision outside this plan.
6. **T13, T17, T12 and T09 size. RESOLVED.** Left to the project lead: packets keep "split if review requires", and the lead MAY pre-split at admission.
7. **Concurrent project overlap. RESOLVED.** Flat-injection owns construction shape for `worker_sessions`, Worker Sessions HTTP handler wiring and `pkg/wire`. Visibility tasks (`docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`) that add constructor dependencies or handler wiring wait for T16, T18/T28 (and T03 for Recordings wiring) to merge; flat-injection does not wait for visibility. Stated in plan §9 shared surfaces and the T16/T18/T28 packets.
