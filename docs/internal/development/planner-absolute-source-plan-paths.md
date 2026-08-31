# Planner absolute source-plan path evidence ledger

Status: implementation evidence for story `planner-must-handle-absolute-source-plan-paths-001`.
The actual provider boundary remains unproven after its bounded timeout; this
ledger does not turn that dependency failure into a passing path assertion.

## Authority and scope

The operator's first-hand report that the planner fails to handle full absolute
`sourcePlan` paths is authoritative. The implementation scope for this story
is the plan workstation contract, its deterministic behavior matrix, the
bounded actual-planner probe, and this ledger. Project Lead and project-guide
authoring remain story 002. Setup-workspace packet discovery, worktree copying,
and live PRDs are outside this story and were not changed.

## Pre-change characterization — GATE-SP-CHAR

Required command, run before the implementation edit:

```text
python tests/factory/planner/source_plan_paths_functional.py --phase pre-change --case windows-backslash
```

Observed exit status: `2`.

```text
C:\Python312\python.exe: can't open file 'C:\Users\andre\work\portos\infinite-you\.claude\worktrees\planner-must-handle-absolute-source-plan-paths\tests\factory\planner\source_plan_paths_functional.py': [Errno 2] No such file or directory
```

The pre-change harness was absent, so the actual planner was not invoked and
the reported corruption/rejection mechanism was not reproduced. The accurate
finding is **mechanism not reproduced**, not that the operator-reported defect
is absent. The operator override authorizes shipping the defensive contract
with local proof while preserving this limitation.

## Corrected actual-planner probe — GATE-SP-FOCUSED dependency edge

Required command:

```text
python tests/factory/planner/source_plan_paths_functional.py --phase post-change --case windows-backslash --case windows-forward --case relative
```

The probe created unique source bytes and a temporary one-workstation factory,
then invoked the real `you run` planner with a 300-second bound. It reached the
provider/runtime boundary but exited `1` after the first case timed out:

```text
source-plan functional probe failed: planner provider timed out after 300 seconds
```

No generated packet was accepted and the temporary fixture/output paths were
cleaned. This proves bounded dependency failure handling only; it does not
prove actual model instruction following or actual-planner path persistence.

## Deterministic local matrix — GATE-SP-LOCAL

Command:

```text
python -m unittest discover -s tests/factory/planner -p "test_source_plan_paths.py"
```

Observed result: exit `0`, `14` tests passed in `65.579s` on the Windows host.
The matrix covers CASE-SP-001 through CASE-SP-015: both exact Windows slash
spellings, relative-to-absolute resolution, empty/missing/directory/denied/
escaped/provider/partial/concurrent/cancelled/repeated/no-plan boundaries,
artifact agreement, and exact bytes read after changing cwd to a consumer
directory. The actual probe additionally uses a real detached Git worktree for
the consumer-cwd read when the provider completes.

The local tests prove the resolution and packet contract, including one root
resolution and one full byte read per request, exact raw/persisted diagnostic
values, no fallback on authorization/read errors, and no packet after
cancellation. They do not substitute for the timed-out remote provider edge.

## Delivered contract and instrumentation

`factory/workstations/plan/AGENTS.md` now requires:

- recognition of `^[A-Za-z]:[\\/]` for both `C:\\...` and `C:/...`;
- verbatim preservation of an absolute decoded input in JSON and Markdown;
- one repository-root lookup for a relative value, followed by absolute
  persistence without worktree scanning;
- a complete read of an existing regular file before planning, with blocking
  errors for empty, missing, directory, unreadable, unauthorized, and escaped
  inputs; and
- `context.sourcePlanResolution.rawSourcePlan` and
  `context.sourcePlanResolution.persistedSourcePlan`, containing paths only.

No source-plan contents are copied into the diagnostic object. The local
validator opens the persisted absolute path after changing cwd, so a relative
cross-stage fallback would fail its byte assertion.

## Blast-radius snapshots and neighboring lane

The planning material records `69 of 299` current top-level worktree PRDs with
a relative `docs/temp` source plan on the 2026-08-30 planning snapshot. The
customer/operator snapshot records `71 of 297`; both values are retained here
as supplied scope evidence, not silently recomputed during implementation.

The neighboring `setup-workspace-preserved-worktree-prd-handoff` lane owns
packet discovery and copy behavior. This change does not edit its files, does
not edit live PRDs, and does not add recursive worktree discovery or filesystem
authority. The three-dot overlap check remains a later contention gate.

## Budget and remaining edge

The lane budget is at most four paid calls, USD `$2`, and 20 minutes. The
required pre-change command made no planner call because its harness was
missing. One corrected post-change attempt consumed a bounded 300-second
provider run and timed out; no retry loop was started. Actual provider/model
compliance, Project Lead/docs consistency, clean-room validation, rebasing,
CI, review feedback, and merge remain owned by their later gates.
