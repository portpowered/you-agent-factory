# Work Dispatch Runtime Boundary Data Model

This artifact records the ownership split for Agent Factory work-dispatch
runtime data. It names which facts remain on the canonical dispatch-owned
record and which facts must move into worker-boundary request types.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch
  `ralph/agent-factory-split-work-dispatch-runtime-and-execution-context`)
- Owner: Agent Factory maintainers
- Packages or subsystems: `pkg/interfaces`, `pkg/factory/subsystems`,
  `pkg/workers`, `pkg/replay`, `pkg/testutil`, `pkg/service`
- Canonical architecture document to update before completion: this file is the
  branch data-model construction artifact. Durable workflow rules live in
  `docs/processes/agent-factory-development.md`.

## Trigger Check

- [x] Shared noun or domain concept
- [ ] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [ ] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Ownership Table

| Fact | Canonical owner | Allowed copies | Notes |
| --- | --- | --- | --- |
| dispatch identity (`dispatch_id`, `transition_id`) | `interfaces.WorkDispatch` | copied into worker-boundary requests under `Dispatch` only | These values stay on the canonical dispatch record and remain the correlation key for replay, logging, and worker responses. |
| dispatch routing (`worker_type`, `workstation_name`) | `interfaces.WorkDispatch` | copied into worker-boundary requests under `Dispatch`; selected worker may also appear on `WorkstationExecutionRequest.worker_type` | Dispatcher and replay own the canonical route facts. Worker execution may derive a selected worker name but must not rewrite the dispatch record. |
| project context | `interfaces.WorkDispatch.project_id` | worker-boundary requests may carry a resolved `project_id` override | Dispatch carries the dispatch-owned project context; workstation rendering may resolve a more specific worker-boundary project for prompts, env, and working-directory expansion. |
| chaining and replay metadata | `interfaces.WorkDispatch.{current_chaining_trace_id,previous_chaining_trace_ids,execution}` | copied into worker-boundary requests under `Dispatch` only | Clone helpers must preserve these immutable facts for replay matching and correlation logging. |
| consumed input refs (`input_tokens`, `input_bindings`) | `interfaces.WorkDispatch` | copied into worker-boundary requests when ordering or provider input context matters | Workstation rendering may reorder token inputs for worker execution without mutating the canonical dispatch-owned token slice. |
| rendered prompt (`system_prompt`, `user_message`, `output_schema`) | `interfaces.WorkstationExecutionRequest`, `interfaces.ProviderInferenceRequest` | none on `WorkDispatch` | Prompt and schema data are resolved by workstation execution and must stay off the dispatch-owned contract. |
| runtime execution context (`env_vars`, `worktree`, `working_directory`) | `interfaces.WorkstationExecutionRequest`, `interfaces.ProviderInferenceRequest`, `workers.CommandRequest` | none on `WorkDispatch` | These values belong to the worker boundary and are derived from workstation config plus runtime context resolution. |
| provider runtime fields (`model`, `model_provider`, `session_id`) | `interfaces.ProviderInferenceRequest` | diagnostics or safe event projections may copy safe values | Provider selection and resume state are worker-owned and must not drift back onto `WorkDispatch`. |

## Canonical Clone Rules

- `pkg/interfaces` owns `CloneWorkDispatch`, `CloneExecutionMetadata`,
  `CloneWorkstationExecutionRequest`, and `CloneProviderInferenceRequest`.
- Downstream packages may clone smaller package-local shapes they own, but must
  not reintroduce package-local `cloneWorkDispatch(...)` helpers.
- Worker-boundary request types must embed the canonical dispatch-owned record
  under `Dispatch` rather than copying dispatch-owned fields into ad hoc local
  mirror structs.

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| canonical dispatch-owned contract | `pkg/interfaces.WorkDispatch` | dispatcher, replay, workstation executor, test doubles | `pkg/interfaces` -> runtime or replay consumers | worker-owned fields drifting back onto `WorkDispatch` re-couples dispatcher and worker boundaries | `pkg/interfaces/work_dispatch.go`, `pkg/interfaces/work_dispatch_test.go` |
| rendered workstation request | `pkg/workers.WorkstationExecutor` | `pkg/workers.AgentExecutor`, `pkg/workers.ScriptExecutor` | workstation executor -> inner worker executors | mutating the incoming dispatch or depending on missing worker fields on `WorkDispatch` blurs ownership | `pkg/workers/workstation_executor.go`, package build verification |
| provider inference request | `pkg/workers.AgentExecutor` | `pkg/workers.ScriptWrapProvider`, replay side effects, provider wrappers | worker executor -> provider boundary | provider paths reading prompt/env/model data from `WorkDispatch` reintroduce the kitchen-sink contract | `pkg/workers/agent.go`, `pkg/workers/inference_provider.go`, package build verification |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Canonical owner note: `pkg/interfaces` owns the dispatch-owned record and its
  clone helpers. Worker execution owns the boundary-specific request types that
  carry resolved prompt, runtime context, and provider state.
