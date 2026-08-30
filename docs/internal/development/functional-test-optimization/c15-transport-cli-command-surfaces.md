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
