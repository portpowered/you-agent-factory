# Workers/mock witness compression baseline evidence

Status: `PASS` for `workers-mock-witness-compression-001` (baseline
characterization only). The package topology is unchanged by this story.
Stories 002 and 003 own migration parity, post-migration count, race safety,
directional performance, clean-room validation, and PR delivery.

## Scope and evidence boundary

| Item | Evidence |
| --- | --- |
| Parent behavior | `BEH-001` — efficient, behavior-complete workers/mock witness topology |
| Story | `workers-mock-witness-compression-001` |
| Frozen source head | `47285a02c116394b0bdf3078cdd59909dc12e825` |
| Source-plan authority | `prd.json` / `prd.md`; `operatorAmendment` is absent |
| Initial progress state | `progress.txt` was absent at iteration start |
| Scope changed | Evidence ledger only; no workers/mock Go source, production code, contract, generated, UI, shared-support, CI, or guarded binary-fixture line changed |
| Dependency fidelity | `local_real` for the built CLI boundary; controlled mock/script edges inside the package |
| Remote or paid calls | None; `$0` |

This document is the story's characterization artifact. It records the exact
current values and intended ownership before structural migration. It does not
claim post-migration behavior, race freedom, a performance improvement, or
terminal CI.

## Environment and source identity

The three package samples were launched as separate PowerShell processes from
the frozen worktree on 2026-08-29:

```text
Go: go1.25.0 windows/amd64
OS: Microsoft Windows 11 Home 10.0.26200 (64-bit)
PowerShell: 7.6.5
Logical processors: 24
GOMAXPROCS: unset
GOFLAGS: unset
FUNCTIONAL_DEFAULT_JOBS: unset
CI: unset
GOCACHE: C:\Users\andre\AppData\Local\go-build
GOMOD: C:\Users\andre\work\portos\infinite-you\.claude\worktrees\workers-mock-witness-compression\go.mod
```

Relevant source SHA-256 values at the frozen head:

```text
5E42ED1A8BD90E52FC51F4886E11D22E9528B3E48B48F4BF638CCA5F7F9C1ED8  tests/functional/workers/mock/batch_invocation_exit_codes_test.go
50B05A562B3877AE31ECAAA7083E11C231480B68FCA79092D7BE9C4D69479839  tests/functional/workers/mock/named_invocation_exit_codes_test.go
54E0919D3D9AB235E14CACE45CD8CFF9E542F3D7CEF30D1EBA1E2F59D6F8711A  tests/functional/workers/mock/compiled_cli_exit_codes_helpers_test.go
E4FD04C3BAEE809B626C8DEB9AA5BBEC918422DA644179D27A3BD18994A8A4D2  tests/functional/workers/mock/shared_process_test.go
```

## Current 15-row behavior matrix

The package discovery command was:

```text
go test ./tests/functional/workers/mock/... -list '^Test'
```

It exited 0 and listed five top-level declarations. The unrelated
`TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` is retained as
an existing shared-process package test but is not one of this lane's 15
compiled-CLI rows. The 15 rows below are the dynamic children of the three
compiled-CLI declarations. A structural parent is not counted as a behavior
row.

