# Validation report: C14 sessions/restart optimization

## Environment and artifact

- Commit/build identifier: `d7d296dbba`, the exact rebased implementation head
  checked out in the detached clean-room worktree.
- Environment and configuration: Windows/amd64, Go `go1.25.0`,
  `CGO_ENABLED=1`, package command with `-count=1`; no paid or remote
  provider calls.
- Customer entry point: the restart functional package's real `you` CLI and
  production-composed daemon/server boundaries.
- Real and substituted dependencies: real local CLI build, OS processes,
  loopback HTTP, filesystem, recording, and lifecycle boundaries; only the
  existing controlled provider/test edges remain substituted.
- Cost/call budget used: `$0`; zero paid or remote-provider calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| One guarded package CLI build | PASS | `GATE-SPINE` and `GATE-REPEAT` logs show exactly one normal-process package build, zero `SCRIPT_WORKER` helper builds, and package teardown after child joins. | Hosted-runner topology. |
| Restart behavior and assertion denominator | PASS | The final package's seven selectors passed; the seven-selector/148 lexical failure-call-site denominator and CASE-H01-H09, CASE-U01-U08, CASE-R01, and CASE-B01-B06 mapping remain unchanged. | Unexercised future selectors. |
| Complete package proof | PASS | From the clean-room worktree at `d7d296dbba`, `go test ./tests/functional/sessions/restart/... -count=1` exited `0` and reported `41.726s`. | Terminal PR CI and merge. |
| Supported race proof | PASS | `go test -race ./tests/functional/sessions/restart/... -count=1` exited `0` on Windows/amd64. | Unsupported platforms and all possible races. |
| Performance or measured floor | PASS | The exact final three-run samples passed but were host-contaminated; the median was `62.5393s` versus the `44.1461053s` baseline. The bounded profile identifies the only removable setup as two of three identical CLI builds and classifies the remaining real process, state, wait, and assertion boundaries as required. | Portable wall-clock behavior; review-host package timing. |
| Scope and C09 preservation | PASS | The rebased three-dot diff is limited to the restart test files and C14 evidence files; fresh 30-second per-operation CLI contexts, no new sleeps, and no public/production/shared-support changes remain. | A future GitHub conflict request. |
| Clean-room reproducibility | PASS | This report was produced after the exact-head detached worktree passed the complete package without repair; the worktree was clean before removal. | Future host behavior. |
| Security, privacy, compatibility, and cost | PASS | The loopback is local and sanitized, public contracts/generated files are unchanged, existing failure/cleanup diagnostics pass, and paid/remote calls are zero. | Unrelated system-wide controls. |
| PR handoff | PENDING REVIEW | The implementation handoff still requires the final push, open PR, started CI, and a comment containing both medians and the CI run URL. | Review-owned terminal CI, conflict resolution, and merge. |

## Customer journey

1. A clean detached worktree at `d7d296dbba` ran
   `go test ./tests/functional/sessions/restart/... -count=1`; it exited `0`
   after all seven selectors exercised the shared binary, real daemon/server
   boundaries, restart/replay/persistence/failure paths, and teardown.
2. The package logs retained one immutable CLI artifact per test process while
   each board scenario kept its own Factory, HOME, recording, release path,
   port, daemon lifecycle, and mutable expectations.
3. The supported race command exited `0`; no data-race report, orphan process,
   helper recursion, or cleanup failure was observed.

## Cross-task integration and usability

- Documentation discoverability: the evidence ledger and this report are in
  `docs/internal/development/functional-test-optimization/` and identify the
  owning gates and remaining review edges.
- Permission and error behavior: not changed; the existing public malformed,
  missing, timeout, cancellation, restart, remap, resume, and history
  diagnostics remain asserted.
- Persistence/reload behavior: the real board recording, durable snapshot,
  replay, logical identity, dispatch, and event-prefix witnesses remain in
  the package proof.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  customer prose behavior changed.
- Operational signals: process output, exit status, phase/last-observation
  diagnostics, helper release boundaries, build count, and package cleanup
  remain observable.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `FINDING-001` | INFO | Compare the final exact package samples with the unchanged-source baseline while PID `50008` is active. | Local timing may vary under shared-host saturation; topology remains the primary bounded optimization signal. | Final samples were `39.6764s`, `62.5393s`, and `88.8193s`; the unrelated process consumed approximately `2.125` CPU-seconds during one wall second. It was preserved and not modified. | `GATE-PERF` in the C14 evidence ledger. |

## Verdict

**PASS** for the local implementation-stage loopback. The final package and
race evidence are complete, the only owned redundant setup has been removed,
and no implementation defect was found. PR CI and merge remain review-owned.

## Delta-plan request [Required for FAIL/BLOCKED]

None; no FAIL or BLOCKED finding was identified.
