# OS-spawn inventory line-drift evidence

## Scope and status

- Story: `os-spawn-inventory-tolerate-line-drift-001` — identity-authoritative
  reconciliation.
- Parent behavior: `BEH-001` — line-only motion is green and reported while
  identity, metadata, count, and admission failures remain closed.
- Status: **PASS for story 001 local evidence**. Story 002 owns the rebased
  real-repository, Make/lint, clean-room, and PR handoff gates.
- Source plan: `null` in `prd.json`; operator amendment 1 authorizes the
  narrowly scoped main-red inventory/baseline repair recorded below.
- Branch: `os-spawn-inventory-tolerate-line-drift`.
- Dependency fidelity: controlled temporary repositories through the
  production scanner, JSON loaders, reconciliation, baseline evaluator, and
  output boundary.
- Cost and network: 0 remote calls, `$0` paid cost, no subprocesses started by
  the checker.
- Scope audit: the operator amendment additionally authorizes the canonical
  inventory/baseline maintenance required by the merged sessions-restart
  spawn. No `tests/functional`, Make, OpenAPI, generated, UI, or existing
  verdict data changed.

## Decision record

`siteId` is the authoritative identity. A matching record whose only differing
field is the required positive `sourceLine` now produces a deterministic
informational stdout notice and does not affect the exit status. The notice is
emitted only after inventory reconciliation and baseline admission succeed.

Read-only tolerance was selected over self-healing because the checker must not
mutate the shared inventory during lint. Self-healing would create worktree
churn and branch contention, hide the authored source/inventory decision, and
make a green lint result depend on writes. The line remains in the schema for
reviewer navigation; `sourcePath`, `packagePath`, `enclosingIdentity`, and
`occurrence` remain fatal metadata comparisons.

The checker still performs one bounded filesystem walk and AST scan. Drift
collection adds only bounded in-memory records and no new process, network,
write, sleep, timing, or mutable package state.

## AMENDMENT-MAIN-RED — merged-source reconciliation

The operator amendment identified a real merge-boundary failure on `origin/main`
after sessions-restart merge #2455. Running the unmodified checker from the
clean `origin/main` worktree at `8b2c6ef8ea` produced exactly these three
violations: the new `TestBoardPersistenceSharedBinaryPartialSetupFailure` call
at line 169 had no inventory verdict; the existing
`buildBoardPersistenceBinary` identity had recorded line 47 but observed line
121; and the existing `startBoardPersistenceDaemonProcess` identity had
recorded line 74 but observed line 252. The checker reported
`LINT_VIOLATION_COUNT: 3` and exited nonzero. The first repair commit,
`38c1d06879`, added the new `INTENTIONAL-OS` `exit-status` row, retained every
existing verdict, refreshed the two moved source-line references, and raised
the sessions/restart baseline from 3 to 4. The real `make
functional-os-boundary-check` then exited 0 with
`observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0`
and `reconciled 70 inventory OS-spawn records`.

Systemic recommendation: this gate pins a per-site inventory that any merge
touching `tests/functional` can invalidate after that merge's own CI has
passed; the repository should add either a post-merge `main` self-heal or a
merge-queue re-check for this checker. This lane recommends that follow-up but
does not build it.

## CHAR-CURRENT — pre-change characterization

Before changing the reconciliation assertion, the required command was run:

```text
go test ./cmd/functionalosboundarycheck -run '^TestC16SiteMetadataMustMatchAST$' -count=1 -v
```

Observed result: `PASS`, exit code `0`. The test passed because the pre-change
production reconciliation path made a source-line-only mismatch fatal. The
customer-supplied PR #2427 diagnostic retained as historical authority was:

```text
[agent-factory:functional-os-boundary] inventory site OSSPAWN-tests-functional-sessions-chat-sessions-root-composition-acp-worker-child-events-test-acpWorkerChildCommandFactory-01 sourceLine=460 does not match AST sourceLine=481
LINT_VIOLATION_COUNT: 1
[agent-factory:functional-os-boundary] inventory reconciliation failed with 1 violation(s)
process exit status: nonzero
```

This characterization proves only the old fatal behavior and does not prove
the corrected behavior or real repository integration.

## UNIT-LINE-DRIFT and UNIT-REPEAT