| ID | Current selector and source | Current helper / boundary | Intended disposition | Boundary-fidelity reason |
| --- | --- | --- | --- | --- |
| WM-01 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/success quiet result exits zero`; `batch_invocation_exit_codes_test.go:43-45` | `runCompiledBatchQuietSuccess`; `:129-141`; built `you` subprocess | Retain | Exact quiet stdout/stderr and operating-system success exit are the canonical batch success boundary. |
| WM-02 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/success default policy keeps result`; `batch_invocation_exit_codes_test.go:48-58` | `runCompiledBatchSuccess(policy=default)`; `:143-159`; built `you` subprocess | Migrate | Only returned success and a sentence in stdout are asserted; no OS-only value is required. |
| WM-03 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/success verbose policy keeps result`; `batch_invocation_exit_codes_test.go:48-58` | `runCompiledBatchSuccess(policy=verbose)`; `:143-159`; built `you` subprocess | Migrate | Verbose output is asserted only for the same successful result sentence; shared CLI execution preserves the value. |
| WM-04 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/failed terminal Work exits nonzero with human detail`; `batch_invocation_exit_codes_test.go:62-65` | `runCompiledBatchHumanFailure`; `:161-181`; built `you` subprocess | Retain | This is the canonical human failure with returned error, parent exit, stdout markers, and stderr at the OS boundary. |
| WM-05 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/failed terminal Work JSON is parseable`; `batch_invocation_exit_codes_test.go:67-70` | `runCompiledBatchJSONFailure`; `:183-203`; built `you` subprocess | Retain | The canonical single-failure JSON report is paired with parent error and nonzero OS exit. |
| WM-06 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/all submitted Work failures are reported deterministically`; `batch_invocation_exit_codes_test.go:85-88` | `runCompiledBatchAllFailedHuman`; `:205-222`; built `you` subprocess | Migrate | Human aggregation checks returned error and rendered Work/state values, not an OS-only distinction. |
| WM-07 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/all submitted Work failures have a complete JSON collection`; `batch_invocation_exit_codes_test.go:90-93` | `runCompiledBatchAllFailedJSON`; `:224-248`; built `you` subprocess | Retain | The canonical aggregate JSON witness keeps parent error/exit, collection cardinality, ordering, and failure fields together at the OS boundary. |
| WM-08 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/mixed success and failure does not round to success`; `batch_invocation_exit_codes_test.go:95-98` | `runCompiledBatchMixed`; `:250-270`; built `you` subprocess | Migrate | The property is typed partial-completion aggregation and JSON shape; the parent OS exit is not required after the retained failure witness. |
| WM-09 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/circuit breaker reports reason in human output`; `batch_invocation_exit_codes_test.go:100-110` | `runCompiledBatchCircuitBreaker(jsonOutput=false)`; `:272-311`; built `you` subprocess | Migrate | Breaker text is produced by controlled retryable script failure; the shared process can preserve Work/state/reason without parent OS semantics. |
| WM-10 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/circuit breaker reports reason in JSON output`; `batch_invocation_exit_codes_test.go:100-110` | `runCompiledBatchCircuitBreaker(jsonOutput=true)`; `:272-311`; built `you` subprocess | Migrate | JSON breaker shape and exact reason are value properties; the script edge and shared CLI retain them. |
| WM-11 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/script non-zero exit reports reason in human output`; `batch_invocation_exit_codes_test.go:114-124` | `runCompiledBatchScriptFailure(jsonOutput=false)`; `:313-347`; built `you` subprocess | Migrate | The asserted parent output is the normalized script failure, while the child script itself remains a controlled edge. |
| WM-12 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/script non-zero exit reports reason in JSON output`; `batch_invocation_exit_codes_test.go:114-124` | `runCompiledBatchScriptFailure(jsonOutput=true)`; `:313-347`; built `you` subprocess | Migrate | JSON failure fields and script reason do not require the parent process boundary. |
| WM-13 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/success preserves primary result`; `named_invocation_exit_codes_test.go:30-55` | Inline named success assertions; `:30-55`; built `you` subprocess | Retain | Exact named quiet stdout/stderr and operating-system success exit are the canonical named success witness. |
| WM-14 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/terminal failure preserves human detail`; `named_invocation_exit_codes_test.go:57-84` | Inline named human failure assertions; `:57-84`; built `you` subprocess | Migrate | Human response-stream status, Work name, and state are observable through the shared process; parent exit is not the retained property. |
| WM-15 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/terminal failure preserves JSON detail`; `named_invocation_exit_codes_test.go:86-118` | Inline named JSON failure assertions; `:86-118`; built `you` subprocess | Retain | The one parseable terminal response plus named parent error/nonzero exit is the canonical JSON named boundary. |

The classification is exactly six retained rows (`WM-01`, `WM-04`, `WM-05`,
`WM-07`, `WM-13`, `WM-15`) and nine migration rows (`WM-02`, `WM-03`,
`WM-06`, `WM-08`, `WM-09`, `WM-10`, `WM-11`, `WM-12`, `WM-14`).

## Count procedure and current result

The dynamic count is based on executions of the test-owned target binary, not
the Go test process, the reusable in-process customer process, or the compiler:

| Source path | Static call sites | Dynamic scenario executions | Current interpretation |
| --- | ---: | ---: | --- |
| Single-Work batch parent, `batch_invocation_exit_codes_test.go:43-70` | 4 call sites | 5 | Quiet success, default success, verbose success, human failure, JSON failure |
| Aggregation batch parent, `batch_invocation_exit_codes_test.go:85-126` | 5 call sites | 7 | Human/JSON all-failed, mixed, human/JSON breaker, human/JSON script failure |
| Named parent, `named_invocation_exit_codes_test.go:30-118` | 3 call sites | 3 | Named success, human failure, JSON failure |
| **Scenario target-binary total** | **12** | **15** | **Baseline denominator** |

`runBuiltYouBinary` is the target-binary edge at
`compiled_cli_exit_codes_helpers_test.go:96-121`. The three parent calls to
`buildYouBinary` at `batch_invocation_exit_codes_test.go:41,83` and
`named_invocation_exit_codes_test.go:28` share one `sync.Once` at
`compiled_cli_exit_codes_helpers_test.go:74-89`; therefore they execute one
`go build -o <temp>/you[.exe] ./cmd/factory` compiler child. That compiler
child is recorded separately and is not a scenario witness.

| Stricter accounting unit | Baseline count | Why it is separate |
| --- | ---: | --- |
| Target `you` scenario children | 15 | Each crosses the OS boundary and runs one matrix row. |
| Test-owned compiler child (`go build`) | 1 | Builds the target once under `sync.Once`; it does not execute a scenario or produce customer output. |
| Compiled-CLI-related children, inclusive view | 16 | `15 + 1`; this is the stricter current total corresponding to the plan's 15–16 ambiguity. |
| Initializer seed `Session.Run` | 1 reusable `Process.Execute` call | `internal/builtcliacceptance/harness.go:443-469` routes through the reusable root and does not invoke the target binary. |

The package's existing `TestSharedProcessWorkersMock` root and its 19 child
rows are not added to this 15-row denominator. They are the reusable process
surface that story 002 may extend.

## Exact MOCK-01..48 assertion ledger

### Counting rule

Each ID is one scenario-observable assertion contract. A compound Go predicate
or a required-marker loop is kept as one property when the current test reports
it as one result contract; all exact terms are written in the value column.
The five calls to `decodeBatchProcessReport` are separate properties because
each scenario independently asserts parseability. The named decoder has four
shape properties; its `recordType` and terminal `response` decode guards are
one envelope-shape contract. Setup failures, fixture writes, and `t.Helper`
calls are not behavior properties. This yields 39 direct scenario assertion
contracts plus nine per-scenario JSON shape contracts, exactly 48 IDs, without
counting the same source check twice.

### Direct scenario contracts: MOCK-01..39

| ID | WM | Current assertion owner | Exact asserted value | Planned owner |
| --- | --- | --- | --- | --- |
| MOCK-01 | WM-01 | `batch_invocation_exit_codes_test.go:138-140` | `err == nil`; exit `0`; stdout exactly `Batch completed successfully.\n`; stderr empty. | Retain same compiled-CLI owner. |
| MOCK-02 | WM-02 | `batch_invocation_exit_codes_test.go:155-158` | `err == nil`; exit `0`; stdout contains `Batch completed successfully.`. | Migrate to `TestSharedProcessWorkersMock/BatchDefaultSuccess`. |
| MOCK-03 | WM-03 | `batch_invocation_exit_codes_test.go:155-158` | `err == nil`; exit `0`; stdout contains `Batch completed successfully.` for `--verbose`. | Migrate to `TestSharedProcessWorkersMock/BatchVerboseSuccess`. |
| MOCK-04 | WM-04 | `batch_invocation_exit_codes_test.go:170-172` | A returned error and nonzero result exit are both required. | Retain same compiled-CLI owner. |
| MOCK-05 | WM-04 | `batch_invocation_exit_codes_test.go:173-176` | stdout contains all three exact markers: `Batch failed:`, `Work "single failing batch Work"`, and `prompt-task:failed`. | Retain same compiled-CLI owner. |
| MOCK-06 | WM-04 | `batch_invocation_exit_codes_test.go:178-180` | Failure stderr is non-empty. | Retain same compiled-CLI owner. |
| MOCK-07 | WM-05 | `batch_invocation_exit_codes_test.go:192-194` | A returned error and nonzero result exit are both required. | Retain same compiled-CLI owner. |
| MOCK-08 | WM-05 | `batch_invocation_exit_codes_test.go:196-198` | Decoded report status is `FAILED` and its failure collection has exactly one entry. | Retain same compiled-CLI owner. |
| MOCK-09 | WM-05 | `batch_invocation_exit_codes_test.go:200-202` | The one failure has exact name `single JSON failing batch Work`, exact state `prompt-task:failed`, and a non-empty reason. | Retain same compiled-CLI owner. |
| MOCK-10 | WM-06 | `batch_invocation_exit_codes_test.go:214-216` | A returned error and nonzero result exit are both required. | Migrate to `TestSharedProcessWorkersMock/BatchAllFailedHuman`. |
| MOCK-11 | WM-06 | `batch_invocation_exit_codes_test.go:217-221` | Human stdout contains both exact Work names (`all failed first Work`, `all failed second Work`) and `prompt-task:failed`. | Migrate to `TestSharedProcessWorkersMock/BatchAllFailedHuman`. |
| MOCK-12 | WM-07 | `batch_invocation_exit_codes_test.go:233-235` | A returned error and nonzero result exit are both required. | Retain same compiled-CLI owner. |
| MOCK-13 | WM-07 | `batch_invocation_exit_codes_test.go:237-238` | Decoded report status is `FAILED` and has exactly two failures. | Retain same compiled-CLI owner. |
| MOCK-14 | WM-07 | `batch_invocation_exit_codes_test.go:240-241` | Failure names are exactly `all failed first JSON Work`, then `all failed second JSON Work`, preserving submitted order. | Retain same compiled-CLI owner. |
| MOCK-15 | WM-07 | `batch_invocation_exit_codes_test.go:243-246` | Each of the two failures has exact state `prompt-task:failed` and a non-empty reason. | Retain same compiled-CLI owner. |
| MOCK-16 | WM-08 | `batch_invocation_exit_codes_test.go:259-261` | A returned error and nonzero result exit are both required. | Migrate to `TestSharedProcessWorkersMock/BatchMixedJSON`. |
| MOCK-17 | WM-08 | `batch_invocation_exit_codes_test.go:263-265` | Decoded report status is `FAILED` and has exactly one failure. | Migrate to `TestSharedProcessWorkersMock/BatchMixedJSON`. |
| MOCK-18 | WM-08 | `batch_invocation_exit_codes_test.go:267-269` | The only failure is exact Work `mixed failed Work` at exact state `failed-task:failed`; the successful Work is absent from failures. | Migrate to `TestSharedProcessWorkersMock/BatchMixedJSON`. |
| MOCK-19 | WM-09 | `batch_invocation_exit_codes_test.go:295-297` | A returned error and nonzero result exit are both required for the breaker case. | Migrate to `TestSharedProcessWorkersMock/BatchCircuitBreakerHuman`. |
| MOCK-20 | WM-09 | `batch_invocation_exit_codes_test.go:306-310` | Human stdout contains exact Work `circuit breaker Work`, state `retry-task:failed`, and breaker text `consecutive failures 1 for transition retry-work exceeds max 1`. | Migrate to `TestSharedProcessWorkersMock/BatchCircuitBreakerHuman`. |
| MOCK-21 | WM-10 | `batch_invocation_exit_codes_test.go:295-297` | A returned error and nonzero result exit are both required for the JSON breaker case. | Migrate to `TestSharedProcessWorkersMock/BatchCircuitBreakerJSON`. |
| MOCK-22 | WM-10 | `batch_invocation_exit_codes_test.go:301-303` | JSON report is `FAILED`, has one failure, exact Work `circuit breaker Work`, exact state `retry-task:failed`, and a reason containing the exact breaker text. | Migrate to `TestSharedProcessWorkersMock/BatchCircuitBreakerJSON`. |
| MOCK-23 | WM-11 | `batch_invocation_exit_codes_test.go:332-334` | A returned error and nonzero result exit are both required for the human script-failure case. | Migrate to `TestSharedProcessWorkersMock/BatchScriptFailureHuman`. |
| MOCK-24 | WM-11 | `batch_invocation_exit_codes_test.go:342-346` | Human stdout contains exact Work `script failure Work`, state `script-task:failed`, and `script worker exited non-zero`. | Migrate to `TestSharedProcessWorkersMock/BatchScriptFailureHuman`. |
| MOCK-25 | WM-12 | `batch_invocation_exit_codes_test.go:332-334` | A returned error and nonzero result exit are both required for the JSON script-failure case. | Migrate to `TestSharedProcessWorkersMock/BatchScriptFailureJSON`. |
| MOCK-26 | WM-12 | `batch_invocation_exit_codes_test.go:337-339` | JSON report is `FAILED`, has one failure, exact Work `script failure Work`, exact state `script-task:failed`, and a reason containing `script worker exited non-zero`. | Migrate to `TestSharedProcessWorkersMock/BatchScriptFailureJSON`. |
| MOCK-27 | WM-13 | `named_invocation_exit_codes_test.go:43-45` | Named success returns nil error. | Retain same compiled-CLI owner. |
| MOCK-28 | WM-13 | `named_invocation_exit_codes_test.go:46-47` | Named success exit code is exactly `0`. | Retain same compiled-CLI owner. |
| MOCK-29 | WM-13 | `named_invocation_exit_codes_test.go:49-51` | Named quiet stdout is exactly `mock worker accepted`. | Retain same compiled-CLI owner. |
| MOCK-30 | WM-13 | `named_invocation_exit_codes_test.go:52-54` | Named success stderr is empty. | Retain same compiled-CLI owner. |
| MOCK-31 | WM-14 | `named_invocation_exit_codes_test.go:70-72` | Named human failure returns a non-nil error. | Migrate to `TestSharedProcessWorkersMock/NamedHumanFailure`. |
| MOCK-32 | WM-14 | `named_invocation_exit_codes_test.go:73-75` | Named human failure result exit is nonzero. | Migrate to `TestSharedProcessWorkersMock/NamedHumanFailure`. |
| MOCK-33 | WM-14 | `named_invocation_exit_codes_test.go:76-79` | Human response stream contains exact status `status: FAILED`, the `workName: ` label, and exact state `workState: goal:failed`. | Migrate to `TestSharedProcessWorkersMock/NamedHumanFailure`. |
| MOCK-34 | WM-14 | `named_invocation_exit_codes_test.go:81-83` | The rendered `workName: ` value is non-empty after trimming. | Migrate to `TestSharedProcessWorkersMock/NamedHumanFailure`. |
| MOCK-35 | WM-15 | `named_invocation_exit_codes_test.go:101-103` | Named JSON failure returns a non-nil error. | Retain same compiled-CLI owner. |
| MOCK-36 | WM-15 | `named_invocation_exit_codes_test.go:104-106` | Named JSON failure result exit is nonzero. | Retain same compiled-CLI owner. |
| MOCK-37 | WM-15 | `named_invocation_exit_codes_test.go:109-110` | Decoded terminal invocation status is exactly `FAILED`. | Retain same compiled-CLI owner. |
| MOCK-38 | WM-15 | `named_invocation_exit_codes_test.go:112-113` | Decoded terminal Work name is present and non-empty after trimming. | Retain same compiled-CLI owner. |
| MOCK-39 | WM-15 | `named_invocation_exit_codes_test.go:115-116` | Decoded terminal Work state is exactly `goal:failed`. | Retain same compiled-CLI owner. |

### JSON shape contracts: MOCK-40..48

| ID | WM | Current assertion owner | Exact asserted value | Planned owner |
| --- | --- | --- | --- | --- |
| MOCK-40 | WM-05 | `batch_invocation_exit_codes_test.go:195`, decoder `:354-360` | The single-failure stdout is parseable as the expected batch report. | Retain same compiled-CLI owner. |
| MOCK-41 | WM-07 | `batch_invocation_exit_codes_test.go:236`, decoder `:354-360` | The two-failure stdout is parseable as the expected batch report. | Retain same compiled-CLI owner. |
| MOCK-42 | WM-08 | `batch_invocation_exit_codes_test.go:262`, decoder `:354-360` | The mixed-outcome stdout is parseable as the expected batch report. | Migrate with WM-08 to `TestSharedProcessWorkersMock/BatchMixedJSON`. |
| MOCK-43 | WM-10 | `batch_invocation_exit_codes_test.go:300`, decoder `:354-360` | The JSON breaker stdout is parseable as the expected batch report. | Migrate with WM-10 to `TestSharedProcessWorkersMock/BatchCircuitBreakerJSON`. |
| MOCK-44 | WM-12 | `batch_invocation_exit_codes_test.go:336`, decoder `:354-360` | The JSON script-failure stdout is parseable as the expected batch report. | Migrate with WM-12 to `TestSharedProcessWorkersMock/BatchScriptFailureJSON`. |
| MOCK-45 | WM-15 | `named_invocation_exit_codes_test.go:142-145` | Named JSON stdout is non-empty before record parsing. | Retain same compiled-CLI owner. |
| MOCK-46 | WM-15 | `named_invocation_exit_codes_test.go:148-151` | Every newline-delimited output record is valid JSON. | Retain same compiled-CLI owner. |
| MOCK-47 | WM-15 | `named_invocation_exit_codes_test.go:153-161` | Each record's `recordType` and terminal `response` decode as the expected JSON envelope/`InvocationResponse` shape. | Retain same compiled-CLI owner. |
| MOCK-48 | WM-15 | `named_invocation_exit_codes_test.go:165-166` | Exactly one `invocation_result` record is present. | Retain same compiled-CLI owner. |

The inventory is contiguous from `MOCK-01` through `MOCK-48`; every row names
one of `WM-01..WM-15`, a current source owner, an exact value, and a planned
retained/migrated owner. No current assertion is intentionally dropped. The
post-migration ledger must replace planned owners with actual selectors and
reconcile each ID one-for-one or with a stronger assertion.

## Story 002 post-migration evidence

Status: `FOCUSED PASS`; the ordinary package and race gates are `BLOCKED` by
the same unchanged global `@you/full-flow` staging-owner contention recorded
for the baseline. The source change is limited to the workers/mock test
topology: six compiled-CLI rows remain, nine rows now execute through the
existing root-built shared process, and the guarded compiled-process helper is
unchanged.

### Actual owner reconciliation

The current-owner column above is the before state. The following table is the
after state for every property; grouped IDs are separate property rows that
share the same exact post owner.

| MOCK IDs | Post-migration owner |
| --- | --- |
| MOCK-01 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/success quiet result exits zero` (retained compiled CLI) |
| MOCK-02 | `TestSharedProcessWorkersMock/BatchDefaultSuccess` |
| MOCK-03 | `TestSharedProcessWorkersMock/BatchVerboseSuccess` |
| MOCK-04..06 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/failed terminal Work exits nonzero with human detail` (retained compiled CLI) |
| MOCK-07..09, MOCK-40 | `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome/failed terminal Work JSON is parseable` (retained compiled CLI) |
| MOCK-10..11 | `TestSharedProcessWorkersMock/BatchAllFailedHuman` |
| MOCK-12..15, MOCK-41 | `TestBuiltCLIBatchExitCodesAggregateFailureCauses/all submitted Work failures have a complete JSON collection` (retained compiled CLI) |
| MOCK-16..18, MOCK-42 | `TestSharedProcessWorkersMock/BatchMixedJSON` |
| MOCK-19..20 | `TestSharedProcessWorkersMock/BatchCircuitBreakerHuman` |
| MOCK-21..22, MOCK-43 | `TestSharedProcessWorkersMock/BatchCircuitBreakerJSON` |
| MOCK-23..24 | `TestSharedProcessWorkersMock/BatchScriptFailureHuman` |
| MOCK-25..26, MOCK-44 | `TestSharedProcessWorkersMock/BatchScriptFailureJSON` |
| MOCK-27..30 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/success preserves primary result` (retained compiled CLI) |
| MOCK-31..34 | `TestSharedProcessWorkersMock/NamedHumanFailure` |
| MOCK-35..39, MOCK-45..48 | `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot/terminal failure preserves JSON detail` (retained compiled CLI) |

