# C10 AGY Worker-Inference Characterization Ledger

Stories `functional-test-optimization-c10-workers-inference-agy-001`, `-002`,
and `-003` are complete for implementation-stage handoff. This ledger records
the frozen baseline, bounded migrated evidence, final clean-room validation,
and the review-owned CI/merge boundary. It does not claim live AGY behavior,
excluded provider recovery, terminal CI, or merge.

## Authority and scope

- Repository: `you-agent-factory`
- Branch: `functional-test-optimization-c10-workers-inference-agy`
- Characterization head: `0c3bb857910bf4e356b01942954e7272572510f9`
- Migration head: `211c11fef9` (with shared-process implementation at
  `5c07bce189`)
- `origin/main`: `0c3bb857910bf4e356b01942954e7272572510f9`
- PRD stories: `functional-test-optimization-c10-workers-inference-agy-001`,
  `-002`, and `-003`
- Parent behavior: `BEH-01` — preserve the two AGY worker-inference golden
  behaviors while reducing eligible root application starts.
- Story outcomes: establish the exact two-test denominator and witnesses;
  migrate both tests to one package-lifecycle process with immutable routes,
  explicit sessions, and bounded cleanup census.
- Owned additions in the characterization story: this ledger only. Owned
  additions in migration story `-002`: `tests/functional/workers/inference/agy/**`
  and the migration evidence recorded below.
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

## Story `functional-test-optimization-c10-workers-inference-agy-002` migration evidence

The migration retains the same local-real production composition and controls
only the AGY command edge. Both top-level tests now obtain one package-scoped
fixture from `TestMain`, open a fresh unique non-default explicit Factory
Session, attach its response stream before submitting Work, and close that
session/stream before returning. The two absolute Factory-directory routes are
registered before `root.BuildProcess`, frozen before `Process.Execute`, and
never select by mutable test order or package-global current-scenario state.

The required bounded repeat passed:

```text
go test -count=3 -timeout=10m ./tests/functional/workers/inference/agy -run '^(TestAgyGoldenFinalOnlySuccess|TestAgyGoldenTimeout)$'
```

Exit status: `0`; package elapsed: `5.817s`.
The focused success-only repeat also passed (`2.287s`) and the timeout-only
repeat passed (`5.197s`). These local timings are diagnostic only.

The repeated package execution observed one `root.BuildProcess` construction,
one API-server start, two frozen routes, and six unique explicit sessions and
six response streams across the three repetitions of each test. Every session
and stream was closed/deleted, active command calls were zero after each
scenario, and finalization canceled/joined the daemon, closed the process and
listener, released both routes, and removed the temporary package root. The
scenario-local route windows observed one success call and nine timeout calls
per repetition (three success calls and 27 timeout calls across the full
`-count=3` package execution).

The success witness remained 11 Factory Events and two response frames in the
characterized order; the timeout witness remained 23 Factory Events and 18
response frames with three retry run IDs and nine message/error pairs. Work,
provider-session, response-event, invocation-result, redaction, and exact
event-order assertions passed, and all checked-in golden comparisons remained
unchanged. Setup also exercises fail-closed empty/duplicate/unknown/canceled
route validation without executing either golden or exposing sensitive input.

| Gate | Result | Property proved | Not proved |
| --- | --- | --- | --- |
| `AGY-MIG-002-SUCCESS` | PASS | Success parity, one command per repetition, explicit-session/stream isolation, final-only response topology, unchanged goldens. | Real AGY or PR timing. |
| `AGY-MIG-002-TIMEOUT` | PASS | Timeout/partial-output parity, nine calls per repetition, retry/run grouping, failure classification, redaction, and success-after-timeout recovery. | Non-timeout outage or malformed native output. |
| `AGY-DET-002` | PASS | One package process/API start across three repetitions, immutable routing, unique sessions, and balanced normal cleanup. | Race safety and injected cleanup-error/assertion-failure exits. |
| `AGY-MIG-002` | PASS | Both characterized behaviors through one process and separate explicit sessions. | Clean-room independence, final package proof, PR timing, and terminal CI. |

At the migration checkpoint, the gates below were intentionally deferred to
story `-003`. Their final dispositions are recorded in the validation-loopback
section below.

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
  did not compare that expected invocation-result or response NDJSON golden;
  the migrated timeout test now compares both through the shared process.

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

