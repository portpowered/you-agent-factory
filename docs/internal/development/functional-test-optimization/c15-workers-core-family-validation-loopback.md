# Validation report: functional-test-optimization-c15-workers-core-family

## Environment and artifact

- Commit/build identifier: executable implementation head
  `30f55f2fcc9a9ce794f7d9fab3f4d7240fa01434`. This report is a read-only
  Story 006 correction; it does not change executable behavior.
- Environment and configuration: Windows 11 build `10.0.26200.0`,
  `windows/amd64`, Go `go1.25.0`, PowerShell `7.6.5`, detached clean Git
  worktree, and isolated `HOME`, `USERPROFILE`, `APPDATA`, and
  `LOCALAPPDATA` directories. No lane `prd.json` or `progress.txt` was
  present in the clean checkout.
- Customer entry point: the seven Workers functional packages through the
  production `root.BuildProcess`/`Process.Execute` path, with retained local
  CLI/OS witnesses where those packages own them.
- Real and substituted dependencies: production wiring, local filesystem and
  retained CLI/OS boundaries; provider and other external effects remained
  controlled through the repository's injected edges. No remote provider or
  paid service was contacted.
- Cost/call budget used: `$0`; one bounded seven-package clean-room journey,
  one bounded focused diagnostic for the observed cleanup contention, the OS
  reconciliation, and the tagged compile gate.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Every `caseMatrix` observation remains exact with no weakened/deleted assertion | BLOCKED | The clean checkout passed `mock`, `agent`, `invoke_continue`, `codex`, `script`, and CLI-help; the exact M-19 nested witness is included in the passing mock package. The retained Claude forced-cleanup witness failed during startup contention, so the complete seven-package journey is not a pass. | A complete clean-room pass for the Claude retained cleanup row. |
| Three accidental OS sites disappear without replacement/reclassification; intentional witnesses remain | PASS | `make functional-os-boundary-check` exited `0` and reported `observed=67 baseline=70 packages=22 intentional=62 accidental=5 decreased=3`. | Future source/topology drift. |
| Final PR coverage is at least `61.6%`, all 82 owned cases pass, and no owned skip/quarantine/failure is present | BLOCKED | The current authoritative PR-host result was not included in this tracked report; its evidence and failure details are recorded in the PR conversation. Coverage/timing disposition was unavailable when the host run stopped on an untouched package failure. | A successful current-head Backend Functional Coverage artifact. |
| Each package improves from the supplied baseline and is under 3 seconds or has accepted irreducible itemization | BLOCKED | Local package times are diagnostic only. The current-head PR timing artifact was not available after the host coverage failure; the prior review result requires a new current-head measurement for the corrected agent fixture. | CI-host package timing and reviewer performance disposition. |
| Named package, OS, cleanup, coverage, PR-CI, and loopback gates report scope, fidelity, procedure, property, and unproven edges | PASS | This report records the exact clean-room procedure, dependency fidelity, observed package/OS/compile outcomes, findings, and named remaining edges. CI-run evidence is intentionally kept in the PR conversation. | Terminal CI and review interpretation. |
| Clean-room loopback reports `PASS` or emits `FAIL`/`BLOCKED` with a delta plan without repair | PASS (BLOCKED verdict emitted) | This report emits `BLOCKED`, records the Claude staging-owner contention and unavailable current PR artifact, and supplies a bounded delta plan. No code or fixture repair was made during validation. | Retest after the listed host/CI gates. |
| Zero remote/paid calls, secret leaks, production/public/generated/schema/excluded changes | PASS | The clean-room used only controlled/local-real dependencies; the tracked diff remains limited to functional tests and development evidence. | Remote-provider behavior remains intentionally unproven. |
| Implementation-stage delivery criterion | BLOCKED at capture | The report is the missing review artifact and must be committed and pushed before the final post-correction CI handoff can be claimed. | Post-report head push, CI start, and reviewer-owned terminal checks/merge. |

## Customer journey

1. A detached checkout at `30f55f2fcc9a9ce794f7d9fab3f4d7240fa01434` was
   created with isolated user-data directories. Each package was then run
   serially with the exact command shown below. Go's package duration and the
   outer PowerShell duration are both retained; host-contaminated wall time is
   diagnostic only.

   | Package | Exact command | Observed result |
   | --- | --- | --- |
   | `workers/mock` | `go test ./tests/functional/workers/mock -count=1` | PASS, exit `0`, Go package `80.634s`, wall `173.402s` |
   | `workers/agent` | `go test ./tests/functional/workers/agent -count=1` | PASS, exit `0`, Go package `11.471s`, wall `17.924s` |
   | `workers/invoke_continue` | `go test ./tests/functional/workers/invoke_continue -count=1` | PASS, exit `0`, Go package `17.771s`, wall `25.847s` |
   | `workers/inference/claude` | `go test ./tests/functional/workers/inference/claude -count=1` | BLOCKED, exit `1`, Go package `27.448s`, wall `33.556s`; the retained `TestClaudeForcedAssertionFailureCleansOwnedResources` child could not start its server while packaged-factory staging ownership was active |
   | `workers/inference/codex` | `go test ./tests/functional/workers/inference/codex -count=1` | PASS, exit `0`, Go package `4.418s`, wall `9.537s` |
   | `workers/script` | `go test ./tests/functional/workers/script -count=1` | PASS, exit `0`, Go package `2.331s`, wall `8.246s` |
   | `workers/transports/cli/run/help` | `go test ./tests/functional/workers/transports/cli/run/help -count=1` | PASS, exit `0`, Go package `6.308s`, wall `11.116s` |

   The Claude output reported an active packaged-factory staging owner and a
   missing process API-server start in the forced-cleanup child; it did not
   report an assertion mismatch in the optimized Claude fixture. A bounded
   follow-up on the current worktree used:

   ```text
   go test ./tests/functional/workers/inference/claude -run '^TestClaudeForcedAssertionFailureCleansOwnedResources$' -count=1 -v
   ```

   The test body emitted `PASS` with Go package time `4.418s`, but the outer
   wrapper did not return after the pass output and was interrupted after a
   bounded 30-second shutdown wait. This is retained as diagnostic evidence,
   not promoted to a clean-room package pass.