The nine migrated owners are actual `TestSharedProcessWorkersMock` table rows,
not helper-only claims. The `BatchAllFailedHuman` row also executes one fresh
accepted batch immediately after its rejected batch, proving the original
failure remains observable while a later shared invocation succeeds; that
recovery invocation is setup/recovery evidence, not an additional matrix row.
The focused run directly spot-checked more than five migrated properties in
the new test code, including default success, both output policies, ordered
human aggregation, mixed JSON omission of the successful Work, the exact
breaker reason, script-failure normalization, and named human response-stream
shape.

### Post-migration count and verification

The dynamic test-owned target-binary count after the edit is:

| Source parent | Retained target-binary children |
| --- | ---: |
| Single-Work batch | 3 (`WM-01`, `WM-04`, `WM-05`) |
| Aggregation batch | 1 (`WM-07`) |
| Named invocation | 2 (`WM-13`, `WM-15`) |
| **Scenario target-binary total** | **6** |

The shared-process table has nine additional migration rows (28 rows total
including the pre-existing 19 rows). `compiledCLIBinary.once` still creates
one compiler child, so the stricter compiled-CLI-related total is `6 + 1 = 7`.

The focused procedures and observed results on the edited worktree were:

```text
go test ./tests/functional/workers/mock -run '^TestSharedProcessWorkersMock/(BatchDefaultSuccess|BatchVerboseSuccess|BatchAllFailedHuman|BatchMixedJSON|BatchCircuitBreakerHuman|BatchCircuitBreakerJSON|BatchScriptFailureHuman|BatchScriptFailureJSON|NamedHumanFailure)$' -count=1 -v
=> exit 0; all nine migrated selectors passed; package duration 28.418s

go test ./tests/functional/workers/mock -run '^TestBuiltCLI(BatchExitCodesReportSingleWorkOutcome|BatchExitCodesAggregateFailureCauses|NamedInvocationExitCodesCharacterizeOneShot)$' -count=1 -v
=> exit 0; six retained selectors passed; package duration 11.906s

go test -race ./tests/functional/workers/mock -run '^TestSharedProcessWorkersMock/(BatchDefaultSuccess|BatchVerboseSuccess|BatchAllFailedHuman|BatchMixedJSON|BatchCircuitBreakerHuman|BatchCircuitBreakerJSON|BatchScriptFailureHuman|BatchScriptFailureJSON|NamedHumanFailure)$' -count=1
=> exit 0; all nine migrated selectors passed without a race report; package duration 44.431s

go test ./tests/functional/workers/mock/... -count=1
=> exit 1; existing TestSharedProcessWorkersMock/UnknownWorker/invalid_runType_in_override_entry
   received the unchanged @you/full-flow global staging-owner diagnostic

go test -race ./tests/functional/workers/mock/... -count=1
=> exit 1; same unchanged @you/full-flow staging-owner diagnostic reached
   TestSharedProcessWorkersMock/UnknownWorker/invalid_runType_in_override_entry;
   no race report was emitted by the changed selectors
```

