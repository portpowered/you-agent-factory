# BOOT-TOURNAMENT-001

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/tournament`
- Repository base: `98a921b0f28864b1f716efb73d280ce3658934b1`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-TOURNAMENT-001`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Current generated artifact:
  `packages/packaged-factories/generated/factories/tournament/factory.yaml`
- Generated artifact SHA-256:
  `B7840559AB8519BF1B2D61772289C77902BDAD6EAF6396DA3C6BB571684B343A`
- Recorded Factory hash:
  `sha256:3ad0db88e324f470267a402188d354d553ebd79eb236d8005980f0361f26cd3c`
- Accepted recordings and SHA-256 values:
  - one-round canary `BOOT-TOURNAMENT-001-R01.replay.json`:
    `8BB774D736848116026A0563883E385C240C18A480F73823CA38A700A0293114`
  - two-round representative `BOOT-TOURNAMENT-001-R02.replay.json`:
    `37711DB41FCB11B2E1EDB2C63A17B4A35888B01BEA7456249D43378B0EC4A74B`

## Exact commands

```powershell
& 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\bin\you-98a921b0f.exe' run --factory 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\worktrees\BOOT-TOURNAMENT-001\packages\packaged-factories\generated\factories\tournament\factory.yaml' --record 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\BOOT-TOURNAMENT-001-R01.replay.json' --rounds 1 --competitor-provider CODEX --competitor-model gpt-5.6-terra --judge-provider CODEX --judge-model gpt-5.6-terra --to 'Decide how this repository should address the mismatch where packaged factories can be installed as YAML or YML but named catalog resolution expects factory.json. Compare extending named resolution to all authored formats against restricting packaged installs to JSON. Recommend one implementation with migration and focused test evidence. Do not modify files.'

& 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\bin\you-98a921b0f.exe' run --factory 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\worktrees\BOOT-TOURNAMENT-001\packages\packaged-factories\generated\factories\tournament\factory.yaml' --record 'C:\Users\andre\work\portos\infinite-you\.artifacts\bootstrap\BOOT-TOURNAMENT-001-R02.replay.json' --rounds 2 --competitor-provider CODEX --competitor-model gpt-5.6-terra --judge-provider CODEX --judge-model gpt-5.6-terra --to 'Propose the best dependency-aware implementation sequence for revising test-improvement.md so it complies with current repository architecture and functional-test standards while still reducing test duration. The proposal must identify what to measure first, which tests to relocate or replace, how to preserve behavior evidence, required verification gates, and risks that should remain separate. Do not modify files.'
```

## Results

The canary queued two blind competitors in the candidate-generation phase and
only then queued one round-one judge. It completed `SUCCEEDED` / `FINAL` in
158.383 seconds and exited zero. The chosen candidate was returned unchanged as
plain text, followed by a non-empty decision rationale. The recommendation was
source-grounded and independently confirmed both the named-path JSON-only gap
and the adjacent defect where YAML/YML installation filenames currently receive
the JSON payload.

The representative queued exactly four competitors, then two parallel
semifinal judges, then one final judge. No eliminated candidate was regenerated
or replaced. It completed `SUCCEEDED` / `FINAL` in 193.082 seconds and exited
zero. The surviving candidate carried exactly two decision-trail entries: its
semifinal advancement and final advancement.

The champion plan was implementation-ready and consistent with current
standards. It required measurement before optimization, retired the
deletion-only `tests/functional/runtime_api/factory_transformation` package
instead of splitting beneath it, separated decision matrices from public
boundary sentinels, preserved ACP wire/event/redaction evidence, conditioned
server reuse on isolation, and kept production ACP timeout policy outside the
test-duration lane. The two judge rationales explained decisive strengths and
material weaknesses instead of merely declaring winners.

The worktree remained unchanged except for expected untracked
`.you-agent-factory/` runtime state. `make functional-boundary-check` passed on
the pinned tree. The short Go suite passed before the trial. Tournament-specific
runtime tests passed for 20 repeats after adding coverage for one-, two-, and
three-round brackets, exact call budgets, stable winner advancement, fenced
judge JSON, invalid selections, empty rationales, and competitor/judge failure
provenance. Command-runner and persistent-ACP integration tests also require a
CLI-renderable champion plus rationale.

## Pre-trial correction

Before freezing the worktree, source and existing tests showed that the bracket
returned an internal object. That would reproduce spawn's default-CLI failure
and did not directly fulfill the advertised “champion result” behavior. Commit
`98a921b0f` changed the terminal value to the champion answer plus its decision
trail and made an empty judge rationale a bounded, round-and-match-specific
failure. The accepted trials use only that committed source.

## Score and decision

- Intended outcome: 5/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 4/4
- Safety and scope: 3/3
- Final result quality: 2/2
- Efficiency: 1/2
- Total: 19/20
- Canary status: `PASSED`.
- Representative status: `PASSED`.
- Goal status: `MEETS_EXPECTATIONS`.
- Limitation: child agent processes could inspect repository files but could
  not execute `go`, `make`, or `git`; parent-side focused verification supplied
  the missing executable evidence.
