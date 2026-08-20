# BOOT-PLAN-PARALLEL-001

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/plan-parallel`
- Workload class: contract canary
- Repository base: `847f28be39e6b0f06bf76bad0bcab88bdbd5d689`
- Generated Factory artifact SHA-256 before the resulting prompt hardening:
  `02c07fff61cb5fc98f99b3bd5c377c1ab5031b72bab96102b9eb30091b4e684c`
- Provider/model roles: planner and merger `CODEX` / `gpt-5.6-sol`;
  executor `CODEX` / `gpt-5.6-terra`
- Sensitive local recordings:
  `.artifacts/bootstrap/BOOT-PLAN-PARALLEL-001-R01.replay.json` and
  `.artifacts/bootstrap/BOOT-PLAN-PARALLEL-001-R02.replay.json`
- Recording SHA-256 values: R01
  `E362D670EBB20F18887DFEFA4B9EBB4D712EB75735D7AD54C38B61B461E4ADAD`;
  R02 `6C52A029BF9D0125B62168C9B145B196C1C8CC123CA552B19A17209FEBD902E2`
- R01 duration: `632364 ms`
- R02 started: `2026-07-30T12:12:00.7404587Z`
- R02 finished: `2026-07-30T12:28:40.8413827Z`
- R02 duration: `1000101 ms`

## Hypothesis

Given `test-improvement.md`, the Factory will create an evidence-gathering DAG,
run independent investigations concurrently, block synthesis on every declared
prerequisite, merge all results, return a source-grounded plan, and leave the
isolated worktree unchanged.

## Frozen request

> Read test-improvement.md and validate its claims against the current
> repository. Decompose the investigation into genuinely independent
> evidence-gathering tasks plus any dependency-aware synthesis check. Do not
> edit files. Return one concrete, prioritized implementation plan that cites
> repository paths, measured or directly observed evidence, verification
> commands, risks, and any claims in test-improvement.md that are unsupported.

## Exact command

```powershell
C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\bin\you.exe run `
  --named @you/plan-parallel `
  --planner-provider CODEX --planner-model gpt-5.6-sol `
  --executor-provider CODEX --executor-model gpt-5.6-terra `
  --merge-provider CODEX --merge-model gpt-5.6-sol `
  --output primary `
  --record C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\BOOT-PLAN-PARALLEL-001-R01.replay.json `
  --to "<frozen request above>"
```

The run used an isolated detached Git worktree and an isolated `HOME` and
`USERPROFILE`; `CODEX_HOME` remained the operator's existing Codex home so the
provider could authenticate.

## Observed orchestration

- Planner produced five `planned-task` Work items, five `PARENT_CHILD`
  relations, and four `DEPENDS_ON` relations.
- Four evidence tasks began at `12:00:23.705Z`, proving parallel readiness and
  dispatch.
- The last prerequisite completed at `12:03:12.123Z`; dependent synthesis
  began at `12:03:12.126Z`.
- Synthesis completed at `12:06:01.458Z`; the terminal merger began at
  `12:06:01.461Z`.
- The final result distinguished static evidence from unsupported timing
  claims, cited repository paths, disclosed that one branch did not provide
  usable corroboration, and did not invent successful verification.
- All provider processes and the Factory Session reported successful terminal
  outcomes.

## Repeat result and limitations

R02 repeated the correct graph behavior with four independent evidence tasks,
four dependency edges into synthesis, and a terminal merger. All four evidence
dispatches began at `12:15:06.670Z`. The slowest finished at `12:24:31.755Z`;
synthesis dispatched immediately and completed at `12:26:24.341Z`; only then
did the merger dispatch. The completed result reconciled conflicting counts,
separated measured observations from historical claims, cited source paths,
and returned an actionable plan.

The `.you-agent-factory/durable-sessions/~default.json` file observed in both
runs is the outer Factory's own documented durable-session snapshot. It is
expected runtime bookkeeping, not a planner-created second session or an agent
workspace edit. Both worktrees retained no tracked changes. R02 created an
ignored `.artifacts/codex-functional-measure` directory while running requested
measurements; it did not change source.

The isolated `HOME`/`USERPROFILE` made normal repository tools unavailable to
some Codex tool shells. One executor recovered explicit Go and Git paths and
produced real measurements, but the environment inconsistency extended the run
and prevented independent measurement replication. Future trial worktrees must
remain separate while preserving a normal customer tool environment; isolate
recordings and runtime state instead of replacing the entire user profile.

One executor observed a single packaged-catalog `@you/spawn` initialization
failure during a heavily concurrent measurement. The same focused case passed
five consecutive reruns on the pinned R02 worktree, so this remains a recorded
non-reproduced flake rather than a confirmed packaged Factory defect.

R02 took 1,000 seconds and the `you` process consumed roughly one CPU core while
waiting on model work. The external 15-minute command wrapper timed out, but
the underlying process continued, completed every dispatch, finalized the
recording, and exited. Busy-wait efficiency and validation timeout sizing must
be corrected before broad live trials.

## Decision

- Canary status: `PASSED`
- Goal status: `NEEDS_ITERATION` until a distinct representative holdout passes.
- The intended Work DAG, parallel dispatch, dependency gating, all-child fan-in,
  and evidence-aware merger are validated by two live runs plus deterministic
  invocation coverage.
- Correct the shared runtime busy-wait and preserve the customer's normal tool
  environment before running the representative holdout.
