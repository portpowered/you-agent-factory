# BOOT-DEEP-RESEARCH-001

## Identity

- Status: `NEEDS_ITERATION`
- Factory: `@you/deep-research`
- Repository base: `34b0440134c25234a495a60c9bea4d66aee5119f`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-DEEP-RESEARCH-001`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Canary recording `BOOT-DEEP-RESEARCH-001-R02.replay.json`:
  `7F86EE0946D582FD7B35C3E554D2C9FD18EDC431054EF4E01629071A440A8DF1`
- Failed representative recordings:
  - `BOOT-DEEP-RESEARCH-001-R03.replay.json`: `A59020B8E40CF80EA189B5E5C45A8978301BDE2388363E8771F7117225F90558`
  - `BOOT-DEEP-RESEARCH-001-R04.replay.json`: `105C0956BC695C93B79DB1DF233441D0ACBF8E9BA5BD2919736CCEA95578EAD2`
  - `BOOT-DEEP-RESEARCH-001-R05.replay.json`: `22069EF9C811FF7D9048E0ACCCC300B9880CAFCE53ADC22FCAC3924A40DCFAFB`
  - `BOOT-DEEP-RESEARCH-001-R06.replay.json`: `36BF8DF063F4CBD98F7F8E3A88F612DB34000BC7BC58AAEC371E9E90B90EA85B`

## Results and required iteration

The lead-only canary succeeded and returned a useful source-grounded answer.
Every representative attempt reached parallel specialist dispatch, but at least
one specialist failed with provider `permanent_bad_request`; the identical
specialist prompt digests succeeded in other attempts, proving the request was
not deterministically malformed. Because the original workflow failed the
entire session on any specialist failure, no representative result was accepted.

The resulting factory improvement reserves a bounded retry for each specialist,
preserves explicit unavailable-specialist diagnostics after retry exhaustion,
and still asks the lead to synthesize an honest result. A fresh worktree and
holdout representative run are required before this factory can meet expectations.

## Decision

- Canary status: `PASSED`.
- Representative status: `FAILED`.
- Goal status: `NEEDS_ITERATION` pending the post-fix holdout.
