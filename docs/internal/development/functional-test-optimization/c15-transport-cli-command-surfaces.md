# C15 transport CLI command surfaces — characterization ledger

Story: `functional-test-optimization-c15-transport-cli-command-surfaces-001`

Status: `GATE-CHAR PASS` for the characterization scope. This ledger freezes
the current denominator and executable spine; it does not claim the later
command/parameter migration, race, coverage, CI, or delivery gates.

## Snapshot and scope

The source snapshot is commit `42eeee4472656b8290f798c36a5b8c871b24d7d0`.
`HEAD`, `origin/main`, and the merge base were identical when this ledger was
created. The environment was Windows `amd64` (`Microsoft Windows NT
10.0.26200.0`) with Go `1.25.0`.

The owned packages are:

- `tests/functional/transport/cli/commands`
- `tests/functional/transport/cli/parameters`

The frozen denominator is exactly 29 command cases (`C01-C29`) and 22
parameter cases (`P01-P22`). `P22` is the only build-tagged case and remains
`functionallong`-tagged. There are no quarantine markers in either package.

The governing path named by the PRD,
`docs/temp/functional-test-optimization.md`, is absent from the worktree and
from the available Git refs. The PRD's named “Addendum Thirteenth” is also not
present in the available source. No central inventory or baseline was edited.
This is recorded as a plan-reconciliation delta, not silently repaired here.

## Test-list evidence

These list-only commands compiled and exited `0`:

```text
go test ./tests/functional/transport/cli/commands -list .
go test ./tests/functional/transport/cli/parameters -list .
go test -tags=functionallong ./tests/functional/transport/cli/parameters -list .
```

The command package's 14 top-level identities were:

```text
TestCLIDocsListsPackagedTopics
TestCLIDocsEveryTopicRendersNonEmptyContent
TestCLIDocsUnknownTopicReturnsActionableFailure
TestCLIFactoryFlattenExpandPreservesMeaning
TestCLIRunNamedFactory
TestCLIRunInvalidFactoryReturnsValidationFailure
TestCLIRunFactoryByPath
TestCLIRunFactoryWritesPrimaryResultFromStdin
TestCLIRunRejectsConflictingPositionalAndStdinInput
TestCLIRunFailureWritesNoSuccessPayloadToStdout
TestCLIRunCleanInvocationStdoutRemainsPipeable
TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup
TestCLISharedRemoteScenarios
TestCLIWorkRenderProducesDeterministicGraph
```

`TestCLISharedRemoteScenarios` owns 15 named subjourneys. The run test's C05
and C06 journeys are subtests of `TestCLIRunNamedFactory`, which is why the
case count is larger than the top-level identity count.

The default parameter identities were:

```text
TestRetiredSessionDispatchesCommandIsUnknown
TestCLIStringBooleanAndRepeatedFlagsReachRequest
TestCLIFlagAfterPositionalValueUsesDocumentedParsing
TestCLIUnknownFlagFailsBeforeLifecycleStart
TestCLIJSONParameterPreservesNestedObjectAndArray
TestCLIInvalidJSONParameterNamesTheParameter
TestCLIJSONNullAndEmptyValuesRemainDistinct
TestRunKeyValueParametersReachFactoryInvocation
TestRunKeyValuePreservesEqualsInValue
TestRunDuplicateKeyUsesDocumentedPrecedence
TestRunMalformedKeyValueFailsWithoutDispatch
TestCLIParameterReusableProcessSpine
TestRunAcceptsOnePositionalPrompt
TestRunRejectsExtraPositionalValues
TestOptionalSessionIDUsesDefaultWhenOmitted
TestCLIRelativeFactoryPathResolvesFromInvocationDirectory
TestCLIWorkingDirectoryDoesNotLeakIntoOutput
TestCLIMissingWorkingDirectoryAssetFailsActionably
```

The long-tag list adds exactly:

```text
TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot
```

## Source and assertion census

The assertion count below is the number of source call sites matching
`t.Fatal*`, `t.Error*`, or their direct forms. It is a source census marker,
not a claim about dynamic assertion executions; shared helper assertions may
execute many times. `t.Skip` is counted separately.