The full ordinary and race commands both identified
`C:\Users\andre\.you-agent-factory\factories\.you--full-flow.staging-owner`
with `outcome=indeterminate-contention`, `owner_pid=6224`, and
`owner_identity=unverified`. No shared global process or file was stopped,
removed, or edited. The changed files contain no new sleeps, and
`compiled_cli_exit_codes_helpers_test.go` retains its baseline SHA-256
`54E0919D3D9AB235E14CACE45CD8CFF9E542F3D7CEF30D1EBA1E2F59D6F8711A`.

The focused retained and migrated runs prove the WM-01..WM-15 value contracts
at their assigned boundaries and the one-shot recovery behavior. The complete
package and race gates remain unproven until the shared staging-owner
contention is resolved outside this lane. Three post-change performance
samples, rebase loopback, and PR handoff belong to story 003.

## Baseline package runs

The exact required command was run three times, in three separate shell
invocations, before any source or evidence-file edit:

```text
go test ./tests/functional/workers/mock/... -count=1
```

The package emitted the same environmental failure on all three attempts. The
wall values below are the tool-observed elapsed times for the complete command
(initial execution plus its bounded wait); the `FAIL ... Ns` values are the
package test durations reported by Go. Both are retained because the first
includes compilation/cache setup while the latter is the package's own report.

