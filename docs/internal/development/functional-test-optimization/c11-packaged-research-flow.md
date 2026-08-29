# C11 packaged research and Full Flow reconciliation ledger

Status: **Story 001 characterization PASS; runtime migration not started**
Recorded: 2026-08-29 02:18:37 -07:00
Lane: `functional-test-optimization-c11-packaged-research-flow`
Story: `functional-test-optimization-c11-packaged-research-flow-001`

This ledger is the operator-authorized replacement evidence record for the
retained C11 lane. It records characterization only. Stories 002–004 own
runtime parity, migration, cleanup, performance, and clean-room evidence.

## Authority and provenance

- PRD: `prd.json`, observed base `e5c43cd6e2`, current checkout `d4ce490f7cc1ac6257f0e98038bc52a09d3601e0`.
- Declared source plan: `docs/temp/functional-test-optimization.md`, revision `functional-test-optimization-v2`. The file is intentionally absent because `docs/temp/` is gitignored; it was not reconstructed.
- Operator replacement authority and retained Scope 10: [issuecomment-5460857653](https://github.com/portpowered/you-agent-factory/pull/2412#issuecomment-5460857653) and [issuecomment-5461024854](https://github.com/portpowered/you-agent-factory/pull/2416#issuecomment-5461024854).
- Checked-in C01-C07 inventory: `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` and `.md`, recorded source commit `ec194b5ab5d24803307b0cd8bb8895cb6d5ab9ee`, with the four package rows source-hash checked below.
- Recovered packaged migrations: [PR #2323](https://github.com/portpowered/you-agent-factory/pull/2323), [PR #2333](https://github.com/portpowered/you-agent-factory/pull/2333), and [PR #2339](https://github.com/portpowered/you-agent-factory/pull/2339). These are ancestry evidence, not new scope.

## Characterization procedure and result

| Procedure | Observed result | Property proved | Boundary |
| --- | --- | --- | --- |
| `go test ./tests/functional/factory/packaged/deep_research -list '^Test' -count=0` | Exit 0; `TestPackagedDeepResearch` listed. | The current package parent is discoverable. | `-list` does not execute runtime behavior or enumerate nested `t.Run` names. |
| `go test ./tests/functional/factory/packaged/full_flow -list '^Test' -count=0` | Exit 0; `TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion` and `TestPackagedFullFlow` listed. | Both the retained worktree declaration and shared parent are discoverable. | No runtime parity or cleanup claim. |
| `go test ./tests/functional/factory/packaged/fusion -list '^Test' -count=0` | Exit 0; `TestPackagedFusion` listed. | The current package parent is discoverable. | Nested rows were reconciled from source/inventory. |
| `go test ./tests/functional/factory/packaged/javascript_families -list '^Test' -count=0` | Exit 0; `TestPackagedJavaScriptFamilies` listed. | The current package parent is discoverable. | The Antigravity `executorProvider` subruns are execution instances, not extra inventory identities. |
| `git show 84285f9a95^1:<owned invocation file> | Select-String '^func Test'` for all four files | The pre-PR-#2333 parent `c18c823582416931b1f120ac0ef2f5167748eb28` contains 15 behavioral top-level declarations: Deep Research 5, Full Flow 4, Fusion 3, JavaScript Families 3. | Historical source is characterization evidence; it does not prove the delivered runtime. |
| Current-source row and body comparison against `84285f9a95^1`, plus `git diff --unified=0 84285f9a95^1 84285f9a95` | The 15 behavior bodies remain represented as current named children. The migration changed fixture/process/session plumbing for the grouped rows; the Full Flow worktree body remains the separate local-real edge. No assertion deletion or weakening was found. | Each pre-migration public/edge witness has a current owner; the four current parent declarations are grouping identities, not additional behavior witnesses. | Stories 002–003 must execute the delivered runtime witnesses. |

### Current source hashes

The following SHA-256 values match the C01-C07 inventory at the current
checkout:

| Package | Invocation source | SHA-256 | Shared fixture source | SHA-256 |
| --- | --- | --- | --- | --- |
| F004 | `tests/functional/factory/packaged/deep_research/invocation_test.go` | `86e6ecd239e4b99c35ff597c4bc7f1c6a0c6645baeeb78f2e5cf0506d760740a` | `tests/functional/factory/packaged/deep_research/shared_fixture_test.go` | `8387f054efaa470859ad8fe9a2e18c4ba44d830edd9f0db08d298b34e2df5a4a` |
| F007 | `tests/functional/factory/packaged/full_flow/invocation_test.go` | `7a27a878b1fda4201ca727e41bafd2c7aee97a2004a812378341fd5dfa7d3564` | `tests/functional/factory/packaged/full_flow/shared_fixture_test.go` | `d4c1ebb4f7aa309d0a6ac2761fed034c850dd4ec562238b0bfc571480f30d1cf` |
| F008 | `tests/functional/factory/packaged/fusion/invocation_test.go` | `01bb733e3d4146df5fbf5245cd9263a3cc43efa93393ff1b583c0ad6cc2e6607` | `tests/functional/factory/packaged/fusion/shared_fixture_test.go` | `8a5a32b1aea90ea68855a4b5caf1711ac97996b4973a467f37de729e77f2325d` |
| F010 | `tests/functional/factory/packaged/javascript_families/invocation_test.go` | `66fc1728fe377f49d637bacbf5fbdeae84a01f9e896510d12170b25d0d342168` | `tests/functional/factory/packaged/javascript_families/shared_fixture_test.go` | `821b096abdf3b82ccd5ac8af4626b7ee6555c21b3070ca60542948b80e25ca71` |

## Denominator reconciliation

The final inventory has exactly 19 rows in the four owned packages:

| Package | Current inventory rows | Current identities | Pre-PR-#2333 behavioral tests | Current shared-host/session shape |
| --- | ---: | --- | ---: | --- |
| F004 Deep Research | 6 | 1 parent + 5 named children | 5 | One package process/host; 5 child sessions. |
| F007 Full Flow | 5 | 1 retained worktree test + 1 parent + 3 named children | 4 | One shared host for 3 bounded children plus one separate worktree host. |
| F008 Fusion | 4 | 1 parent + 3 named children | 3 | One package process/host; 3 child sessions. |
| F010 JavaScript Families | 4 | 1 parent + 3 named children | 3 | One package process/host; 4 session instances because the Antigravity row runs two executor-provider subruns. |
| **Total** | **19** | **4 parents + 15 behavior identities** | **15** | **5 current hosts; 15 explicit non-default sessions plus 1 Full Flow default-session execution.** |

The four parent declarations are structural aggregation identities. They do not
duplicate a child assertion. The 15 named child identities map one-to-one to
the 15 pre-migration behavioral tests in the table below. This is why the
inventory denominator is 19 while the behavioral denominator is 15.

## Structural parent identities

| Final inventory row | Current selector and source | Child identities owned | Pre-migration mapping | Witness meaning |
| --- | --- | --- | --- | --- |
| `C07-F004-top-level-test-TestPackagedDeepResearch` | `^TestPackagedDeepResearch$`, `deep_research/invocation_test.go:24` | Five Deep Research rows below | No separate pre-migration behavior; old tests were top-level. | One package fixture owns the compatible Deep Research process and child cleanup. |
| `C07-F007-top-level-test-TestPackagedFullFlow` | `^TestPackagedFullFlow$`, `full_flow/invocation_test.go:145` | Three bounded Full Flow rows below | No separate pre-migration behavior; old tests were top-level. | One package fixture owns the compatible bounded Full Flow process and child cleanup. |
| `C07-F008-top-level-test-TestPackagedFusion` | `^TestPackagedFusion$`, `fusion/invocation_test.go:21` | Three Fusion rows below | No separate pre-migration behavior; old tests were top-level. | One package fixture owns the compatible Fusion process and child cleanup. |
| `C07-F010-top-level-test-TestPackagedJavaScriptFamilies` | `^TestPackagedJavaScriptFamilies$`, `javascript_families/invocation_test.go:18` | Tournament, Codex Spawn, and Antigravity Spawn rows below | No separate pre-migration behavior; old tests were top-level. | One package fixture owns the compatible JavaScript-family process and child cleanup; the Antigravity nested subruns remain within their one named row. |

## Exact 15-row behavioral map

Each row below names the current nested selector, the historical top-level
function and line in `84285f9a95^1`, the current assertion owner, and the same
observable property. “One-to-one” means the named child owns that historical
behavior; no parent row is counted as another assertion.

| Final inventory row and current selector | Historical C01/pre-migration test | Current assertion owner | Preserved observable assertion and surfaces |
| --- | --- | --- | --- |
| `C07-F004-named-scenario-TestPackagedDeepResearch-TestPackagedDeepResearchStaleNamedInvocationRefreshesThroughCustomerProcess`  \
`^TestPackagedDeepResearch/TestPackagedDeepResearchStaleNamedInvocationRefreshesThroughCustomerProcess$` | `C01-P010-TestPackagedDeepResearchStaleNamedInvocationRefreshesThroughCustomerProcess`, `deep_research/invocation_test.go:29` | `deep_research/invocation_test.go:47-91` | Stale materialization is refreshed to hyphenated names and stale names disappear; exactly one distinct backup preserves the stale payload; named and structured customer invocations reach Codex with the exact topic and complete. Surfaces: CLI, Factory Session, Work/Factory Event, filesystem, provider edge. **One-to-one.** |
| `C07-F004-named-scenario-TestPackagedDeepResearch-TestPackagedDeepResearchRequiredInputCompletes`  \
`^TestPackagedDeepResearch/TestPackagedDeepResearchRequiredInputCompletes$` | `C01-P010-TestPackagedDeepResearchRequiredInputCompletes`, `deep_research/invocation_test.go:236` | `deep_research/invocation_test.go:256-363` | Required topic completes with one primary synthesis containing topic/depth 2/maxSubagents 2/lead and both roles; three completed dispatches have technical, tradeoffs, and lead labels; three provider calls occur and lead input contains both validated specialist results. Surfaces: Factory Session, Work, Factory Event, dispatch, provider edge. **One-to-one.** |
| `C07-F004-named-scenario-TestPackagedDeepResearch-TestPackagedDeepResearchOptionalInputsReachWorkers`  \
`^TestPackagedDeepResearch/TestPackagedDeepResearchOptionalInputsReachWorkers$` | `C01-P010-TestPackagedDeepResearchOptionalInputsReachWorkers`, `deep_research/invocation_test.go:354` | `deep_research/invocation_test.go:369-482` | Optional depth 3, specialist cap 1, CODEX/gpt-5, and medium effort reach the primary result and both completed dispatch selections; the tradeoffs specialist is absent; two provider calls occur and lead input contains validated evidence. Surfaces: Factory Session, Work/Factory Event, dispatch, model/effort, provider edge. **One-to-one.** |
| `C07-F004-named-scenario-TestPackagedDeepResearch-TestPackagedDeepResearchRetriesSchemaMismatchBeforeSynthesis`  \
`^TestPackagedDeepResearch/TestPackagedDeepResearchRetriesSchemaMismatchBeforeSynthesis$` | `C01-P010-TestPackagedDeepResearchRetriesSchemaMismatchBeforeSynthesis`, `deep_research/invocation_test.go:477` | `deep_research/invocation_test.go:487-552` | Wrong specialist shape fails once, the bounded retry succeeds, and lead synthesis uses recovered evidence only; three provider calls and three dispatches are observed with initial failed, retry completed, and lead completed statuses. Surfaces: Factory Session, Work/Factory Event, dispatch, retry, provider edge. **One-to-one.** |
| `C07-F004-named-scenario-TestPackagedDeepResearch-TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome`  \
`^TestPackagedDeepResearch/TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome$` | `C01-P010-TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome`, `deep_research/invocation_test.go:554` | `deep_research/invocation_test.go:558-610` | Provider/worker failure returns FAILED with no success primary result; the durable response has a distinct session identity and exactly one failed lead dispatch with stable failure detail. Surfaces: Factory Session, Work/Factory Event, dispatch, failure, provider edge. **One-to-one.** |
| `C07-F007-top-level-test-TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion`  \
`^TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion$` | `C01-P013-TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion`, `full_flow/invocation_test.go:25` | `full_flow/invocation_test.go:26-143` | Two task files are committed and merged, `core.longpaths=true` persists, at least two implementations overlap, two planner calls show completion replan, both merge orders are accepted, two Work-request waves and two `merge-task` dispatches replay, and cursor replay returns exactly all remaining events. Surfaces: local Git/worktrees, Factory Session, Work, Factory Event, dispatch, replay, provider edge. **One-to-one; topology gap recorded below.** |
| `C07-F007-named-scenario-TestPackagedFullFlow-TestPackagedFullFlowBoundsImplementationContinueLoopAndFailsProject`  \
`^TestPackagedFullFlow/TestPackagedFullFlowBoundsImplementationContinueLoopAndFailsProject$` | `C01-P013-TestPackagedFullFlowBoundsImplementationContinueLoopAndFailsProject`, `full_flow/invocation_test.go:144` | `full_flow/invocation_test.go:158-182` | Implementation continuation exhaustion returns FAILED with a failed Work state, a finite guarded implementation-call count (2–42), and zero merges. Surfaces: Factory Session/Worker Session, Work/Factory Event, bounded failure. **One-to-one.** |
| `C07-F007-named-scenario-TestPackagedFullFlow-TestPackagedFullFlowEnforcesCallerSelectedTaskBound`  \
`^TestPackagedFullFlow/TestPackagedFullFlowEnforcesCallerSelectedTaskBound$` | `C01-P013-TestPackagedFullFlowEnforcesCallerSelectedTaskBound`, `full_flow/invocation_test.go:174` | `full_flow/invocation_test.go:184-203` | An over-budget planner wave returns FAILED after one planner call and before implementation or merge: implementations 0, merges 0. Surfaces: Factory Session/Worker Session, Work/Factory Event, caller-bound failure. **One-to-one.** |
| `C07-F007-named-scenario-TestPackagedFullFlow-TestPackagedFullFlowEnforcesCallerSelectedCycleBound`  \
`^TestPackagedFullFlow/TestPackagedFullFlowEnforcesCallerSelectedCycleBound$` | `C01-P013-TestPackagedFullFlowEnforcesCallerSelectedCycleBound`, `full_flow/invocation_test.go:198` | `full_flow/invocation_test.go:205-224` | One caller-selected cycle returns FAILED after one planner wave and exactly two merges, with no second plan. Surfaces: Factory Session/Worker Session, Work/Factory Event, caller-bound failure. **One-to-one.** |
| `C07-F008-named-scenario-TestPackagedFusion-TestPackagedFusionRequiredInputCompletes`  \
`^TestPackagedFusion/TestPackagedFusionRequiredInputCompletes$` | `C01-P014-TestPackagedFusionRequiredInputCompletes`, `fusion/invocation_test.go:27` | `fusion/invocation_test.go:38-94` | Required input returns COMPLETED with one refined primary containing the controlled worker outcome and not the raw input; two accepted dispatches occur in `draft-fusion`, then `refine-fusion` order. Surfaces: Factory Session, Work/Factory Event, dispatch, provider edge. **One-to-one.** |
| `C07-F008-named-scenario-TestPackagedFusion-TestPackagedFusionOptionalInputsReachWorkers`  \
`^TestPackagedFusion/TestPackagedFusionOptionalInputsReachWorkers$` | `C01-P014-TestPackagedFusionOptionalInputsReachWorkers`, `fusion/invocation_test.go:94` | `fusion/invocation_test.go:100-182` | Provider/model/effort overrides select Claude/claude-sonnet-4-20250514/low for the drafter and Codex/gpt-5/high for the refiner; two dispatches preserve order, two provider requests carry those selections, and agent-run summaries retain low/high effort. Surfaces: Factory Session, Work/Factory Event, dispatch, model/effort, provider edge. **One-to-one.** |
| `C07-F008-named-scenario-TestPackagedFusion-TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome`  \
`^TestPackagedFusion/TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome$` | `C01-P014-TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome`, `fusion/invocation_test.go:189` | `fusion/invocation_test.go:188-261` | Provider failure returns FAILED with no primary and `task:failed`; retained events contain only failed `draft-fusion` dispatches with public error detail and no `refine-fusion` dispatch. Surfaces: Factory Session, Work/Factory Event, dispatch, failure. **One-to-one.** |
| `C07-F010-named-scenario-TestPackagedJavaScriptFamilies-TestPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner`  \
`^TestPackagedJavaScriptFamilies/TestPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner$` | `C01-P016-TestPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner`, `javascript_families/invocation_test.go:18` | `javascript_families/invocation_test.go:31-75` | Tournament succeeds with a candidate plus decision trail/rationale; exactly three Codex requests use `operator-js-model`, and the judge request contains the candidates. Surfaces: CLI-renderable result, Work/Factory Event, provider edge. **One-to-one.** |
| `C07-F010-named-scenario-TestPackagedJavaScriptFamilies-TestPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner`  \
`^TestPackagedJavaScriptFamilies/TestPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner$` | `C01-P016-TestPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner`, `javascript_families/invocation_test.go:61` | `javascript_families/invocation_test.go:77-117` | Spawn returns exactly `merged travel answer`; exactly four Codex requests represent planner, two children, and merger; merger input preserves index 1/2 order and both child findings. Surfaces: CLI-renderable result, Work/Factory Event, fanout/fan-in, provider edge. **One-to-one.** |
| `C07-F010-named-scenario-TestPackagedJavaScriptFamilies-TestPackagedSpawnRunsAntigravityChildrenWithExactModel`  \
`^TestPackagedJavaScriptFamilies/TestPackagedSpawnRunsAntigravityChildrenWithExactModel$` | `C01-P016-TestPackagedSpawnRunsAntigravityChildrenWithExactModel`, `javascript_families/invocation_test.go:101` | `javascript_families/invocation_test.go:120-156`, nested execution at line 125 | Both executor-provider modes succeed with the merged Antigravity answer; each run makes planner/task/merger requests using `agy` and the exact `gemini-3.6-flash-medium` model without a separate effort argument. Surfaces: CLI-renderable result, Work/Factory Event, model selection, provider edge. **One-to-one; two runtime subruns remain within this one inventory row.** |

The C01 JSON currently labels the Deep Research, Full Flow bounded, Fusion,
and JavaScript historical identities as “retired” when their declaration
identity changed during the grouped migration. That is an inventory identity
disposition, not evidence that the behavior was removed: the exact one-to-one
child mapping above is the C11 reconciliation.

## Process, session, and timing topology

### Current five-host observation

The current source has four package-owned `BuildProcessWithContext` calls and
one additional `StartFunctionalAPIServer` caller:

| Host | Source evidence | Current sessions/rows | Current status |
| --- | --- | --- | --- |
| Deep Research | `deep_research/shared_fixture_test.go:269-325` builds one process, injects the API starter, and starts one continuous host; `deepResearchExpectedSessions = 5` at line 27. | 5 explicit non-default child sessions; 6 inventory rows including parent. | Already on the package host; Story 002 preserves it. |
| Full Flow shared host | `full_flow/shared_fixture_test.go:265-305` builds one process and starts one continuous host; `fullFlowExpectedSessions = 3` at line 28. | 3 explicit non-default child sessions; 4 inventory rows including parent. | Already on the package host; Story 003 extends its session ledger to the worktree row. |
| Full Flow worktree host | `full_flow/invocation_test.go:26-35` calls `support.StartFunctionalAPIServer` directly. `invokeFullFlow` posts to `factorysessions.DefaultSessionID` at line 73. | One separate default-session execution; its local-real repository/worktree/merge witness is otherwise intact. | Remaining eligible migration caller; Story 003 owns it. |
| Fusion | `fusion/shared_fixture_test.go:269-324` builds one process and starts one continuous host; `fusionExpectedSessions = 3` at line 27. | 3 explicit non-default child sessions; 4 inventory rows including parent. | Already on the package host; Story 002 preserves it. |
| JavaScript Families | `javascript_families/shared_fixture_test.go:271-334` builds one process and starts one continuous host; `javascriptFamiliesExpectedSessions = 4` at line 27. | 4 session instances for 3 named rows because Antigravity runs `executorProvider=""` and `SCRIPT_WRAP`; 4 inventory rows including parent. | Already on the package host; Story 002 preserves it. |
| **Total** | **4 shared fixture process builds + 1 separate Full Flow server** | **5 hosts; 15 explicit non-default sessions + 1 default-session execution** | **Current topology is characterized, not yet repaired.** |

The target is four package hosts and 16 distinct non-default scenario sessions:

| Package host | Target scenario-session count | Target row coverage |
| --- | ---: | --- |
| Deep Research | 5 | Five named behavior rows. |
| Full Flow | 4 | Worktree/merge plus three bounded rows, all explicit and scenario-private. |
| Fusion | 3 | Three named behavior rows. |
| JavaScript Families | 4 | Tournament, Codex Spawn, and two Antigravity executor-provider instances. |
| **Total** | **16** | **15 named behavior rows plus the Antigravity second execution instance.** |

The recovered C03 timing map is retained as historical directional context,
not as a C11 pass/fail threshold:

| Package | Recovered pre-migration median seconds | Environment |
| --- | ---: | --- |
| Deep Research | 14.884 | `go1.25.0-windows-amd64-cpu24-goamd64-v1` |
| Full Flow | 20.169 | `go1.25.0-windows-amd64-cpu24-goamd64-v1` |
| Fusion | 7.928 | `go1.25.0-windows-amd64-cpu24-goamd64-v1` |
| JavaScript Families | 10.531 | `go1.25.0-windows-amd64-cpu24-goamd64-v1` |

These values are from the recovered [PR #2333 characterization](https://github.com/portpowered/you-agent-factory/pull/2333). C11 PERF evidence must collect delivered package timings on its own head; no portable wall-clock or quiet-host threshold is claimed here.

## Mismatch and repair register

| ID | Exact identity/property | Impact | Owner and smallest repair delta | Status |
| --- | --- | --- | --- | --- |
| `C11-GAP-F007-WORKTREE-HOST` | `C07-F007-top-level-test-TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion`; current line 31 calls `StartFunctionalAPIServer`, line 73 invokes `~default`, and no `fullFlowLifecycleLedger` registration occurs for this row. | The row still uses a second application host, lacks the target explicit non-default session witness, and is not included in the shared fixture’s `expected=3` cleanup count. Its local-real Git/worktree, overlap, merge, Work, dispatch, and replay assertions remain present. | Story 003: construct the scenario through `newFullFlowSharedFixture`/`newScenario`, preserve its private repository/worktrees and runner routing, invoke through the scenario session, register/close the fourth session, and retain every current assertion. | **Witnessed; no silent repair in Story 001.** |
| `C11-NOTE-PARENT-ROWS` | Four current parent declarations did not exist as separate pre-migration top-level tests. | Counting them as new behavior would duplicate the 15-test behavioral denominator. | Ledger convention: parent rows are structural identities only; their child mappings are explicit above. | **Resolved by characterization.** |
| `C11-NOTE-JS-SUBRUNS` | The Antigravity behavior row contains two nested `executorProvider` executions at `javascript_families/invocation_test.go:125`. | Counting the nested executions as inventory rows would change the 19-row denominator. | Keep one inventory identity and record the two target session instances in topology/performance evidence. | **Resolved by characterization.** |

## Scope and ancestry characterization

At the starting checkout:

- `git merge-base --is-ancestor cca0cdc4ef14a054e41d1c0ffead82900d1e4b53 HEAD` exited 0: PR #2323 merge is an ancestor.
- `git merge-base --is-ancestor 84285f9a956b3f97d3e5cc64111a4635d21fc7aa HEAD` exited 0: PR #2333 merge is an ancestor.
- `git merge-base --is-ancestor 56eed9d6f23f2255bf5a021aa663b8b6387ea943 HEAD` exited 0: PR #2339 merge is an ancestor.
- `HEAD`, `origin/main`, and the worktree branch initially resolved to `d4ce490f7cc1ac6257f0e98038bc52a09d3601e0`; no branch-only implementation history was imported.
- Story 001 changes only this operator-authorized ledger and ignored scaffolding (`prd.json`, `progress.txt`). No shared support, inventory, baseline, public contract, generated file, production code, packaged asset, CI, Makefile, UI, or sibling package is in scope.

The complete three-dot diff, final delivered ancestry, runtime cleanup, and
clean-room verdict belong to Story 004 and are not claimed by this
characterization ledger.

## Story 001 acceptance outcome

| Criterion | Result | Evidence |
| --- | --- | --- |
| Exactly 19 unique current identities map to all 15 pre-migration behavioral tests with no missing, duplicate, or weakened witness | **PASS** | Inventory counts 6+5+4+4; source enumeration lists all 15 historical functions and all 15 current child selectors; parent rows are explicitly non-behavioral. |
| Current five-host topology and target four-host/16-session topology | **PASS** | Four shared fixture construction/start paths plus one direct Full Flow server; expected-session constants and Full Flow default-session call are recorded with source lines. |
| Any mismatch is exact, impact-scoped, and assigned without silent repair | **PASS** | `C11-GAP-F007-WORKTREE-HOST` assigns the smallest migration delta to Story 003; no package code changed in Story 001. |

### Evidence boundary

This story establishes the executable spine and the repair contract. It does
not prove delivered-head runtime parity, failure/recovery behavior, cleanup
absence, race safety, package timings, complete scope, or clean-room behavior;
those edges remain assigned to `PARITY-RESEARCH`, `FAILURE-RESEARCH`,
`RECOVERY-RESEARCH`, `PARITY-FLOW`, `REPLAY-FLOW`, `CLEAN-C11`, `RACE-C11`,
`PERF-C11`, `SCOPE-C11`, and `VAL-C11` in the PRD.