| Package/file | Lines | SHA-256 | Assertion sites | Skip sites |
| --- | ---: | --- | ---: | ---: |
| commands/docs_wiring_test.go | 126 | `113981C9C7D136BF533CE7B32E4FE7C1B970ECB29337A1895CE5A7FF1ECAA9C9` | 11 | 0 |
| commands/factory_wiring_test.go | 290 | `CEE2582F4140141EBD38B5AEB1074125C4EE3B572022E01006B962049D1CF9F7` | 25 | 1 |
| commands/human_approval_wiring_test.go | 58 | `44800FAFA8DA17FD3B6FDED28624BEB5BEFC7E877A48500E9D8D5520A79CA359` | 7 | 0 |
| commands/local_reusable_harness_test.go | 12 | `FAC860AEF1E34CE45F171B05884ACDFB49B495D6E5C001DDF258D94D7116DDF7` | 0 | 0 |
| commands/run_wiring_test.go | 607 | `3DA550650ED9EB5F59604585C8945B4074B88E7EA0B44B55233FB259B7B52D09` | 38 | 7 |
| commands/session_wiring_test.go | 547 | `6E8F4AC03E40DF0CFA75C4CFE12178F7FA0750A52FFA265CCDD1CED9D46B9FCC` | 56 | 0 |
| commands/shared_remote_harness_test.go | 322 | `06D1553468BC41E0452306740A6522ED3DAB5B5CAFA2106163A7499246D48AFA` | 21 | 1 |
| commands/submit_wiring_test.go | 214 | `FCE10C921A0341EE29094F678C7EB1F4B33E45EDD0D1174F84F087F46345461F` | 11 | 0 |
| commands/work_wiring_test.go | 329 | `0C12E676572CD468F3F5420DECE0AC27AEC5A06EAA0892EFFD6D80FF48BF5339` | 31 | 0 |
| **commands total** | **2505** | — | **200** | **9** |
| parameters/flags_test.go | 124 | `081ABD4B505C8BCAE77730DD5D7532F702DE0032F0524F3D3ADFA91BD416FFB9` | 13 | 0 |
| parameters/json_values_test.go | 334 | `68DBB5B6D594C4FE09C7EF59578A11DE9B5E49913F3D041416A7FC5D5D4A7934` | 23 | 0 |
| parameters/key_value_test.go | 294 | `56FAD22AF0412473BE591E58A109A7F70F93BFA13F3157A1535AB8370EFD8F67` | 17 | 0 |
| parameters/parameter_spine_test.go | 467 | `C1CDFAE741FA67755858685B106F7171BE62D4455237AB10CFEDDF48FE2FF46E` | 20 | 0 |
| parameters/positional_values_test.go | 187 | `49FE71DD42143459298C91978CFDA744C854E793AFA18A5B8E6FC92C8F46689C` | 13 | 0 |
| parameters/working_directory_long_test.go | 118 | `320CD4EC4084978D7B59EF48F17A0E4FE0CE05C9CA0792A37608FCB8A2C4B655` | 8 | 1 |
| parameters/working_directory_test.go | 182 | `C3510EE2321135A536108ECF8E2DCB4F173DBF7943E01ACA7B027DAE7AEF07EB` | 15 | 0 |
| **parameters total** | **1706** | — | **109** | **1** |

The 9 command skips are all explicit `testing.Short()` gates (seven in
run-wiring, one in factory flatten/expand, and one around the shared remote
group). The parameter skip is the explicit long-functional gate around P22.
These are named baseline gates, not unexplained quarantine. Later stories
must exercise the owned denominator without deleting or weakening these
assertions.

## Complete C01-C29 assertion mapping

Line ranges refer to the frozen source hashes above. A row includes the direct
public-output, error, state, privacy, effect, and cleanup assertions in that
selector, including assertions delegated to the named helper.