| Sample | Complete command wall | Go package duration | Exit | Observed result |
| ---: | ---: | ---: | ---: | --- |
| 1 | 48.558s | 26.530s | 1 | `TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` failed during system bootstrap because global `@you/full-flow` staging owner was contended. `TestSharedProcessWorkersMock/UnknownWorker` then received the same bootstrap contention diagnostic. |
| 2 | 32.210s | 29.864s | 1 | Same global staging-owner contention and same JavaScript bootstrap failure; `UnknownWorker` received the same environmental diagnostic. |
| 3 | 30.236s | 29.176s | 1 | Same global staging-owner contention and same JavaScript bootstrap failure; `UnknownWorker` received the same environmental diagnostic. |

The exact repeated diagnostic identified the shared resource as:

```text
resource=C:\Users\andre\.you-agent-factory\factories\.you--full-flow.staging-owner
outcome=indeterminate-contention
owner_pid=6224
owner_identity=unverified
```

The staging-owner path existed during characterization. It belongs to the
shared host, not this worktree's task-owned test topology. No process was
stopped and no global file was removed or edited. These are environmental
failures retained under the implementation standard; they are not claimed as
package behavior passes and do not authorize changing a guarded fixture.

For the eventual performance comment, the raw pre-change command-wall samples
are `48.558s`, `32.210s`, and `30.236s`; sorted order is `30.236s`, `32.210s`,
`48.558s`, so the baseline median is `32.210s`. The corresponding Go-reported
package durations are `26.530s`, `29.864s`, and `29.176s`; sorted order is
`26.530s`, `29.176s`, `29.864s`, so that diagnostic median is `29.176s`.
Neither median is a portable threshold or a quiet-host requirement.

