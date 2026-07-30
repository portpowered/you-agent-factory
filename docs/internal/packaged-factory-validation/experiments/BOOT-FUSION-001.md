# BOOT-FUSION-001

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/fusion`
- Repository base: `34b0440134c25234a495a60c9bea4d66aee5119f`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-FUSION-001`
- Model for both roles: provider `CODEX`, model `gpt-5.6-terra`
- Accepted recordings and SHA-256 values:
  - canary `BOOT-FUSION-001-R01.replay.json`:
    `514500CF77BF07C8FF0C17DBDE2C652D5228CA1C9D140D0B3F56D28D8A859082`
  - representative `BOOT-FUSION-001-R03.replay.json`:
    `4F75A688E7D9D1D3D25C37913791AFD7F2D857DC71FFB1A80513427838BDFC66`

## Results

The canary produced the requested concise, cited explanation. The representative
run first drafted and then independently refined a package-update strategy. Its
final answer recommended managed defaults with editable overrides, separated
alternatives, described migration and customization risks, and cited repository
evidence. The second pass materially improved structure and decision usefulness.

One concurrent attempt failed in the refiner with a transient provider
`permanent_bad_request`; the isolated retry completed. The accepted answer also
repeated the disputed peripheral claim that canonical JSON preparation for a
YAML target is itself a defect, although the downstream renderer and passing
format materialization test show that the installed output is YAML. Neither
issue defeated the requested two-stage decision outcome.

The worktree had no tracked changes or unrequested artifacts; only documented
runtime state was created.

## Score and decision

- Intended outcome: 5/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 3/4
- Safety and scope: 3/3
- Final result quality: 1/2
- Efficiency: 1/2
- Total: 17/20
- Canary status: `PASSED`.
- Representative status: `PASSED`.
- Goal status: `MEETS_EXPECTATIONS` with the provider flake and disputed
  peripheral defect label retained as limitations.
