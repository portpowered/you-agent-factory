# Validation report: C14 sessions/restart optimization

## Environment and artifact

- Commit/build identifier: rebased implementation/test head
  `15f8c2fd7bd4ab1d9aed55930ec2619086dc41c8`, validated in the clean-room
  worktree below; this report-only refresh is included in the exact pushed PR
  head and does not alter the implementation tree.
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
| Restart behavior and assertion denominator | PASS | The final package's eight selectors passed: seven original behavior selectors plus the direct partial-setup probe. The seven-selector/148 lexical failure-call-site pre-change denominator and CASE-H01-H09, CASE-U01-U08, CASE-R01, and CASE-B01-B06 mapping remain intact; the probe adds only setup-cleanup assertions. | Unexercised future selectors. |
| Complete package proof | PASS | From the clean-room worktree at the rebased implementation head, `go test ./tests/functional/sessions/restart/... -count=1` exited `0` and reported `32.456s`; the final package samples were also all successful. | Terminal PR CI and merge. |
| Supported race proof | PASS | `go test -race ./tests/functional/sessions/restart/... -count=1` exited `0` on Windows/amd64 and reported `63.771s` on the rebased implementation head. | Unsupported platforms and all possible races. |
| Performance or measured floor | PASS | The exact final three-run samples passed at `60.00s`, `36.20s`, and `40.13s`; median `40.1260758s` versus the `44.1461053s` baseline is a `9.10%` reduction. The bounded profile identifies one authorized setup optimization and classifies the remaining real process, state, wait, and assertion boundaries as required, so the permitted measured-floor branch is used rather than claiming 40%. | Portable wall-clock behavior; review-host package timing. |
| Scope and C09 preservation | PASS | The rebased three-dot diff is limited to the three restart test files and C14 evidence files; fresh 30-second per-operation CLI contexts, no new sleeps, and no public/production/shared-support changes remain. | A future GitHub conflict request. |
| Clean-room reproducibility | PASS | This report was produced after the exact-head detached worktree passed the complete package without repair; the worktree was clean before removal. | Future host behavior. |
| Security, privacy, compatibility, and cost | PASS | The loopback is local and sanitized, public contracts/generated files are unchanged, existing failure/cleanup diagnostics pass, and paid/remote calls are zero. | Unrelated system-wide controls. |
| PR handoff | PASS | The final PR head is pushed/open with required CI started; the handoff comment records the baseline/final medians and the CI URL. | Review-owned terminal CI, conflict resolution, and merge. |

## Customer journey

1. A clean detached worktree at rebased implementation head `15f8c2fd7b`
   (the exact final PR head's implementation parent) ran
   `go test ./tests/functional/sessions/restart/... -count=1`; it exited `0`
   after all eight selectors exercised the shared binary, real daemon/server
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
| `FINDING-001` | INFO | Compare the final exact package samples with the unchanged-source baseline on the shared Windows host. | Local timing may vary under host load; topology remains the primary bounded optimization signal. | Final samples were `60.00s`, `36.20s`, and `40.13s`; all passed, with the required median `40.1260758s`. No unrelated process was terminated or modified. | `GATE-PERF` in the C14 evidence ledger. |

## Verdict

**PASS** for the local implementation-stage loopback. The final package, direct
partial-setup cleanup probe, and race evidence are complete; the only owned
redundant setup has been removed and no implementation defect was found. PR CI
and merge remain review-owned.

## Delta-plan request [Required for FAIL/BLOCKED]

None; no FAIL or BLOCKED finding was identified.
