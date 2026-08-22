# BOOT-PLAN-EXECUTE-001

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/plan-execute`
- Repository base: `9cdd70fef3aa202cf5702d8aac002a05436ac044`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-PLAN-EXECUTE-001`
- Planner: provider `CODEX`, model `gpt-5.6-terra`
- Executor: provider `CODEX`, model `gpt-5.6-sol`
- Generated artifact SHA-256:
  `A21DA3F32EFA52474F8478E85BAA7413ABE2CB26EE84A5A7334C8522E41D7594`
- Recording: `.artifacts/bootstrap/BOOT-PLAN-EXECUTE-001-R01.replay.json`
- Recording SHA-256:
  `01896E1F5BE078D3D921C677404584F51C393C4C04DD224B85D56AB643D1B489`

## Exact command

```powershell
& '.artifacts/bootstrap/bin/you-9cdd70fef.exe' --json run --factory 'packages/packaged-factories/generated/factories/plan-execute/factory.yaml' --record '../../BOOT-PLAN-EXECUTE-001-R01.replay.json' --output primary --planner-provider CODEX --planner-model gpt-5.6-terra --executor-provider CODEX --executor-model gpt-5.6-sol --to 'Improve the focused regression coverage for the packaged full-flow cycle decision script. In tests/factory/scripts/test_full_flow_decide_cycle.py, add subprocess-level tests proving that running decide-cycle.py with continue prints exactly continue and exits zero, while running it with completion prose exits nonzero and reports the exact-route error on stderr. Keep production code unchanged, preserve existing tests, avoid generated artifacts, and run the focused Python test module. The durable PRD files must accurately describe this test-only scope and the executor must record actual verification evidence in every completed story.'
```

The command ran from the experiment worktree with isolated `HOME` and
`USERPROFILE` values rooted at
`.artifacts/bootstrap/homes/BOOT-PLAN-EXECUTE-001`.

## Results

The Factory completed in 257.3 seconds with exactly two dispatches: first
`plan-request`, then `execute-plan`. The planner wrote matching
`tasks/todo/work-1.md` and `tasks/todo/work-1.json` files. The JSON contained
two ordered stories; the executor marked both `passes: true` and recorded
non-empty notes containing the actual verification command and outcome.

The executor changed only
`tests/factory/scripts/test_full_flow_decide_cycle.py`, adding the two requested
subprocess-level cases. Independent verification passed:

```text
python -m unittest tests.factory.scripts.test_full_flow_decide_cycle
Ran 4 tests in 0.048s
OK
```

`git diff --check` passed, and the worktree contained no production or generated
changes. Runtime artifacts under `.you-agent-factory/` remained untracked.

The trial exposed one prompt-contract ambiguity: repository planning standards
could ask for PR, CI, and merge delivery steps even though this two-stage
Factory intentionally ends after verified implementation in the current
workspace. The authored planner prompt and customer-facing description were
subsequently corrected to state that boundary explicitly, with a focused
regression assertion. The accepted live trial predates only that clarification;
its planner/executor sequencing, durable handoff, implementation, and
verification evidence remain representative.

## Score and decision

- Intended outcome: 5/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 4/4
- Safety and scope: 3/3
- Final result quality: 2/2
- Efficiency: 1/2
- Total: 19/20
- Canary status: `PASSED`
- Representative status: `PASSED`
- Goal status: `MEETS_EXPECTATIONS`
- Limitation: the live recording uses the immediately preceding prompt wording;
  the current boundary clarification is covered deterministically.