The focused witnesses use temporary repository files and call the production
`run` boundary. The line-drift fixture inserts blank lines above an inventoried
call, retains the old positive line and all other metadata, and observes the
same `siteId`. It asserts the exact output order:

```text
[agent-factory:functional-os-boundary] tolerated sourceLine drift for inventory site <siteId>: recorded=<old-line> observed=<new-line>; siteId matched
[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=1 baseline=1 packages=1 intentional=1 accidental=0 decreased=0
[agent-factory:functional-os-boundary] reconciled 1 inventory OS-spawn records
```

The test also asserts empty stderr and byte-identical inventory and baseline
files. A separate two-site witness permutes inventory order and asserts exact
stable-ID-sorted drift notices followed by the unchanged success summaries.

The required repeat procedure was:

```text
go test ./cmd/functionalosboundarycheck -run 'TestInventory(ReconciliationToleratesSourceLineDriftByIdentity|RejectsUninventoriedSpawnIncrease|AllowsPairedRemovalAsDecrease|RejectsRenamedOrMovedIdentity)$' -count=20
```

Observed result: `PASS`, exit code `0`. All four named customer witnesses ran
for 20 repetitions. Their exact stdout/stderr and exit assertions make drift
ordering byte-stable and exercise fresh temporary state on every repetition.

## Behavior matrix

| Case | Witness and observed property | Result |
| --- | --- | --- |
| U01 line-only motion | `TestInventoryReconciliationToleratesSourceLineDriftByIdentity`: unchanged ID, exact recorded/observed notice, success summaries, no file mutation | PASS |
| U02 new unjustified spawn | `TestInventoryRejectsUninventoriedSpawnIncrease`: exact AST identity/path/line admission guidance, `LINT_VIOLATION_COUNT: 1`, empty stdout, no writes | PASS |
| U03 paired removal | `TestInventoryAllowsPairedRemovalAsDecrease`: matching source/inventory removal, `observed=1 baseline=2 decreased=1`, unchanged baseline | PASS |
| U04 rename | `TestInventoryRejectsRenamedOrMovedIdentity/enclosing function rename`: old identity absent and new identity un-inventoried; no drift notice | PASS |
| U05 move | `TestInventoryRejectsRenamedOrMovedIdentity/source move`: old identity absent and new path identity un-inventoried; no drift notice | PASS |
| U06 multiple drifts | `TestInventoryReconciliationSortsSourceLineDriftBySiteID`: permuted inventory still gives exact stable ordering | PASS |
| U07 no drift | Existing equal-baseline success coverage in the full package preserves the no-drift output | PASS |
| U08 non-line metadata | `TestInventoryReconciliationRejectsNonLineMetadataDrift`: source-path mismatch remains fatal and exact | PASS |
| U09 stale removal | Existing extra-inventory coverage retains the absent-identity failure | PASS |
| U10 occurrence/count admission | Existing C03/C05/C07 coverage retains unadmitted growth and intentional-admission policy | PASS |
| U11 bad input/recovery | Existing malformed input, parser, recovery, and read-only coverage remains green | PASS |
| U12 dependency/state boundary | The checker remains synchronous, repository-local, standard-library-only, and free of new shared mutable state; repeat evidence is green | PASS |

## UNIT-REGRESSION

The complete package procedure was:

```text
go test ./cmd/functionalosboundarycheck -count=1
```

Observed result: `PASS`, exit code `0`; package-reported result was
`github.com/portpowered/infinite-you/cmd/functionalosboundarycheck`.
`git diff --check` also passed. This proves the changed checker matrix plus
existing schema, scanner, admission, metadata, ordering, recovery, and
read-only regressions at controlled package fidelity.

It does not prove the amended actual 70-site authored census, Make/lint-driver
wiring, clean final-commit reproduction, hosted CI, or merge.

## Remaining gates

- Actual amended 70-site inventory/baseline reconciliation and exact
  `70/70`, `23`, `62/8`, `decreased=0` counts -> `INT-REAL`.
- Existing Make and lint-driver invocation -> `INT-LINT`.
- Clean final-commit, read-only validation-loopback report ->
  `VAL-CLEAN-ROOM`.
- Hosted Backend Lint on the final PR head -> `CI-BACKEND-LINT`, review-owned
  after implementation handoff.
- Merge -> `REVIEW-MERGE`, review-owned.

