# Current world state

## System architecture

- Live `~default` PETRI session: `87cc7a00-4f23-4d36-adef-36ff99ea8754`.
- Customer ask: maximize test-throughput and lint/validation cleanup with
  maximal parallelism; fix underlying causes on failure. Additional ask block
  empty on this pass.
- Capacity: `executor-slot` 0/16, `concurrent-planners` 0/1 (saturated).
- Tip includes delivered lint/test slices: #1666 UI dead-code, #1667/#1671 UI
  unit throughput, #1668 Bun unit foundation, #1670 LTV-FUN-02.

## Operational notes

- Session replacement can strand queues; recover from Git/PR/worktree evidence
  with new request IDs. Do not re-admit same-name work already live.
- Plan dual-output defect: COMPLETE + dual PRDs without `plan:init` under
  concurrency. Never blanket `failed→init` after retry 1; reconstruct with
  `idea:to-complete` + injected `plan:init`, restore guarded loopbacks.
- 2026-08-01 18:06: 0 FAILED; tokens initial 61 / processing 65 / terminal 74 /
  in-flight 17 / total 200. ~10 tasks PROCESSING + ~50 queued; reconstructed
  ideas at `to-complete`; harness + five Bun cohorts now `idea:init`
  PROCESSING (plan dispatched). Hold admission while saturated.
- Chart #1665 OPEN/MERGEABLE, all required checks SUCCESS — merge still
  required. Two same-name chart tasks both PROCESSING; empty duplicate move
  rejected mid-dispatch (retry next pass if still idle). Do not planner-
  terminalize green-but-unmerged delivery-contract work.
- `docs/temp/**` is gitignored planner state.

# Progressive change notes

## High-level track state

- Phase 1 delivered: UI lint baseline, UI unit throughput, FUN-02.
- Open product lanes: LTV-GO, LTV-FUN-01, BSZ lanes, Bun cohorts (mostly past
  plan; tasks in process/queue), chart merge (#1665), residual backend-size,
  Phase 3 terminal proof.
- Plan capacity reopened for the last six PRD-less ideas (5 Bun + harness).
- Harness track still required before trusting natural plan dual-output.
- Next loopback: inspect merges/CI/migration evidence; retry chart-duplicate
  terminalization only if the empty twin is moveable; do not auto-refill
  while executors are saturated.
