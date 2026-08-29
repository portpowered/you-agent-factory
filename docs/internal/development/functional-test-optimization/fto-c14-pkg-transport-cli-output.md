# FTO-C14 CLI output characterization ledger

## Scope and authority

- Behavior lane: `BEH-PROFILE` — the CLI output package has an exact,
  reproducible behavioral and performance denominator before optimization.
- Story: `fto-c14-pkg-transport-cli-output-001`.
- Repository: `you-agent-factory`.
- Characterization head and `origin/main`: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Owned source: `tests/functional/transport/cli/output/`.
- The PRD plan reference `tasks/todo/fto-c14-pkg-transport-cli-output.md`
  is not present in this checkout. The PRD, repository standards, and current
  source are the authorities for this characterization; the missing plan was
  recorded rather than reconstructed.
- No production, generated, shared-support, workflow, sibling functional
  package, remote, or paid surface was changed.

## Owned-source hashes

SHA-256 hashes were captured before any owned functional source edit:

| Source | SHA-256 |
| --- | --- |
| `tests/functional/transport/cli/output/json_result_test.go` | `F2EF8BF038B2767254BBA5357432DFC09358AD43227781B16EA875FEDE258D4E` |
| `tests/functional/transport/cli/output/ndjson_stream_test.go` | `09DBEF7C5EA7CA6C4B7839EE58C5F29F5387B3B60B93136EF41AF336617BE661` |
| `tests/functional/transport/cli/output/shared_fixture_test.go` | `71832B2E816B13531D46821C3F49C05CC4AC8CDD99DA9507DAA7CA0DFE2268FA` |
| `tests/functional/transport/cli/output/stream_backpressure_test.go` | `CA65105955D35612222FC418710E206F32A35BE94D690315542D5F8211D3EA97` |
| `tests/functional/transport/cli/output/text_stream_test.go` | `48BD9B868F1FE146CA4B9C94F8971A7A2363238F9C352D670C608596FD5A6EAB` |

## Environment and procedure

The samples ran on 2026-08-29 in PowerShell on Windows `amd64`, Go
`go1.25.0`, a 13th Gen Intel(R) Core(TM) i7-13700K with 24 logical processors.
`GOFLAGS` was empty, `GOTOOLCHAIN=auto`, and the module was the current
worktree's `go.mod`. Each baseline used the same native
`System.Diagnostics.Stopwatch` around the exact command below, with
`-count=1`, on the same clean characterization head:

```text
go test ./tests/functional/transport/cli/output/... -count=1
```

The command was run three times before any functional source edit. All three
exited zero. The host was visibly compute-contended, so the raw samples are
retained rather than filtered.

## GATE-BASELINE — pre-edit samples

| Sample | Native wall time | Package-reported time | Exit |
| --- | ---: | ---: | ---: |
| 1 | 51,128.811 ms | 34.848 s | 0 |
| 2 | 59,921.491 ms | 56.821 s | 0 |
| 3 | 90,727.170 ms | 84.673 s | 0 |

Sorted native wall samples are `51,128.811`, `59,921.491`, and `90,727.170`
milliseconds. The pre-edit median is therefore **59,921.491 ms (59.921 s)**.
This is the admitted local denominator; it is not a portable absolute target.

## Profile and focused contributions

One bounded JSON profile pass used:

```text
go test -json ./tests/functional/transport/cli/output/... -count=1
```

It exited 1 after 155.368 s. The ordinary baseline had already passed three
times; this diagnostic pass retained the host-contended failures instead of
repeating it to reduce variance. The observed failures were timeout guards in
`TestCLIWriterFailureCancelsInvocation` (provider external-work start),
`TestCLITextStreamSurfacesIncrementalMessages` (first human stdout chunk), and
`TestCLITextStreamInterruptedRunDoesNotClaimCompletion` (provider
external-work start). The output also contained packaged-factory staging
contention diagnostics. This profile is evidence of noisy scheduling and
startup cost, not a behavioral pass for those selectors.

The needed focused group samples completed successfully after the diagnostic
pass. They are contribution indicators, not additive package timings because
each group has its own process scheduling and cache state:

