# Validation report: C14 sessions/restart optimization

## Environment and artifact

- Commit/build identifier: rebased implementation/test head
  `c9e3e727d5f6b22e00e0f2b5cdedc2f847ef567d`; exact post-rebase runtime proof
  remains blocked by the shared host's Go build completion failure.
- Environment and configuration: Windows/amd64, Go `go1.25.0`,
  `CGO_ENABLED=1`, package command with `-count=1`; no paid or remote
  provider calls.
- Customer entry point: the restart functional package's real `you` CLI and
  production-composed daemon/server boundaries.
- Real and substituted dependencies: the direct CASE-U08 probe used the real
  test binary, filesystem, and process boundary; the full restart journey was
  not reached because its real CLI build command did not return on the shared
  host. Existing controlled provider/test edges remain the only substitutes.
- Cost/call budget used: `$0`; zero paid or remote-provider calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| One guarded package CLI build | BLOCKED | Direct CASE-U08 passed and the new probe is bounded, but two exact focused runs timed out inside the unchanged normal-process `go build ./cmd/factory` before package build completion could be observed. | Normal-process one-build topology on this exact head. |
| Restart behavior and assertion denominator | BLOCKED | The prior-head seven-selector/148 lexical failure-call-site denominator and case mapping remain unchanged; exact post-rebase full-selector behavior was not reached. | All CASE-H/U/R/B runtime witnesses on this exact head. |
| Complete package proof | BLOCKED | The required focused package attempt exited `1` after the existing `10m0s` test bound, before daemon/scenario execution. | Complete package behavior and cleanup. |
| Supported race proof | BLOCKED | Not rerun after the rebase because the exact package build cannot complete on the shared host. | Race behavior on the exact head. |
| Performance or measured floor | BLOCKED | Prior implementation-head samples remain historical evidence only; no comparable final samples were collected for `c9e3e727d5f6`. | Exact-head median/floor and PR package latency. |
| Scope and C09 preservation | PASS | The rebased three-dot diff is limited to the three restart test files and C14 evidence files; fresh 30-second per-operation CLI contexts, no new sleeps, and no public/production/shared-support changes remain. | A future GitHub conflict request. |
| Clean-room reproducibility | BLOCKED | The exact-head clean-room package proof was not run because the same real CLI build completion is unavailable under current host contention. | Clean checkout/worktree package proof. |
| Security, privacy, compatibility, and cost | PASS | The loopback is local and sanitized, public contracts/generated files are unchanged, existing failure/cleanup diagnostics pass, and paid/remote calls are zero. | Unrelated system-wide controls. |
| PR handoff | BLOCKED | PR #2455 remains open on the prior pushed head; this rebased head is not pushed and has no new CI run. | Final push, CI start, terminal CI, conflict resolution, and merge. |

## Customer journey

1. The direct exact-head CASE-U08 command
   `go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceSharedBinaryPartialSetupFailure$' -count=1 -v`
   exited `0` in `0.108s`. It launched an isolated child copy of the test
   binary, observed the injected nonzero setup failure and diagnostic, decoded
   the allocated directory/artifact report, and verified both paths plus the
   scenario marker were absent after child exit.
2. The required focused real-process command
   `go test ./tests/functional/sessions/restart/... -run '^TestBoardPersistenceCLIRestartRoundTrip$' -count=1 -v`
   was attempted twice. Both attempts exited `1` after `10m0s` while waiting
   for the unchanged `go build ./cmd/factory` child; neither reached a daemon,
   CLI operation, or restart assertion.
3. A separate real `go build ./cmd/factory` diagnostic produced a
   `75,951,616`-byte artifact, but its command also failed to return under the
   shared host's concurrent unrelated Go workloads. No build substitute was
   used to claim the missing package proof.

## Cross-task integration and usability

- Documentation discoverability: the evidence ledger and this report are in
  `docs/internal/development/functional-test-optimization/` and identify the
  owning gates and remaining review edges.
- Permission and error behavior: not changed; the existing public malformed,
  missing, timeout, cancellation, restart, remap, resume, and history
  diagnostics remain asserted.
- Persistence/reload behavior: not exercised on this exact head; historical
  prior-head evidence remains explicitly separated in the ledger.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  customer prose behavior changed.
- Operational signals: the bounded probe reports timeout phase, kill/join
  outcome, and captured output; the package's existing daemon observations were
  not altered.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `FINDING-001` | BLOCKING | Run the exact focused selector twice on the rebased head under the current shared Windows host. | The package-owned CLI build returns and the real restart journey begins within the existing test bound. | Both runs reached the existing `10m0s` Go test timeout in `buildBoardPersistenceBinary` before scenario execution; the host had dozens of unrelated Go test/build processes. | C14 evidence ledger review-follow-up section. |
| `FINDING-002` | BLOCKING | Attempt the exact-head clean-room package proof. | The clean worktree reports the complete package and race/cleanup evidence. | Deferred because the required real CLI build completion is currently unavailable; no substitute was used. | Project criteria above. |

## Verdict

**BLOCKED** pending a successful exact post-rebase real-process package build and
the resulting package, race, clean-room, performance, and PR handoff gates.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: BEH-C14-RESTART, project criteria for the
  exact package/race/loopback proof and final delivery handoff.
- Root-cause evidence or remaining uncertainty: the unchanged nested
  `go build ./cmd/factory` command creates a valid artifact but does not return
  within the existing `10m0s` test bound while unrelated Go jobs saturate the
  shared Windows host; the direct bounded CASE-U08 correction itself passes.
- Smallest recommended correction/prerequisite: rerun the exact focused and
  complete package commands when the shared Go build queue permits command
  completion, then run supported race, clean-room loopback, final timing, push,
  and CI-start handoff. Do not change the test timeout or substitute the build.
- Dependencies and retest scope: GATE-SPINE, GATE-REPEAT, GATE-PACKAGE,
  GATE-RACE, GATE-PERF, GATE-LOOPBACK, and GATE-PR-CI on the pushed exact head.