| Case | Selector/source | Frozen assertion and effect witness |
| --- | --- | --- |
| C01 | `TestCLIDocsListsPackagedTopics` (`commands/docs_wiring_test.go:19-47`) | `docs` stdout is non-empty; packaged/discovery markers and every topic retrieval hint are present. |
| C02 | `TestCLIDocsEveryTopicRendersNonEmptyContent` (`docs_wiring_test.go:52-71`) | Every topic parsed from the public index gets a subtest with non-empty rendered stdout. |
| C03 | `TestCLIDocsUnknownTopicReturnsActionableFailure` (`docs_wiring_test.go:76-91`) | Unknown topic fails, names the unsupported topic, and emits no stdout success payload. |
| C04 | `TestCLIFactoryFlattenExpandPreservesMeaning` (`factory_wiring_test.go:101-149`) | Expand success marker and split `AGENTS.md` assets exist; flatten-after-expand deeply equals the original canonical meaning. Short gate is recorded. |
| C05 | `TestCLIRunNamedFactory/named_from_unrelated_working_directory` (`run_wiring_test.go:31-99`) | Named Factory run succeeds from an unrelated cwd; exact accepted summary is stdout, prompt is not echoed, stderr is empty. The current prompt is generated ASCII, so the PRD's Unicode aspect is not present in this source witness and remains unproven. |
| C06 | `TestCLIRunNamedFactory/packaged_goal_summary_primary_result` (`run_wiring_test.go:101-156`) | Packaged goal run succeeds; exact summary is stdout, goal text is not echoed, stderr is empty. |
| C07 | `TestCLIRunInvalidFactoryReturnsValidationFailure` (`run_wiring_test.go:159-202`) | Invalid Factory run fails; stdout has no success result; stderr contains the actionable `handlingBehavior DEFAULT` diagnostic. Short gate is recorded. |
| C08 | `TestCLIRunFactoryByPath` (`run_wiring_test.go:204-248`) | Authored Factory path run succeeds; stdout is the exact prompt; stderr is empty; `functionalevidence.Covers` records `cli/you.run`. |
| C09 | `TestCLIRunFactoryWritesPrimaryResultFromStdin` (`run_wiring_test.go:250-296`) | Stdin-only run succeeds; stdout is the exact stdin prompt and stderr is empty. Short gate is recorded. |
| C10 | `TestCLIRunRejectsConflictingPositionalAndStdinInput` (`run_wiring_test.go:298-346`) | Positional plus stdin fails; stdout is empty; stderr contains `INVOCATION_INPUT_SOURCE_CONFLICT`. Short gate is recorded. |
| C11 | `TestCLIRunFailureWritesNoSuccessPayloadToStdout` (`run_wiring_test.go:348-395`) | Unresolved primary result fails; stdout is empty; stderr contains `INVOCATION_PRIMARY_RESULT_UNRESOLVED`. Short gate is recorded. |
| C12 | `TestCLIRunCleanInvocationStdoutRemainsPipeable` (`run_wiring_test.go:397-441`) | Two positional and one stdin clean invocations succeed; each stdout is only the exact primary result and excludes operator chatter. Short gate is recorded. |
| C13 | `TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup` (`run_wiring_test.go:443-480`) | Ambiguous input fails before startup; stdout is empty; stderr contains conflict, `positional_text`, and `stdin_text`. Short gate is recorded. |
| C14 | `TestCLISharedRemoteScenarios/TestCLISubmitBatchInlineJSON` (`submit_wiring_test.go:39-73`) | Inline canonical batch succeeds against the running session host and exposes request ID, trace ID, work count, name, and type markers. |
| C15 | `TestCLISharedRemoteScenarios/TestCLISubmitBatchFile` (`submit_wiring_test.go:75-115`) | File-backed canonical batch succeeds and exposes the same acknowledgment markers for the staged batch. |
| C16 | `TestCLISharedRemoteScenarios/TestCLISubmitUnavailableServer` (`submit_wiring_test.go:117-151`) | Unavailable server fails with safe diagnostic; no request/trace/work success markers leak; shared host remains healthy. |
| C17 | `TestCLISharedRemoteScenarios/TestCLISubmitBackendErrorPreservesPublicMessage` (`submit_wiring_test.go:153-214`) | HTTP 400 failure preserves safe typed `BAD_REQUEST` markers while suppressing unsafe message, credential, request, trace, and work payload; shared host recovers. |
| C18 | `TestCLISharedRemoteScenarios/TestCLIWorkListAndShowReflectSubmittedWork` (`work_wiring_test.go:31-122`) | Submitted work appears in human/JSON list, reaches a valid customer state, and show returns matching ID/name/state in human and JSON forms. |
| C19 | `TestCLISharedRemoteScenarios/TestCLIWorkMoveChangesState` (`work_wiring_test.go:124-180`) | Work begins at `init`; move emits expected human markers and changes show/list state to `complete`. |
| C20 | `TestCLISharedRemoteScenarios/TestCLIWorkShowMissingReturnsNotFound` (`work_wiring_test.go:182-202`) | Human and JSON missing-work show fail with safe not-found diagnostics, no work success payload or ID leak, and host health remains intact. |
| C21 | `TestCLIWorkRenderProducesDeterministicGraph` (`work_wiring_test.go:204-329`) | Two render invocations are byte-equal; flowchart header and all expected graph markers are present. |
| C22 | `TestCLISharedRemoteScenarios/TestCLIFactoryInitValidateAndShow` (`factory_wiring_test.go:31-99`) | Factory create reports identity/directory and writes `factory.json`; config validate succeeds; default-session factory show exposes expected header, identity, and directory. |
| C23 | `TestCLISharedRemoteScenarios/TestCLIFactoryReplaceCurrentChangesSessionFactory` (`factory_wiring_test.go:151-193`) | Pre/post factory identity remains stable, replace reports success, and logical version advances. |
| C24 | `TestCLISharedRemoteScenarios/TestCLISessionCreateListShowDelete` (`session_wiring_test.go:25-155`) | Session create/list/show expose ID and folder/runtime status; terminate/delete succeed; deleted show is not-found and emits no false success payload. |
| C25 | `TestCLISharedRemoteScenarios/TestCLISessionListUsesIsolatedRecordingHome` (`session_wiring_test.go:157-227`) | History-only listing uses the injected isolated recording home, returns no live/recorded sessions, and leaks neither external artifact path nor malformed content. |
| C26 | `TestCLISharedRemoteScenarios/TestCLISessionPauseBuffersAndResumeDispatches` (`session_wiring_test.go:229-300`) | Pause is accepted and visible; submitted work remains undispatched; resume is accepted/visible and work reaches complete. |
| C27 | `TestCLISharedRemoteScenarios/TestCLISessionMissingIDReturnsNotFound` (`session_wiring_test.go:302-332`) | Show/delete/cancel/terminate of an unknown session fail safely without ID or success payload leakage; host recovers. |
| C28 | `TestCLISharedRemoteScenarios/TestCLIWorkApprovalListAndShowExposePendingApprovalAndSafeEmptyErrors` (`human_approval_wiring_test.go:14-47`) | Empty approval list is valid and empty; unknown and missing-ID approval show fail without leaking protected Work content. |
| C29 | `TestCLISharedRemoteScenarios/TestCLIExplicitSessionIsolation` (`shared_remote_harness_test.go:177-223`) | Two explicit sessions receive distinct work/session IDs, lists exclude foreign work, work reaches complete independently, replacement preserves each Factory directory, and show identities/directories remain disjoint. |

## Complete P01-P22 assertion mapping