| Group command | Wall/package result | Exit | Property observed |
| --- | ---: | ---: | --- |
| `go test ./tests/functional/transport/cli/output/... -run '^TestCLIJSON' -count=1` | 8.891 s | 0 | JSON success, failure, argument, privacy, and selector group executes. |
| `go test ./tests/functional/transport/cli/output/... -run '^TestCLINDJSON' -count=1` | 7.340 s | 0 | NDJSON success, ordering, failure, and terminality group executes. |
| `go test ./tests/functional/transport/cli/output/... -run '^TestCLITextStream' -count=1` | 11.356 s | 0 | Human, quiet, continuous, and interruption group executes. |
| `go test ./tests/functional/transport/cli/output/... -run '^(TestCLISlowWriter|TestCLIWriterFailure)' -count=1` | 4.760 s | 0 | Isolated backpressure and writer-failure group executes. |

The profile-supported optimization candidates are: duplicate successful JSON
and NDJSON journeys; duplicate human response-stream success evidence where
the gated lifecycle witness can be retained; serialized pre-activation cases
whose root is already shared; and polling loops in the gated stdout and
continuous-listener witnesses. The four isolated lifecycle roots and their
external-effect edges are not candidates for removal.

## Executable topology and cleanup ownership

The package uses the required `support.BuildProcess` plus public
`Process.Execute` boundary. One lazy `BuildProcessWithContext` result in
`sharedMachineOutputFixture` serves 18 serialized journeys. Four explicit
`support.BuildProcess` calls provide independent roots for the slow writer,
writer failure, continuous listener, and interrupted human-output cases. The
denominator is exactly **five application roots and 22 Process.Execute
journeys**:

| Root | Journeys | Current witnesses |
| --- | ---: | --- |
| Shared machine-output root | 18 | JSON success 1; JSON failure 2; argument validation 4; JSON privacy 2; output selection 3; NDJSON success shape 1; NDJSON sequence 1; NDJSON failure 1; gated human output 1; human envelope/quiet 2. Calls are serialized by `machineOutputFixture.mu`. |
| Slow-writer isolated root | 1 | `TestCLISlowWriterDoesNotReorderResponseEvents`. |
| Writer-failure isolated root | 1 | `TestCLIWriterFailureCancelsInvocation`. |
| Continuous-listener isolated root | 1 | `TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet`. |
| Interrupted-output isolated root | 1 | `TestCLITextStreamInterruptedRunDoesNotClaimCompletion`. |
| **Total** | **22** | **Five roots; all journeys cross `Process.Execute`.** |

The shared `TestMain` close owns the shared process, source root removal,
route-count check, active-invocation check, and root-build count check. Each
isolated root registers `support.CleanupProcess`; its test cleanup releases
writers or cancellation, waits for the execution goroutine, and the runner
owns external-work completion. `t.TempDir`, factory staging cleanup, route
unbind, listener close/rebind, and buffer ownership remain part of the
current cleanup contract.

Current synchronization census:

| Mechanism | Static guards/sites | Ownership and purpose |
| --- | ---: | --- |
| `context.WithTimeout` | 1 | Shared-process close guard, 5 s. |
| `time.After` selects | 12 source sites; 13 runtime waits | Completion, cleanup, external-work, buffered-write, and readiness failure guards; `waitFinished` is called twice. |
| `time.Now` deadline loops | 4 | Stdout-write attempt, continuous server startup, first human chunk, and interrupt lifecycle observation. |
| `time.Sleep` in loops | 4 | Existing 1 ms/25 ms observation polling; all are bounded fixture guards and are optimization candidates, not new behavior. |
| HTTP client timeout | 1 | 2 s reachability request guard for the continuous listener. |
| Top-level `t.Parallel` | 8 | JSON/NDJSON tests may schedule concurrently, while the shared fixture serializes its executions. |

## GATE-INVENTORY — exact pre-edit 22-case assertion denominator

Every current atomic journey is mapped below. The table is the pre-edit
inventory that later stories must retain or strengthen; no row is a placeholder
and no assertion is authorized for deletion or weakening.

