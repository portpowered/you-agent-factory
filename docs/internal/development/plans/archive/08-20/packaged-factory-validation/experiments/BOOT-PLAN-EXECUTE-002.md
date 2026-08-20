# BOOT-PLAN-EXECUTE-002

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/plan-execute`
- Repository base: `4019c27122df9238aa99a92aeabf29e8520917c9`
- Worktree: `.artifacts/bootstrap/worktrees/TEST-IMPROVEMENT-001`
- Planner: provider `CODEX`, model `gpt-5.6-sol`
- Executor: provider `cursor-acp`, model `composer-2.5`
- Original recording:
  `.artifacts/bootstrap/TEST-IMPROVEMENT-001/PLAN-EXECUTE-R01.replay.json`
- Original recording SHA-256:
  `9B3DB0F14D2F183053DD0A1D762D8FCD7F98642C77B99779409F6E9C5E3D26F4`
- Fixed holdout recording:
  `.artifacts/bootstrap/TEST-IMPROVEMENT-001/PLAN-EXECUTE-R02.replay.json`
- Fixed holdout recording SHA-256:
  `3028C083F71FE47E7381496CA593AF1D12F679ED4C987FBBC49F25A7E503AB23`
- Fixed generated artifact SHA-256:
  `F3D8FCEFF6EBECA84952472DF18DBC8379985B73261DD99CE262FD6A227394A5`

## Frozen request and command

The request told the Factory to converge the current `test-improvement.md`
workspace after the parallel trial, write matching durable Markdown and JSON
PRDs, implement every story in order, preserve justified performance work,
resolve remaining result-contract failures from authoritative evidence, remove
an unrelated `coverage` rewrite, and record three green functional timings.

Both attempts used this command shape, with the second using the fixed binary
and generated artifact:

```powershell
& '.artifacts/bootstrap/bin/you-b7ead2eef.exe' --json run `
  --factory 'packages/packaged-factories/generated/factories/plan-execute/factory.yaml' `
  --record '../../TEST-IMPROVEMENT-001/PLAN-EXECUTE-R02.replay.json' `
  --output primary `
  --planner-provider CODEX --planner-model gpt-5.6-sol `
  --executor-provider cursor-acp --executor-model composer-2.5 `
  --to '<the frozen convergence request above>'
```

The complete unabridged request is retained in both recordings.

## R01: useful execution rejected by a fragile stop token

The planner wrote matching `tasks/todo/work-1.md` and `work-1.json` documents
with seven ordered stories. The fresh Cursor executor read them, implemented
the stories, updated each story's `passes` and `notes`, and reported three green
functional runs.

The executor ended with backticked `` `<COMPLETE>` ``. The workstation required
the exact raw token, so the model response succeeded but the agent-run and
dispatch outcomes were `REJECTED`; the invocation failed with
`INVOCATION_PRIMARY_RESULT_UNRESOLVED` after 938.3 seconds.

The packaged prompt was hardened to require `<COMPLETE>` as the exact final
non-empty line with no backticks, fence, prefix, suffix, or following prose.
A functional regression assertion was added and the generated catalog was
refreshed in commit `b7ead2eef`.

## R02: fixed sequencing and independent output audit

The fixed artifact completed in 1003.2 seconds. It again produced agreeing
durable PRDs, dispatched only the planner followed by the executor, accepted
both exact stop tokens, marked all seven stories complete with verification
notes, and ended with invocation status `COMPLETED`. This validates the intended
PRD-to-fresh-executor handoff with Cursor Composer as the implementation model.

The executor reported three green functional runs at 76.028, 77.075, and 76.589
seconds. Independent review rejected one substantive choice: it had changed
JavaScript string projection from `TEXT` to JSON and changed tests to match.
Repository history established that plain text was intentional customer-facing
behavior. The accepted parent correction restored `TEXT`, updated every stale
consumer assertion, and reran the full suite three times at 75.277, 75.884, and
76.178 seconds, all green.
The combined promoted root then passed another full functional run in 81.394
seconds.

## Score and decision

- Intended outcome: 4/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 3/4
- Safety and scope: 2/3
- Final result quality: 2/2
- Efficiency: 0/2
- Total: 15/20
- Canary status: `PASSED`.
- Representative orchestration status: `PASSED`.
- Representative implementation status: `NEEDS_INDEPENDENT_CORRECTION`.
- Goal status: `MEETS_EXPECTATIONS` based on the complete evidence set.

The fixed trial proves reliable serial planning, durable PRD handoff, fresh
Cursor execution, story tracking, verification, and exact completion routing.
It also shows that green agent-authored tests are not sufficient acceptance
evidence when an executor can change both a contract and its consumers.