| Case | Selector/source | Frozen assertion and effect witness |
| --- | --- | --- |
| P01 | `TestRetiredSessionDispatchesCommandIsUnknown` (`parameters/flags_test.go:12-23`) | Retired `session dispatches` fails at the CLI boundary with the expected unknown-command diagnostic. |
| P02 | `TestCLIStringBooleanAndRepeatedFlagsReachRequest` (`parameters/flags_test.go:25-61`) | Command path/zero positionals and string, boolean, `-v`, JSON, and repeated state values reach the observation edge exactly. |
| P03 | `TestCLIFlagAfterPositionalValueUsesDocumentedParsing` (`parameters/flags_test.go:63-89`) | Positional work ID remains intact while following `--json` and `--session` flags are parsed with exact values. |
| P04 | `TestCLIUnknownFlagFailsBeforeLifecycleStart` (`parameters/flags_test.go:91-112`) | Unknown flag emits stable diagnostic and operator-settings mutation delta remains zero. |
| P05 | `TestCLIJSONParameterPreservesNestedObjectAndArray` (`parameters/json_values_test.go:15-58`) | One canonical submission preserves nested object/array JSON, exact values, valid JSON, named source, and one submission. |
| P06 | `TestCLIInvalidJSONParameterNamesTheParameter` (`parameters/json_values_test.go:60-132`) | Invalid metadata returns named string-validation/`BAD_REQUEST` error; stdout, submission delta, and provider-call delta remain zero. |
| P07 | `TestCLIJSONNullAndEmptyValuesRemainDistinct` (`parameters/json_values_test.go:134-203`) | `null`, empty string, empty object, and empty array each remain exact valid JSON values, named, and pairwise distinct in one submission. |
| P08 | `TestRunKeyValueParametersReachFactoryInvocation` (`parameters/key_value_test.go:14-53`) | Unicode value and scalar key/value inputs reach one canonical invocation with exact named values and source kinds. |
| P09 | `TestRunKeyValuePreservesEqualsInValue` (`parameters/key_value_test.go:55-94`) | URL/query value containing embedded equals signs reaches one invocation without truncation. |
| P10 | `TestRunDuplicateKeyUsesDocumentedPrecedence` (`parameters/key_value_test.go:96-137`) | Repeated named `tag` values preserve alpha/beta order in one submission. |
| P11 | `TestRunMalformedKeyValueFailsWithoutDispatch/missing named value after key` (`parameters/key_value_test.go:139-215`) | Missing named value returns `INVOCATION_ARGUMENT_MISSING_VALUE` diagnostic with no submission or provider-call delta. |
| P12 | `TestRunMalformedKeyValueFailsWithoutDispatch/bare key=value without named prefix` (`parameters/key_value_test.go:139-215`) | Bare positional `key=value` returns positional-overflow diagnostic with no submission or provider-call delta. |
| P13 | `TestCLIParameterReusableProcessSpine/observer root parses generic flags` (`parameters/parameter_spine_test.go:225-272`) | Two detached observer invocations preserve independent command paths, server/state values, changed flags, and no positionals. |
| P14 | `TestCLIParameterReusableProcessSpine/full handler submits combined signature once` (`parameters/parameter_spine_test.go:274-334`) | One Process.Execute produces exactly one submission/provider call; positional, scalar, repeated, JSON, null, and empty values retain exact source/value semantics. |
| P15 | `TestRunAcceptsOnePositionalPrompt` (`parameters/positional_values_test.go:18-42`) | One spaces-and-Unicode positional prompt reaches the observation edge exactly with command path `you run`. |
| P16 | `TestRunRejectsExtraPositionalValues` (`parameters/positional_values_test.go:44-93`) | Two prompts return positional-overflow/`BAD_REQUEST`; provider-call delta remains zero. |
| P17 | `TestOptionalSessionIDUsesDefaultWhenOmitted/omitted session positional targets default session` (`parameters/positional_values_test.go:95-126`) | Omitted session targets the default session endpoint path through the controlled HTTP boundary. |
| P18 | `TestOptionalSessionIDUsesDefaultWhenOmitted/explicit session positional overrides default targeting` (`parameters/positional_values_test.go:128-155`) | Explicit session targets exactly the override endpoint path through the controlled HTTP boundary. |
| P19 | `TestCLIRelativeFactoryPathResolvesFromInvocationDirectory` (`parameters/working_directory_test.go:16-56`) | Relative Current Factory resolves from invocation cwd and customer-visible stdout names the resolved directory. |
| P20 | `TestCLIWorkingDirectoryDoesNotLeakIntoOutput` (`parameters/working_directory_test.go:58-90`) | Flatten output is non-empty and contains no absolute invocation cwd. |
| P21 | `TestCLIMissingWorkingDirectoryAssetFailsActionably` (`parameters/working_directory_test.go:92-143`) | Missing `factory.json` returns `CURRENT_FACTORY_NOT_FOUND`; stderr is one typed response naming the asset, stdout is empty, provider and lifecycle deltas are zero. |
| P22 | `TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot` (`parameters/working_directory_long_test.go:19-96`, `functionallong`) | Tagged long witness expects one complete work, zero init/failed work, one Codex provider call, exact command/args, Factory-root-relative workdir, and non-empty stdin prompt. Its execution is owned by the later tagged gate. |