| Case | Current witness | Assertions retained in the denominator |
| --- | --- | --- |
| CASE-001 | `TestCLIJSONSuccessDecodesToPublicInvocationResult` | Default JSON decodes; terminal status is `COMPLETED`; primary text is exactly `mock worker accepted`. |
| CASE-002 | `TestCLIJSONFailureRemainsValidJSON` / pre-terminal subtest | Execute fails; stdout is empty; exactly one decodable stderr `ErrorResponse` has failed/internal-server classification. |
| CASE-003 | `TestCLIJSONFailureRemainsValidJSON` / terminal subtest | Execute fails; one failed terminal invocation response has pinned error code/message; stderr error matches code, family, and message prefix. |
| CASE-004 | `TestCLIInvocationArgumentFailuresAreBadRequest` / unknown normal JSON | Unknown argument is `BAD_REQUEST`; stdout empty; provider calls zero. |
| CASE-005 | `TestCLIInvocationArgumentFailuresAreBadRequest` / unknown quiet | Unknown argument is `BAD_REQUEST`; stdout empty; provider calls zero in quiet mode. |
| CASE-006 | `TestCLIInvocationArgumentFailuresAreBadRequest` / missing planner model | Missing value before next flag is `BAD_REQUEST`; stdout empty; provider calls zero. |
| CASE-007 | `TestCLIInvocationArgumentFailuresAreBadRequest` / missing factory value | Missing run-flag value is `BAD_REQUEST`; stdout empty; provider calls zero. |
| CASE-008 | `TestCLIJSONOutputSelectionFailsBeforeProductActivation` / quiet plus global JSON | Exact output-conflict error; stdout empty; product effects zero. |
| CASE-009 | `TestCLIJSONOutputSelectionFailsBeforeProductActivation` / quiet plus explicit output | Exact output-conflict error; stdout empty; product effects zero. |
| CASE-010 | `TestCLIJSONOutputSelectionFailsBeforeProductActivation` / unsupported output | Exact unsupported-output error; stdout empty; product effects zero. |
| CASE-011 | `TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult` | Public Factory Events decode; precede one completed terminal result; records contain only expected envelope fields; private literals absent. |
| CASE-012 | `TestCLINDJSONFailureEndsWithOneTerminalResult` | Rejection returns failure; Factory Events precede exactly one failed terminal result; terminal result is last. |
| CASE-013 | `TestCLIJSONContainsNoPrivateRuntimeFields` / success | Terminal public JSON decodes and has no private keys or forbidden runtime/provider literals. |
| CASE-014 | `TestCLIJSONContainsNoPrivateRuntimeFields` / provider failure | Failed stdout and stderr remain public, decodable, and free of private keys/literals. |
| CASE-015 | `TestCLITextStreamSurfacesIncrementalMessages` | Gated human lifecycle appears before completion; release permits completion; stderr progress is stable; canonical lifecycle, separator, and primary result lines are exact. |
| CASE-016 | `TestCLISlowWriterDoesNotReorderResponseEvents` | Invocation cannot complete while stdout is blocked; release completes; event/session sequences are monotonic; one completed terminal result is last; stderr empty. |
| CASE-017 | `TestCLIWriterFailureCancelsInvocation` | Buffered events exist before writer failure; writer error wins; no terminal result appears; external work observes cancellation and joins. |
| CASE-018 | `TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise` / quiet subtest | Quiet stdout is only the primary result; stderr is empty; no structured or operator noise. |
| CASE-019 | `TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet` | Factory initiated and Dashboard URL text appear; one real reachability request succeeds; cancellation joins; listener closes and port rebinds. |
| CASE-020 | `TestCLITextStreamInterruptedRunDoesNotClaimCompletion` | Cancellation error contains `INVOCATION_CANCELED`; declared run exit is 130; canceled outcome appears; no success/primary claim; external work joins with `context.Canceled`. |
| CASE-021 | JSON/NDJSON decode helpers and terminal loops in cases 001–004, 011–014, 016–017 | Expected terminal/error record count is exact, streams decode to EOF, and no trailing record follows the terminal result. |
| CASE-022 | `TestMain`, shared fixture close, and all four isolated cleanup paths | Failure-to-success reuse has fresh streams/routes; active routes and roots return to zero; owned processes, listeners, work, buffers, goroutines, homes, Factories, workspaces, and worktrees leave no residue. |

