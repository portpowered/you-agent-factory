# BOOT-GOAL-004

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/goal`
- Repository base: `9078bf7d74cd84b1bdd623a512fd6ef80f7f1c07`
- Canary worktree: `.artifacts/bootstrap/worktrees/BOOT-GOAL-003`
- Representative worktree: `.artifacts/bootstrap/worktrees/BOOT-GOAL-004`
- Model for the executor: provider `CODEX`, model `gpt-5.6-terra`
- Current generated artifact:
  `packages/packaged-factories/generated/factories/goal/factory.yaml`
- Generated artifact SHA-256:
  `BB5FD122193E2D6FAC8919E9DFB224BC46C5C1572C119FFFE533E30528B6849B`
- Recorded Factory hash:
  `sha256:4e3ee8cbee36bd4b0e7cf83160d777c6999f56219e281538d20091e1550c35d4`
- Accepted recordings and SHA-256 values:
  - two-pass canary `BOOT-GOAL-003-R01.replay.json`:
    `31FF31FD8C80D1B103DA137436BC787D7B189D1C72A5A62749B96FF5138939AF`
  - mutating representative `BOOT-GOAL-004-R01.replay.json`:
    `A373B5820422149C09B448F8643692943FD5BC7B1ED4249FC53868C2BE09608F`

## Exact commands

```powershell
& 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\bin\you-9078bf7d7.exe' run --factory 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\worktrees\BOOT-GOAL-003\packages\packaged-factories\generated\factories\goal\factory.yaml' --record 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\BOOT-GOAL-003-R01.replay.json' --executor-provider CODEX --executor-model gpt-5.6-terra --to 'Perform exactly two read-only passes. On pass 1, inspect the authored @you/goal Factory and verify its completion markers and visit bound from source; report those facts and end with <CONTINUE>. On pass 2, use the prior output, inspect the packaged goal functional tests that prove repeat and exhaustion behavior, then return one consolidated source-grounded assessment and end with <COMPLETE> only if both sets of facts are verified. Do not modify files.'

& 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\bin\you-9078bf7d7.exe' run --factory 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\worktrees\BOOT-GOAL-004\packages\packaged-factories\generated\factories\goal\factory.yaml' --record 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\BOOT-GOAL-004-R01.replay.json' --executor-provider CODEX --executor-model gpt-5.6-terra --to 'Complete this bounded Provider Sessions root cleanup. Remove the exported CanonicalProvider and CloneMetadata functions from pkg/services/provider_sessions while preserving their exact behavior at the owning consumer boundaries or in private helpers; do not introduce cross-service imports of peer internal packages. Add or update focused tests that prove provider alias normalization and nil-safe detached metadata cloning. Update a package-structure baseline only if the checker identifies the corresponding entries as stale, and do not absorb unrelated violations. Completion requires: production searches show neither exported root helper remains; focused Provider Sessions, Workers, and Factory Sessions tests pass; relevant package boundary and structure checks introduce no new violation; the diff stays scoped and formatted. Inspect current standards and source before editing, preserve unrelated work, iterate as needed, and report commands actually run. End with <COMPLETE> only when every condition you can verify is satisfied; otherwise end with <CONTINUE> after coherent progress.'
```

## Results

The canary completed two real executor dispatches in 59.308 seconds. The first
attempt reported the authored completion, continuation, routing, and visit-bound
facts and returned `CONTINUE`. The loopback Work retained the complete original
request in canonical content while `_last_output` retained the first result.
The second attempt used both contexts, checked the repeat and exhaustion tests,
returned one consolidated answer, and completed. No third dispatch occurred.

The representative completed a bounded Provider Sessions cleanup in one pass
and 191.151 seconds. It removed the two requested root helpers, preserved alias
normalization and detached metadata cloning in the owning consumers, added
focused tests, and removed only the corresponding two stale structure-baseline
entries. Completing in one pass was correct because all stated conditions were
met; the canary independently proves continuation and preserved progress.

Parent-side verification confirmed the production helper search was empty,
`git diff --check` passed, and the following focused test command passed:

```text
go test ./pkg/services/provider_sessions/... ./pkg/services/workers ./pkg/services/workers/internal/services/workstations/execution ./pkg/services/factory_sessions ./pkg/services/factory_sessions/internal/stream -count=1
```

`make pkg-structure` still reports the pinned base's unrelated 21 new violations
and 28 stale entries, with no remaining Provider Sessions root-helper violation.
`make pkg-boundary` remains blocked by the pre-existing Factory Runtime Petri
baseline entry. The experiment patch remains on its trial branch and is evidence,
not an automatically promoted production change.

Deterministic coverage additionally proves accepted completion summaries,
continue and rejection loopbacks, exact twelve-visit exhaustion with no
thirteenth dispatch, content and prior-output preservation, and final-line-only
recognition of `<COMPLETE>`. The full Go short suite passed before the trials.

## Rejected prior trials

`BOOT-GOAL-001-R01` stopped after one dispatch because prose containing
`<COMPLETE>` was treated as completion even though the final marker was
`<CONTINUE>`. `BOOT-GOAL-002-R01` dispatched twice after marker recognition was
fixed, but the loopback summary replaced the original customer request. Those
failures motivated commits `a9a93b81a` and `9078bf7d7`; both accepted trials use
the latter committed artifact.

## Score and decision

- Intended outcome: 5/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 4/4
- Safety and scope: 3/3
- Final result quality: 2/2
- Efficiency: 2/2
- Total: 20/20
- Canary status: `PASSED`.
- Representative status: `PASSED`.
- Goal status: `MEETS_EXPECTATIONS`.
- Limitation: repository-wide structure and boundary gates have documented,
  unrelated baseline failures; focused verification proves this trial did not
  add to them.