## Story result and remaining edges

| Criterion | Result | Evidence | Not proved |
| --- | --- | --- | --- |
| Exactly 15 current matrix rows | PASS | Dynamic expansion of the three compiled parents: 5 + 7 + 3 | Post-migration row ownership |
| Six retained / nine migrated classification | PASS | Matrix and boundary-fidelity reasons above | Equal-or-stronger post owners |
| Exact `MOCK-01..48` inventory | PASS | 39 direct contracts + 9 JSON shape contracts, contiguous and source-referenced | Post-migration parity and clean-room spot-check |
| Baseline target-binary count | PASS | `runBuiltYouBinary` expansion: 15 scenario children; one compiler child recorded separately | Post count of six / inclusive at most seven |
| Three pre-change package samples | BLOCKED by environment, retained | Three separate exact command runs, raw timings/status, repeated staging-owner diagnostic | A green package run on this shared host |
| `GATE-PACKAGE-001` | UNPROVEN | This story does not alter package behavior; all three package attempts were environmentally blocked | Complete package matrix after migration |

Remaining unproven edges and owners:

- migrated/retained value parity and recovery -> `GATE-SPINE-001`;
- post-migration target-binary count -> `GATE-COUNT-001`;
- race freedom -> `GATE-RACE-001`;
- three post-change samples and directional package result -> `GATE-PERF-001`;
- clean rebased final artifact -> `GATE-LOOP-001`;
- terminal CI and merge -> `GATE-PR` / review-owned.