2. The OS boundary reconciliation used:

   ```text
   make functional-os-boundary-check
   ```

   It exited `0` with:

   ```text
   static OS-spawn baseline holds: observed=67 baseline=70 packages=22 intentional=62 accidental=5 decreased=3
   reconciled 67 inventory OS-spawn records
   ```

3. The applicable tagged compatibility gate used:

   ```text
   make test-functional-long-compile
   ```

   It exited `0` after running the tagged functional vet/compile path.

4. The detached checkout reported no Git residue and `git diff --check`
   exited `0`; the temporary Git worktree was removed after the journey.

## Cross-task integration and usability

- Documentation discoverability: this report is under the canonical
  `docs/internal/development/functional-test-optimization/` evidence
  directory and links the current executable head, exact commands, findings,
  and remaining gates.
- Permission and error behavior: the passing Workers package runs retain the
  characterized Work, Event, dispatch, provider-call, output, cancellation,
  concurrency, help, and cleanup assertions. M-19's current nested witness
  retains both zero command-factory construction and zero provider-runner
  calls; the clean-room failure was a retained Claude cleanup startup edge.
- Persistence/reload behavior: no persisted schema, public contract, or
  production runtime behavior changed. Factory Session, Worker Session,
  Work, response/event, worktree, and cleanup observations remain owned by
  the package tests and their controlled/local-real boundaries.
- Accessibility/keyboard/responsive behavior: not applicable; this lane
  changes backend functional tests and development evidence only.
- Operational signals: package exit status, staging-owner diagnostics, OS
  inventory reconciliation, tagged compile status, residue status, and
  `git diff --check` are retained. The source-plan path named by the PRD is
  absent in this checkout; the customer payload remains the documented
  non-weakening assumption.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `C15-LOOPBACK-001` | blocking / environment-dependent | `go test ./tests/functional/workers/inference/claude -count=1` in the isolated detached checkout at `30f55f2fcc` | All seven owned packages complete with zero owned failures and cleanup witnesses remain observable | Six packages passed; the retained Claude forced-cleanup child failed before its server-start assertion because packaged-factory staging ownership was active. The focused current-worktree test body passed, so the failure did not reproduce as a deterministic fixture assertion regression. | Clean-room package output and the bounded focused diagnostic above |
| `C15-PR-001` | blocking for handoff, not executor-owned | Current PR-host Functional Coverage result; exact run/test evidence is kept in the PR conversation rather than this commit | Current head produces coverage, owned-case, and package-timing disposition | The host coverage lane stopped on an unchanged `tests/functional/providers/acp` permission-selection test; coverage floors and current package timing were not evaluated. The required one-time rerun and a fresh post-report-head CI run remain handoff actions. | PR conversation CI comment and linked run artifact |
| `C15-REPORT-001` | resolved | Review requested the template-shaped loopback artifact | A report contains Environment, criteria, journey, integration/usability, findings, verdict, and delta-plan sections | This document supplies all required sections and explicitly emits `BLOCKED` without repairing the observed edges. | This report |

## Verdict

BLOCKED

The corrected implementation has a passing local functional spine for six
owned packages, a passing OS reconciliation with three fewer accidental sites,
and a passing tagged compile gate. The loopback cannot emit `PASS` because the
clean-room Claude cleanup witness encountered staging-owner contention and the
current PR-host coverage/timing artifact was not available as a successful
measurement. No implementation repair was made in response to either
environmental edge.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: Story 006's complete clean-checkout matrix
  criterion, plus the authoritative `GATE-COVERAGE`/`GATE-PR-CI` performance
  and 61.6% coverage criteria.
- Root-cause evidence or remaining uncertainty: the Claude clean-room failure
  is tied to active packaged-factory staging ownership and is not reproduced
  by the bounded focused test body; the PR-host coverage lane stopped in an
  untouched provider package before measuring coverage floors or current
  package timing.
- Smallest recommended correction/prerequisite: keep the optimized fixtures
  unchanged, perform the permitted one-time failed-job rerun and a fresh
  post-report-head PR CI run, and repeat the seven-package clean-room command
  on an uncontended host if the Claude cleanup edge remains required. If the
  Claude failure repeats without staging contention, open a narrow
  fixture-specific correction; do not broaden into production/provider code.
- Dependencies and retest scope: the rerun must report current `workers/agent`
  directional improvement and the full coverage outcome; the clean-room
  retest covers all seven package commands, the OS boundary check, tagged
  compile, residue check, and diff check. Terminal CI, conflict resolution,
  and merge remain review-owned.