## Runtime and topology census

### Commands

The command source contains two `builtcliacceptance.NewReusableHarness` wrapper
sites. In the current default full matrix those wrappers produce 15 reusable
root instances: six local harness instances (docs, flatten/expand, render, and
the shared remote group) and nine run-wiring instances (two C05/C06 subtests
plus the seven other run identities). Every command invocation then follows
the retained path `Harness.Command*` -> `Command.Run` or
`Command.CombinedOutput` -> `root.Process.Execute`; the command package has no
direct `.Execute` call site.

| Construct | Current count/observation | Classification |
| --- | --- | --- |
| Reusable root wrapper sites | 2 | Test setup; candidate for story-002 reuse consolidation, with serialized invocation safety retained. |
| Full-matrix reusable root instances | 15 | Current redundant assembly cost; no product behavior. The remote host's explicit-session reason remains behavioral. |
| `StartFunctionalAPIServer` sites | 2 | One shared service-mode host and one isolated-recording-home host; both cross the real in-process HTTP/service boundary. |
| `httptest.NewServer` sites | 1 | Controlled backend-error boundary for C17; retained effect witness. |
| `FactorySessionResolveHomeDirectory` edges | 2 | Isolated recording-home boundary for C25 and the shared host; retained effect witness. |
| Mock-worker configuration | CLI `--with-mock-workers` in run cases and one service-host `UseMockWorkers` setting | Existing controlled worker behavior; no real provider calls. Migration must preserve these existing witnesses. |
| Explicit remote session | One shared host, per-scenario Factory/Session IDs and cleanup | Behavioral isolation boundary for C14-C20 and C22-C29; must not be collapsed into global state. |
| Short gates | 9 explicit sites | Named baseline gating, not quarantine; later focused selectors must exercise the cases without weakening assertions. |

No command-package `ProviderCommandRunner` edge is currently present. This
ledger records the existing mock-worker path only; it does not authorize a
provider-edge redesign in story 001.

### Parameters

`TestMain` builds three root process variants once per test binary through
`support.BuildProcessWithContext`: observer, full handler, and missing asset.
There are 31 direct `Process.Execute` source call sites, with fresh
`root.Input` values and temporary working directories supplied per scenario.

| Variant/effect | Current observation | Behavioral reason |
| --- | --- | --- |
| Observer process | `CLIObserver` plus mutation-tracking `OperatorSettingsFileSystem` | P01-P04 and P13 need parser observation and zero-mutation evidence before handler execution. |
| Full-handler process | `ProviderCommandRunner` plus `SubmissionRecorder` | P05-P20 need canonical submission/provider/HTTP output and before/after effect deltas. |
| Missing-asset process | Provider runner plus APIServerStarter, BrowserOpener, RuntimeHostObserver, and Session-ID generator observers | P21 must prove failure before provider or lifecycle activation; its controlled edges are intentionally distinct. |
| Provider result capacity | 64 shaped results | Deterministic capacity for repeated parameter scenarios; unexpected consumption remains observable. |
| Controlled HTTP servers | 2 `httptest.NewServer` sites | P17 and P18 assert default/override session endpoint paths. |
| Process cleanup | Three process `Close` calls in `TestMain`, bounded by a 5-second context | Package-owned roots are closed once after the test run; build failure paths close already-built variants. |
| Long gate | 1 `SkipLongFunctional` site in P22 | Tagged workdir/provider fidelity is preserved for the later `-short=false` run. |

All shared observer/submission logs use mutex-protected snapshots, and the
listed parameter assertions compare deltas rather than absolute global counts.

## Focused representative evidence

Each required representative selector was run once with `-count=1`; no full
package timing run was added in this characterization story.

| Witness | Exact procedure | Observed result |
| --- | --- | --- |
| C05 | `go test ./tests/functional/transport/cli/commands -run '^TestCLIRunNamedFactory$' -count=1 -timeout=120s` | Exit `0`; package wall `10.247s`. Named and packaged subjourneys passed. |
| C07 | `go test ./tests/functional/transport/cli/commands -run '^TestCLIRunInvalidFactoryReturnsValidationFailure$' -count=1 -timeout=120s` | Exit `0`; package wall `3.762s`. Failure classification and empty success stdout passed. |
| C29 | `go test ./tests/functional/transport/cli/commands -run '^TestCLISharedRemoteScenarios$/TestCLIExplicitSessionIsolation$' -count=1 -timeout=180s -v` | Exit `0`; subtest `1.28s`, package wall `3.723s`. Two-session IDs, work-list privacy, completion, Factory directories, and cleanup passed. Expected structured discovery logs for absent nested targets were emitted; no test failure resulted. |
| P02 | `go test ./tests/functional/transport/cli/parameters -run '^TestCLIStringBooleanAndRepeatedFlagsReachRequest$' -count=1 -timeout=120s -v` | Exit `0`; test `0.03s`, package wall `0.539s`. Exact parser observation passed. |
| P06 | `go test ./tests/functional/transport/cli/parameters -run '^TestCLIInvalidJSONParameterNamesTheParameter$' -count=1 -timeout=120s -v` | Exit `0`; test `1.16s`, package wall `1.556s`. Named `BAD_REQUEST` failure and zero-effect deltas passed. |
| P21 | `go test ./tests/functional/transport/cli/parameters -run '^TestCLIMissingWorkingDirectoryAssetFailsActionably$' -count=1 -timeout=120s -v` | Exit `0`; test `0.06s`, package wall `0.526s`. Typed missing-asset error, empty stdout, and zero provider/lifecycle deltas passed. |