## Baseline cleanup and lifecycle obligations

The baseline helper source establishes the obligations that migration must
retain. Story `-001` did not claim balanced cleanup counters; the migrated
normal path records them in the story `-002` evidence above:

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
  claimed. The migrated tests use explicit non-default Factory Sessions and
  delete them after each scenario.
- Listener/port closure, route release, active-call counters, and normal
  cancellation cleanup are directly counted by story `-002`. The final
  loopback adds behavioral error-joining coverage for assertion and cleanup
  failures; it does not claim OS process-tree evidence.

## Complete matrix disposition

Every `functionalTestCaseMatrix` row in the PRD is represented below. The
baseline characterization and the migrated status are kept distinct so an
explicit absence is not mistaken for a migrated pass.

| ID | Current characterization | Disposition / owning later gate |
| --- | --- | --- |
| `AGY-S01` | Success Work, one command call, successful Model Response, empty provider-session ID, exact COMPLETE-bearing output, and success goldens pass. | Baseline and migrated PASS; `AGY-MIG-002-SUCCESS`. |
| `AGY-S02` | Final-only success has one completed run and one completed assistant message, with no delta/tool/usage lifecycle. | Baseline and migrated PASS; `AGY-MIG-002-SUCCESS`. |
| `AGY-T01` | Controlled deadline produces failed Work, failed timeout Model Response, AGY provider metadata, and failed response stream. | Baseline and migrated PASS; `AGY-MIG-002-TIMEOUT`. |
| `AGY-P01` | Partial stdout is exposed only as completed partial message frames; Work never becomes done and forbidden sensitive text is absent. | Baseline and migrated PASS; `AGY-MIG-002-TIMEOUT`. |
| `AGY-C01` | Existing helper has bounded command stop and process close; the migration adds package finalizer cancellation/join and process/listener census. | Normal migrated path and cleanup coordinator's release-after-error behavior PASS; OS process-tree evidence remains unclaimed. |
| `AGY-A01` | Existing public assertions fail loudly; adverse cleanup ownership is source-derived, not directly exercised. | Scenario cleanup and primary-plus-cleanup error preservation PASS in `TestAgyCleanupPreservesAssertionAndCleanupErrors`. |
| `AGY-M01` | Valid manifest/request/process fixtures load successfully; malformed fixture injection is not a current test case. | Pre-start validation and final cleanup reconciliation PASS; malformed-fixture behavior remains outside this lane. |
| `AGY-M02` | No malformed AGY-native output witness exists in this package. | Explicitly unproven; `PROVIDERS-AGY-RECOVERY`. |
| `AGY-E01` | Current tests require non-empty model and native session ID, but have no empty route/session registration case. | Empty registration/session validation is migrated; `AGY-MIG-002`. |
| `AGY-D01` | No duplicate canonical Factory-directory route exists before migration. | Duplicate route rejection is migrated; `AGY-MIG-002`. |
| `AGY-U01` | Current tests do not resolve an unknown controlled route. | Fail-closed unknown route is migrated; `AGY-MIG-002`. |
| `AGY-O01` | Probe captured stable success order and timeout attempt-group order above; dynamic IDs preserve correlation relationships. | Baseline and migrated order/correlation PASS; `AGY-MIG-002`. |
| `AGY-O02` | Success response frames are run then message; timeout frames are repeated message/error pairs ending in failure. | Baseline and migrated stream isolation/order PASS; `AGY-MIG-002`. |
| `AGY-N01` | Top-level tests are serial and have no package-global scenario selector or `t.Parallel` claim. | Immutable route-read safety and serial explicit sessions PASS; `AGY-MIG-002`. |
| `AGY-CAP01` | Denominator is exactly two tests and source-derived starts are exactly two, one per test. | Migrated bound is one process and two explicit sessions per package execution; `AGY-MIG-002` / `AGY-PKG-003`. |
| `AGY-CL01` | Each helper owns a temporary root, server, default session, response stream, command, and process close; balanced counters are not exposed. | Scenario session/stream/activity cleanup and final package census PASS; the cleanup coordinator test covers every release/check operation. |
| `AGY-CL02` | `t.Cleanup` joins commands and removes temporary roots; cancellation, assertion failure, cleanup-error promotion, and route counters are not directly proven. | Normal package finalization plus injected primary/cleanup error joining PASS; OS-level process-tree proof remains outside the fixture. |
| `AGY-R01` | The timeout selector reaches failed Work and closes its process; no shared-process success-after-timeout recovery is exercised. | Shared-process success-after-timeout recovery PASS; `AGY-MIG-002-TIMEOUT`. |
| `AGY-I01` | Cleanup helpers are registered more than once across explicit stop and `t.Cleanup`, but no counter/panic assertion exists. | Scenario close is idempotent, counters remain balanced, and cleanup errors are joined without skipping later operations. |
| `AGY-AUTH01` | The injected runner prevents live AGY authorization and remote execution; no authorization-boundary behavior is claimed. | Zero-cost control and final zero-remote fixture census PASS; authorization semantics remain unclaimed. |
| `AGY-OUT01` | No non-timeout AGY outage witness exists. | Explicitly unproven; `PROVIDERS-AGY-RECOVERY`. |
| `AGY-PS01` | Both helpers pass `--no-record`; persistence/restart behavior is not exercised. | No persistence claim; final no-record reconciliation PASS. |

