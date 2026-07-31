# BOOT-SUBAGENT-002

## Identity

- Status: `INCONCLUSIVE`
- Factory: `@you/subagent`
- Workload class: contract canary
- Repository base: `d8edfaaa2fa13702de5a7ae7457fdce5dc30af62`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-SUBAGENT-002`
- Model: provider `CODEX`, model `gpt-5.6-terra`
- Recording: ignored local artifact
  `.artifacts/bootstrap/BOOT-SUBAGENT-002-R01.replay.json`
- Recording SHA-256:
  `3D7196F15BCCD9948985A5A23A8923A710F919159D6936F1413C8375221DCB09`

## Frozen request

> Inspect packages/packaged-factories/factories/subagent/factory.yaml without
> editing files. Report the factory name, its customer-facing purpose, and
> whether its worker defaults skipPermissions to true. Cite the inspected path.

## Observed result

- The invocation exited successfully in 24 seconds and returned all three
  requested facts with the exact inspected source path.
- The replay contains one `DISPATCH_REQUEST`, one `MODEL_REQUEST`, one
  `AGENT_RUN_RESPONSE`, and one terminal `SESSION_COMPLETED` event.
- The worktree had no tracked changes or unrequested artifacts. Its only new
  path was the documented `.you-agent-factory` runtime state.
- The run used neither an invocation-level `--skip-permissions` flag nor an
  isolated replacement for the customer's normal process environment.

## Decision

- This run is not accepted as current-package evidence. Debugging the distinct
  workload showed that `--named @you/subagent` resolved an editable global
  installation created before `skipPermissions: true` was published. The
  successful answer did not prove that repository tools executed.
- Preserve this result as evidence that live trials must record the resolved
  Factory source and must not silently mix current source with an older global
  installation.