No browser criterion applies to this backend/CLI characterization story.

## Story 003 validation and post-change performance evidence

Status: `BLOCKED` for `GATE-PERF-001`; the rebased source passes the focused
matrix, complete package, and race checks in a fresh isolated user/config
environment, but the three pre-change samples were stopped by the shared
global staging-owner failure before the package completed. The successful
post-change samples therefore cannot support a directional before/after
claim. This is an evidence limitation, not a source assertion failure.

### Final source and clean-room reconciliation

The validation source commit was `8de795530937ff610717f0c6794bff8e7fd1d155`.
It is directly based on `origin/main`
(`5c997439079adf8761959897ba2fb70fdad8a1f9`); `git merge-base HEAD
origin/main` returned the origin/main SHA. The source files exercised by the
focused and package commands were unchanged after the rebase.

An independent source/count pass found:

- six actual `runBuiltYouBinary` scenario calls after excluding the helper
  declaration: three single-Work rows, one aggregation row, and two named
  rows;
- three `buildYouBinary` callers sharing the one `compiledCLIBinary.once`
  compiler child; and
- nine actual migration rows in the `TestSharedProcessWorkersMock` table:
  `BatchDefaultSuccess`, `BatchVerboseSuccess`, `BatchAllFailedHuman`,
  `BatchMixedJSON`, `BatchCircuitBreakerHuman`, `BatchCircuitBreakerJSON`,
  `BatchScriptFailureHuman`, `BatchScriptFailureJSON`, and
  `NamedHumanFailure`.

