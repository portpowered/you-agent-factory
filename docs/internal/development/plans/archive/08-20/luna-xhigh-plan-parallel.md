# Luna xhigh plan-parallel implementation

## Outcome

`@you/plan-parallel` can run its task executor workers with
`gpt-5.6-luna` at canonical `xhigh` reasoning effort. The Factory definition
expresses the effort without provider-specific flags, Workers and Providers
preserve it through execution, and the Codex adapter alone renders the native
CLI configuration.

Cursor execution remains ACP-only. Cursor ACP models encode their effort in
their exact advertised model identifiers, such as
`cursor-grok-4.5-medium-fast`; this change does not recreate a native Cursor
provider or synthesize Cursor model identifiers from a separate effort field.

## Behavior contract

- Authored model-backed workers may set optional `reasoningEffort`.
- Supported canonical efforts include `minimal`, `low`, `medium`, `high`,
  `xhigh`, and `max`.
- Surrounding whitespace and case are normalized.
- Omitted effort preserves the provider default.
- Unsupported effort fails before provider execution.
- Codex renders a requested effort as
  `--config model_reasoning_effort="<effort>"`.
- Claude renders its supported values as `--effort <effort>` and rejects the
  provider-neutral `minimal` value before starting the CLI.
- `@you/plan-parallel` exposes `--executor-reasoning-effort`, defaults it to
  `xhigh`, and applies it only to `parallel-executor`.
- Planner and merger effort remain unspecified unless separately implemented
  by a future change.

## Package and artifact scope

- `pkg/services/factory_definitions`: authored Worker contract, invocation
  interpolation, validation, and runtime-config merging.
- `pkg/services/workers`: workstation and Agent Runner propagation.
- `pkg/services/providers`: provider-neutral execution contract, Codex and
  Claude rendering, and ACP exact-model semantics.
- `pkg/services/factory_sessions`: live-child request propagation.
- `pkg/transports/mapping`, `api/`, `contracts/`, `packages/api`, and
  `ui/**/generated`: public contract sources, mappings, and generated clients.
- `packages/packaged-factories`: authored and generated `@you/plan-parallel`
  and `@you/subagent` definitions.
- `docs/reference`: public Worker and JavaScript-workflow documentation.

No native Cursor package is added; `cursor-acp` remains the only Cursor
execution surface.

## Test and integration matrix

- Unit/contract tests cover canonicalization, invalid values, request cloning,
  runtime overlay behavior, ACP rejection, and exact Codex/Claude arguments.
- Packaged Factory functional tests cover default `xhigh`, a `low` override,
  executor-only scoping, invalid resolved effort before executor startup,
  concurrent task dispatch, and the subagent Luna/xhigh command.
- Generation checks cover OpenAPI, JSON Schema, Go/TypeScript clients, and the
  packaged Factory catalog.
- Live bootstrap checks use the binary built from this worktree to run
  `@you/subagent` and `@you/plan-parallel`, then inspect the actual child
  process arguments.
- Delivery integration runs repository verification and required GitHub CI,
  repairs failures, and repeats until the PR is merged.

## Implementation checklist

### Authored and public contracts

- [x] Add `reasoningEffort` to the authored Worker contract and clone path.
- [x] Add Worker OpenAPI/schema documentation for the field.
- [x] Extend canonical effort normalization through `xhigh` and `max`.
- [x] Validate literal worker effort and resolved invocation interpolation.
- [x] Allow invocation parameters to interpolate `worker.reasoningEffort`.
- [x] Add focused positive and invalid-value contract tests.

### Runtime propagation

- [x] Carry effort in workstation and provider inference requests.
- [x] Preserve effort through request clones and worker adaptations.
- [x] Carry effort through the Agent Runner to `providers.ExecuteRequest`.
- [x] Preserve effort through native and conductor provider-root adaptation.
- [x] Fix the existing JavaScript live-child handoff so it does not drop
      `reasoningEffort`.

### Provider behavior

- [x] Render the exact Codex config arguments for non-empty effort.
- [x] Emit no Codex effort arguments when effort is omitted.
- [x] Test exact argument order/value for `xhigh`.
- [x] Reject Claude `minimal` at both the public and legacy provider
      boundaries without starting a provider process.
- [x] Resolve native provider aliases before provider-specific effort policy so
      case/whitespace variants retain `invalid_request` classification.
- [x] Reject unsupported effort defensively in direct Codex/Claude adapters and
      legacy provider argument builders.
- [x] Keep Cursor ACP model selection exact and avoid a native Cursor path.

### Packaged Factory

- [x] Add the `executorReasoningEffort` invocation parameter to
      `@you/plan-parallel`.
- [x] Default the parameter to `xhigh`.
- [x] Bind it only to the `parallel-executor` worker.
- [x] Regenerate packaged Factory catalog output.
- [x] Update plan-parallel functional coverage to prove executor-only effort.

### Generated artifacts and documentation

- [x] Regenerate public API and contract artifacts from authored sources.
- [x] Update public reasoning-effort documentation.
- [x] Confirm generated-artifact drift checks pass.

## Verification checklist

- [x] Focused Factory Definitions tests pass.
- [x] Focused Workers tests pass.
- [x] Focused Providers argument and adapter tests pass.
- [x] Focused Factory Sessions live-child tests pass.
- [x] Plan-parallel functional tests prove concurrent Luna/xhigh executors.
- [x] Contract generation and checks pass.
- [x] Packaged Factory generation and checks pass.
- [x] `make verify-fast` passes.
- [ ] `make verify-pr` passes.
- [x] `make build-all` passes.

## Bootstrap and independent confirmation

- [x] Build a local `you` bootstrap binary from the changed source.
- [x] Run an initial read-only `@you/subagent` review using
      `gpt-5.6-luna` with `xhigh`.
- [x] If the subagent identifies a blocking gap, fix it and repeat the
      subagent review until it completes without a blocking finding.
- [x] Run a read-only `@you/plan-parallel` Luna/xhigh canary and inspect its
      retained dispatch/provider evidence.
- [x] Use the bootstrapped Luna/xhigh executor path for a bounded follow-up
      audit or repair task.

## Delivery checklist

- [x] Review the final diff for unrelated or generated-only edits.
- [x] Commit the implementation intentionally.
- [ ] Push `codex/luna-xhigh-parallel`.
- [ ] Open a draft PR and monitor required CI to terminal state.
- [ ] Use Luna/xhigh subagent/factory review to inspect failures and blocking
      review feedback.
- [ ] Fix failures, regenerate as needed, push, and repeat until required CI is
      green and blocking feedback is resolved.
- [x] Resolve merge conflicts against the target branch.
- [ ] Mark the PR ready when the implementation and evidence are complete.
- [ ] Merge the PR and confirm the repository reports the merged state.

Completion requires every applicable checkbox above, terminal green required
CI, resolved blocking review feedback and conflicts, and the PR actually
merged.