## AGY-CHAR-001 evidence boundary

| Criterion / gate | Result in this story | Property proved / remaining edge |
| --- | --- | --- |
| Exact denominator | PASS | `-list '^Test'` returned exactly `TestAgyGoldenFinalOnlySuccess` and `TestAgyGoldenTimeout`; no named subtests. |
| Success behavior | PASS | Exact selector passed through local-real root/Workers/Providers with the controlled command edge; Work, Factory Event, provider-session, response-stream, invocation-result, and golden assertions passed. |
| Timeout behavior | PASS | Exact selector passed with timeout/partial-output/failure/redaction assertions; the supplemental isolated probe recorded 9 calls, 23 Factory Events, and 18 response frames. |
| Stable identity | PASS | Both manifest IDs, request models/session IDs, process metadata, fidelity class, and all 16 non-empty/empty fixture hashes are recorded. |
| Current topology | PASS | Source-derived baseline starts are `2`; the migrated topology is recorded separately as one package process with explicit sessions in story `-002`. |
| Dependency fidelity | PASS for characterization | Production `root.BuildProcess`/`Process.Execute` and the Workers-to-Providers path were exercised; only the AGY command effect was controlled. |
| Cleanup | RECORDED, NOT CLAIMED | Existing close/join/temp-root obligations are recorded. Balanced route/session/stream/listener/activity counters and adverse exits belong to `AGY-CLEAN-003`. |
| Matrix | PASS as disposition | All 23 PRD rows are listed with current evidence or named N/A/later gate; no native malformed-output or outage behavior was invented. |
| Remote/paid | PASS by controlled boundary | No live credential or paid AGY call was used. Authorization semantics and an independently instrumented network census remain outside this story. |
| Migration parity | RECORDED IN `-002` | Shared process, immutable routes, explicit sessions, recovery, normal cleanup, and parity passed in the bounded migration repeat; clean-room and final package proof remain story `-003`. |
| PR timing | NOT CLAIMED | Review-owned `PR-CI-004` remains pending; local timings are diagnostic and contaminated by shared-host variance. |

## Story `functional-test-optimization-c10-workers-inference-agy-003` final validation loopback

### VAL-005 report

| Field | Result |
| --- | --- |
| Validation status | PASS for the implementation-stage handoff; review owns terminal CI, conflict resolution, and merge. |
| Runtime procedure | `go test -count=1 -timeout=10m ./tests/functional/workers/inference/agy` run exactly once after the final functional-test refactor and size-gate split. Exit `0`; package elapsed `4.469s`. |
| Repetition procedure | `go test -count=3 -timeout=10m ./tests/functional/workers/inference/agy -run '^(TestAgyGoldenFinalOnlySuccess|TestAgyGoldenTimeout)$'` after the same split. Exit `0`; package elapsed `6.383s`. |
| Full local gate | Fresh `make test` after the lifecycle/error/stream changes: exit `0`, `153` tests passed, `1` skipped, `0` failed. The later helper-only file split was covered by the final package run and backend-size check. |
| Size gate | `go run ./cmd/backendsizecheck -- -root .` exit `0`; all owned Go files are within the 1000-line file and 100-line function limits. |
| Dependency fidelity | Actual `root.BuildProcess`/`Process.Execute`, Factory Session, Work, Factory Event, response-stream, Workers-to-Providers path, and `edges.Edges` controlled command boundary; no built CLI or live AGY. |
| Cost and data | Zero remote/paid AGY calls, zero credentials, zero customer data; checked-in sanitized fixtures and unchanged golden files. |

