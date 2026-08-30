# Validation report: C14 sessions/restart optimization

## Environment and artifact

- Commit/build identifier: implementation/test head
  `8a1f3edb09df6a99b452338dd6cec0ff79bf5d4d`; the report and evidence-ledger
  edits are documentation-only descendants of this validated source.
- Environment and configuration: Windows/amd64, Go `go1.25.0`,
  `CGO_ENABLED=1`; package commands used `-count=1` unless a repeat gate is
  named; no paid or remote-provider calls.
- Customer entry point: the restart functional package's real `you` CLI and
  production-composed daemon/server boundaries.
- Real and substituted dependencies: real built CLI, Windows processes,
  loopback, filesystem, recording, child-process, and production composition;
  only the existing controlled provider/test edges are substituted as already
  characterized by the lane.
- Cost/call budget used: `$0`; zero paid or remote-provider calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| One guarded package CLI build | PASS | Focused, complete-package, race, and repeat runs logged one normal-process package build (`builds=1`) and zero helper builds; package teardown followed all joins. | Other operating systems and future toolchains. |
| Partial-setup cleanup and failure status | PASS | Direct CASE-U08 exited `0`; its child failed nonzero after temp-root allocation, emitted the injected diagnostic and allocated paths, skipped the scenario marker, and both partial paths were absent after teardown. | Filesystem failures other than the deterministic injected branch. |
| Restart behavior and assertion denominator | PASS | Complete package and race runs passed all eight selectors; the seven-selector/148 lexical failure-call-site pre-change denominator is unchanged, and the added probe only strengthens cleanup coverage. | Unsupported platforms and future source changes. |
| Complete package proof | PASS | `go test ./tests/functional/sessions/restart/... -count=1 -v` exited `0` in `50.933s`; helper, board restart/persistence, remap, resume, history, and cleanup witnesses passed. | Hosted CI and merge. |
| Supported race proof | PASS | `go test -race ./tests/functional/sessions/restart/... -count=1 -v` exited `0` in `77.541s` on Windows/amd64; no race report or process-cleanup failure occurred. | Unsupported platforms and unexercised concurrency. |
| Performance or measured floor | PASS | Final successful outer samples were `57.45s`, `59.86s`, and `52.64s`, median `57.45s` versus baseline `44.1461053s`; the permitted one-build profile-backed measured-floor explanation is retained rather than claiming the 40% branch. | Portable or quiet-host wall time. |
| Scope and C09 preservation | PASS | Rebase ancestry, `git diff --check`, and the three-dot path audit passed; only the three restart test files and two C14 evidence documents changed, with fresh per-operation CLI contexts and no new sleep/timeout padding. | Future GitHub conflict resolution. |
| Clean-room reproducibility | PASS | A detached clean worktree at the exact implementation/test head ran the complete package successfully in `20.266s`; no repair was made and the worktree was removed afterward. | Remote runner behavior. |
| Repository quality gate | PASS | `make test` exited `0` in `202.29s`; the unitlane report recorded 169 tests, 168 passed, 0 failed, and 1 skipped for an existing Windows privilege/platform condition. | Full hosted CI policy. |

## Customer journey

1. The direct CASE-U08 command
   `go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceSharedBinaryPartialSetupFailure$' -count=1 -v`
   exited `0` in `0.083s`. The parent launched an isolated copy of the test
   binary, observed the injected nonzero setup failure and diagnostic, decoded
   the allocated directory/artifact report, and verified both paths plus the
   scenario marker were absent after child exit.
2. The focused real-process command
   `go test ./tests/functional/sessions/restart/... -count=1 -run '^TestBoardPersistenceCLIRestartRoundTrip$' -v`
   exited `0` in `13.382s`. The real CLI submitted work, crossed readiness,
   Worker Session, clean-stop, replay/re-arm, release-file, response,
   persistence, and second-restart boundaries, and logged one package build.
3. The complete package command
   `go test ./tests/functional/sessions/restart/... -count=1 -v` exited `0`
   in `50.933s`; all eight selectors passed with private scenario resources.
4. The supported race command exited `0` in `77.541s` with the same eight
   selectors and no race diagnostics.
5. A detached clean worktree at
   `8a1f3edb09df6a99b452338dd6cec0ff79bf5d4d` ran
   `go test ./tests/functional/sessions/restart/... -count=1`, exited `0` in
   `20.266s`, and was removed after the run.
6. The prescribed three-run final timing procedure ran the complete package
   three times sequentially, enforced exit `0` for every run, and retained
   outer samples `57.45s`, `59.86s`, and `52.64s`; the middle sorted value is
   `57.45s`.

## Cross-task integration and usability

- Documentation discoverability: the evidence ledger and this report are in
  `docs/internal/development/functional-test-optimization/` and identify the
  owning gates, exact artifact, and remaining review-owned edges.
- Permission and error behavior: unchanged; malformed and missing persistence,
  timeout, cancellation, restart, remap, resume, history, and cleanup
  diagnostics remain asserted.
- Persistence/reload behavior: complete package and clean-room runs exercised
  the real durable board/session restart and event-history paths.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  customer-prose behavior changed.
- Operational signals: the bounded failure probe reports timeout phase,
  kill/join outcome, and captured output; existing daemon observations and
  per-operation CLI diagnostics were not altered.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | — | No local loopback defect requiring correction. | All applicable local criteria passed without repair. | Commands and outputs above. |

## Verdict

**PASS** for the local clean-room loopback and story-003 implementation gates.
The review-owned hosted CI terminal result, conflict-free merge, and future
platform behavior remain outside this local report and are not claimed here.

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no local criterion is FAIL or BLOCKED. Implementation handoff
still requires the final head to be pushed, required CI to be started, and the
before/after medians plus CI run URL to be posted in the PR conversation;
review owns terminal CI and merge.
