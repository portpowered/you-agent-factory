# Validation report: C06 workers/mock contention

## Environment and artifact

- Commit/build identifier: `0ba837e2da5f0db76a69c9430b0649a73cb954d1`; the
  validation ran from a separate detached clean worktree at that commit.
- Environment and configuration: Windows `amd64`, Go `go1.25.0`; the clean
  checkout had an empty `git status --short --untracked-files=all` result.
- Customer entry point: the existing `tests/functional/workers/mock` package,
  using the production root composition and `Process.Execute`; the three
  built-CLI rows retain their real executable/process boundary.
- Real and substituted dependencies: production root/runtime composition and a
  locally built `./cmd/factory` binary; provider/script command runners, the
  worker gate, session-ID generator, and input-directory walker remain the
  package's injected external-effect edges. No remote provider or paid call
  was used.
- Cost/call budget used: `$0`, zero remote calls, zero paid calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Clean checkout and source identity | PASS | Detached checkout resolved to `0ba837e2d`; accepted-to-current workers/mock diff was empty; package discovery exited 0 and emitted the same four default top-level tests. | A future source change would require a new validation run. |
| Topology | PASS | Clean source inspection found one `TestSharedProcessWorkersMock` fixture/root for 19 child rows, 15 explicit hosted-session rows, four serialized invocation-local rows, one `sync.Once` CLI build, and the three built-CLI rows' 5 + 7 + 3 = 15 required subprocess invocations. | Process scheduling interleavings are not exhaustively explored. |
| MATRIX-001 through MATRIX-037 behavior | PASS | Normal package execution exited 0; the existing four top-level tests and 19 shared child rows exercised the unchanged public Work, Factory Event, dispatch, mock-worker, capacity, drain, cancellation, CLI, failure, and cleanup assertions. | Remote providers, exhaustive schedules, and parent-project full-suite criteria are outside this package gate. |
| Repeatability | PASS | `go test -count=3 -timeout=45m ./tests/functional/workers/mock` exited 0 and reported `58.798s`; all three executions passed. | This is not the 146-package contention profile. |
| Supported race safety | PASS | `go test -race -count=1 -timeout=20m ./tests/functional/workers/mock` exited 0 and reported `30.160s`; no race report was emitted. | Other platforms and exhaustive concurrency schedules are not proved. |
| CLEAN-001 | PASS | Before/after census found no process whose command line contained the validation marker; listening sockets stayed at 40; the seven pre-existing matching temp-prefix directories were unchanged. Package session deletion, server/TestMain teardown, joined cancellation, `t.TempDir`, and compiled-CLI cleanup assertions passed. | Residue outside the observed process/listener/temp census or unrelated host contamination is not attributable to this run. |
| Handoff boundary | PASS | This report records local proof separately from PR-CI-001, terminal CI, merge, and parent-project gates; no local result is used to claim those gates. | Review must record this PR's package timing in a PR comment and own terminal CI, conflicts, and merge. |

## Customer journey

1. A separate detached worktree was created at the validation commit. `git
   status --short --untracked-files=all` returned no entries, and `git
   rev-parse HEAD` returned `0ba837e2da5f0db76a69c9430b0649a73cb954d1`.
2. `go test ./tests/functional/workers/mock -list '^Test'` exited 0 and
   emitted exactly `TestBuiltCLIBatchExitCodesReportSingleWorkOutcome`,
   `TestBuiltCLIBatchExitCodesAggregateFailureCauses`,
   `TestBuiltCLINamedInvocationExitCodesCharacterizeOneShot`, and
   `TestSharedProcessWorkersMock`. Source inspection of the shared fixture
   reconciled its 19 named child rows with the C06 topology artifact.
3. The declared package gates then produced these independent clean-checkout
   observations:

   ```text
   go test -count=1 -timeout=15m ./tests/functional/workers/mock
   ok github.com/portpowered/infinite-you/tests/functional/workers/mock 24.715s
   EXIT=0

   go test -count=3 -timeout=45m ./tests/functional/workers/mock
   ok github.com/portpowered/infinite-you/tests/functional/workers/mock 58.798s
   EXIT=0

   go test -race -count=1 -timeout=20m ./tests/functional/workers/mock
   ok github.com/portpowered/infinite-you/tests/functional/workers/mock 30.160s
   EXIT=0
   ```

4. The post-run census returned `none` for validation-marker processes and
   retained the same 40 listening sockets and seven matching temp-prefix
   directories observed before execution. The four older
   `you-cli-mock-worker-package-*` directories, plus the three pre-existing
   C06/workers-mock diagnostic directories, were retained as host evidence and
   not attributed to this clean-room run.

The behavior grouping covered by the unchanged package is:

| Matrix rows | Observed contract family |
| --- | --- |
| 001–010, 020–022, 032 | Dispatch metadata, routing, artifact enforcement, usage, and successful CLI outcomes |
| 011–015 | JavaScript/mock capacity admission, safe reduction, conflict, recording, replay, and cursors |
| 016–019 | Plain-batch drain, finite/continuous liveness, pre-activation cancellation, and joined post-activation cancellation |
| 023–034 | Terminal, aggregate, circuit-breaker, script, human, JSON, and named CLI failure outcomes |
| 035–037 | Assertion/host-exit cleanup, authorization non-applicability, and existing boundary coverage |

## Cross-task integration and usability

- Documentation discoverability: the accepted/current topology decision is in
  `docs/internal/development/functional-test-optimization/c06-workers-mock-contention.md`;
  this independent report is at
  `validation/functional-test-optimization-c06-workers-mock-contention.md`.
- Permission and error behavior: unchanged package assertions cover unknown
  worker overrides, stable mock failure, capacity conflict, and CLI failure
  reporting. MATRIX-036 remains not-applicable because no local authorization
  boundary exists; no authorization behavior was invented.
- Persistence/reload behavior: the clean run includes the existing Factory
  Session, Work, Factory Event, recording/replay, cursor, deletion, and next
  session usability assertions.
- Accessibility/keyboard/responsive behavior: not applicable; this story
  changes no UI or browser surface.
- Operational signals: each declared command returned an explicit exit code;
  package timing, race output, process census, listener count, and temp-prefix
  census were retained in this report.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| VAL-F-001 | Info | `Test-Path docs/temp/functional-test-optimization.md` in the clean checkout | The PRD's named source-plan provenance would be present if available. | The path is absent, but PRD, C01 P002, accepted PR #2336 evidence, current source, and the C06 artifact agree; no topology conflict or implementation change is required. | C06 topology artifact's retained provenance note and clean-checkout source inspection |

## Verdict

PASS

This clean-room run independently proves the planned local topology, complete
unchanged package behavior, supported race safety, and zero-new-residue cleanup.
It does not claim PR-CI-001, terminal CI, merge, universal performance, or the
parent-project Acceptance Criteria 1, 3, and 5. Review owns the remaining
delivery gates after the final head is pushed and CI starts.