The delivered assertion inventory is not yet a post-edit claim: this ledger
freezes the complete current denominator so stories 002–005 can reconcile it
explicitly. The pre-edit behavioral witness passed the named package command in
all three baseline samples.

## Security, privacy, and cost boundaries

All provider and server edges in this characterization were controlled local
edges. Remote/paid calls: **0**; cost: **USD 0**. No credential, sensitive
payload, remote response, CI transcript, or runtime log was committed. The
privacy cases explicitly reject provider-session, response-event, Petri,
diagnostic, progress, compaction, and retired record vocabulary. UI,
accessibility, keyboard, responsive, and localization checks are not
applicable because this lane changes only functional-test sources and its
evidence ledger.

## Story 002 — GATE-SUCCESS

The successful-output assertions were consolidated onto the existing captured
executions without changing CLI arguments, output modes, controlled provider
results, root construction, or cleanup boundaries:

| Cases | Retained witness | Consolidation and assertion parity |
| --- | --- | --- |
| CASE-001 and CASE-013 | `TestCLIJSONFailureRemainsValidJSON/success_recovers_after_terminal_failure_on_the_shared_root` | One default-JSON capture now proves completed status, exact primary text, empty stderr, public decoding, and private-runtime-field absence. The former success subtest in `TestCLIJSONContainsNoPrivateRuntimeFields` was removed as a duplicate; story 003 later placed the retained success capture after a terminal-failure recovery step. |
| CASE-011 and applicable CASE-021 | `TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult` | One response-stream capture now proves public Factory Event shape, event-before-terminal ordering, strictly increasing Factory and Factory Session sequences, privacy, exactly one terminal result, and terminal EOF. The former `TestCLINDJSONSequenceIsMonotonic` journey was removed as a duplicate. |
| CASE-015 | `TestCLITextStreamSurfacesIncrementalMessages` | The existing first-chunk-gated capture now also runs the final structured-envelope/operator-noise assertions after release, retaining the in-flight ordering, stable progress, canonical lifecycle, separator, and primary-result checks. |
| CASE-018 | `TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise/quiet_clean_primary_result` | Quiet remains a distinct accepted execution and still proves raw primary stdout, empty stderr, and no structured/operator noise. |
| CASE-021 | Retained JSON/NDJSON decode helpers and terminal loops | Existing all-record decoding and terminal-position checks remain on the retained captures; no trailing output is accepted by the existing stream assertions. |
| CASE-022 | Shared fixture and existing cleanup paths | No root, route, stream, or cleanup ownership changed. |

The shared root's successful journey count fell from 18 to 15: accepted JSON
success/privacy went from 2 to 1, accepted NDJSON shape/sequence went from 2
to 1, and gated human success/presentation went from 2 to 1. Quiet remains
one journey. The four isolated lifecycle roots remain unchanged, so the total
package denominator is now 19 `Process.Execute` journeys across five roots,
down from 22.

Focused verification used the real in-process CLI and controlled command-runner
edges:

```text
go test ./tests/functional/transport/cli/output/... -run '^(TestCLIJSONSuccessDecodesToPublicInvocationResult|TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult|TestCLITextStreamSurfacesIncrementalMessages|TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise)$' -count=1 -v
```

PASS, package-reported 11.679 s. The same combined `-count=3` command had one
host-contended timeout in the existing 5 s first-human-chunk guard while
packaged-factory staging logged an active-owner contention diagnostic. The
bounded isolated repeat then passed the changed human selector 3/3 in 9.819 s,
and the grouped JSON/NDJSON/quiet selectors passed 3/3 in 22.812 s. The
contended run is retained as diagnostic evidence; no timeout, sleep, or
assertion was changed.

This story proves successful public output parity and the lower duplicate
journey count. It does not prove bad-input overlap, failure recovery, isolated
lifecycle behavior, the final package median, exact-head loopback, terminal
CI, or merge; those remain owned by GATE-PREACT, GATE-FAILURE,
GATE-LIFECYCLE, GATE-PERF, VAL-001, and review.

