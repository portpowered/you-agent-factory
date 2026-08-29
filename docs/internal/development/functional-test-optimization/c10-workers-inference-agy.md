# C10 AGY Worker-Inference Characterization Ledger

Story `functional-test-optimization-c10-workers-inference-agy-001` is
complete. Stories `-002` and `-003` remain open and own migration, direct
teardown, clean-room, scope, and delivery evidence. This ledger records the
pre-structural baseline only; it does not claim migrated parity.

## Authority and scope

- Repository: `you-agent-factory`
- Branch: `functional-test-optimization-c10-workers-inference-agy`
- Characterization head: `0c3bb857910bf4e356b01942954e7272572510f9`
- `origin/main`: `0c3bb857910bf4e356b01942954e7272572510f9`
- PRD stories: `functional-test-optimization-c10-workers-inference-agy-001`,
  `-002`, and `-003`
- Parent behavior: `BEH-01` — preserve the two AGY worker-inference golden
  behaviors while reducing eligible root application starts.
- Story outcome: establish the exact two-test denominator, current public
  witnesses, ordered event/response signatures, fixture identity, current
  topology, cleanup obligations, and named unproven edges.
- Owned additions in this story: this ledger only. The AGY test package and
  shared functional support were inspected but not structurally changed.
- Excluded and untouched: provider recovery behavior, live or paid AGY,
  shared inventory files, package timing baselines, CI workflows, and all
  other provider/test surfaces.
- Source-plan reference: `docs/temp/functional-test-optimization.md` is not
  present in this checkout or in the inspected Git refs. It is recorded as a
  missing planning authority; it was not reconstructed or silently replaced
  with new scope.
- Paid/remote validation: zero calls, `$0`, no credentials, and no customer
  data. Both tests inject `ProviderCommandRunner` at `edges.Edges`.

The process counts below are source-derived application-process starts, not an
OS process census. Each call to the current completion helper builds one
`root.BuildProcess`, starts one `Process.Execute`, stops it, and closes the
process. The current package therefore starts two application processes: one
for each top-level test.

## Characterization procedure and result

The package discovery command reported exactly the two PRD-owned top-level
tests:

```text
go test -count=1 -timeout=5m ./tests/functional/workers/inference/agy -list '^Test'
```

Exit status: `0`.

```text
TestAgyGoldenFinalOnlySuccess
TestAgyGoldenTimeout
ok  github.com/portpowered/infinite-you/tests/functional/workers/inference/agy  0.080s
```

Each selector was then run once with `-count=1` and `-timeout=5m` through the
same local-real root composition and the checked-in controlled AGY command
edge:

```text
go test -json -count=1 -timeout=5m ./tests/functional/workers/inference/agy -run '^TestAgyGoldenFinalOnlySuccess$'
```

Exit status: `0`. Test elapsed: `2.06s`; package elapsed: `2.145s`.

```text
go test -json -count=1 -timeout=5m ./tests/functional/workers/inference/agy -run '^TestAgyGoldenTimeout$'
```

Exit status: `0`. Test elapsed: `2.94s`; package elapsed: `3.027s`.

The timeout selector emits the existing structured `command runner: request
failed` and `transitioner: result failed` diagnostics for the controlled
`context.DeadlineExceeded` edge. Those diagnostics are expected and did not
change the exit status. Local wall-clock values are diagnostic only; they are
not a threshold or a PR timing verdict.