The six retained scenario calls are the only target-binary executions. The
compiler child remains separate, so the inclusive compiled-CLI-related total
is `6 + 1 = 7`. The complete `MOCK-01..48` before/after ledger above remains
contiguous, and a clean-room spot-check of the actual migrated test bodies
confirmed default success, verbose success, ordered human aggregation, mixed
JSON failure omission, breaker reason, script-failure normalization, and
named human response-stream shape. No assertion was deleted or weakened; the
guarded compiled-process helper has no diff from the baseline hash.

### Commands and results

The focused selectors on the rebased source passed:

```text
go test ./tests/functional/workers/mock -run '^TestSharedProcessWorkersMock/(BatchDefaultSuccess|BatchVerboseSuccess|BatchAllFailedHuman|BatchMixedJSON|BatchCircuitBreakerHuman|BatchCircuitBreakerJSON|BatchScriptFailureHuman|BatchScriptFailureJSON|NamedHumanFailure)$' -count=1 -v
=> exit 0; all nine migrated rows passed; observed package duration 16.827s

go test ./tests/functional/workers/mock -run '^TestBuiltCLI(BatchExitCodesReportSingleWorkOutcome|BatchExitCodesAggregateFailureCauses|NamedInvocationExitCodesCharacterizeOneShot)$' -count=1 -v
=> exit 0; all six retained rows passed; observed package duration 11.328s
```

The required ordinary package command was also run three times in the
default environment, separately from the isolated runs. All three reproduced
the pre-existing global setup failure:

| Sample | Wall | Go package duration | Exit | Result |
| ---: | ---: | ---: | ---: | --- |
| Default 1 | 49.781s | 45.223s | 1 | `TestSharedProcessWorkersMock/UnknownWorker/invalid_runType_in_override_entry`; `@you/full-flow` staging owner, `indeterminate-contention`, PID 6224 |
| Default 2 | 48.822s | 44.686s | 1 | Same failure and shared resource |
| Default 3 | 46.648s | 42.531s | 1 | Same failure and shared resource |

To prove the package behavior without modifying that shared state, the exact
ordinary command was then run three times as separate processes with fresh
temporary values for `HOME`, `USERPROFILE`, `APPDATA`, and `LOCALAPPDATA` on
each run. All passed:

| Sample | Wall | Go package duration | Exit |
| ---: | ---: | ---: | ---: |
| Isolated 1 | 96.459s | 41.404s | 0 |
| Isolated 2 | 124.878s | 59.989s | 0 |
| Isolated 3 | 147.600s | 76.331s | 0 |

The isolated post medians are `124.878s` wall and `59.989s` Go-reported
package duration. The required race command, with the same fresh isolated
environment shape, exited 0 in 357.635s wall / 244.750s Go-reported package
duration and emitted no race report. The default-environment race attempt
exited 1 in 88.199s at the same staging-owner diagnostic and emitted no race
report before that setup failure.

### Validation verdict

| Gate | Result | Evidence | Unproven edge |
| --- | --- | --- | --- |
| `GATE-COUNT-001` | PASS | Six target-binary scenario calls; one separate compiler child; inclusive total seven | Future source/topology drift |
| `GATE-MAP-001` | PASS | Contiguous `MOCK-01..48` ledger and actual-code spot-checks listed above | Independent review may choose additional spot-checks |
| `GATE-SPINE-001` | PASS | Six retained and nine migrated focused selectors plus isolated complete package pass | OS semantics for migrated rows remain intentionally waived |
| `GATE-PACKAGE-001` | PASS | Three isolated exact package commands exited 0 | Default shared-host setup remains unavailable |
| `GATE-RACE-001` | PASS | Isolated `go test -race ... -count=1` exited 0 without a race report | Exhaustive schedules and future host behavior |
| `GATE-PERF-001` | BLOCKED | Pre median is from failed default-environment runs; post median is from successful isolated-home runs, so dependency/setup fidelity differs | Same-environment before/after package direction |
| `GATE-LOOP-001` | PASS | Clean ancestry, count, map, focused, package, and race reconciliation on the rebased source | Terminal CI and merge |

Overall verdict: `BLOCKED` only on directional performance evidence. The
smallest delta is a benchmark-only rerun of the frozen pre-change package and
the final package in the same isolated user/config environment, or an
operator-approved baseline result that makes the existing samples comparable;
no source topology expansion is requested. No shared staging-owner process or
file was stopped, removed, or edited.