## Story 003 — GATE-PREACT / GATE-FAILURE

The bad-input and failure paths retain the pre-edit assertion denominator while
admitting only the proven pre-activation cases concurrently. No production,
shared-support, generated, or isolated-lifecycle source changed.

### Post-edit source hashes

| Source | SHA-256 |
| --- | --- |
| `tests/functional/transport/cli/output/json_result_test.go` | `5864FE8BD0E4F919CE63C13FD742A7DBFD2D6D93D2A2B9E83D351CCB72D7F17E` |
| `tests/functional/transport/cli/output/ndjson_stream_test.go` | `48F8DB2C3CFA0D97D6DE4BE6924B2BE5412F7B6070FA0520F379254E5941AEC2` |
| `tests/functional/transport/cli/output/shared_fixture_test.go` | `0BADA8079152BF3F69B8ED0532568E0770C22C6B7DFF6BBFAB7FCE5C2F68C03A` |
| `tests/functional/transport/cli/output/stream_backpressure_test.go` | `CA65105955D35612222FC418710E206F32A35BE94D690315542D5F8211D3EA97` |
| `tests/functional/transport/cli/output/text_stream_test.go` | `751C26B1F0FC1A130E82E838A614AA5A2E9791D6270EE51B717F6936DAEBB088` |

### Bounded pre-activation batch and assertion parity

`TestCLIInvalidInputsAreBadRequest` now prepares seven independent
`Process.Execute` inputs and admits them through one shared-root,
test-owned start barrier. CASE-004 through CASE-007 retain their distinct
unknown-argument and missing-value inputs; CASE-008 through CASE-010 retain
their quiet/JSON conflict and unsupported-output inputs. Each input owns a
fresh home, working directory, captured stdout/stderr, and controlled provider
route. Every provider runner recorded zero calls, the aggregate activation
effect counter remained zero, and the router and active-invocation counts were
zero after the batch. The observed maximum concurrency was 7 for all 7 cases.

The batch is bounded to this exact seven-case pre-activation set. The shared
fixture mutex still excludes all ordinary shared-root executions, and the four
isolated lifecycle roots remain independent. No product activation, successful
runtime, listener, or external-work path is admitted to this overlap.

The failure denominator remains distinct: missing Factory and logical terminal
failure stay in `TestCLIJSONFailureRemainsValidJSON`; controlled provider
rejection remains in `TestCLIJSONFailureContainsNoPrivateRuntimeFields` and
`TestCLINDJSONFailureEndsWithOneTerminalResult`. The JSON failure test now
executes a fresh successful JSON input immediately after its terminal failure
subtest and checks completed status, primary text, privacy, route cleanup, and
active-invocation cleanup. This proves shared-root failure-to-success recovery
without adding a duplicate success journey. CASE-021 terminal/EOF checks and
CASE-022 package and isolated cleanup ownership remain unchanged or stronger.

The package topology is still five roots and 19 `Process.Execute` journeys:
15 shared-root journeys (including the seven-entry pre-activation batch) and
four isolated lifecycle journeys. The four existing polling/sleep sites in
the isolated lifecycle witnesses are unchanged; this story adds only channel
barriers and no sleep or timeout site.

### Verification evidence

All commands exercised the real in-process customer CLI through
`support.BuildProcess`/`Process.Execute`, with controlled provider and effect
edges and unique invocation streams:

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLIInvalidInputsAreBadRequest$' -count=1 -v
```

PASS, package-reported 2.662 s. The test log recorded
`cases=7 max_concurrent=7 provider_calls=0 product_effects=0 routes_after=0 active_after=0`;
all seven subtests returned their expected BAD_REQUEST response, empty stdout,
and zero provider calls.

```text
go test ./tests/functional/transport/cli/output/... -run '^(TestCLIInvalidInputsAreBadRequest|TestCLIJSONFailureRemainsValidJSON|TestCLIJSONFailureContainsNoPrivateRuntimeFields|TestCLINDJSONFailureEndsWithOneTerminalResult)$' -count=3
```

PASS, package-reported 36.804 s across three repetitions on the final formatted
source. This proves repeat
stability for the bounded overlap, missing-Factory/logical/provider/NDJSON
failure contracts, privacy, and shared-root recovery.

```text
go test -race ./tests/functional/transport/cli/output/... -run '^TestCLIInvalidInputsAreBadRequest$' -count=1 -v
```

PASS, package-reported 8.892 s. No race was detected in the test-owned
pre-activation overlap, per-case stream/route handling, or aggregate effect
observation.

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLIInvalidInputsAreBadRequest$' -count=1 -v
```