No current PR hosted-CI result is claimed or recorded in this ledger; the
operator-supplied pre-fix `main` failure is recorded above as the scope
amendment's finding.

## Validation report: BEH-002 — repository-integrated delivery

## Environment and artifact

- Commit/build identifier: `a949e3cc4f1c2c6aad4955abf0eaf43b0dfd7f07` (the
  rebased implementation head validated in the clean-room pass below; this
  report adds no executable, test, inventory, or baseline behavior).
- Base identity: rebased cleanly onto current `origin/main`
  `8b7f496b60`; the operator-authorized sessions-restart reconciliation is
  retained.
- Environment and configuration: Windows `10.0.26100`, `go1.25.0
  windows/amd64`, PowerShell, repository `go.mod`, default Go build cache, no
  remote credentials, and no paid provider calls.
- Customer entry point: the repository's production `cmd/functionalosboundarycheck`
  checker through `go run`, `make functional-os-boundary-check`, and the
  focused `make lint` driver.
- Real and substituted dependencies: real local filesystem, Go parser/AST
  scan, JSON loaders, reconciliation, baseline evaluator, Make/lint driver,
  and a detached clean worktree; no product runtime or external service is
  applicable.
- Cost/call budget used: zero remote or paid calls; maximum cost `$0`.

## Exact-head run record

The clean-room worktree was detached at the exact implementation head above.
`git status --short --untracked-files=no` emitted no tracked changes. The
following commands ran in that worktree and all exited `0`:

| Procedure | Observed result |
| --- | --- |
| `go test ./cmd/functionalosboundarycheck -count=1` | `ok github.com/portpowered/infinite-you/cmd/functionalosboundarycheck 0.315s` |
| `go run ./cmd/functionalosboundarycheck -root . -baseline docs/internal/baselines/functional-os-spawn-baseline.json -inventory docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` | `observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0`; `reconciled 70 inventory OS-spawn records` |
| `make functional-os-boundary-check` | checker output above; `LINT PASSED` is not emitted by this target, exit `0` |
| `make lint LINT_TARGETS=functional-os-boundary-check` | `functional-os-boundary-check: PASS`; `LINT PASSED: 1 target(s) completed successfully` |

The focused lint run emitted one transient Windows file-lock warning before
the lint driver started, but the driver completed the checker successfully
with exit `0`; it did not alter the result or the canonical inputs. The exact
clean-room checker summaries were:

```text
[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0
[agent-factory:functional-os-boundary] reconciled 70 inventory OS-spawn records
```

The canonical input hashes were identical before and after all clean-room
commands:

```text
baseline sha256: 30656D69EE00BF37EDF149033D08FC41D9B0D16310D2E149CDEB777850FA0797
inventory sha256: DDB618D20CCF4B956F652D6EAE57CD9B609D107CE5435158784043A0F46478BB
```

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Line-only source motion is informational and read-only | PASS | The package witness `TestInventoryReconciliationToleratesSourceLineDriftByIdentity` passed with exact sorted stdout, empty stderr, and byte-identical fixture files; the clean current tree emitted no drift. | Future scanner behavior on unsupported platforms. |
| New unjustified spawn retains paired baseline/inventory admission failure | PASS | `TestInventoryRejectsUninventoriedSpawnIncrease` passed with the existing actionable admission diagnostic, nonzero result, empty stdout, and unchanged inputs. | A future new site in the live repository. |
| Paired removal remains a permitted decrease | PASS | `TestInventoryAllowsPairedRemovalAsDecrease` passed with `observed=1 baseline=2 decreased=1` and unchanged baseline bytes. | Repository-wide removal scenarios. |
| Renamed or moved identity fails closed | PASS | `TestInventoryRejectsRenamedOrMovedIdentity` passed for both enclosing-function rename and source move; absent/new identity findings remained fatal and no drift notice was emitted. | Platform-specific path behavior. |
| Complete checker package regression suite | PASS | `go test ./cmd/functionalosboundarycheck -count=1` exited `0` at the rebased head. | Hosted CI and other package families. |
| Twenty-run deterministic witness repeat | PASS | The required four-witness `-count=20` command exited `0`; temporary fixtures proved stable ordering and no state leakage. | Host-wide concurrency outside the package. |
| Actual 69-site ratchet criterion amended by the operator | PASS under `operatorAmendment1` | Direct checker, Make target, focused lint driver, and clean checkout all proved `observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0` and `reconciled 70 inventory OS-spawn records`. The amendment authorizes the truthful sessions-restart row and count increase. | Terminal hosted CI. |
| Scope and read-only boundary | PASS under `operatorAmendment1` | `git diff --name-status origin/main...HEAD` contains only the checker, its package tests, the authorized baseline/inventory maintenance, and this ledger; no tests/functional, Make, API, generated, UI, or unrelated user files changed. Clean-room hashes stayed unchanged. | Future merges can invalidate the per-site inventory; the systemic re-check recommendation remains follow-up. |
| Committed clean-room loopback report | PASS | This report records the exact detached head, commands, exits, 70-site outputs, dependency fidelity, cost, input hashes, findings, verdict, and remaining gates using the validation-loopback template. | The report commit itself is documentation-only and is not self-referenced. |
| Hosted Backend Lint terminal result | BLOCKED — review-stage gate | The earlier run on superseded head `33332393126` was cancelled/failed; no terminal result is claimed here. The final push must start a fresh required run. | Review-owned terminal Backend Lint and Verification Policy on the pushed head. |
| Implementation-stage delivery | BLOCKED until handoff | Local implementation and clean-room evidence are complete; final push, open-PR head update, and required CI start remain to be performed after this report commit. | Review-owned terminal CI, conflict resolution, and merge. |

