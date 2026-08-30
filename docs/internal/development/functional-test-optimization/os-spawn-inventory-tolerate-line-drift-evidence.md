# OS-spawn inventory line-drift evidence

## Scope and status

- Story: `os-spawn-inventory-tolerate-line-drift-001` — identity-authoritative
  reconciliation.
- Parent behavior: `BEH-001` — line-only motion is green and reported while
  identity, metadata, count, and admission failures remain closed.
- Status: **PASS for story 001 local evidence**. Story 002 owns the rebased
  real-repository, Make/lint, clean-room, and PR handoff gates.
- Source plan: `null` in `prd.json`; no operator amendment is present.
- Branch: `os-spawn-inventory-tolerate-line-drift`.
- Dependency fidelity: controlled temporary repositories through the
  production scanner, JSON loaders, reconciliation, baseline evaluator, and
  output boundary.
- Cost and network: 0 remote calls, `$0` paid cost, no subprocesses started by
  the checker.
- Scope audit: only `cmd/functionalosboundarycheck` and this ledger are
  implementation-owned; no inventory, baseline, `tests/functional`, Make,
  OpenAPI, generated, UI, or verdict data changed.

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

It does not prove the actual 69-site authored census, Make/lint-driver wiring,
clean final-commit reproduction, hosted CI, or merge.

## Remaining gates

- Actual authored 69-site inventory/baseline reconciliation and exact
  `69/69`, `23`, `61/8`, `decreased=0` counts -> `INT-REAL`.
- Existing Make and lint-driver invocation -> `INT-LINT`.
- Clean final-commit, read-only validation-loopback report ->
  `VAL-CLEAN-ROOM`.
- Hosted Backend Lint on the final PR head -> `CI-BACKEND-LINT`, review-owned
  after implementation handoff.
- Merge -> `REVIEW-MERGE`, review-owned.

No hosted-CI result is claimed or recorded in this ledger.