PASS on the final source, with the same 7-case/7-concurrent observation. The
full package command also passed:

```text
go test ./tests/functional/transport/cli/output/... -count=1
```

PASS, package-reported 32.926 s. Two earlier bounded attempts on this same
source were retained as host-contended diagnostics: one timed out in the
unchanged `TestCLISlowWriterDoesNotReorderResponseEvents` stdout-write guard,
and one timed out in the unchanged `TestCLIWriterFailureCancelsInvocation`
provider-work-start guard. No lifecycle timeout, sleep, production, or shared
support source was changed; the subsequent exact command passed. All current success, failure, privacy,
backpressure, listener, interruption, ordering, and cleanup witnesses passed
together. `git diff --check` passed.

This story proves the bounded pre-activation overlap, all seven distinct
BAD_REQUEST cases, failure parity, recovery, race safety for the changed
concurrent path, and package-level cleanup. It does not prove the final
three-sample performance target, isolated lifecycle synchronization changes,
clean exact-head loopback, terminal CI, or merge; those remain owned by
GATE-LIFECYCLE, GATE-PERF, VAL-001, and review.

## Story 004 — GATE-LIFECYCLE

The four independent lifecycle roots remain unchanged: slow stdout,
writer-failure cancellation, continuous listener startup/shutdown, and
interrupted human output. The shared gated human-output witness also retains
its existing root and assertions. Four polling loops were replaced with
one-shot notifications emitted by the exact observed test boundary:

| Witness | Notification boundary | Assertion preserved |
| --- | --- | --- |
| `TestCLISlowWriterDoesNotReorderResponseEvents` | `gatedStdoutWriter.Write` entry, before the gate blocks | Invocation is not complete while stdout is blocked; release still yields monotonic records and one terminal result. |
| `TestCLITextStreamSurfacesIncrementalMessages` | First non-empty `firstChunkGatedStdoutWriter.Write` completes its buffer write | Lifecycle output is observable before completion; release still yields canonical human output and stable stderr progress. |
| `TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet` | `interruptibleStdoutCapture.Write` has buffered both required startup messages | One real `/work` reachability request follows startup output; cancellation joins and the reserved port rebinds. |
| `TestCLITextStreamInterruptedRunDoesNotClaimCompletion` | `interruptibleStdoutCapture.Write` has buffered a canonical lifecycle line | Cancellation remains interruptible only after lifecycle output, joins external work, and never claims success. |

The existing external-work start, buffered-record, invocation-completion,
and cleanup channels remain one-shot synchronization boundaries. No isolated
root was shared, no production readiness hook was added, no listener was
faked, and no timeout was broadened. The continuous test now performs exactly
one real local HTTP readiness request after its startup notification; a
request error fails the witness rather than retrying through elapsed-time
polling.

### Post-edit source hashes

| Source | SHA-256 |
| --- | --- |
| `tests/functional/transport/cli/output/json_result_test.go` | `F9CC26CA3AF3B2643DED8FEF94BBA71FFB1CB53E5FAA9D1C11F95F9B51748940` |
| `tests/functional/transport/cli/output/ndjson_stream_test.go` | `48F8DB2C3CFA0D97D6DE4BE6924B2BE5412F7B6070FA0520F379254E5941AEC2` |
| `tests/functional/transport/cli/output/shared_fixture_test.go` | `0BADA8079152BF3F69B8ED0532568E0770C22C6B7DFF6BBFAB7FCE5C2F68C03A` |
| `tests/functional/transport/cli/output/stream_backpressure_test.go` | `53E9D04E8FD209FC49096C16C6562ADB23E77706C6524F606178BAB73C7223C5` |
| `tests/functional/transport/cli/output/text_stream_test.go` | `926AC9582358C7C90655ED2A0E6E83813CC588472228D8264DEF959292B3586A` |

