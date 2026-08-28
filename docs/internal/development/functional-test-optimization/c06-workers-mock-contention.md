# C06 workers/mock contention evidence

## Decision

**Story:** `functional-test-optimization-c06-workers-mock-contention-001`

**Behavior lane:** BEH-001 — evidence-backed topology decision

**Decision:** **PASS — no functional-test rewrite.**

The accepted PR #2336 topology is present at the current source. There is no
eligible repeated application start left to remove. The remaining process work
is required by the 15 real executable witnesses and the shared root/session and
invocation-local lifecycle boundaries. The current 30.795–32.271 second
package results come from the `jobs=2`, 146-package Backend Functional
Coverage profile; this artifact attributes the residual cost to required-edge
fidelity under that profile, including suite contention, rather than to a new
package-owned migration opportunity.

This artifact is intentionally evidence-only. It changes no functional test,
shared support, production, CI, contract, generated, baseline, or configuration
file.

## Evidence inputs and identity

| Input | Evidence |
| --- | --- |
| Accepted change | [PR #2336](https://github.com/portpowered/you-agent-factory/pull/2336), accepted final head `50b6e6af85ce23140e09cb0fddba48e09bbce7ca`, merge commit `a1f3a44125c43ba7d4c05025396c9d5e1561a2e3` |
| Current checkout | `67710223e327d02c0de93a6ad826c754fe5c1702`; `origin/main` and this branch point at this commit before the artifact change |
| Current source comparison | `git diff --exit-code 50b6e6af85ce23140e09cb0fddba48e09bbce7ca..HEAD -- tests/functional/workers/mock` exited `0` with no output |
| Inventory | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md` and `.json`, P002 |
| Current discovery | `go test ./tests/functional/workers/mock -list '^Test'` exited `0`; four default top-level tests were emitted: the three retained built-CLI rows and `TestSharedProcessWorkersMock` |
| CI diagnostic 1 | [run 33140823961](https://github.com/portpowered/you-agent-factory/actions/runs/33140823961), `functional-test-diagnostics` artifact `9674040193`; artifact files were downloaded and compared locally |
| CI diagnostic 2 | [run 33139122545](https://github.com/portpowered/you-agent-factory/actions/runs/33139122545), `functional-test-diagnostics` artifact `9673395254`; artifact files were downloaded and compared locally |
| Parent plan availability | The PRD names `docs/temp/functional-test-optimization.md`, but that path is absent in this checkout. The PRD, C01 P002 artifact, accepted PR description/evidence, and current source provide the complete scoped claims used here; no conflicting topology or timing claim was found. |

The downloaded diagnostic files are not committed. Their SHA-256 identities
were:

| Run | `command.log` | `functional-timing-summary.json` |
| --- | --- | --- |
| 33140823961 | `77AC4A35974A1858BC21844D3F7B2570963BE4D66A6692F0843A71795A2E3FE4` | `3E3A0DFF674DDE18E9F459B3C1FFBD7CADF4B8AA8DA4C5F1E28F5238CE5D79A0` |
| 33139122545 | `126DA4C4BB4251EFDAE3190101B7C92534324754B161EB32BAA8BD7EEA5911D2` | `2FD1E799C802F8D58E634C3A1E3A3DE30A9F95031346E8D40C2A77B91C9015BD` |

The second run's two files were read and their package values agree with the
command log. The hashes identify the downloaded evidence; the run ID and
artifact ID remain the reuse keys for both records.

## Reconciliation of all 22 P002 rows

C01 P002 records 22 pre-migration top-level rows: 19
`shareable-with-mock` rows and three `isolated-with-reason` built-CLI rows.
The accepted PR moved the 19 eligible rows under the shared parent while
retaining the three executable witnesses at top level. The current source table
at `tests/functional/workers/mock/shared_process_test.go:60-78` contains the
same 19 stable behaviors.

| # | C01 P002 row | Current owner/source | Current topology and process reason |
| ---: | --- | --- | --- |
| 1 | `TestNamedAgyMockPreservesDispatchMetadataAndCompletionLog` | `TestSharedProcessWorkersMock/NamedAgy`; `agy_named_mock_test.go:21` | Shared root; one explicit Factory Session because public Work, dispatch metadata, and completion log are observed through the hosted session. |
| 2 | `TestExpectedArtifactsEnforceThroughRootBuildProcess` | `TestSharedProcessWorkersMock/ExpectedArtifacts`; `artifact_registry_test.go:35` | Shared root; one explicit Factory Session because artifact enforcement is observed through the public Work result. |
| 3 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome` | Top-level `batch_invocation_exit_codes_test.go:36` | Isolated with reason `executable-selection`; five `exec.CommandContext` invocations preserve built executable selection, stdout/stderr, and OS exit status. |
| 4 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses` | Top-level `batch_invocation_exit_codes_test.go:78` | Isolated with reason `executable-selection`; seven `exec.CommandContext` invocations preserve aggregate failure output and OS exit status. |
| 5 | `TestScriptWorkerClassifierRoutesWithoutModelCalls` | `TestSharedProcessWorkersMock/ScriptClassifier`; `classifier_test.go:26` | Shared root; one explicit Factory Session and injected script edge preserve classifier routing and zero model calls. |
| 6 | `TestJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren` | `TestSharedProcessWorkersMock/JavaScriptLiveCapacity`; `live_capacity_javascript_test.go:25` | Shared root; one explicit durable Factory Session preserves capacity admission and child completion. |
| 7 | `TestLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch` | `TestSharedProcessWorkersMock/LiveCapacityIncrease`; `live_capacity_test.go:53` | Shared root; one explicit Factory Session preserves waiting dispatch admission and the resource-set CLI observation. |
| 8 | `TestLiveResourceCapacityReductionPreservesActiveWork` | `TestSharedProcessWorkersMock/LiveCapacitySafeReduction`; `live_capacity_test.go:119` | Shared root; one explicit Factory Session preserves active Work while capacity is reduced legally. |
| 9 | `TestLiveResourceCapacityRejectsReductionBelowActiveUse` | `TestSharedProcessWorkersMock/LiveCapacityUnsafeReduction`; `live_capacity_test.go:156` | Shared root; one explicit Factory Session preserves typed capacity conflict and unchanged Work. |
| 10 | `TestLiveResourceCapacityRecordingReplayAndCursor` | `TestSharedProcessWorkersMock/LiveCapacityRecording`; `live_capacity_test.go:223` | Shared root; one explicit Factory Session preserves recording, replay, cursor, no-op, stale, and not-found observations. |
| 11 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot` | Top-level `named_invocation_exit_codes_test.go:23` | Isolated with reason `executable-selection`; three `exec.CommandContext` invocations preserve named-run stdout/stderr and OS exit status. |
| 12 | `TestPlainBatchDrainReportsStrandedWork` | `TestSharedProcessWorkersMock/PlainBatchDrainReportsStrandedWork`; `plain_batch_drain_test.go:62` | Serialized no-server invocation-local default session; the public local CLI has no Factory Session selector, and `Process.Execute` completion is the required incomplete-drain witness. |
| 13 | `TestPlainBatchDrainPreservesFiniteAndContinuousCounterexamples` | `TestSharedProcessWorkersMock/PlainBatchDrainCounterexamples`; `plain_batch_drain_test.go:100` | Serialized no-server invocation-local default session; finite completion and continuous-idle/cancellation are distinct liveness witnesses. |
| 14 | `TestPlainBatchDrainRejectsCancellationBeforeRuntimeActivation` | `TestSharedProcessWorkersMock/PlainBatchDrainRejectsPreActivationCancellation`; `plain_batch_drain_test.go:169` | Serialized no-server invocation-local default session; the session-ID edge must observe cancellation before runtime activation. |
| 15 | `TestPlainBatchDrainStopsAfterWorkerActivationCancellation` | `TestSharedProcessWorkersMock/PlainBatchDrainStopsAfterWorkerActivationCancellation`; `plain_batch_drain_test.go:193` | Serialized no-server invocation-local default session; the input-directory edge must observe post-activation cancellation and joined cleanup. |
| 16 | `TestMockWorkersReplaceOnlyNamedChildren` | `TestSharedProcessWorkersMock/MockWorkersReplace`; `replacement_test.go:44` | Shared root; one explicit Factory Session and injected command edges preserve named replacement and passthrough routing. |
| 17 | `TestUnknownWorkerOverrideFailsActionably` | `TestSharedProcessWorkersMock/UnknownWorker`; `replacement_test.go:85` | Shared root; one explicit Factory Session preserves fail-closed invalid override behavior and zero provider calls. |
| 18 | `TestFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved` | `TestSharedProcessWorkersMock/FutureFields`; `replacement_test.go:149` | Shared root; one explicit Factory Session preserves forward-compatible field handling and dispatch. |
| 19 | `TestMockWorkerFailureReturnsStablePublicFailure` | `TestSharedProcessWorkersMock/MockWorkerFailure`; `replacement_test.go:176` | Shared root; one explicit Factory Session preserves rejection, configured gate timeout, stable failure, and next-session usability. |
| 20 | `TestMockWorkerSelectedThroughCustomerProcess` | `TestSharedProcessWorkersMock/RootSelection`; `root_build_test.go:24` | Shared root; one explicit Factory Session preserves customer-process mock selection through `Process.Execute`. |
| 21 | `TestServiceConfigOverrideAlignment_CustomerProcessSharesScriptAndProviderCommandRunner` | `TestSharedProcessWorkersMock/ServiceConfigAlignment`; `service_config_alignment_helpers_test.go:15` | Shared root; one explicit Factory Session and injected runner edges preserve script/provider alignment. The `functionallong` file is not part of default discovery. |
| 22 | `TestMockWorkerUsageIsVisibleAndPriceableThroughPublicCLI` | `TestSharedProcessWorkersMock/MockUsage`; `usage_costs_test.go:23` | Shared root; one explicit Factory Session preserves public usage and price observations. |

The resulting count is exact: 15 hosted rows (1–2, 5–10, and 16–22), four
serialized invocation-local rows (12–15), and three isolated built-CLI rows
(3–4 and 11). Therefore `15 + 4 + 3 = 22`, with no lost or newly invented
behavior row.

## Exact process/start classification

The source inspection covered `StartFunctionalAPIServer`, `BuildProcess`,
`Process.Execute`, `StartProcessCommand`, `exec.CommandContext`, `sync.Once`,
and all package test declarations. Every process-sensitive path has a reason:

| Start or boundary | Count in the accepted/current topology | Source witness | Exact reason | Classification |
| --- | ---: | --- | --- | --- |
| Root-built hosted application | 1 package-scoped root/process group | `shared_process_test.go:209-261`, especially `StartFunctionalAPIServer` at line 247 | Fifteen hosted rows require a public hosted endpoint and explicit Factory Session selection. The same root also owns the shared CLI execution path for the four local lifecycle rows. Rebuilding per row would be repeated work already removed by PR #2336. | Required shared start; eligible and not repeatable per row |
| Explicit Factory Sessions | 15 scenario scopes | `shared_process_test.go:339-425` and the first 15 eligible row owners above | Session-scoped Work, Factory Events, dispatch, durable state, capacity, usage, and deletion observations are the public isolation witness. | Required public state boundary |
| Invocation-local `Process.Execute` | 4 serialized row scopes | `shared_process_test.go:282-330`, `plain_batch_drain_test.go:62-216` | The no-server command has no public Factory Session selector. Unique Factory, request/trace, selector, gate, HOME, temporary-path, and runtime identities plus joined completion/cancellation provide the only valid isolation boundary. | Required no-server exception; no additional root |
| Asynchronous `StartProcessCommand` wrapper | 2 source call sites, covering the stranded and continuous-idle command observations | `plain_batch_drain_test.go:74` and `:138`; support contract `process.go:542-570` | `Done`, `Err`, and `Stop` are needed to prove incomplete drain, negative liveness, and deterministic cancellation cleanup. This starts a goroutine over the existing reusable process; it does not construct another application graph or OS executable. | Required invocation lifecycle boundary |
| Built CLI compilation | 1 test-owned binary build | `compiled_cli_exit_codes_helpers_test.go:68-87`, guarded by `compiledCLIBinary.once` | A real `./cmd/factory` executable is a prerequisite for executable selection and OS exit assertions. `sync.Once` prevents three top-level rows from repeating the build. | Required local-real setup, once per package |
| Built CLI subprocesses | 15 total: 5 in row 3, 7 in row 4, 3 in row 11 | `compiled_cli_exit_codes_helpers_test.go:90-116` and the three top-level test files | Injected edges cannot cross the OS boundary. Each invocation must preserve executable selection, captured stdout/stderr, and the operating-system exit status; these are the witness, not redundant application construction. | Required isolated process edge |

No other `exec.CommandContext`, built-binary invocation, root build, or server
construction exists in the default `tests/functional/workers/mock` source beyond
the rows and helpers classified above. The build-tagged `functionallong` test is
excluded from the default package command and is not a C06 row.

## Accepted versus current timing

### Accepted PR #2336 post-migration evidence

The accepted PR records three post-migration package samples on its final head:

| Sample | Package seconds | Profile |
| ---: | ---: | --- |
| 1 | 44.626 | Accepted PR #2336 local post-migration sample |
| 2 | 50.759 | Accepted PR #2336 local post-migration sample; median |
| 3 | 53.417 | Accepted PR #2336 local post-migration sample |

The accepted median is `50.759s`, with spread
`(53.417 - 44.626) / 50.759 = 17.319%`. The accepted evidence also records
one shared `StartFunctionalAPIServer` root for 19 eligible rows, one package
local binary, and 15 built-CLI subprocess starts.

### Current jobs=2 / 146-package diagnostics

Both required diagnostic files report the same suite profile and a complete
package result. `command.log` contains the exact `jobs=2` and
`selected-packages=146` lines; `functional-timing-summary.json` independently
reports the package value, counts, and outcome.

| Run | Jobs / selected packages | Package seconds | Suite wall seconds | Package elapsed sum | Tests | Outcome |
| --- | --- | ---: | ---: | ---: | --- | --- |
| [33140823961](https://github.com/portpowered/you-agent-factory/actions/runs/33140823961) | 2 / 146 | 30.795 | 476.876 | 649.196 | 1101 = 1100 pass + 1 skip | pass |
| [33139122545](https://github.com/portpowered/you-agent-factory/actions/runs/33139122545) | 2 / 146 | 32.271 | 482.142 | 659.519 | 1101 = 1100 pass + 1 skip | pass |

The package timing values are 39.3% and 36.4% below the accepted median,
respectively, but the profiles are not identical and this is not claimed as a
universal performance improvement. The useful evidence is topology parity:
the current source has no eligible repeated root/application start, while the
current CI result includes the required real executable edges in the saturated
146-package schedule. The residual cost is consequently recorded as required
edge fidelity plus schedule contention, with no test rewrite or CI-policy
change proposed.

## EVID-001 result and boundaries

| Property | Result | Evidence | Not proved |
| --- | --- | --- | --- |
| Accepted/current topology parity | **PASS** | Accepted PR description/final head, C01 P002, current source table, and empty accepted-to-current package diff | Runtime execution on this C06 head |
| All 22 rows reconciled | **PASS** | The 22-row table above: 15 hosted, four invocation-local, three built-CLI | Full MATRIX-001 through MATRIX-037 behavior gate |
| Every remaining process-sensitive start classified | **PASS** | Exact-start table and source references above; no eligible repeated start remains | Exhaustive process scheduling/interleavings |
| Current CI-profile comparison | **PASS** | Both downloaded `command.log` and `functional-timing-summary.json` files agree on jobs, package count, package timing, counts, and pass outcome | This PR's future PR-CI-001 result |
| Rewrite decision | **PASS** | No source discrepancy or eligible repeated start was found | A future delta if new source or CI evidence identifies repeated eligible work |

The source-plan path discrepancy is retained for provenance. It is not treated
as a topology conflict because the requested claims are independently present
in the PRD, C01 artifact, accepted PR evidence, and current source. If a later
authority supplies a plan statement that conflicts with this record, the next
implementation iteration must stop and request the smallest plan delta before
editing tests.

## Scope, cost, and unproven edges

- Product/provider remote calls: `0`; paid calls: `0`; cost: `$0`.
- GitHub reads/downloads were limited to the explicitly requested PR and two
  diagnostic artifacts; no customer payloads or secrets were accessed or
  recorded.
- The artifact owns no production, contract, configuration, generated, UI,
  shared-support, baseline, CI, or stability-cleanup change.
- Parent-project Acceptance Criteria 1, 3, and 5 remain explicitly unclaimed
  by this C06 slice.
- `PKG-001`, `REPEAT-001`, supported `RACE-001`, `CLEAN-001`, `PR-CI-001`, and
  `VAL-001` remain later story/gate evidence. This artifact proves only
  EVID-001's accepted/current topology and CI-profile comparison.

## Reproducible procedure

The evidence was collected on 2026-08-28 from the current checkout:

```text
gh pr view 2336 --json number,state,title,headRefName,baseRefName,mergeCommit,url
gh pr diff 2336 --name-only
git diff --exit-code 50b6e6af85ce23140e09cb0fddba48e09bbce7ca..HEAD -- tests/functional/workers/mock
go test ./tests/functional/workers/mock -list '^Test'
gh run download 33140823961 --name functional-test-diagnostics --dir <temporary-run-directory>/33140823961
gh run download 33139122545 --name functional-test-diagnostics --dir <temporary-run-directory>/33139122545
```

The two downloaded `command.log` files were searched for the suite header,
`selected-packages=146`, and the workers/mock package timing. The two
`functional-timing-summary.json` files were parsed for `wallSeconds`,
`packageElapsedSecondsSum`, package count, test counts, and the workers/mock
package object. The command and JSON observations agree for both runs.