## Customer journey

1. A clean detached checkout ran the production checker against the authored
   AST, baseline, and inventory. It exited `0` and reported the amended
   `70/70`, `23`, `62/8`, `decreased=0` census with 70 reconciled records.
2. The same checkout ran the existing `make functional-os-boundary-check`
   entry and the focused `make lint LINT_TARGETS=functional-os-boundary-check`
   driver. Both invoked the checker through the real repository wiring and
   exited `0`.
3. Package and repeated fixture witnesses retained line-drift tolerance,
   identity failures, admission policy, ordering, malformed-input handling,
   and read-only behavior. No product runtime, browser, or paid dependency is
   part of this checker journey.

## Cross-task integration and usability

- Documentation discoverability: this ledger now contains the story-001
  decision/evidence and the required BEH-002 validation report under the
  canonical functional-test optimization directory.
- Permission and error behavior: not applicable to this repository-local
  static checker; malformed data and reconciliation failures remain explicit
  nonzero diagnostics.
- Persistence/reload behavior: not applicable; the checker reads authored
  files and performs no writes.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  customer-facing copy changed.
- Operational signals: stdout/stderr separation, exact exit statuses,
  deterministic counts, stable drift ordering, and before/after input hashes
  provide the relevant diagnostics.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| ENV-001 | advisory, resolved | Clean-room `make lint LINT_TARGETS=functional-os-boundary-check` on Windows | Focused lint driver completes and reports the checker result | One transient file-lock warning preceded a successful `functional-os-boundary-check: PASS` and exit `0`; canonical hashes were unchanged. | Exact-head run record above. |
| REVIEW-001 | review-stage | Final head is not pushed at report capture | Required CI starts on the final pushed head | Local evidence is complete; review still owns fresh CI terminal status and merge. | PR #2497 handoff. |

## Verdict

**PASS** for the local implementation and clean-room validation. The amended
70-site checker journey is integrated through the real Make/lint path and is
read-only. Final push, required CI start, terminal hosted CI, and merge remain
review-stage responsibilities and are not claimed by this report.

## Delta-plan request [Required for BLOCKED]

- Affected behavior and criterion: implementation-stage delivery and
  `CI-BACKEND-LINT` terminal result.
- Root-cause evidence or remaining uncertainty: the report was captured before
  the documentation commit was pushed, and the prior required run was tied to
  superseded head `e17beb5f5b`; no current-head terminal result exists yet.
- Smallest recommended correction/prerequisite: commit this authored report,
  push the rebased final head, confirm the PR remains open, and confirm the
  required Backend Lint and Verification Policy checks have started. Do not
  add CI results or audit transcripts as commits.
- Dependencies and retest scope: review owns terminal CI, one allowed rerun of
  a failed job if needed, conflict resolution, and merge; implementation
  retesting is limited to failures caused by this diff.