### Synchronization and topology reconciliation

The package remains at five application roots and 19 `Process.Execute`
journeys: 15 shared-root journeys and four isolated lifecycle journeys. The
post-edit source census has zero `time.Sleep` sites and zero `time.Now`
deadline loops. The 16 remaining `time.After` sites are bounded failure
ceilings for completion, cleanup, external-work, buffered-write, and startup
guards; none drives success synchronization. The four former polling loops
now wait on test-owned notifications, and the continuous listener retains its
2-second HTTP client timeout as a single request guard.

### Verification evidence

The following commands exercised the real in-process CLI through
`support.BuildProcess`/`Process.Execute`, with controlled command-runner edges
and the actual local listener where applicable:

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLIWriterFailureCancelsInvocation$' -count=1 -v
```

PASS, package-reported 17.737 s. The writer error remained primary, no
terminal record was emitted, and canceled external work joined.

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLITextStreamSurfacesIncrementalMessages$' -count=3
```

PASS, package-reported 18.970 s. The first-chunk notification preserved the
in-flight lifecycle assertion across all three repetitions.

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet$' -count=3
```

PASS, package-reported 6.073 s. All three repetitions observed the startup
notification, completed one real local readiness request, canceled and joined
the continuous run, and successfully rebound the same port.

```text
go test -race ./tests/functional/transport/cli/output/... -run '^TestCLITextStreamSurfacesIncrementalMessages$' -count=1
```

PASS, package-reported 12.884 s. No race was detected in the changed gated
writer notification or shared fixture execution path.

The initially contended `-count=3` and `-race` attempts are retained as
diagnostics: packaged-factory staging or initialization timed out before the
owned notification boundary. Serial reruns then admitted every owned lifecycle
boundary and completed the required evidence:

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLISlowWriterDoesNotReorderResponseEvents$' -count=3 -v
```

PASS, package-reported 26.843 s. All three blocked-writer journeys remained
incomplete before release and then produced monotonic records with one
terminal result.

```text
go test ./tests/functional/transport/cli/output/... -run '^TestCLITextStreamInterruptedRunDoesNotClaimCompletion$' -count=3 -v
go test ./tests/functional/transport/cli/output/... -run '^TestCLIWriterFailureCancelsInvocation$' -count=3 -v
```

PASS, package-reported 17.235 s and 11.854 s respectively. Every repetition
preserved cancellation error/output, writer-error precedence, absence of a
terminal record after writer failure, and external-work join.

Targeted `-race` selectors passed for slow writer (11.373 s), writer failure
(8.927 s), interrupted run (9.405 s), continuous listener (4.785 s), and the
previously recorded gated human writer (12.884 s); no race was reported. The
continuous listener repeat evidence above and this race run each performed one
real local readiness request, cancellation/join, and same-port rebind. The
final exact package command passed with package-reported time 42.406 s. The
post-edit census remains zero `time.Sleep` sites and zero clock-driven
deadline loops; remaining `time.After` calls are bounded failure or cleanup
ceilings and do not drive success synchronization. `git diff --check` passed.

This story proves deterministic success synchronization for the changed gated
writers, one-request real listener readiness, cancellation/join behavior,
isolated-root preservation, repeat stability, targeted race safety, package
cleanup, and the absence of polling sleeps. It does not prove the final
three-sample performance target, exact-head loopback, terminal CI, or merge;
those remain owned by GATE-PERF, VAL-001, and review.

## Later-gate inputs and remaining edges

- `GATE-PERF`: record three comparable final samples and compare their median
  with the 59.921 s baseline; if the 40% target is not reached, add the
  quantified profile-backed irreducible-floor explanation after the bounded
  optimization pass.
- `GATE-SCOPE` / `VAL-001`: prove the final rebased exact head, clean-room
  behavior, and allowed diff at handoff.

Final numbers, post-edit hashes, final validation-loopback report, and PR
current-head CI URL are intentionally pending later stories. This initial
ledger does not claim those edges.