## Timing and gate disposition

The supplied before medians are command `38.327s` and parameter `28.615s`.
The supplied diagnostic local walls are command `39.704s` and parameter
`29.716s`. They are recorded as supplied baseline evidence, not independently
repeated here and not claimed as portable measurements.

The six focused observations above are selector diagnostics only. Final local
package `-count=1` n=1 medians, current-head PR-CI package timing, a possible
bounded profile-led pass, per-test/setup/teardown/compile floors, and the
61.6% Backend Functional Coverage result are later story-004 evidence.

`GATE-CHAR` passes because the exact C/P denominator, source hashes, assertion
mapping, current topology, explicit skip/quarantine status, supplied timing
record, and representative success/failure/recovery spine are recorded. It
does not pass any later performance or coverage gate.

## Property boundaries and handoff

This story proves:

- the exact current test-list denominator and long-tag identity;
- source identity and direct assertion-site census for both owned packages;
- C01-C29 and P01-P22 mapping to current public assertions/effect witnesses;
- the current production-root/Process.Execute topology and controlled edges;
- representative docs/run/isolation, parser/error, and missing-asset behavior;
- supplied timing and the 61.6% coverage floor as requirements to be measured
  later; and
- absence of quarantine markers without changing a test or assertion.

It does not prove command or parameter migration, fresh-session behavior after
structural edits, focused `-count=3`, race safety, full package post-change
behavior, final n=1 timing, current-head PR timing, coverage measurement,
clean-room validation, built executable/OS behavior, terminal CI, or merge.
The C05 Unicode prompt edge and P22 execution remain explicitly unproven and
are owned by the subsequent behavior/delivery gates.

## Story 004 — exact-head behavior, performance, and delivery evidence

The implementation/test tree validated for this story is commit
`9c89ba607f57ff68f08bd6e314220fbc228cc7a2` on Windows `amd64` with Go
`1.25.0`. The only tracked paths in the implementation diff are this ledger
and the two owned functional-test packages; `prd.json` and `progress.txt`
remain ignored local scaffolding. `git diff --check` and
`go run ./cmd/functionalboundarycheck` both exited `0`.

The branch was rebased once immediately before final validation from the
characterization base onto `origin/main`/`upstream/main`
`3c6acf6794b0f7ecace31e6d13504a2bce6ffe1`. The base delta touched none of the
owned CLI/runtime/support paths, and the owned source tree is identical before
and after the rebase.

### Final topology reconciliation

The command package now has one `support.BuildProcessWithContext` site in
`package_runtime_test.go`, shared by all command fixtures through a serialized
`commandRuntime`. The parameter package has two such sites: one observer root
and one handler root. The observer root remains separate because it intercepts
handler execution; the handler root carries the provider/submission and
missing-asset lifecycle edges. Every retained root has that named behavioral
reason. No production, shared-support, contract, generated, inventory,
workflow, or baseline path changed.

Final changed-source SHA-256 identities are recorded below so later evidence
can distinguish the implementation tree from this documentation refresh:

| File | SHA-256 |
| --- | --- |
| `commands/docs_wiring_test.go` | `568FE727E0899F54597212AB8C5A635B5B424D528CE55DD0E09428BC7915E169` |
| `commands/factory_wiring_test.go` | `55C0B96EAF775B51B33901642F19B159A88662B40234707FC281B6B9B5AA10E6` |
| `commands/local_reusable_harness_test.go` | `6A01B78C23F04FAC374446A7A290CD3364CB1F4E11A95251D020BB3396F3DC1C` |
| `commands/package_runtime_test.go` | `D35B7512D55B67F2D2EBEBBD6F44F1DDF16A282936D980F90E4FF7C309384AFB` |
| `commands/run_wiring_test.go` | `3F32DDB96B8943744C9FA1135C08CB03C459D60BA2967A1CEBE160B84CF79689` |
| `commands/session_wiring_test.go` | `16EB8AB9BF79A0B0CA8F29C05CE8FD4BE831797901D28748D0FA92DBB1DA0A11` |
| `commands/shared_remote_harness_test.go` | `6E292EFF471F6166E9568F5D15FEAA272A7AE5A04B937D860C226F85A12BAFC9` |
| `parameters/flags_test.go` | `6E5DEC09239BB46B8F5C1E362A43B8CF76D9530CEE37ABE7390CAAA3FA2D8C08` |
| `parameters/json_values_test.go` | `5F63243FC67E4C8C4C604B8243C1DC0DEF518A7AF49962F526A1FA712F2A3517` |
| `parameters/key_value_test.go` | `A3684560701920D1D982F07CE8774C2C8E92A8AE1B4FFCCCE2EAB485980320E2` |
| `parameters/parameter_spine_test.go` | `8E9BECF853391C082E438E3A3B27E33D1B5F1F6FC4F94AA358BF34FE2F582AE2` |
| `parameters/positional_values_test.go` | `249ED3299A3B99F458DD2F6E0005D3C5D386B9DA57F61149EF6C938A4C30672A` |
| `parameters/working_directory_test.go` | `21804FB87B61A216D1A8B1CC8CBCA445FB22AB5D637D67F7C02EB0965CAA3428` |

