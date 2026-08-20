# BOOT-SUBAGENT-003

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/subagent`
- Workload classes: contract canary and representative repository analysis
- Repository base: `1ef39245d74acd1cf0659ca0bd68dfb6969697bf`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-SUBAGENT-003`
- Model: provider `CODEX`, model `gpt-5.6-terra`
- Current packaged artifact:
  `packages/packaged-factories/generated/factories/subagent/factory.yaml`
- Accepted recordings and SHA-256 values:
  - representative `BOOT-SUBAGENT-003-R04.replay.json`:
    `890AA2A252EF5FF8B6F4E37936BE67D9023C06B329BBC62AB592FE2F2F6CF750`
  - canary `BOOT-SUBAGENT-003-R05.replay.json`:
    `5129F419BE56E8FCEDF4AF5877943C1B171832D1FBD2674B268B6948F4BE8BAB`

## Workloads and results

The frozen canary from `BOOT-SUBAGENT-001` passed against the current generated
package in 13 seconds. It returned the canonical `@you/subagent` name, exact
customer purpose, `skipPermissions: true`, and the inspected authored path.

The distinct representative request required correlating the authored Factory
with its functional invocation test. It passed in 47 seconds and correctly:

- traced `input` through `task:init`, `run-subagent`, `subagent-worker`, and the
  terminal primary result;
- verified the packaged permission and read-only tool defaults;
- cited both requested files; and
- identified that the mock functional test verifies wiring and result shaping
  but does not exercise permission behavior.

Each replay has exactly one dispatch and model request. The worktree had no
tracked edits or unrequested artifacts; only documented runtime session state
was created.

## Rejected diagnostics

`R01` and `R02` used `--named` and failed repository reads. Debug output proved
that named resolution selected the operator's editable July 29 global install,
whose worker predates `skipPermissions: true`. `R03` selected the authored
packaging source and was rejected before execution because `promptFile` is a
catalog-generation input, not a runtime schema field. These runs are diagnostic
evidence, not accepted outcome evidence.

## Decision

- Canary status: `PASSED`.
- Representative status: `PASSED`.
- Goal status: `MEETS_EXPECTATIONS` for the current published artifact.
- Limitation: global packaged installs intentionally remain editable and are
  not silently replaced. Validation must select the current generated artifact
  or explicitly refresh a disposable install; it must report the resolved
  source either way.