The exact package run passed both top-level goldens through one production-
composed process. Its normal finalizer reconciled one root/API start, explicit
Factory Sessions and response streams, zero active command calls, released
routes, closed listener/process state, and absent temporary roots. The response
reader consumed exactly 2 success frames or 18 timeout frames, explicitly
deleted the session to close the session-owned stream, and then required a
bounded normal EOF; an extra frame, early closure, or terminal read error
fails the test and is retained for cleanup reporting.

The adverse cleanup test, `TestAgyCleanupPreservesAssertionAndCleanupErrors`,
injects one primary assertion error and independent errors from session,
daemon, process, API/listener, command-activity, route, census, and temporary-
root operations. It verifies the primary error and every cleanup error remain
discoverable with `errors.Is`, every operation still releases/checks its owned
state, and all sessions, streams, active calls, routes, daemon/process/listener
flags, and the root are released. This is coordinator-level failure injection;
it does not claim an OS process-tree census after a forcibly killed child.

### Gate reconciliation

| Gate | Result | Exact property proved | Remaining unproven edge |
| --- | --- | --- | --- |
| `AGY-CLEAN-003` | PASS | Normal finalization plus injected primary/assertion and cleanup failures preserve all errors and run every cleanup/check operation. | Forced OS child-process kill and nonzero `m.Run` path are not separately injected. |
| `AGY-PKG-003` | PASS | The exact one-run package gate proves one process/API start, both golden outcomes, balanced sessions/streams, zero active calls, released routes, closed listener/process, and removed root. | OS-wide process inventory is not claimed. |
| `AGY-SCOPE-003` | PASS | Final PR scope is limited to the c10 ledger and `tests/functional/workers/inference/agy/**`; no `prd.json`, `progress.txt`, provider/recovery, inventory, or unrelated product file is included. | The absent `docs/temp/functional-test-optimization.md` source plan remains a recorded planning dependency. |
| `PR-CI-004` | RECORDED / REVIEW-OWNED | The prior hosted Backend Functional Coverage report at head `d23666349df3dfca97b21220bd7a38da17c0f074` (run `33235638878`) recorded the AGY package timing as `1.448s` with a passing package result. | A fresh hosted timing comparison on the final head, terminal CI, and the bounded non-improvement decision occur after push in review; local timings are diagnostic only. |
| `VAL-005` | PASS | This report records the exact final package procedure, result, fidelity, cleanup census, scope, cost boundary, and proved/not-proved edges without mutating fixtures or runtime state. | Live AGY, malformed native output, non-timeout outage, canonical inventory, and merge remain outside this lane. |

### Scope and ancestry audit

The read-only audit procedure is:

```text
git diff --name-only origin/main...HEAD
git log --format='%H %P %s' origin/main..HEAD
```

At final delivery the expected owned paths are:

```text
docs/internal/development/functional-test-optimization/c10-workers-inference-agy.md
tests/functional/workers/inference/agy/golden_test.go
tests/functional/workers/inference/agy/shared_process_test.go
tests/functional/workers/inference/agy/shared_process_cleanup_test.go
tests/functional/workers/inference/agy/shared_process_edges_test.go
```

The final lane commits contain only the characterization ledger, the AGY
golden migration, and the two focused shared-process support/test files. No
provider/AGY recovery implementation or inventory history is in the ancestry;
the missing source-plan path is recorded above rather than reconstructed.

No production behavior or fixture checksum changed in characterization,
migration, or this validation loopback. The implementation-stage finish line
is final head pushed, PR open, and required CI started; terminal CI and merge
remain review-stage responsibilities, and CI-run evidence belongs in the PR
conversation rather than a commit.