The exact Factory Event and response-frame signatures below were obtained with
a temporary, uncommitted diagnostic probe using the same setup code and public
completion helper. The probe was removed after observation. Its combined
two-subtest attempt exposed a harness lifecycle race (`process API server
starter was never invoked` during the second subtest's packaged initialization)
and was not used as a passing gate; the timeout probe was rerun in isolation and
passed. The two official selector runs above are the characterization gate.

## Exact denominator and current topology

| Test and source | Fixture and controlled edge | Current process/session | Work and command witness | Retained public observations |
| --- | --- | --- | --- | --- |
| `TestAgyGoldenFinalOnlySuccess` — `tests/functional/workers/inference/agy/golden_test.go:32` | Copy of `executor_success`; AGY final-only-success stdout; injected `ProviderCommandRunner` returns exit `0`. | One `root.BuildProcess` and one `Process.Execute`; one invocation-local temporary Factory root and `HOME`; public API default Factory Session (`~default` internally), not an explicit non-default session. | `task:done=1`, `task:failed=0`; measured one command call. | 11 Factory Events; 2 response frames; expected provider-session, response-event, and invocation-result goldens compare successfully. |
| `TestAgyGoldenTimeout` — `tests/functional/workers/inference/agy/golden_test.go:137` | Copy of `executor_success`; AGY timeout stdout; injected runner returns partial stdout and `context.DeadlineExceeded`. | One `root.BuildProcess` and one `Process.Execute`; one invocation-local temporary Factory root and `HOME`; public API default Factory Session (`~default` internally), not an explicit non-default session. | `task:done=0`, `task:failed=1`; the isolated diagnostic probe measured 9 runner calls across 3 retry attempts. The test's source assertion requires at least one call. | 23 Factory Events; 18 response frames; failed `MODEL_RESPONSE` includes AGY metadata and timeout detail; response stream ends in failure and never reports successful run completion. |

Current source-derived application starts: `1 + 1 = 2`. No `t.Parallel`
claim or package-global current-scenario selector exists in the baseline.
The two test functions each own their copied Factory root, provider runner,
server/listener, default session, response capture, and cleanup registration;
there is no shared route table to characterize before story `-002`.

## Fixture identity and hashes

Both manifests are schema version `1`, provider `agy`, provider version
`fixture-1`, fidelity class `final-only`, sanitizer version `1`, and normalize
only `eventId`, `recordedAt`, `factorySessionId`, `runId`, and `itemId`.
The request model is `gemini-3.6-flash-high`. The fixture `session_id` values
are `ses_fixture_agy_final_only_success` and `ses_fixture_agy_timeout`; they
are native fixture metadata, not Factory Session identities.

The following SHA-256 values were read from the checked-in fixture files at
the characterization head. No fixture file was rewritten by either selector.

| Case | File | SHA-256 |
| --- | --- | --- |
| final-only-success | `manifest.json` | `1ff8e030ea762e64cd3baaa5db2ea610d08b1f142732c7b4c6cc1f52d6f9590f` |
| final-only-success | `request.json` | `55c728bc2da33a0d14c483107549b61eed5d972c41864027433cc36e8eeafe8d` |
| final-only-success | `process.json` | `28c675e7ac4f40b494ae0bcc6b6512063dd4dc307789004be124987cf1cf889b` |
| final-only-success | `stdout.txt` | `8380eb1b690cd478ab784db4818616ab8333aef9b2370285223733287fc36f54` |
| final-only-success | `stderr.txt` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| final-only-success | `expected-provider-session.json` | `78a645af44de0cfd7852e995febad430b60ea092a517e6223e11c5f269b0de71` |
| final-only-success | `expected-response-events.ndjson` | `b5f61bf51aab2ec56543828b6a3e764f06bb91d5953181ba23532e6341e2741f` |
| final-only-success | `expected-invocation-result.json` | `17bbcadd8c399efd5b20825f1b4cbbb49911401f699ee8f1be38c134b5b48275` |
| timeout | `manifest.json` | `e19ced81a2176b40bec2e454fef489eddabe0aa687b51ebba6a670cf39a7ad90` |
| timeout | `request.json` | `3f07bce10682baf4abdb98bd15d00651f13edbc5d3b231e78d1229016ffb5039` |
| timeout | `process.json` | `329adc6d5848ed0425561383f3ca1d8c2746f53b2deaedd78e405a1d76017b6d` |
| timeout | `stdout.txt` | `c96f6257fa17e7d59e95772adb6856a8d5d1ab4ce0bbfb5705252ef4ad5ca56f` |
| timeout | `stderr.txt` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| timeout | `expected-provider-session.json` | `2885791ff846ef5e712380c16f85d9505c7a745449036440b2d9a20995790f69` |
| timeout | `expected-response-events.ndjson` | `03d59e72d2e51e3d84c7937812757ef5d5d2ee2fb24282abdc70215659557157` |
| timeout | `expected-invocation-result.json` | `4897aab75aef3e37967e3ee54b9770d64bc84c67214ad716c08a22e211374f7b` |

The success process metadata declares exit code `0`, no stream, no timeout
cancel, and no terminal error. The timeout metadata declares exit code `-1`,
`context-deadline-exceeded`, and `structured-timeout`. Timeout stdout is the
30-byte text `partial answer before timeout`; both stderr fixtures are empty.

## Normalized public witness ledger

### Success: `AGY-S01` and `AGY-S02`

- The manifest, request, process metadata, and all expected goldens load and
  validate before the process runs.
- The Work listing contains exactly one `task:done` item and no `task:failed`
  item. The controlled AGY command edge is called exactly once.
- The retained `MODEL_RESPONSE` has outcome `SUCCEEDED`, model
  `gemini-3.6-flash-high`, provider `antigravity`, no provider-session ID, and
  output text `Parity agy complete response COMPLETE`.
- The retained `DISPATCH_RESPONSE` output is the same exact
  `Parity agy complete response COMPLETE` text and contains `COMPLETE`.
- The final-only response stream has exactly two observed frames: one
  `RUN/COMPLETED` frame with status `completed`, followed by one
  `MESSAGE/COMPLETED` assistant text frame. It has no message delta, tool,
  usage, or fabricated structured lifecycle.
- The provider-session golden is `antigravity` / empty provider-session ID /
  `final-only` / `completed`; the invocation-result golden is `ok: true` with
  the same output and finish reason `stop`.

Normalized success Factory Event order is:

```text
RUN_REQUEST
INITIAL_STRUCTURE_REQUEST
SESSION_STARTED
FACTORY_STATE_RESPONSE
WORK_REQUEST
DISPATCH_REQUEST
DISPATCH_WORKER_SESSION_ASSOCIATION
MODEL_REQUEST
MODEL_RESPONSE
AGENT_RUN_RESPONSE
DISPATCH_RESPONSE
```

The correlation invariant is one request/trace/work chain. The worker-session
association ID prefixes the `modelRequestId` and `agentRunId`; generated IDs,
timestamps, paths, and correlation IDs are compared by relationship rather
than literal value.

### Timeout: `AGY-T01` and `AGY-P01`

- The manifest, request, process metadata, stdout, and empty stderr load and
  validate before the process runs.
- The Work listing contains no `task:done` item and exactly one `task:failed`
  item. The public failure is not reclassified as success.
- Each of the three observed retry attempts has a failed `MODEL_RESPONSE`
  with model `gemini-3.6-flash-high`, provider `antigravity`, outcome
  `FAILED`, and failure detail `{reason: timeout, message: provider invocation
  timed out}`. Provider-session ID is absent.
- The final `DISPATCH_RESPONSE` outcome is `FAILED`, its provider failure is
  `{family: retryable, type: timeout}`, and its output Work remains at the
  `task:init` output state before the failure projection moves the listed Work
  to `task:failed`.
- The response stream contains 18 frames: for each of 9 controlled command
  calls, a completed assistant message `partial answer before timeout` is
  followed by a failed error frame with code `timeout`, message `Agy request
  timed out.`, and `retryable: true`. The 9 calls group into 3 public retry
  run IDs, each with 3 message/error pairs.
- No completed response `RUN` frame reports status `completed`. The existing
  sensitive-output assertion finds no `invalid api key` in the public Factory
  Event or response-event observations.
- The recorded timeout invocation result is `ok: false`, failure reason
  `timeout`, and message `Agy request timed out.`. The current timeout test
  does not compare that expected invocation-result or response NDJSON golden;
  direct migrated parity remains a story `-002` obligation.

Normalized timeout Factory Event order is the initial five events:

```text
RUN_REQUEST
INITIAL_STRUCTURE_REQUEST
SESSION_STARTED
FACTORY_STATE_RESPONSE
WORK_REQUEST
```

followed by this six-event attempt group repeated three times:

```text
DISPATCH_REQUEST
DISPATCH_WORKER_SESSION_ASSOCIATION
MODEL_REQUEST
MODEL_RESPONSE
AGENT_RUN_RESPONSE
DISPATCH_RESPONSE
```

The retry attempt values are `1`, `2`, and `3`. The worker-session association
ID prefixes the corresponding model request and agent run IDs. The request,
trace, Work, transition, and Factory Session correlations remain route-local.

## Cleanup and lifecycle obligations

The current helper source establishes the obligations that migration must
retain, but story `-001` does not claim balanced cleanup counters:

- `testutil.CopyFixtureDir` and the invocation-local `HOME` use `t.TempDir`;
  temporary Factory roots are removed by the test framework after the test.
- `runFactoryToCompletionWithHome` creates one `ProcessAPIServer`, one
  root-built process, and one asynchronous `Process.Execute` command.
- The helper attaches response capture before terminal Work observation,
  waits for the response terminal frame, stops the command, cancels response
  capture after process shutdown, and explicitly calls the process closer.
- `StartProcessCommand` registers a cleanup cancellation/join with a five
  second bound. The helper's explicit stop and the registered cleanup are
  expected to be idempotent.
- The current tests use `--no-record`; no persistence or restart behavior is
  claimed. The current default session is closed as part of process lifecycle;
  there is no explicit non-default Factory Session deletion path yet.
- Listener/port closure, route release, active-call counters, and assertion-
  failure/cancellation cleanup are obligations for the shared process in
  `AGY-CLEAN-003` / story `-002` and the final loopback in story `-003`.

## Complete matrix disposition

Every `functionalTestCaseMatrix` row in the PRD is represented below. A
characterization disposition records a current witness or an explicit absence;
it is not a migrated pass.

| ID | Current characterization | Disposition / owning later gate |
| --- | --- | --- |
| `AGY-S01` | Success Work, one command call, successful Model Response, empty provider-session ID, exact COMPLETE-bearing output, and success goldens pass. | Baseline PASS; migrated parity -> `AGY-MIG-002-SUCCESS`. |
| `AGY-S02` | Final-only success has one completed run and one completed assistant message, with no delta/tool/usage lifecycle. | Baseline PASS; migrated response parity -> `AGY-MIG-002-SUCCESS`. |
| `AGY-T01` | Controlled deadline produces failed Work, failed timeout Model Response, AGY provider metadata, and failed response stream. | Baseline PASS; migrated timeout parity -> `AGY-MIG-002-TIMEOUT`. |
| `AGY-P01` | Partial stdout is exposed only as completed partial message frames; Work never becomes done and forbidden sensitive text is absent. | Baseline PASS; migrated partial-output/redaction parity -> `AGY-MIG-002-TIMEOUT`. |
| `AGY-C01` | Existing helper has bounded command stop and process close, but no direct nonzero-`m.Run` or package-finalizer witness. | Not claimed; direct cancellation/finalization -> `AGY-CLEAN-003`. |
| `AGY-A01` | Existing public assertions fail loudly; adverse cleanup ownership is source-derived, not directly exercised. | Not claimed; assertion-failure cleanup -> `AGY-CLEAN-003`. |
| `AGY-M01` | Valid manifest/request/process fixtures load successfully; malformed fixture injection is not a current test case. | Not applicable to baseline run; malformed pre-start validation -> `AGY-MIG-002`. |
| `AGY-M02` | No malformed AGY-native output witness exists in this package. | Explicitly unproven; `PROVIDERS-AGY-RECOVERY`. |
| `AGY-E01` | Current tests require non-empty model and native session ID, but have no empty route/session registration case. | Validation target, not baseline pass; -> `AGY-MIG-002`. |
| `AGY-D01` | No duplicate canonical Factory-directory route exists before migration. | Explicitly unproven; pre-start route validation -> `AGY-MIG-002`. |
| `AGY-U01` | Current tests do not resolve an unknown controlled route. | Explicitly unproven; fail-closed route -> `AGY-MIG-002`. |
| `AGY-O01` | Probe captured stable success order and timeout attempt-group order above; dynamic IDs preserve correlation relationships. | Baseline PASS; migrated order/correlation parity -> `AGY-MIG-002`. |
| `AGY-O02` | Success response frames are run then message; timeout frames are repeated message/error pairs ending in failure. | Baseline PASS; migrated stream isolation/order -> `AGY-MIG-002`. |
| `AGY-N01` | Top-level tests are serial and have no package-global scenario selector or `t.Parallel` claim. | Baseline topology recorded; immutable route-read safety -> `AGY-MIG-002`. |
| `AGY-CAP01` | Denominator is exactly two tests and source-derived starts are exactly two, one per test. | Baseline PASS; one-process/two-session bound -> `AGY-MIG-002` / `AGY-PKG-003`. |
| `AGY-CL01` | Each helper owns a temporary root, server, default session, response stream, command, and process close; balanced counters are not exposed. | Obligations recorded; scenario cleanup -> `AGY-CLEAN-003`. |
| `AGY-CL02` | `t.Cleanup` joins commands and removes temporary roots; cancellation, assertion failure, cleanup-error promotion, and route counters are not directly proven. | Explicitly unproven; package finalization -> `AGY-CLEAN-003` / `VAL-005`. |
| `AGY-R01` | The timeout selector reaches failed Work and closes its process; no shared-process success-after-timeout recovery is exercised. | Baseline failure captured; shared-route recovery -> `AGY-MIG-002-TIMEOUT`. |
| `AGY-I01` | Cleanup helpers are registered more than once across explicit stop and `t.Cleanup`, but no counter/panic assertion exists. | Explicitly unproven; idempotent teardown -> `AGY-CLEAN-003`. |
| `AGY-AUTH01` | The injected runner prevents live AGY authorization and remote execution; no authorization-boundary behavior is claimed. | Zero-cost control recorded; final zero-remote census -> `AGY-PKG-003` / `VAL-005`. |
| `AGY-OUT01` | No non-timeout AGY outage witness exists. | Explicitly unproven; `PROVIDERS-AGY-RECOVERY`. |
| `AGY-PS01` | Both helpers pass `--no-record`; persistence/restart behavior is not exercised. | No persistence claim; final reconciliation -> `AGY-PKG-003` / `VAL-005`. |

## AGY-CHAR-001 evidence boundary

| Criterion / gate | Result in this story | Property proved / remaining edge |
| --- | --- | --- |
| Exact denominator | PASS | `-list '^Test'` returned exactly `TestAgyGoldenFinalOnlySuccess` and `TestAgyGoldenTimeout`; no named subtests. |
| Success behavior | PASS | Exact selector passed through local-real root/Workers/Providers with the controlled command edge; Work, Factory Event, provider-session, response-stream, invocation-result, and golden assertions passed. |
| Timeout behavior | PASS | Exact selector passed with timeout/partial-output/failure/redaction assertions; the supplemental isolated probe recorded 9 calls, 23 Factory Events, and 18 response frames. |
| Stable identity | PASS | Both manifest IDs, request models/session IDs, process metadata, fidelity class, and all 16 non-empty/empty fixture hashes are recorded. |
| Current topology | PASS | Source-derived process starts are `2`; each test uses one root/process and the default Factory Session. One-process/two-explicit-session topology remains unproven. |
| Dependency fidelity | PASS for characterization | Production `root.BuildProcess`/`Process.Execute` and the Workers-to-Providers path were exercised; only the AGY command effect was controlled. |
| Cleanup | RECORDED, NOT CLAIMED | Existing close/join/temp-root obligations are recorded. Balanced route/session/stream/listener/activity counters and adverse exits belong to `AGY-CLEAN-003`. |
| Matrix | PASS as disposition | All 23 PRD rows are listed with current evidence or named N/A/later gate; no native malformed-output or outage behavior was invented. |
| Remote/paid | PASS by controlled boundary | No live credential or paid AGY call was used. Authorization semantics and an independently instrumented network census remain outside this story. |
| Migration parity | NOT CLAIMED | Stories `-002` and `-003` own immutable routes, explicit sessions, shared process, recovery, cleanup, clean-room, and final package proof. |
| PR timing | NOT CLAIMED | Review-owned `PR-CI-004` remains pending; local timings are diagnostic and contaminated by shared-host variance. |

No production behavior or fixture checksum changed in this story. The next
iteration should implement only story `functional-test-optimization-c10-workers-inference-agy-002` after this ledger is reviewed as the frozen baseline.
