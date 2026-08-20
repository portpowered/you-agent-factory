# BOOT-SUBAGENT-001

## Identity

- Status: `FAILED`
- Factory: `@you/subagent`
- Workload class: contract canary
- Repository base: `git rev-parse HEAD` at execution time
- Model: provider `CODEX`, model `gpt-5.6-terra`
- Repeats: `R01`, `R02`
- Sensitive recordings: ignored local artifacts
  `.artifacts/bootstrap/BOOT-SUBAGENT-001-R01.replay.json` and
  `.artifacts/bootstrap/BOOT-SUBAGENT-001-R02.replay.json`

## Hypothesis

One bounded subagent will inspect a single packaged Factory definition without
editing the workspace, return the requested facts with a source path, and leave
the working tree unchanged.

## Frozen request

> Inspect packages/packaged-factories/factories/subagent/factory.yaml without
> editing files. Report the factory name, its customer-facing purpose, and
> whether its worker defaults skipPermissions to true. Cite the inspected path.

## Exact command shape

```powershell
.\.artifacts\bootstrap\bin\you.exe run --named @you/subagent `
  --provider CODEX `
  --model gpt-5.6-terra `
  --output primary `
  --record .artifacts/bootstrap/BOOT-SUBAGENT-001-R01.replay.json `
  "<frozen request above>"
```

`R02` changed only the recording suffix from `R01` to `R02`.

## Observed result

- Both runs exited with code `0` and the replay marked the model response and
  Factory Session successful.
- Both primary results said the requested file could not be inspected because
  a workspace shell or read sandbox failed to start.
- The requested facts were not returned, so both runs fail the intended-outcome
  hard gate despite their successful process and session states.
- Each replay contains exactly one `run-subagent` dispatch and one model
  request. No nested Factory Work was generated.
- A control invocation of the same Codex executable, model, working directory,
  and unrestricted permission flag read the file successfully. An explicit
  Codex `read-only` sandbox reproduced a Windows
  `CreateProcessWithLogonW failed: 2` failure in this environment.
- No tracked file was changed by either canary.

## Failure classification

`Provider/model behavior` and `runtime policy integration`. The live outcome
shows that a successful provider exit is insufficient evidence of task success.
The current `AGENT_RUN` harness owns a read-only tool policy, while the Codex
subprocess also has provider-native tools and receives the packaged
`skipPermissions` setting. The external read-only guarantee is therefore not
yet proven end to end.

## Decision

- Goal status: `NEEDS_ITERATION`
- Do not tune the workload or mark the Factory successful.
- Preserve the frozen case and rerun it after provider-native tool policy and
  false-success handling have an enforceable design.
- A passing rerun must return all three requested facts, cite the path, contain
  exactly one child dispatch, and leave the workspace unchanged.