### Focused repeat, race, and boundary gates

All commands below ran against the implementation tree above and exited `0`.
The package elapsed values are diagnostic observations under a shared,
contended Windows host, not portable latency claims.

| Gate | Procedure | Result and property proved |
| --- | --- | --- |
| `GATE-CMD-REPEAT` | Command changed-witness selector group with `-count=3` | Package `25.079s`; C03, C12, C17, C21, C26, and C29 preserved public output/error/effect/isolation behavior. |
| `GATE-CMD-RACE` | Same command changed-witness selector group with `-race -count=1` | Package `36.891s`; no race report. |
| `GATE-PARAM-REPEAT` | P01-P21 default selector group with `-count=3` | Package `154.822s`; exact parser, invalid-to-valid, effect-delta, path, and cleanup assertions remained active. |
| `GATE-PARAM-RACE` | P01-P21 default selector group with `-race -count=1` | Package `48.204s`; no race report. |
| `GATE-C13-DIAGNOSTIC` | `TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup` with `-count=3` | Package `9.795s`; the isolated conflict witness passed three times after the cold loopback diagnostic. |
| `GATE-BOUNDARY` | `go run ./cmd/functionalboundarycheck` | Exit `0`; the repository functional-boundary policy passed. |

### Final local package runs

Each ordinary package was run exactly once with `-count=1` after the focused
gates. The supplied medians remain the before record. The final local result is
an `n=1` observation, not a portable threshold claim.

| Package | Supplied before median | Final Go package result | Final measured wall | Result |
| --- | ---: | ---: | ---: | --- |
| `commands` | `38.327s` | `49.903s` | `54.440s` | exit `0`; all C01-C29 active under the ordinary suite |
| `parameters` | `28.615s` | `25.910s` | `29.945s` | exit `0`; all default P01-P21 active |

P22 was run exactly once with the required tag and short policy:

```text
go test -tags=functionallong ./tests/functional/transport/cli/parameters \
  -run '^TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot$' \
  -count=1 -short=false -timeout=15m
```

It exited `0` with package time `1.757s` and measured wall `6.361s`. The
assertion retained one complete Work, zero init/failed Work, one Codex command,
the exact provider arguments, Factory-root-relative workdir, and non-empty
stdin prompt.

### One bounded profile-led floor pass

Because both ordinary packages remain above three seconds, one and only one
CPU-profile/JSON event pass was run for each package. The pass itemizes every
top-level test identity; it is not an additional variance sample. The
package-level setup/teardown remainder is the package event minus the sum of
top-level test events. The compile/runner column is an upper bound from the
outer invocation window and is not attributed to product behavior.

| Package | Profile package event | Top-level test sum | Package setup/teardown remainder | Outer invocation window | Compile/runner upper bound |
| --- | ---: | ---: | ---: | ---: | ---: |
| `commands` | `68.887s` | `68.480s` | `0.407s` | `84.778s` log window | `15.891s` upper bound |
| `parameters` | `81.770s` | `81.020s` | `0.750s` | `89.666s` | `7.896s` upper bound |

Command top-level profile identities:

| Test | Seconds |
| --- | ---: |
| `TestCLIDocsListsPackagedTopics` | `0.040` |
| `TestCLIDocsEveryTopicRendersNonEmptyContent` | `1.670` |
| `TestCLIDocsUnknownTopicReturnsActionableFailure` | `0.070` |
| `TestCLIFactoryFlattenExpandPreservesMeaning` | `0.220` |
| `TestCLIRunNamedFactory` | `12.460` |
| `TestCLIRunInvalidFactoryReturnsValidationFailure` | `3.160` |
| `TestCLIRunFactoryByPath` | `5.520` |
| `TestCLIRunFactoryWritesPrimaryResultFromStdin` | `3.870` |
| `TestCLIRunRejectsConflictingPositionalAndStdinInput` | `2.740` |
| `TestCLIRunFailureWritesNoSuccessPayloadToStdout` | `3.600` |
| `TestCLIRunCleanInvocationStdoutRemainsPipeable` | `10.180` |
| `TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup` | `4.050` |
| `TestCLISharedRemoteScenarios` | `20.730` |
| `TestCLIWorkRenderProducesDeterministicGraph` | `0.170` |

Parameter top-level profile identities:

| Test | Seconds |
| --- | ---: |
| `TestRetiredSessionDispatchesCommandIsUnknown` | `0.050` |
| `TestCLIStringBooleanAndRepeatedFlagsReachRequest` | `0.030` |
| `TestCLIFlagAfterPositionalValueUsesDocumentedParsing` | `0.020` |
| `TestCLIUnknownFlagFailsBeforeLifecycleStart` | `0.020` |
| `TestCLIJSONParameterPreservesNestedObjectAndArray` | `2.960` |
| `TestCLIInvalidJSONParameterNamesTheParameter` | `2.200` |
| `TestCLIJSONNullAndEmptyValuesRemainDistinct` | `4.800` |
| `TestRunKeyValueParametersReachFactoryInvocation` | `4.600` |
| `TestRunKeyValuePreservesEqualsInValue` | `6.190` |
| `TestRunDuplicateKeyUsesDocumentedPrecedence` | `9.270` |
| `TestRunMalformedKeyValueFailsWithoutDispatch` | `19.290` |
| `TestCLIParameterReusableProcessSpine` | `16.100` |
| `TestRunAcceptsOnePositionalPrompt` | `0.080` |
| `TestRunRejectsExtraPositionalValues` | `7.600` |
| `TestOptionalSessionIDUsesDefaultWhenOmitted` | `0.230` |
| `TestCLIRelativeFactoryPathResolvesFromInvocationDirectory` | `7.270` |
| `TestCLIWorkingDirectoryDoesNotLeakIntoOutput` | `0.210` |
| `TestCLIMissingWorkingDirectoryAssetFailsActionably` | `0.100` |

The floor shows that the remaining local cost is overwhelmingly retained
customer-path execution and shared remote/session observation, not redundant
root assembly: the package-level setup/teardown remainder is under one second
for each profile pass. No second optimization pass was authorized after this
bounded profile result; the current-head PR runner is the authoritative shared
host performance verdict.

### VAL-001 clean-room validation report

This report follows `factory/docs/standards/validation-loopback-template.md`.
It was run read-only from detached clean worktrees at the exact implementation
SHA. No implementation repair was made during any loopback attempt.

#### Environment and artifact

- Commit/build identifier: `9c89ba607f57ff68f08bd6e314220fbc228cc7a2`.
- Environment: Windows `10.0.26200.0`, Go `1.25.0`, `windows/amd64`; the host
  had unrelated long-running Go/test/runtime processes, which is retained as
  an environmental observation.
- Customer entry point: production `root.BuildProcess` and `Process.Execute`,
  with local-real filesystem/listener/HTTP boundaries and controlled effects
  through `serviceedges.Edges`.
- Cost/call budget: zero paid calls, zero real-remote/provider calls, `$0`.

#### Project criteria

| Criterion | Result | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Complete command matrix | PASS | Retry detached worktree at the exact SHA ran `go test ./tests/functional/transport/cli/commands -count=1`; exit `0`, package `59.252s`, wall `100.920s`. | Hosted Linux/CI topology. |
| Complete default parameter matrix | PASS | Same detached retry ran `go test ./tests/functional/transport/cli/parameters -count=1`; exit `0`, package `32.578s`, wall `37.665s`. | Hosted Linux/CI topology. |
| P22 tagged witness | PASS locally; cold loopback diagnostic recorded | Exact local tagged command exited `0`; detached cold starts twice reached the existing helper deadline before the injected API server was invoked. | Cold detached startup under this shared host; no assertion or timeout was changed. |
| Repeat/race/cleanup support | PASS | Focused count-three and race gates above exited `0`; package-owned runtimes close once through their serialized locks. | Unexercised schedules and future host behavior. |
| Scope and compatibility | PASS | Three-dot diff and boundary check contain only the owned test paths and ledger; no API, generated, production, shared-support, inventory, or UI change. | Future base changes before push. |
| Security/privacy/cost | PASS | Controlled local effects only; no credentials, customer data, paid calls, or real remote providers. | None within this lane. |
| UI/browser/accessibility/localization | PASS — not applicable | This lane changes functional test runtime construction only. | None. |

#### Clean-room findings

| ID | Severity | Reproduction | Expected | Actual | Disposition |
| --- | --- | --- | --- | --- | --- |
| `ENV-C15-001` | environmental diagnostic | First detached full command run at the exact SHA | C13 emits `INVOCATION_INPUT_SOURCE_CONFLICT` | Existing 20-second invocation context expired; stderr was `RUN_INVOCATION_TIMEOUT` after `27.43s`. The focused C13 selector then passed three times locally. | Not used as behavior evidence; no code or timeout change. |
| `ENV-C15-002` | environmental diagnostic | Detached tagged P22 run at the exact SHA | Injected API server starts and the provider workdir witness completes | `process_factory.go:314` reported the API server starter was never invoked; daemon stderr ended with `Error: context canceled` under the existing 10-second helper deadline. The exact tagged local run passed. | Not used as behavior evidence; no code or timeout change. |

#### Verdict

`PASS` for the exact-head local-real implementation and package loopback, with
the two cold-start environmental diagnostics explicitly retained as
unproven edges. The diagnostics do not reproduce on the current worktree’s
focused C13/P22 runs, and no assertion, skip, quarantine, or timeout was
weakened to obtain the pass.

The remaining hosted edges are current-head Backend Functional Coverage,
current-head package timing, terminal CI, and merge. CI-run output and URLs
must be recorded in the PR conversation only, never in this ledger commit.

### Implementation-stage handoff status

Local story evidence is complete and ready for final push. The
implementation-stage delivery criterion becomes satisfied only after this
final head is pushed, the named PR is open, required CI has started on that
head, and any blocking review feedback is addressed. Review owns terminal CI,
conflict resolution, the hosted coverage/timing verdict, and merge.
