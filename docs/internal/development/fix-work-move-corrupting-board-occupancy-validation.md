# Validation report: work move restore board occupancy

## Environment and artifact

- Commit/build identifier: implementation SHA `7be62418b5330a8e7b0a41ef1657b9592d0d8750`; the validation report is documentation-only after that tested implementation head. The isolated `go build -o <scratch>/you-verify.exe ./cmd/factory` completed with exit `0` and produced a 76,047,872-byte binary.
- Environment and configuration: Windows `Microsoft Windows NT 10.0.26200.0`, `windows/amd64`, Go `go1.25.0`; compact replay fixture and all transient homes/logs were local scratch inputs.
- Customer entry point: built `you-verify.exe run --resume`, followed by public `you-verify.exe work list`.
- Real and substituted dependencies: compact replay and controlled command-runner edges were available; no remote provider or paid call was used. The required supplied 20,502-event/247-Work recording was not available.
- Cost/call budget used: zero paid/remote calls; zero supplied-recording runs of the one allowed six-minute run.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Compact functional, repeat/race, and sibling behavior | BLOCKED | Current-head focused Runtime/Recordings selectors, `tests/functional/sessions/board_occupancy_resume` `-count=1`, functional `-race`, and the supported automation selector were recorded as passing; no OS process was spawned. | The exact final clean-room supplied-recording journey and complete F-01–F-16 real-artifact applicability remain unproven. |
| Supplied binary resume and exact public Work list | BLOCKED | The isolated binary build passed, but no supplied recording path or SHA-256 exists in the task inputs or checked local locations. | Resume success, exact named Work occurrence at `idea:complete`, unaffected samples, and total count. |
| Sibling missing-occupancy behavior | PASS for focused local scope | The PR #2320 automation recovery selector and unsupported active-without-occupancy owner coverage were recorded as passing; no sibling package was changed. | Behavior of other production recordings. |
| Repository quality gates | BLOCKED | `git diff --check`, focused suites, `make backend-size`, `make pkg-maint`, `make mcp-contract-check`, and the Go unit lane passed. `make verify-fast` stopped at missing Bun type definitions. `make lint` completed backend/static checks but stopped on missing UI `@biomejs/biome`/`knip` dependencies and deadcode baseline drift (`3074` observed versus `3073` recorded). | A dependency-complete, baseline-current fast/lint result. No baseline was edited. |
| VAL-001 exact real journey | BLOCKED | The required real artifact is a mandatory local-real dependency; the one supported availability check on 2026-08-30 at 19:12 PDT found no usable candidate. | Exact event count/order, Work values, successor recording, log isolation, and clean-room recovery. |
| Final implementation handoff | BLOCKED | No PR exists for this branch, so no final push/CI-start evidence is claimed. | Rebase onto `origin/main`, final push, open PR, CI start, and any blocking feedback. |

## Customer journey

1. The implementation SHA tested was `7be62418b5330a8e7b0a41ef1657b9592d0d8750`; the later report-only commit does not change production or test code. The compact root-built resume/list witness and focused owner gates had already passed at the implementation head, with deterministic synchronization, controlled external-effect edges, and zero OS spawns.
2. The required built-binary command completed successfully: `go build -o <scratch>/you-verify.exe ./cmd/factory` (exit `0`).
3. The supplied-recording command was not started because the required 20,502-event/247-Work artifact and its path were absent. The exact pre-fix failure, compact fixture hash, and search boundary are recorded in `docs/internal/development/fix-work-move-corrupting-board-occupancy-evidence.md`; the supplied event payloads and SHA-256 remain unavailable.

## Cross-task integration and usability

- Documentation discoverability: the characterization ledger and this validation report are under `docs/internal/development`; no raw recording, prompt, model output, or secret was copied.
- Permission and error behavior: focused tests retain the exact duplicate-current-place, invalid-reference/topology, resource-place, and unsupported no-occupancy errors; the real artifact could not be used to validate them end to end.
- Persistence/reload behavior: compact recording recovery and cancellation/lifecycle unwind were recorded as passing; supplied persistence/reload remains unproven.
- Accessibility/keyboard/responsive behavior: not applicable to this backend/CLI-only task.
- Operational signals: safe reconciliation warning behavior is covered by focused Runtime evidence; no supplied-run log path could be resolved because the run was not started.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| VAL-001-ARTIFACT | blocking | Locate the PRD-named 20,502-event/247-Work recording and run the bounded built-binary resume/list procedure. | A readable immutable recording with a verifiable SHA-256 and the named event sequence. | No usable artifact/path was present in `prd.json`, this checkout, sibling worktrees, Downloads, Desktop, or TEMP; the single supported availability check found no candidate. | Characterization ledger, dated 2026-08-30 19:12 PDT. |
| VAL-001-FAST | blocking/environment | Run `make verify-fast` and `make lint` from this checkout. | Dependency-complete fast and lint gates report their named properties. | Fast verification stopped at missing Bun type definitions; lint stopped at missing UI binaries/dependencies and recorded deadcode drift (`3074` versus `3073`). | Story progress log; no shared baseline was changed. |

## Verdict

BLOCKED

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: BEH-1 supplied-event characterization, BEH-3 exact built-artifact recovery, and acceptance criteria requiring the 20,502-event/247-Work Work-list proof and VAL-001.
- Root-cause evidence or remaining uncertainty: the mandatory supplied recording is not in the task inputs or permitted local search locations, so its SHA-256, exact sequence/state/place values, and final public list cannot be independently observed. Fast/lint dependency failures are environmental and were not silently repaired.
- Smallest recommended correction/prerequisite: provide the immutable supplied recording or an operator-authorized path/access method, then run the one declared local-real six-minute procedure at this exact SHA; separately provide the checkout's Bun/UI dependencies or the repository-supported baseline disposition before rerunning the blocked fast/lint gates.
- Dependencies and retest scope: rerun GATE-CHAR artifact inspection, the built-binary `run --resume`/`work list` proof, VAL-001, and only the minimal affected gates after the artifact and environment are available; then rebase, push, open the PR, and confirm CI starts. Do not edit the production implementation or shared baselines as part of validation.
