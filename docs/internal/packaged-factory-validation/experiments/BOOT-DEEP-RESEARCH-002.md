# BOOT-DEEP-RESEARCH-002

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/deep-research`
- Repository base: `754d98dbb373e6e46658984b13b8da8fd9daa3cd`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-DEEP-RESEARCH-002`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Accepted recordings and SHA-256 values:
  - representative `BOOT-DEEP-RESEARCH-002-R01.replay.json`:
    `62617E759FBEC1F554E9BC95E85E2935820E73C13EBACEFD58A50BD56317A839`
  - canary `BOOT-DEEP-RESEARCH-002-R02.replay.json`:
    `360B79CDA2E65AE1F80A19431528B96FDEE4138483C02B1F03885A019F77C6B5`

## Results

The representative holdout exercised the recovery path introduced after
`BOOT-DEEP-RESEARCH-001`. The technical specialist completed, the trade-off
specialist received an intermittent provider `permanent_bad_request`, and the
bounded `research-specialist-tradeoffs-retry` then completed. The lead synthesis
ran only after those attempts and returned `SUCCEEDED` / `FINAL` with both
logical specialist statuses reported as `COMPLETED`.

The resulting recommendation was coherent and source-grounded. It verified the
current ensure-installed behavior, editable-install contract, global resolution
priority, and lack of provenance metadata; distinguished facts from inference;
compared explicit refresh, pristine-only update, three-way merge, and immutable
versioned-cache options; and proposed a safe migration and regression suite.

The lead-only canary correctly explained `@you/subagent` from current authored
sources. The worktree had no tracked changes or unrequested artifacts; only
documented runtime state was created.

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
- The intermittent provider rejection remains a runtime limitation, but this
  holdout proves the factory now recovers within a bounded, observable budget.
