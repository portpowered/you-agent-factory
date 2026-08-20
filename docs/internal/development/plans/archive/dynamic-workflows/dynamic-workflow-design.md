# Plan: Dynamic Workflows

## Context

Dynamic workflows add a JavaScript-authored orchestration layer above the existing factory runtime. The product outcome is that a customer can embed `you-agent-factory` as an MCP server in a project, ask Cursor, Codex, Kiro, Gemini, OpenCode, Claude Code, or another MCP-capable agent to run a dynamic workflow, and get the same workflow behavior that is also available through the website, API, and CLI.

The core idea is deliberately narrow:

- The durable factory/session/work/dispatch model remains canonical.
- A `Factory` is the authored definition of one orchestration. It can use the current Petri-style orchestrator, a JavaScript orchestrator, or a future stream/task orchestrator.
- A `FactorySession` is one running instance of one factory, regardless of orchestrator kind.
- A dynamic workflow is customer shorthand for a JavaScript-orchestrated `Factory`, not a separate runtime product. Its source may be a Claude-compatible workflow file, inline workflow source, a saved project workflow, a saved personal workflow, or a built-in/global workflow.
- The JavaScript runtime coordinates phases, script variables, checkpoints, and child-agent fan-out. The shared execution loop remains responsible for factory sessions, dispatches, worker/provider invocations, event streams, persistence, API transport, CLI commands, logs, metrics, and worker/provider execution.
- MCP is an additional interface point over the same API/service contracts, not a separate runtime.

Anthropic's public dynamic workflow materials describe the parity target as a JavaScript file with special functions for spawning and coordinating subagents, standard JavaScript helpers, model/worktree choices, interruption/resume behavior, token budgets, fan-out-and-synthesize, adversarial verification, generate-and-filter, tournament, and loop-until-done patterns. Their parallel-agent docs also frame dynamic workflows as the option for jobs that outgrow a handful of subagents and need results verified against each other. See sources at the end of this document.

The GitHub issue comments add concrete implementation references that should shape the requirements:

- AgentLoom proves a thin workflow VM can expose `agent`, `parallel`, `pipeline`, `phase`, `log`, and runtime helpers; validate structured outputs with Ajv; persist NDJSON run events; replay runs; stream progress over WebSocket; and show a phase/agent/token UI. Its known gaps are also requirements here: queued agents need visible state, SSE or stream reconnect must reconcile missed messages, dynamic MCP overrides cannot be only boot-time config, and live-provider verification must be separated from offline mock coverage.
- `codex-dynamic-workflows-lab` proves a safe MCP-first MVP shape: read-only by default, explicit artifact root outside the target repo, background job submission, status/result/artifact polling, event JSONL, per-agent prompts/results/command capture, secret-aware artifact hygiene, route profiles, policy hashes, per-agent temp home/Codex home, bounded output capture, and strict policy checks for max agents, concurrency, network, connector, commands, models, reasoning effort, writable roots, and sandbox mode.
- `pi-dynamic-workflows` shows an adversarial review generator where the script is static, receives all runtime input through global `args`, declares `meta.phases`, calls `phase()`, fans out reviewer agents with `parallel()`, validates structured output through schemas, and filters findings by an agreement threshold.
- Bun's `edit-round.workflow.ts` is the most important customer-style fixture: it accepts an array of `{file,count,diagPath}`, uses explicit JSON schemas, runs a per-file fixer through `pipeline()`, follows with an adversarial reviewer, forbids broad tools in prompts, restricts edits to one file, and returns only structured per-file verdicts.

These references turn several nice-to-have ideas into first-class requirements: static reusable scripts, global structured `args`, phase metadata, explicit progress events, schema-validated worker output, artifact hygiene, read-only MVP defaults, bounded local resources, resumable/replayable run storage, and visible queue/reconnect behavior.

## Contract repair implementation status (2026-06-11 UTC)

The contract-repair kernel on branch `dynamic-workflows-contract-repair-kernel-resubmit`
implements the ownership and preview boundaries described in this document:

- **Factory preview semantics:** canonical `POST /factories/preview` with
  `FactoryPreviewRequest` / `FactoryPreviewResult`; `POST /workflow-previews` is
  a deprecated compatibility alias with successor headers. Preview preparation
  lives in `pkg/orchestrators/javascript/preview`; transport mapping in
  `pkg/transports/mapping/factory_preview.go`; UI adapter in `ui/src/api/factory-preview/`.
- **JavaScript orchestrator ownership:** source, validation, policy, preview,
  result, and store behavior under `pkg/orchestrators/javascript/*`. Root
  `pkg/workflow*` packages are documented compatibility shims only.
- **Durable session contract alignment:** `SESSION_COMPLETED.finalStatus` and
  `FactoryEventSessionResultStatus` match durable REST enums; fake service emits
  canonical `FactoryEvent` envelopes with reconnect filtering; durable read
  projections include `budgets`/`usage` and dispatch `providerSessionRefs`;
  `GetResult` honors `mode` and `includeArtifacts`; fixtures include canonical
  `events[]` and an `AWAITING_APPROVAL` scenario.

Batch 002 fake-session skeleton wiring remains the next implementation phase;
real JavaScript execution and durable persistence are still out of scope here.

## Goals

- Customers can run dynamic workflows through every interface point: website, API, CLI, and MCP server.
- Customers can embed the MCP server in Cursor, Codex, Kiro, Gemini, OpenCode, Claude Code, or any compatible MCP host.
- Workflows can fan out many child agent runs, throttle concurrency, wait at barriers, synthesize results, and run adversarial verification.
- Workflow runs are durable, resumable, inspectable, cancelable, pausable, and exportable.
- Workflow behavior is testable with real customer-style workflow files, including Bun-inspired migration/audit shapes.
- The implementation extends the current factory/session/work/dispatch/provider-session model rather than inventing a second orchestration product.
- The MVP defaults to safe local execution: read-only workflow policy, no direct workflow filesystem/shell/network access, bounded concurrency, explicit artifact storage, and agents as the only host-effect boundary unless policy allows more.

## Non-Goals

- Do not replace `factory.json` or the Petri-net-backed factory engine with JavaScript. JavaScript is another orchestrator kind inside the factory model.
- Do not expose raw JavaScript runtime internals as the primary customer data model.
- Do not make MCP the only supported interface.
- Do not require one provider. The first slice may default to existing runner support, but the contract must admit catalog identities such as `codex` and `cursor-acp`.
- Do not hand workflow scripts unlimited host access. File, process, network, model, and tool capabilities must be declared and enforced.
- Do not make live provider credentials mandatory for the core test suite. Live-provider tests are an opt-in verification layer; mock/fake runner coverage must prove the orchestration contract offline.

## Product Behaviors

### Author And Run

- A customer can run an inline workflow from CLI, API, UI, or MCP by providing a prompt plus optional workflow source.
- A customer can save a workflow as a named reusable asset under a project or factory.
- A customer can run a saved workflow with parameters.
- A saved workflow can receive structured `args` at invocation time. Scripts must not require source rewriting or string interpolation for routine inputs.
- A customer can ask an MCP host to generate a workflow plan, preview its intended fan-out/cost/capabilities, and then execute it after approval.
- A customer can import known workflow examples into the repo and execute them against fixture projects.
- A customer can save a successful run's script as a reusable project workflow or personal/global workflow, with project workflow precedence when names collide.

### Orchestrate

- A workflow script can spawn child agent tasks with explicit prompt, model/runner preference, worktree isolation preference, allowed tools, timeout, token budget, and structured output schema.
- A workflow script can declare `meta.name`, `meta.description`, and ordered `meta.phases`; the runtime must validate and project that metadata before execution.
- A workflow script can call `phase(name)` to move the visible run state through a phase timeline.
- A workflow script can run child tasks sequentially, in parallel, in bounded batches, or in loops until a stop condition is met.
- A workflow script can call `pipeline(items, worker, next)` for per-item staged flows such as edit -> review.
- A workflow script can read child results, filter them, retry failed tasks, spawn verifier tasks, and synthesize a final result.
- A workflow script can choose different runners for different child tasks when policy permits.
- A workflow script can emit progress milestones and structured intermediate artifacts without flooding the parent agent's context.
- Worker output must be schema-validated when a schema is supplied. Invalid output should become an explicit failed/null child result according to the primitive contract, not silently pass through as untrusted text.

### Observe And Control

- A customer can list workflow runs, inspect status, inspect child tasks, read logs, view events, pause, resume, cancel, retry failed children, and export results from API, CLI, UI, and MCP.
- A customer can see queued child tasks before they start, not only after a worker process begins.
- A customer can drill from run -> phase -> task/agent -> prompt, recent tool calls/events, result, warnings, usage, and artifacts.
- Workflow progress appears in the existing dashboard event stream alongside factory session activity.
- Workflow children map to provider sessions or factory work records so existing request/detail panels can explain what happened.
- Cost, token, runtime duration, queue depth, child counts, and failure categories are visible.

### Recover

- A workflow run interrupted by a process restart or session disconnect can resume from the last persisted checkpoint.
- Completed child results are not re-run unless the workflow explicitly requests retry or invalidation.
- Cancellation propagates to active child runs and records partial results.
- Stream reconnect must reconcile missed child events from durable run/task/provider-session state rather than assuming a final idle/completed event is enough.
- Timeouts, provider failures, script exceptions, invalid output, permission denial, and resource exhaustion produce actionable failure states.

## Data Model Changes

### Canonical Resource Semantics

Update `docs/architecture/data-model.md`, `api/openapi.yaml`, generated API types, CLI wording, and UI naming so these semantics are consistent:

| Resource | Meaning |
| --- | --- |
| `Factory` | The authored definition of one orchestration. A factory is not inherently Petri-specific. |
| `FactoryOrchestrator` | The strategy a factory uses to orchestrate work: `PETRI`, `JAVASCRIPT`, and future `STREAM` or `TASK_GRAPH` kinds. |
| `FactorySession` | One running instance of one factory. A JavaScript workflow run is a factory session whose factory has a JavaScript orchestrator. |
| `Work` | One customer-visible unit of work owned by a factory session. Petri sessions use work as graph tokens; JavaScript sessions may use work for invocation input/output, generated child work, and final results. |
| `Dispatch` | One concrete execution request issued by the orchestrator to a worker/agent/script/provider boundary. Petri transitions and JavaScript `agent()` calls both produce dispatches. |
| `ProviderSession` | One provider-backed execution session or transcript-bearing interaction linked to a dispatch. |
| `FactoryArtifact` | A session-owned output such as final result, child result, finding list, patch, log, checkpoint, or worktree summary. |
| `FactoryEvent` | The canonical event stream for session lifecycle, dispatches, worker/provider invocations, artifacts, Petri marking changes, and JavaScript phase/checkpoint changes. |

Compatibility vocabulary:

- `DynamicWorkflow` remains acceptable customer shorthand for a factory whose orchestrator is `JAVASCRIPT`.
- `DynamicWorkflowRun` remains acceptable shorthand in docs and UI affordances when the context is explicitly JavaScript workflows, but API/CLI/session concepts should resolve to `FactorySession`.
- `DynamicWorkflowTask` maps to `Dispatch` plus JavaScript task metadata.
- `DynamicWorkflowArtifact` maps to `FactoryArtifact`.
- `DynamicWorkflowCapabilityPolicy` maps to the factory/session effective orchestrator policy.

### `Factory`

Add orchestrator identity to the persisted factory definition:

- `id`: stable id.
- `name`: customer-visible name.
- `description`: optional purpose.
- `orchestrator`: one `FactoryOrchestratorConfig`.
- `resources`: shared runtime resources, including model, provider quota, invocation slot, workflow slot, and runner quota resources.
- `invocationReturn`: existing return policy, applicable to orchestrator kinds that produce final work content.
- `version`, `layout`, `resourceManifest`, `createdAt`, `updatedAt`.

`FactoryOrchestratorConfig`:

- `kind`: `PETRI`, `JAVASCRIPT`, future `STREAM`, or future `TASK_GRAPH`.
- `dialect`: optional sub-kind, such as `CLAUDE_WORKFLOW` for JavaScript.
- `sourceRef`: optional source file reference. Native reusable workflows live under `~/.you-agent-factory/workflows` or package-relative workflow directories; Claude-compatible imports may resolve from `.claude/workflows/<name>.workflow.js` when compatibility lookup is enabled.
- `sourceInline`: optional inline source for ephemeral or generated factories.
- `sourceHash`: exact source hash that was validated or run.
- `entrypoint`: default `main` or runtime default.
- `meta`: validated script/workflow metadata, including name, description, phases, and authoring hints.
- `argsSchema` or `parametersSchema`: JSON Schema for accepted session args.
- `defaultPolicy`: bounded orchestrator capability policy.
- `petri`: Petri-specific graph config, when `kind=PETRI`.
- `javascript`: JavaScript-specific runtime config, when `kind=JAVASCRIPT`.

Petri orchestrator factories continue to carry current work types, states, workstations, workers, guards, resources, topology, and layout. JavaScript orchestrator factories may be minimal: name, orchestrator config, resources/policy, and optional invocation return behavior.

### `FactorySession`

Runtime execution shape:

- `sessionId`: stable session id.
- `factoryId`: persisted, generated, or ephemeral factory id.
- `factoryName`: resolved factory name.
- `orchestratorKind`: `PETRI`, `JAVASCRIPT`, future `STREAM`, or future `TASK_GRAPH`.
- `orchestratorDialect`: optional dialect such as `CLAUDE_WORKFLOW`.
- `orchestratorSourceRef`: optional source file reference for file-backed orchestrators.
- `orchestratorSourceHash`: exact source that ran.
- `projectId` or `projectPath`: owning project boundary.
- `status`: `QUEUED`, `AWAITING_APPROVAL`, `RUNNING`, `PAUSED`, `RESUMING`, `SUCCEEDED`, `FAILED`, `CANCELING`, `CANCELED`, `TIMED_OUT`.
- `orchestratorState`: kind-specific state projection. For Petri this includes marking and enabled transitions. For JavaScript this includes source ref, args digest, phases, checkpoints, and script runtime status.
- `phase`: optional JavaScript script-defined phase label.
- `phases`: ordered phase summaries from JavaScript metadata plus observed counts, usage, and duration.
- `args`: sanitized invocation args when safe to expose.
- `argsDigest`: digest of structured invocation args after redaction.
- `policy`: effective approved capability policy.
- `policyHash`: stable hash of the effective policy for audit and reproducibility.
- `progress`: counts for dispatches queued/running/succeeded/failed/canceled/skipped plus script-defined progress.
- `budgets`: effective max children, max concurrency, max tokens, max duration, max cost if configured.
- `usage`: observed children, tokens, duration, provider calls, tool calls, worktree count.
- `artifactRoot`: internal or diagnostic artifact root metadata. Public surfaces should expose refs/metadata by default, not unrestricted host paths.
- `finalResult`: optional canonical `WorkContentPart[]` plus JSON summary.
- `validity`: output validity state, warnings, and audit completeness.
- `failure`: canonical error code/message/details.
- `createdAt`, `startedAt`, `updatedAt`, `completedAt`.

### `Dispatch`

Extend the existing dispatch model so both orchestrators can explain agent/script/provider work through one shared concept:

- `dispatchId`: stable dispatch id.
- `sessionId`: parent factory session id.
- `orchestratorKind`: owning orchestrator kind.
- `orchestratorDispatchKind`: `TRANSITION`, `AGENT`, `VERIFY`, `SYNTHESIZE`, `CLASSIFY`, `TOOL`, `SCRIPT`, `SYSTEM`.
- `parentTaskId`: optional task that spawned this task.
- `phase`: JavaScript workflow phase when applicable.
- `status`: `QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELED`, `TIMED_OUT`, `SKIPPED`.
- `label`: script-authored task label such as `edit:foo.rs` or `refute 1.2`.
- `workstationName`: Petri workstation name when applicable.
- `transitionId`: Petri transition id when applicable.
- `runnerId`: a selectable Providers catalog identity such as `codex` or `cursor-acp`.
- `model`: optional model/runtime selection.
- `reasoningEffort`: optional effort level such as `minimal`, `low`, `medium`, or `high` when the runner supports it.
- `routeProfile`: optional policy-checked route profile such as `scout`, `reviewer`, `security`, or `synthesizer`.
- `worktree`: isolation metadata: `NONE`, `SHARED_READONLY`, `DEDICATED`, `REUSED`.
- `promptDigest`: safe prompt digest and optional redacted preview.
- `schemaDigest`: digest of requested structured-output schema, if any.
- `inputArtifactIds`: consumed workflow artifacts.
- `outputArtifactIds`: produced workflow artifacts.
- `providerSession`: existing `ProviderSessionMetadata` when the child is agent-backed.
- `workIds`: optional related work ids.
- `attempt`: retry attempt number.
- `usage`: child tokens/duration/tool calls.
- `warnings`: output truncation, schema failure, secret suppression, fallback capture, or reconnect warnings.
- `failure`: canonical error code/message/details.

### `FactoryArtifact`

Artifact shape:

- `artifactId`, `sessionId`, optional `dispatchId`.
- `kind`: `FINAL_RESULT`, `CHILD_RESULT`, `FINDING`, `PATCH`, `LOG`, `DATASET`, `CHECKPOINT`, `WORKTREE_SUMMARY`.
- `content`: one of `WorkContentPart[]`, JSON, text, file ref, or binary ref.
- `visibility`: `PUBLIC_RESULT`, `DIAGNOSTIC`, `INTERNAL_CHECKPOINT`.
- `auditMode`: `FULL`, `METADATA_ONLY`, `NONE`, or `AUTO`.
- `secretFindingCount`: number of secret-like findings suppressed or redacted.
- `captureMetadata`: command, args, stdout/stderr/last-message capture status, truncation status, event parse status, and result source.
- `createdAt`, `sizeBytes`, `contentHash`.

### `FactoryOrchestratorPolicy`

The effective policy must be persisted with each run and included in approval previews.

MVP defaults:

- `mode: READ_ONLY`
- `maxAgents: 16`
- `concurrency: min(4, maxAgents)` unless overridden
- `maxDepth: 1`
- `maxRetries: 0`
- `maxRunDurationMs`
- `maxWorkerDurationMs`
- `maxOutputBytesPerWorker`
- `maxArtifactBytes`
- `maxTokens`
- `allowNetwork: false`
- `allowConnectors: false`
- `allowDangerFullAccess: false`
- `writableRoots: []`
- `allowedCommands`
- `allowedRunners`
- `allowedModels`
- `allowedReasoningEfforts`
- `allowedRouteProfiles`
- `secrets: NONE` or `RUNNER_AUTH_ONLY`
- `outputAuditMode: AUTO`

Validation rules:

- Read-only policy cannot define writable roots or allow workspace-write workers.
- Write-worktree policy requires explicit writable roots and worktree cleanup.
- Network, connectors, and danger-full-access are denied in the MVP unless a later phase explicitly introduces policy-checked host APIs.
- Concurrency must be `1..maxAgents`.
- Max agents must be bounded. Use `16` as the interactive/default local cap and `1000` as the absolute run-planning cap unless a deployment policy lowers it.
- Unknown route profiles, reasoning efforts, commands, runners, models, or sandbox modes are rejected before execution.
- Artifact roots must not be inside the target repository unless the run explicitly opts into tracked artifacts.

### Factory Config Changes

Extend `pkg/interfaces.FactoryConfig` and OpenAPI factory schema with one required orchestrator block:

- `orchestrator FactoryOrchestratorConfig`

Add resource/capability references rather than embedding runner internals:

- `resources[].type: "WORKFLOW_SLOT"` for workflow concurrency capacity.
- `resources[].type: "RUNNER_QUOTA"` for provider/runner budgets if needed.
- Existing `MODEL`, `PROVIDER_QUOTA`, and `INVOCATION_SLOT` remain valid and should be reusable by JavaScript agent dispatches.
- Existing Petri factories should load with `orchestrator.kind: PETRI` through migration/defaulting, so current `factory.json` files do not need immediate user-authored changes.

### Factory Session Projection Changes

Extend session snapshot/read models:

- `runtime.orchestrator.kind`
- `runtime.orchestrator.dialect`
- `runtime.orchestrator.source_ref`
- `runtime.orchestrator.source_hash`
- `runtime.orchestrator.policy_hash`
- `runtime.orchestrator.petri` for Petri marking/topology/transition projection.
- `runtime.orchestrator.javascript` for JavaScript args digest, phases, checkpoints, queued/running/completed agent counts, and script runtime status.
- `runtime.dispatches` for shared dispatch state across Petri and JavaScript.
- `runtime.artifacts` for shared session artifacts.

Do not place raw JavaScript VM state into the general engine snapshot. Persist checkpoints in a workflow store and project only summaries into session/world views.

## System Architecture Changes

### New Or Changed Layers

1. `pkg/orchestrators`
   - Defines the shared interface for Petri, JavaScript, and future coordination subsystems.
   - Owns orchestration decisions only: what dispatches should be created, when phases/checkpoints advance, and when the session is terminal.
   - Does not duplicate session lifecycle, dispatch execution, worker/provider invocation, provider sessions, events, logs, metrics, artifacts, or API/CLI/MCP transport.

2. `pkg/orchestrators/javascript/source`
   - Owns JavaScript orchestrator source loading and resolution: inline source, saved workflow names, Claude-compatible paths, package-relative workflow directories, and artifact-root boundaries.
   - Normalizes resolved source into orchestrator-owned `WorkflowSource` values before validation or session start.
   - Does not own AST validation, policy resolution, preview projection, or runtime execution.

3. `pkg/orchestrators/javascript/validation`
   - Parses JavaScript workflow source into an AST-like validation model.
   - Mirrors the existing config parser/validator style: parse first, produce path/source-location-aware validation diagnostics second, and keep execution out of validation.
   - Validates `meta`, `args` schema, primitive usage shape, supported globals, forbidden direct host access, and policy compatibility before session start.

4. `pkg/orchestrators/javascript/store`
   - Owns durable orchestrator-specific session state such as JavaScript source snapshots, phase summaries, checkpoints, artifacts, and replay indexes.
   - Stores only projected/resumable state; raw VM state is not part of the general session snapshot.

5. `pkg/orchestrators/javascript/policy`
   - Validates declared capabilities and effective session policy.
   - Resolves requested policy into the effective session policy and blocks disallowed host operations.
   - Does not own preview projection or result validation.

6. `pkg/orchestrators/javascript/preview`
   - Owns Factory preview and Factory Session preview preparation for JavaScript orchestrator factories.
   - Projects validation, policy, source, and cost/approval signals into preview responses without starting a session.
   - Does not own public REST transport or standalone workflow-preview product routes.

7. `pkg/orchestrators/javascript/result`
   - Owns JavaScript orchestrator result validation and projection: structured-cloneable final results, worker output schema checks, artifact URI normalization, and result hashes.
   - Does not own dispatch execution or provider-session capture.

8. `pkg/orchestrators/javascript/runtime_subsystems`
   - Implements the shared `pkg/orchestrators` interface for JavaScript/Claude-compatible workflow files.
   - Owns `goja` execution, deterministic host APIs, sandbox policy enforcement hooks, phase state, checkpoint hooks, and resume summaries.
   - Does not directly call providers, subprocesses, or the filesystem except through injected host capabilities.

9. `pkg/orchestrators/petri`
   - Moves the current Petri orchestration into the same orchestrator structure.
   - Suggested subpackages: `runtime_subsystems`, `core_data_models`, `store`, and `validation`.
   - Keeps existing behavior while making Petri one orchestrator kind rather than the implicit definition of a factory.

10. `pkg/factory/subsystems/javascript`
   - Wires JavaScript orchestrator factories into factory/session runtime construction.
   - Inherits the shared subsystem shape used by new factory subsystems.

11. `pkg/factory/subsystems/petri`
   - Wires Petri orchestrator factories into factory/session runtime construction.
   - Owns compatibility/defaulting for existing `factory.json` files that do not yet declare `orchestrator.kind`.

12. `pkg/transports/mcp`
   - Exposes MCP tools/resources/prompts over the same service layer used by CLI/API/UI.
   - Should share normal backend session runtime/server instancing whenever practical.
   - Supports stdio and HTTP/SSE transport if the repo's server architecture supports both cleanly.

13. `examples/workflows`
   - Stores curated sample workflow files and fixture projects.
   - Includes Bun-inspired migration, fan-out audit, adversarial verification, tournament, and loop-until-done examples.

JavaScript orchestrator ownership rule:

- Validation, source loading, storage, policy, result validation, and preview preparation belong under `pkg/orchestrators/javascript` subpackages.
- Root `pkg/workflow*` packages are transitional Batch 001 surfaces and are not the intended final ownership boundary for JavaScript orchestration.

### Core Loop Boundary

The JavaScript orchestrator should swap the coordination subsystem, not fork the runtime product.

Keep shared:

- factory session lifecycle
- event stream
- dispatch records
- worker/provider execution
- provider sessions
- work/content/artifact projections
- pause/resume/cancel controls
- logging and metrics
- CLI/API/MCP service layer

Orchestrator-specific:

- Petri marking and transition enablement for `PETRI`
- JavaScript phases, script variables, host API calls, and checkpoints for `JAVASCRIPT`
- future stream/task-graph coordination state for future orchestrators

### Execution Flow

1. A request enters through CLI, API, UI, or MCP.
2. The request is normalized into `StartFactorySessionRequest` with an `orchestrator` block.
3. The policy layer validates factory source, workflow source when applicable, args, declared capabilities, budgets, and runner availability.
4. If approval is required, the factory session is created as `AWAITING_APPROVAL` with a preview.
5. Once approved, the session runner starts the configured orchestrator. For JavaScript sessions it starts the JS runtime and injects host APIs.
6. The JS workflow calls host APIs such as `agent.run`, `agent.parallel`, `workflow.checkpoint`, `artifact.write`, and `workflow.final`.
7. Each child agent task becomes a shared `Dispatch` and dispatches through the existing runner/provider boundary.
8. Child results are persisted as artifacts and returned to JS as structured values.
9. The JS workflow synthesizes a final result, writes final artifacts, and marks the factory session terminal.
10. Events, logs, metrics, API projections, CLI output, UI projections, and MCP responses all read from the same session/dispatch/artifact state.

### JavaScript Runtime Choice

Use an embedded runtime first, behind a narrow interface:

```go
type WorkflowRuntime interface {
	Run(ctx context.Context, request RuntimeRunRequest, host WorkflowHost) (RuntimeRunResult, error)
	Resume(ctx context.Context, request RuntimeResumeRequest, host WorkflowHost) (RuntimeRunResult, error)
	Validate(source WorkflowSource) (RuntimeValidationResult, error)
}
```

Runtime decision: ship `goja` first.

Rationale:

- `goja` is pure Go, fits the existing backend runtime, and avoids packaging Node/Bun/QuickJS/cgo as part of the first production slice.
- Claude-compatible workflows should use standard JavaScript orchestration primitives, but the MVP does not need arbitrary Node/Bun module compatibility because the workflow script itself should not directly perform filesystem, shell, network, or package-manager work.
- The runner interface should remain process-friendly so a later supervised Node/Bun/QuickJS runtime can replace or complement `goja` if customer workflows require broader JS compatibility.

### Host API Design

Expose a small workflow SDK inside JavaScript:

```ts
type AgentRunOptions = {
  id?: string;
  label?: string;
  phase?: string;
  prompt: string;
  runner?: string;
  model?: string;
  reasoningEffort?: "minimal" | "low" | "medium" | "high";
  profile?: "scout" | "reviewer" | "security" | "synthesizer";
  worktree?: "none" | "dedicated" | "shared_readonly";
  cwd?: string;
  sandbox?: "read-only" | "workspace-write";
  timeoutMs?: number;
  tokenBudget?: number;
  schema?: object;
  outputSchema?: object; // Alias accepted during migration only.
  tools?: string[];
};

export const meta: {
  name: string;
  description?: string;
  phases?: Array<{ title: string; detail?: string }>;
};

declare const args: unknown;

declare const workflow: {
  checkpoint(label?: string, state?: unknown): Promise<void>;
  log(message: string, fields?: Record<string, unknown>): void;
  artifact(kind: string, content: unknown, options?: object): Promise<string>;
  final(result: unknown): Promise<void>;
  sleep(ms: number): Promise<void>;
  budget(): WorkflowBudgetState;
};

declare function phase(name: string): void;
declare function log(message: string, fields?: Record<string, unknown>): void;

declare function agent(prompt: string, options?: AgentRunOptions): Promise<unknown>;
declare function parallel<T>(items: Array<() => Promise<T>>, options?: { concurrency?: number }): Promise<T[]>;
declare function pipeline<T, U, V>(
  items: T[],
  worker: (item: T, index: number) => Promise<U>,
  next?: (result: U, item: T, index: number) => Promise<V>,
): Promise<Array<U | V>>;

declare const agent: {
  run(options: AgentRunOptions): Promise<AgentResult>;
  parallel<T>(items: T[], worker: (item: T, index: number) => Promise<unknown>, options?: { concurrency?: number }): Promise<unknown[]>;
  verify(subject: unknown, rubric: string, options?: AgentRunOptions): Promise<AgentResult>;
};
```

Host API constraints:

- All host calls must be async and cancellation-aware.
- Host calls must require an active session id and policy context.
- Host calls must return structured serializable values only.
- Direct filesystem/process/network access should be disabled by default and reintroduced only through policy-checked APIs.
- Workflow scripts should not receive direct filesystem or shell APIs in the MVP. Agents perform reads, edits, shell, web, and MCP tool calls through their runner permissions.
- The primitive contract should support both ergonomic globals (`agent`, `parallel`, `pipeline`, `phase`, `log`, `args`) and namespaced APIs (`workflow.*`, `agent.run`) so customer examples can be imported with minimal rewrites.
- The runtime must validate that returned workflow results are structured-cloneable and fail with a clear error when a script returns unresolved promises, functions, cyclic values, or host handles.
- Each host call must emit logs, metrics, and workflow events.

## API Data Changes

### OpenAPI Paths

This section should be revisited in the API-specific interface discussion. The data-model direction is already chosen: JavaScript workflows run as factory sessions, so API paths should be session/orchestrator-centered with workflow aliases only where they help customers.

Add or extend:

- `GET /factories`
- `POST /factories`
- `GET /factories/{factory_id}`
- `PUT /factories/{factory_id}`
- `POST /factories/validate`
- `POST /factories/preview`
- `POST /factory-sessions`
- `GET /factory-sessions`
- `GET /factory-sessions/{session_id}`
- `POST /factory-sessions/{session_id}/approve`
- `POST /factory-sessions/{session_id}/pause`
- `POST /factory-sessions/{session_id}/resume`
- `POST /factory-sessions/{session_id}/cancel`
- `POST /factory-sessions/{session_id}/retry-dispatch`
- `GET /factory-sessions/{session_id}/dispatches`
- `GET /factory-sessions/{session_id}/dispatches/{dispatch_id}`
- `GET /factory-sessions/{session_id}/artifacts`
- `GET /factory-sessions/{session_id}/artifacts/{artifact_id}`
- `GET /factory-sessions/{session_id}/world`
- `GET /factory-sessions/{session_id}/events`
- `POST /factory-sessions/{session_id}/invocations`
- Factory import/export endpoints and schemas.

Workflow compatibility aliases may be added after the API interface discussion. Any preview alias must project **Factory preview** or **Factory Session preview** semantics from `pkg/orchestrators/javascript/preview`; transitional Batch 001 `POST /workflow-previews` is **obsolete** and not the target public route.

- `GET /workflows`
- `POST /workflows/validate`
- `POST /workflows/preview` (alias over Factory or Factory Session preview semantics)
- `POST /workflows/{workflow_name}/sessions`

### OpenAPI Schemas

Add schemas:

- `FactoryOrchestrator`
- `FactoryOrchestratorKind`
- `FactoryOrchestratorDialect`
- `FactoryOrchestratorSource`
- `FactoryOrchestratorPolicy`
- `FactoryPreviewRequest`
- `FactoryPreviewResponse`
- `FactorySessionCreateRequest`
- `FactorySession`
- `FactorySessionStatus`
- `FactorySessionOrchestratorState`
- `Dispatch`
- `DispatchStatus`
- `FactoryArtifact`
- `FactoryCheckpointSummary`
- `FactorySessionBudget`
- `FactorySessionUsage`
- `FactorySessionFailure`
- `FactoryEventPayload`

Extend existing schemas:

- `Factory`
  - add `orchestrator`
  - add orchestrator policy/resource config fields
- `FactoryWorldRuntimeView`
  - add orchestrator state, phase summaries, shared dispatch summaries, and artifact summaries
- `InvocationRequest`
  - optionally allow direct orchestrator source/session creation after core session APIs are stable
- `ProviderSessionMetadata`
  - optionally add `sessionId` and `dispatchId` correlation fields

### API Contract Rules

- API and CLI must share input normalization for factory source, workflow source, saved workflow name, args, and budget settings.
- API responses must never require clients to parse logs to determine status.
- Schema fields must distinguish requested policy from effective approved policy.
- Generated backend and frontend types must be updated from OpenAPI and checked in.
- Contract fixtures must cover `1`, `2`, and `N` dispatches; loading, paused, failed, timed out, canceled, and succeeded sessions; Petri and JavaScript orchestrator states; and missing/unsupported runner states.

## MCP Server Contract

Expose MCP as a first-class interface over the same service layer.

### Tools

Primary product tools:

- `you.workflow.list`
- `you.workflow.get`
- `you.workflow.validate`
- `you.workflow.preview`
- `you.workflow.create`
- `you.workflow.run`
- `you.workflow.approve`
- `you.workflow.status`
- `you.workflow.tasks`
- `you.workflow.artifacts`
- `you.workflow.pause`
- `you.workflow.resume`
- `you.workflow.cancel`
- `you.workflow.retry`
- `you.workflow.export`

MVP compatibility aliases should also be considered because the Codex lab implementation proves this small tool set works well for MCP hosts:

- `workflow_validate`: validate a deterministic workflow script without side effects.
- `workflow_submit`: start a bounded local workflow job and return a run id for polling.
- `workflow_status`: read a workflow run summary.
- `workflow_result`: read a completed workflow result.
- `workflow_cancel`: request cancellation.
- `workflow_artifacts`: return artifact metadata and refs.

Alias rules:

- Aliases call the same service layer as `you.workflow.*`.
- `workflow_submit` requires script, cwd/project path, artifact root or managed artifact mode, and policy.
- Artifact roots supplied by MCP clients must be absolute, policy-checked, and outside the target repository by default.
- `workflow_result` must return a not-ready error for non-terminal runs rather than blocking indefinitely.
- Cancel must be real for managed service runs. If a stdio one-shot fallback cannot cancel a detached completed process, it must report that limitation explicitly and remain an MVP-only fallback.

### Resources

- `you://dynamic-workflows/{workflow_id}`
- `you://dynamic-workflow-runs/{run_id}`
- `you://dynamic-workflow-runs/{run_id}/tasks`
- `you://dynamic-workflow-runs/{run_id}/artifacts/{artifact_id}`
- `you://factory-sessions/{session_id}/dynamic-workflow-runs`

### Prompts

- `create-dynamic-workflow`
- `review-dynamic-workflow`
- `run-adversarial-verification-workflow`
- `run-fanout-audit-workflow`
- `run-migration-workflow`

### Host Compatibility

Acceptance must explicitly test MCP configuration examples for:

- Cursor
- Codex
- Kiro
- Gemini
- OpenCode
- Claude Code

Each host smoke test should prove:

- Host can start the MCP server.
- Host can discover tools.
- Host can preview a workflow.
- Host can start a dry-run or fake-run workflow.
- Host can poll status and retrieve final artifacts.
- Host receives structured errors for unsupported capabilities.
- Host receives explicit not-ready responses for pending results.
- Host can pass structured `args` without rewriting workflow source.

## Event Stream Changes

Add canonical `FactoryEvent` payloads:

- `DYNAMIC_WORKFLOW_RUN_CREATED`
- `DYNAMIC_WORKFLOW_RUN_AWAITING_APPROVAL`
- `DYNAMIC_WORKFLOW_RUN_STARTED`
- `DYNAMIC_WORKFLOW_RUN_PHASE_CHANGED`
- `DYNAMIC_WORKFLOW_RUN_PAUSED`
- `DYNAMIC_WORKFLOW_RUN_RESUMED`
- `DYNAMIC_WORKFLOW_RUN_CANCEL_REQUESTED`
- `DYNAMIC_WORKFLOW_RUN_COMPLETED`
- `DYNAMIC_WORKFLOW_RUN_FAILED`
- `DYNAMIC_WORKFLOW_TASK_CREATED`
- `DYNAMIC_WORKFLOW_TASK_STARTED`
- `DYNAMIC_WORKFLOW_TASK_COMPLETED`
- `DYNAMIC_WORKFLOW_TASK_FAILED`
- `DYNAMIC_WORKFLOW_ARTIFACT_CREATED`
- `DYNAMIC_WORKFLOW_CHECKPOINT_WRITTEN`
- `DYNAMIC_WORKFLOW_BUDGET_EXHAUSTED`

Event rules:

- Every event includes `run_id`, optional `workflow_id`, optional `task_id`, `factory_session_id`, `trace_id`, `request_id`, and canonical timestamp.
- Events are projections from workflow run/task state and should be replay-safe.
- Events must not include raw prompts or secrets. Use digests/redacted previews and artifact refs.
- The dashboard must be able to reconstruct active runs from event replay plus current snapshot.

## Logging And Metrics

### Logs

Structured logs must include:

- `workflow_run_id`
- `workflow_id`
- `workflow_task_id`
- `factory_session_id`
- `request_id`
- `trace_id`
- `runner_id`
- `phase`
- `status`
- `duration_ms`
- `attempt`
- `failure_code`

Required log points:

- run validation
- preview creation
- approval
- run start
- runtime start/resume
- host API call start/finish
- child task enqueue/start/finish
- checkpoint write
- budget decision
- pause/resume/cancel
- final result write
- failure classification

### Metrics

Counters:

- `dynamic_workflow.run.started`
- `dynamic_workflow.run.completed`
- `dynamic_workflow.run.failed`
- `dynamic_workflow.task.started`
- `dynamic_workflow.task.completed`
- `dynamic_workflow.task.failed`
- `dynamic_workflow.checkpoint.written`
- `dynamic_workflow.artifact.created`
- `dynamic_workflow.budget.exhausted`
- `dynamic_workflow.mcp.tool.called`

Histograms:

- `dynamic_workflow.run.duration_ms`
- `dynamic_workflow.task.duration_ms`
- `dynamic_workflow.host_call.duration_ms`
- `dynamic_workflow.queue.wait_ms`
- `dynamic_workflow.resume.duration_ms`

Gauges:

- `dynamic_workflow.runs.active`
- `dynamic_workflow.tasks.active`
- `dynamic_workflow.tasks.queued`
- `dynamic_workflow.concurrency.used`
- `dynamic_workflow.tokens.used`
- `dynamic_workflow.worktrees.active`

## Function-Level Breakdown

The exact package names may shift during implementation, but these function boundaries should remain stable.

### Interfaces And Domain Types

`pkg/interfaces/factory_orchestrator.go`

- `type FactoryOrchestratorConfig struct`
- `type FactoryOrchestratorKind string`
- `type FactoryOrchestratorDialect string`
- `type FactoryOrchestratorPolicyConfig struct`
- `type FactorySessionOrchestratorState struct`
- `type JavaScriptOrchestratorConfig struct`
- `type JavaScriptOrchestratorState struct`
- `type PetriOrchestratorConfig struct`
- `type PetriOrchestratorState struct`
- `type FactoryArtifact struct`
- `type FactorySessionBudget struct`
- `type FactorySessionUsage struct`
- `type FactorySessionFailure struct`
- `type DispatchStatus string`
- `func NormalizeFactoryOrchestratorKind(value string) FactoryOrchestratorKind`
- `func NormalizeFactorySessionStatus(value string) RuntimeStatus`
- `func FactorySessionTerminal(status RuntimeStatus) bool`
- `func DispatchTerminal(status DispatchStatus) bool`
- `func CloneFactorySessionOrchestratorState(state FactorySessionOrchestratorState) FactorySessionOrchestratorState`
- `func CloneFactoryArtifact(artifact FactoryArtifact) FactoryArtifact`
- `func RedactOrchestratorSource(source string) string`
- `func OrchestratorSourceHash(source string) string`

`pkg/interfaces/factory_config.go`

- Add `Orchestrator FactoryOrchestratorConfig`.
- Default existing configs to `Orchestrator.Kind=PETRI` during load/migration.
- Add resource constants `ResourceTypeWorkflowSlot` and `ResourceTypeRunnerQuota`.

`pkg/interfaces/factory_events.go`

- Add orchestrator event payload structs and event type constants.
- Add `func FactorySessionOrchestratorEventPayload(...)`.
- Add `func JavaScriptPhaseEventPayload(...)`.
- Add `func DispatchEventPayload(...)`.
- Add `func FactoryArtifactEventPayload(...)`.

### Source Loading

`pkg/orchestrators/javascript/source/resolve.go`

- `func ResolveWorkflowSource(ctx context.Context, request SourceRequest) (WorkflowSource, error)`
- `func LookupSavedWorkflow(name string, lookup LookupContext) (WorkflowSource, error)`
- `func ResolveClaudeCompatiblePath(projectPath string, name string) (WorkflowSource, error)`
- `func ResolveArtifactRoot(request SourceRequest, policy EffectivePolicy) (ArtifactRoot, error)`
- `func NormalizeInlineSource(source string) (WorkflowSource, error)`

Acceptance:

- Source resolution order matches CLI/API/MCP normalization rules.
- Resolved source includes stable refs, hashes, and lookup provenance before validation.
- Artifact roots are policy-checked and outside the target repository by default.

### Validation

`pkg/orchestrators/javascript/validation/validate.go`

- `func ParseWorkflowSource(source WorkflowSource) (WorkflowAST, []ValidationIssue)`
- `func ValidateWorkflowAST(ast WorkflowAST, policy interfaces.FactoryOrchestratorPolicyConfig) []ValidationIssue`
- `func ValidateWorkflowMeta(ast WorkflowAST) []ValidationIssue`
- `func ValidateWorkflowArgsSchema(ast WorkflowAST) []ValidationIssue`
- `func ValidateWorkflowArgs(schema json.RawMessage, args json.RawMessage) []ValidationIssue`
- `func ValidateJavaScriptOrchestratorConfig(config interfaces.FactoryOrchestratorConfig) []ValidationIssue`
- `func ValidateJavaScriptResourceReferences(factory interfaces.FactoryConfig) []ValidationIssue`
- `func ValidateJavaScriptRunnerReferences(factory interfaces.FactoryConfig) []ValidationIssue`

`pkg/orchestrators/petri/validation/validate.go`

- `func ValidatePetriOrchestratorConfig(config interfaces.FactoryOrchestratorConfig) []ValidationIssue`
- `func ValidatePetriCoreDataModels(factory interfaces.FactoryConfig) []ValidationIssue`
- `func ValidatePetriResourceReferences(factory interfaces.FactoryConfig) []ValidationIssue`

`pkg/factoryvalidation/factory_orchestrator.go`

- `func ValidateFactoryOrchestratorConfig(config interfaces.FactoryOrchestratorConfig) []ValidationIssue`
- `func ValidateFactoryOrchestratorPolicy(policy interfaces.FactoryOrchestratorPolicyConfig) []ValidationIssue`
- `func ValidateFactoryOrchestrator(factory interfaces.FactoryConfig) []ValidationIssue`

Acceptance:

- Invalid JavaScript source is rejected before session creation.
- JavaScript validation follows a parser/AST-validation shape similar to the existing config parser/validator.
- JavaScript validation reports source-location-aware diagnostics for invalid `meta`, unsupported globals, unsupported primitive calls, invalid schemas, and forbidden direct host access.
- Missing runner/resource references produce path-aware validation issues.
- Policy values reject negative budgets, zero concurrency, excessive child limits, unsupported host capabilities, and unknown runner ids.

### Store

`pkg/orchestrators/javascript/store/store.go`

- `type Store interface`
- `func NewFileStore(root string, clock interfaces.Clock) (*FileStore, error)`
- `func (s *FileStore) CreateSession(ctx context.Context, session interfaces.FactorySessionRecord) error`
- `func (s *FileStore) UpdateSession(ctx context.Context, session interfaces.FactorySessionRecord, expectedVersion int64) error`
- `func (s *FileStore) AppendDispatch(ctx context.Context, dispatch interfaces.DispatchRecord) error`
- `func (s *FileStore) UpdateDispatch(ctx context.Context, dispatch interfaces.DispatchRecord, expectedVersion int64) error`
- `func (s *FileStore) WriteArtifact(ctx context.Context, artifact interfaces.FactoryArtifact) error`
- `func (s *FileStore) WriteCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error`
- `func (s *FileStore) LatestCheckpoint(ctx context.Context, sessionID string) (WorkflowCheckpoint, error)`

`pkg/orchestrators/petri/store/store.go`

- Move or adapt current Petri runtime/session persistence behind the same shared session/store expectations where possible.
- Keep Petri-specific marking/topology persistence in Petri-owned store code.

Acceptance:

- Store operations are atomic enough for resume and concurrent task completion.
- Store test fixtures cover fresh session, resumed session, concurrent dispatch updates, artifact retrieval, and corrupted checkpoint handling.

### Policy

`pkg/orchestrators/javascript/policy/policy.go`

- `func ResolveEffectivePolicy(request RequestedPolicy, factory interfaces.FactoryConfig, user PolicySubject) (EffectivePolicy, error)`
- `func ValidateCapabilityRequest(policy EffectivePolicy, request HostCapabilityRequest) error`
- `func StablePolicyHash(policy EffectivePolicy) string`
- `func ResolveRouteProfile(options AgentRunOptions, policy EffectivePolicy) (AgentRunOptions, error)`
- `func ResolveOutputAuditMode(policy EffectivePolicy) OutputAuditMode`
- `func RedactPolicyDiagnostics(policy EffectivePolicy) map[string]string`

### Preview Preparation

`pkg/orchestrators/javascript/preview/build.go`

- `func BuildFactoryPreview(source WorkflowSource, params json.RawMessage, policy EffectivePolicy) (FactoryPreview, error)`
- `func BuildFactorySessionPreview(source WorkflowSource, params json.RawMessage, policy EffectivePolicy) (FactorySessionPreview, error)`
- `func EstimateFactorySessionCost(preview FactorySessionPreview, policy EffectivePolicy) FactorySessionCostEstimate`
- `func RequireApproval(preview FactorySessionPreview, policy EffectivePolicy) bool`
- `func ProjectPreviewConstraints(preview FactorySessionPreview) PreviewConstraints`

Acceptance:

- Preview preparation reports max children, max concurrency, runners, worktree behavior, token budget, timeout, and requested host capabilities.
- Preview responses use Factory and Factory Session preview semantics rather than a standalone workflow-preview product model.

### Result Validation

`pkg/orchestrators/javascript/result/validate.go`

- `func ValidateStructuredCloneable(value any) error`
- `func ValidateWorkerOutput(schema json.RawMessage, value any) (any, error)`
- `func ProjectFinalResult(content []interfaces.WorkContentPart) (FactoryResultProjection, error)`
- `func NormalizeArtifactURI(uri string) (string, error)`
- `func ResultHash(content any) string`

Acceptance:

- Invalid worker output and non-cloneable final results become explicit failures with path-aware diagnostics.
- Result projection preserves canonical `Work`, `FactoryArtifact`, and `FactoryEvent` semantics.

`pkg/orchestrators/javascript/store/artifact_hygiene.go`

- `func NewArtifactHygiene(policy EffectivePolicy) ArtifactHygiene`
- `func SanitizeText(surface string, value string, mode OutputAuditMode) SanitizedText`
- `func SanitizeValue(surface string, value any, mode OutputAuditMode) SanitizedValue`
- `func AssertExportableSafe(value any) error`
- `func ScanSecretLikeText(surface string, text string) []SecretFinding`
- `func TruncateUTF8(value string, maxBytes int) (string, bool)`
- `func AssertInsideArtifactRoot(path string, root string) error`
- `func RejectSymlinkArtifact(path string) error`

Acceptance:

- Disallowed filesystem/network/process access fails before runtime execution.
- Policy hashes are stable across map ordering.
- Secret-like prompt, result, stdout, stderr, event, and artifact content is suppressed or redacted according to output audit mode.
- Artifact path validation rejects path escape and symlink artifacts.

### JavaScript Runtime

`pkg/orchestrators/javascript/runtime_subsystems/runtime.go`

- `type Runtime interface`
- `type Host interface`
- `func NewRuntime(config RuntimeConfig) Runtime`
- `func ValidateSource(ctx context.Context, source WorkflowSource) (ValidationResult, error)`
- `func Run(ctx context.Context, request RuntimeRunRequest, host Host) (RuntimeRunResult, error)`
- `func Resume(ctx context.Context, request RuntimeResumeRequest, host Host) (RuntimeRunResult, error)`

`pkg/orchestrators/javascript/runtime_subsystems/host_api.go`

- `func RegisterWorkflowAPI(vm RuntimeVM, host Host) error`
- `func RegisterAgentAPI(vm RuntimeVM, host Host) error`
- `func RegisterPrimitiveGlobals(vm RuntimeVM, host Host) error`
- `func ExtractWorkflowMeta(source WorkflowSource) (WorkflowMeta, error)`
- `func ValidateWorkflowMeta(meta WorkflowMeta) []ValidationIssue`
- `func ValidateAgentOptions(options AgentRunOptions, policy EffectivePolicy) error`
- `func ValidateStructuredCloneable(value RuntimeValue) error`
- `func ValidateOutputSchema(schema json.RawMessage) error`
- `func ValidateWorkerOutput(schema json.RawMessage, value any) (any, error)`
- `func RegisterArtifactAPI(vm RuntimeVM, host Host) error`
- `func MarshalHostValue(value any) (RuntimeValue, error)`
- `func UnmarshalRuntimeValue(value RuntimeValue, target any) error`
- `func ClassifyScriptError(err error) interfaces.FactorySessionFailure`
- `func RemapSourceLocation(err error, sourceMap SourceMap) interfaces.FactorySessionFailure`

Acceptance:

- Script validation catches syntax errors.
- Runtime execution propagates context cancellation.
- Host API calls fail closed when policy denies them.
- Script exceptions mark the run failed with source location where available.
- Imported workflows using `meta`, `args`, `phase`, `agent`, `parallel`, and `pipeline` execute without source rewriting.
- Schema validation can intentionally return a null/failed child result where the primitive contract requests that behavior.
- Source-map or generated-wrapper line numbers are remapped in failures before they reach CLI/API/UI/MCP.

### Orchestrator

`pkg/orchestrators/service.go`

- `func NewService(deps Dependencies) *Service`
- `func (s *Service) CreateFactory(ctx context.Context, request CreateFactoryRequest) (FactoryResponse, error)`
- `func (s *Service) ValidateFactory(ctx context.Context, request ValidateFactoryRequest) (ValidateFactoryResponse, error)`
- `func (s *Service) PreviewFactorySession(ctx context.Context, request PreviewFactorySessionRequest) (PreviewFactorySessionResponse, error)`
- `func (s *Service) StartSession(ctx context.Context, request StartFactorySessionRequest) (FactorySessionResponse, error)`
- `func (s *Service) ApproveSession(ctx context.Context, sessionID string, approval Approval) (FactorySessionResponse, error)`
- `func (s *Service) PauseSession(ctx context.Context, sessionID string) (FactorySessionResponse, error)`
- `func (s *Service) ResumeSession(ctx context.Context, sessionID string) (FactorySessionResponse, error)`
- `func (s *Service) CancelSession(ctx context.Context, sessionID string) (FactorySessionResponse, error)`
- `func (s *Service) RetryDispatch(ctx context.Context, sessionID string, dispatchID string) (DispatchResponse, error)`
- `func (s *Service) GetSession(ctx context.Context, sessionID string) (FactorySessionResponse, error)`
- `func (s *Service) ListSessions(ctx context.Context, filter SessionFilter) ([]FactorySessionResponse, error)`

`pkg/orchestrators/javascript/runtime_subsystems/host.go`

- `func (h *Host) RunAgent(ctx context.Context, request AgentRunRequest) (AgentRunResult, error)`
- `func (h *Host) RunParallel(ctx context.Context, request ParallelRequest) (ParallelResult, error)`
- `func (h *Host) Verify(ctx context.Context, request VerifyRequest) (AgentRunResult, error)`
- `func (h *Host) WriteArtifact(ctx context.Context, request ArtifactWriteRequest) (ArtifactWriteResult, error)`
- `func (h *Host) Checkpoint(ctx context.Context, request CheckpointRequest) error`
- `func (h *Host) EmitLog(ctx context.Context, request WorkflowLogRequest) error`
- `func (h *Host) Budget(ctx context.Context) WorkflowBudgetState`

`pkg/orchestrators/scheduler.go`

- `func NewDispatchScheduler(policy EffectivePolicy, store Store, executor DispatchExecutor) *DispatchScheduler`
- `func (s *DispatchScheduler) Enqueue(ctx context.Context, dispatch interfaces.DispatchRecord, request AgentRunRequest) error`
- `func (s *DispatchScheduler) Wait(ctx context.Context, dispatchIDs []string) ([]AgentRunResult, error)`
- `func (s *DispatchScheduler) CancelSession(ctx context.Context, sessionID string) error`
- `func (s *DispatchScheduler) PauseSession(ctx context.Context, sessionID string) error`
- `func (s *DispatchScheduler) ResumeSession(ctx context.Context, sessionID string) error`
- `func (s *DispatchScheduler) enforceConcurrency(ctx context.Context) error`
- `func (s *DispatchScheduler) enforceBudgets(ctx context.Context) error`

`pkg/orchestrators/replay/replay.go`

- `func AppendSessionEvent(ctx context.Context, store Store, event FactoryEvent) error`
- `func ReplaySession(ctx context.Context, store Store, sessionID string, sink EventSink) error`
- `func ReconstructSessionSummary(events []FactoryEvent) (interfaces.FactorySessionRecord, error)`
- `func ReconcileAfterStreamDrop(ctx context.Context, sessionID string, lastEventID string) (SessionReconcileResult, error)`
- `func ReconcileProviderSessionMessages(ctx context.Context, dispatch interfaces.DispatchRecord) (ProviderSessionReconcileResult, error)`

Acceptance:

- Scheduler proves `1`, `2`, and `N` children.
- Concurrency limit is honored under parallel host calls.
- Cancellation and timeout propagate to child agent execution.
- Budget exhaustion prevents new children and marks the session failed or paused according to policy.
- Session event replay reconstructs phase, queue, dispatch, usage, and terminal result summaries.
- Stream reconnect recovers missed dispatch/provider events from durable state.

### JavaScript Dispatch Bridge

`pkg/orchestrators/javascript/runtime_subsystems/dispatch_bridge.go`

- `func NewBridge(provider workers.Provider, sessionHost FactorySessionHost, worktree WorktreeManager) *Bridge`
- `func (b *Bridge) ExecuteAgentDispatch(ctx context.Context, dispatch interfaces.DispatchRecord, request AgentRunRequest) (AgentRunResult, error)`
- `func (b *Bridge) BuildProviderInferenceRequest(dispatch interfaces.DispatchRecord, request AgentRunRequest) (interfaces.ProviderInferenceRequest, error)`
- `func (b *Bridge) ResolveRunner(request AgentRunRequest, policy EffectivePolicy) (interfaces.ResolvedRunnerSelection, error)`
- `func (b *Bridge) PrepareWorktree(ctx context.Context, request AgentRunRequest) (WorktreeLease, error)`
- `func (b *Bridge) ReleaseWorktree(ctx context.Context, lease WorktreeLease, outcome TaskOutcome) error`
- `func (b *Bridge) LinkProviderSession(dispatch interfaces.DispatchRecord, response interfaces.InferenceResponse) *interfaces.ProviderSessionMetadata`

Acceptance:

- Runner ids normalize through existing `interfaces.NormalizeRunnerID`.
- Provider identities are accepted or rejected based on catalog availability and policy with consistent errors.
- Worktree isolation is deterministic and cleaned up.

### API Handlers And Surface Mapping

`pkg/transports/mapping/factory_orchestrator.go`

- `func FactoryOrchestratorFromGenerated(value factoryapi.FactoryOrchestrator) (interfaces.FactoryOrchestratorConfig, error)`
- `func GeneratedFactoryOrchestrator(value interfaces.FactoryOrchestratorConfig) factoryapi.FactoryOrchestrator`
- `func StartFactorySessionFromGenerated(value factoryapi.FactorySessionCreateRequest) (orchestrator.StartSessionRequest, error)`
- `func GeneratedFactorySession(value interfaces.FactorySessionRecord) factoryapi.FactorySession`
- `func GeneratedDispatch(value interfaces.DispatchRecord) factoryapi.Dispatch`
- `func GeneratedFactoryArtifact(value interfaces.FactoryArtifact) factoryapi.FactoryArtifact`
- `func FactoryOrchestratorErrorResponse(err error) factoryapi.ErrorResponse`

`pkg/transports/http/factory_session_handlers.go`

- `func (s *Server) ListFactories(...)`
- `func (s *Server) CreateFactory(...)`
- `func (s *Server) ValidateFactory(...)`
- `func (s *Server) PreviewFactorySession(...)`
- `func (s *Server) StartFactorySession(...)`
- `func (s *Server) GetFactorySession(...)`
- `func (s *Server) PauseFactorySession(...)`
- `func (s *Server) ResumeFactorySession(...)`
- `func (s *Server) CancelFactorySession(...)`
- `func (s *Server) ListSessionDispatches(...)`
- `func (s *Server) GetSessionArtifact(...)`

Acceptance:

- Transport handlers only translate request/response/errors.
- Service layer owns behavior and is shared with CLI/MCP.

### CLI

#### CLI Modeling Decision

The CLI should expose the base product concepts directly: factories, sessions, work, dispatches, events, and artifacts. It should not add a parallel `you workflows ...` runtime command tree for the first implementation.

Decision:

- `Factory` is the authored orchestration definition.
- `FactorySession` is the running instance.
- `Factory.orchestrator.kind` determines whether the session runs the current Petri orchestrator or the new JavaScript orchestrator.
- CLI users start and inspect JavaScript workflows through session/factory commands.
- MCP may expose workflow-shaped tools because MCP hosts benefit from task-shaped tool names, but the human CLI should keep the canonical nouns.

Rationale:

- The user can learn one operational model: create/list/show sessions, then inspect work, dispatches, events, and artifacts by session id.
- JavaScript workflows reuse the same CLI lifecycle as Petri factories.
- Avoiding workflow aliases reduces command surface area and prevents drift between "workflow runs" and "factory sessions."
- Claude-compatible workflow file discovery can still exist as a source resolver behind `you session create --workflow`.

CLI resolution rules for `--workflow`:

- A workflow name resolves in this order:
  1. `./.claude/workflows/<name>` when explicitly enabling Claude-compatible project lookup.
  2. `~/.you-agent-factory/workflows/<name>`.
  3. Package-relative workflow directories for installed/global workflow packages.
  4. Built-in/global named JavaScript factories.
  5. Existing named factories when explicitly requested with `--factory` or when no workflow match exists.
- Project workflows win over personal/global/package workflows when names collide.
- A workflow file run materializes an implicit factory with `orchestrator.kind=JAVASCRIPT`, `orchestrator.dialect=CLAUDE_WORKFLOW`, and `orchestrator.sourceRef=<file>`.
- `you session show --session <id>` must expose the internal factory reference, orchestrator kind/dialect/source, source hash, args digest, phases, policy hash, dispatch counts, work counts, artifact counts, and final result summary.
- `you dispatches list --session <id>` is the shared child-agent/task listing for both Petri and JavaScript sessions.
- `you work list --session <id>` stays available for both orchestrators. JavaScript sessions may have sparse work if the workflow only uses dispatches/artifacts, but invocation input/output and final result should still be representable as work content where useful.

`pkg/transports/cli/session/session.go`

- `func NewCommand() *cobra.Command`
- `func newCreateCommand() *cobra.Command`
- `func newListCommand() *cobra.Command`
- `func newShowCommand() *cobra.Command`
- `func newPhasesCommand() *cobra.Command`
- `func newReplayCommand() *cobra.Command`
- `func newPauseCommand() *cobra.Command`
- `func newResumeCommand() *cobra.Command`
- `func newCancelCommand() *cobra.Command`
- `func sessionCreateRequestFromWorkflowFlags(...) (factoryapi.FactorySessionCreateRequest, error)`
- `func sessionCreateRequestFromFactoryFlags(...) (factoryapi.FactorySessionCreateRequest, error)`
- `func renderFactorySessionSummary(...) error`
- `func renderFactorySessionPhases(...) error`
- `func renderSessionReplayEvent(...) error`

`pkg/transports/cli/dispatches/dispatches.go`

- `func NewCommand() *cobra.Command`
- `func newListCommand() *cobra.Command`
- `func renderDispatchTable(...) error`

`pkg/transports/cli/artifacts/artifacts.go`

- `func NewCommand() *cobra.Command`
- `func newListCommand() *cobra.Command`
- `func newGetCommand() *cobra.Command`
- `func renderFactoryArtifact(...) error`

Commands:

- `you session create --workflow <file|name> --args <json|path>`
- `you session create --factory <factory|factory.json>`
- `you session list`
- `you session show --session <id>`
- `you session phases --session <id>`
- `you work list --session <id>`
- `you dispatches list --session <id>`
- `you events list --session <id>`
- `you artifacts list --session <id>`
- `you artifacts get --session <id> --artifact <artifact-id>`
- `you mcp serve`

Acceptance:

- CLI can run inline, file-backed, saved personal/global/package JavaScript workflows, and Petri factories through the same session runtime.
- CLI output is useful in plain text and JSON modes.
- CLI surfaces loading, approval, pause, failure, timeout, and final result states.
- CLI does not expose a separate workflow runtime model. Workflow file/name resolution is part of session creation.

### MCP Server

`pkg/transports/mcp/server.go`

- `func NewServer(service orchestrators.Service, options ServerOptions) *Server`
- `func (s *Server) RegisterWorkflowTools() error`
- `func (s *Server) RegisterWorkflowResources() error`
- `func (s *Server) RegisterWorkflowPrompts() error`
- `func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error`
- `func (s *Server) ServeHTTP(ctx context.Context, listener net.Listener) error`

`pkg/transports/mcp/workflow_tools.go`

- `func workflowListTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowValidateTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func factorySessionPreviewTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowRunTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowStatusTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowTasksTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowArtifactsTool(ctx context.Context, args ToolArgs) (ToolResult, error)`
- `func workflowCancelTool(ctx context.Context, args ToolArgs) (ToolResult, error)`

Acceptance:

- MCP tools use identical service requests to API/CLI.
- Tool schemas include descriptions that help host agents choose preview before run.
- MCP resource reads are bounded and redact unsafe prompt/source details by default.

### UI

`ui/src/api/dynamic-workflows.ts`

- `listDynamicWorkflows`
- `getDynamicWorkflow`
- `validateDynamicWorkflow`
- `previewFactorySession`
- `startDynamicWorkflowRun`
- `getDynamicWorkflowRun`
- `listDynamicWorkflowTasks`
- `listDynamicWorkflowArtifacts`
- `pauseDynamicWorkflowRun`
- `resumeDynamicWorkflowRun`
- `cancelDynamicWorkflowRun`

`ui/src/features/dynamic-workflows/hooks/`

- `useDynamicWorkflows`
- `useDynamicWorkflowRun`
- `useDynamicWorkflowTasks`
- `useDynamicWorkflowArtifacts`
- `useStartDynamicWorkflowRunMutation`
- `useWorkflowRunControlMutation`

`ui/src/features/dynamic-workflows/lib/`

- `projectWorkflowRunSummary`
- `projectWorkflowTaskRows`
- `projectWorkflowTimeline`
- `projectWorkflowBudgetState`
- `workflowRunStatusTone`
- `workflowTaskStatusTone`
- `buildWorkflowRunRequest`
- `validateWorkflowRunDraft`

`ui/src/features/dynamic-workflows/components/`

- `DynamicWorkflowRunList`
- `DynamicWorkflowRunDetail`
- `DynamicWorkflowRunToolbar`
- `FactorySessionPreviewDialog`
- `DynamicWorkflowApprovalPanel`
- `DynamicWorkflowTaskTable`
- `DynamicWorkflowTaskDetail`
- `DynamicWorkflowArtifactList`
- `DynamicWorkflowArtifactViewer`
- `DynamicWorkflowBudgetMeter`
- `DynamicWorkflowEventTimeline`

UI state ownership:

- Canonical server state: generated API models for workflow definitions, runs, tasks, and artifacts through React Query.
- Editable state: workflow source, parameters, policy overrides, approval decision drafts.
- Projection state: table rows, timeline points, status tones, artifact previews.
- Component state: selected task id, expanded sections, active tab, dialog open state.

Acceptance:

- Loading, empty, error, success, awaiting approval, paused, running, failed, canceled, timed out, and completed states render explicitly.
- UI uses shared primitives such as `Button`, `DashboardActionButton`, `DashboardStatusPill`, `Dialog`, tables, selects, inputs, and expandable panels.
- Browser verification covers desktop and mobile for run detail, task table, approval, and artifact viewing.

## Testing Plan

### Unit Tests

- Source hashing/redaction.
- Status normalization and terminal-state helpers.
- Factory config validation.
- Policy resolution and denied capabilities.
- Policy hash stability across map ordering.
- Route profile and reasoning-effort validation.
- Artifact hygiene: secret-like content suppression, metadata-only mode, truncation, path escape rejection, symlink rejection.
- API generated/handwritten mapping.
- JS host value marshalling.
- Workflow metadata extraction and validation.
- Structured-cloneable workflow return validation.
- JSON Schema/Ajv-equivalent worker output validation.
- Source-map or wrapper line remapping for script errors.
- Budget arithmetic.
- UI projection helpers and status tone mapping.

### Package Integration Tests

- Store create/update/list/resume checkpoint behavior.
- NDJSON/event-log append and replay behavior.
- Runtime host API with fake JS runtime or fake host.
- Primitive globals with imported customer scripts using `meta`, `args`, `phase`, `agent`, `parallel`, and `pipeline`.
- Orchestrator with fake JavaScript runtime subsystem and fake dispatch bridge.
- Scheduler bounded concurrency and cancellation.
- Scheduler queued-state projection before workers start.
- Runner bridge for selectable Providers catalog identities.
- Runner bridge output capture for command metadata, stdout/stderr, last message, provider events, warnings, and usage.
- API handlers against fake orchestrator service.
- MCP tools against fake orchestrator service.
- Stream reconnect reconciliation from durable run/task/provider state.

### Functional Tests

Required workflow fixtures:

- `single-agent.workflow.js`: one child agent returns final result.
- `two-agent-verify.workflow.js`: one generator and one verifier.
- `fanout-synthesize.workflow.js`: N items, bounded concurrency, barrier, synthesis.
- `adversarial-audit.workflow.js`: finding agents plus verifier agents.
- `adversarial-review-static.workflow.js`: imported from or adapted from `pi-dynamic-workflows`, using static source, global `args`, phase transitions, reviewer count, agreement threshold, and skeptical reviewers.
- `tournament.workflow.js`: N candidates, pairwise judging, winner.
- `loop-until-done.workflow.js`: repeats until no new findings.
- `bun-edit-round.workflow.ts`: adapted from Bun's `scripts/clippy-loop/edit-round.workflow.ts`, preserving the shape of `args`, `RESULT_SCHEMA`, `REVIEW_SCHEMA`, per-file `pipeline()`, edit phase, adversarial review phase, and structured verdict output while running against a tiny fixture project.
- `bun-migration-map.workflow.js`: Bun-inspired fixture that maps many source files to target migration findings without requiring the real Bun repo.
- `bun-lifetime-review.workflow.js`: Bun-inspired fixture that assigns per-file/per-struct analysis tasks and verifies results.
- `agentloom-minimal.workflow.js`: minimal VM/agent fixture modeled after AgentLoom's `examples/minimal.workflow.js`.
- `agentloom-bun-port-style.workflow.js`: pipeline/parallel/phase fixture modeled after AgentLoom's Bun-port-style example.

Functional coverage requirements:

- `1`, `2`, and `N` child task counts.
- Concurrency limit of `1`, `2`, and `N`.
- Default local policy: read-only, max 16 interactive agents, max depth 1, network/connectors disabled, danger-full-access denied.
- Success, failure, timeout, cancellation, pause/resume, interrupted process resume.
- Missing runner, unsupported runner, runner failure, malformed child output, script exception, budget exhaustion.
- Artifact write/read and final result selection.
- Prompt/result/stdout/stderr/event artifact hygiene and secret suppression.
- Event stream reconstruction.
- Replay command/API can re-stream a completed run.
- CLI/API/MCP equivalence for the same workflow fixture.
- MCP not-ready result behavior and artifact-root validation.

### E2E And Browser Tests

Keep sparse:

- UI preview -> approve -> run -> inspect final artifact using fake backend.
- UI pause/resume/cancel controls on a running workflow.
- UI queued-agent badge or row state for agents waiting on concurrency.
- UI phase timeline with counts, tokens, and elapsed time.
- MCP host smoke tests for Cursor/Codex/Kiro/Gemini/OpenCode config examples where automation is practical.
- CLI `you session create --workflow` with fake server.

### Stress And Race Tests

- 100 child tasks with concurrency 8 using fake runners.
- 1,000 child task planning/projection without real provider calls.
- Concurrent task completion and checkpoint writes under `-race`.
- Cancel while tasks are queued/running.
- Resume while some child tasks already completed.
- Reconnect after stream drop and reconcile missed provider-session messages.
- Queue saturation and budget exhaustion.
- Repeated load/unload if hosted runtimes are used by workflow children.

### Contract And Generated Checks

- OpenAPI validates.
- Generated Go API contracts are current.
- Generated TypeScript API contracts are current.
- Fixture JSON for runs/tasks/artifacts round-trips through generated contracts.
- `make lint`, backend tests, frontend typecheck/lint/test, and focused Playwright/Storybook checks pass for touched surfaces.

## Quality Gates

Required before customer-facing release:

- OpenAPI generation and generated diff checks pass.
- Backend unit/integration/functional/stress tests pass.
- Frontend typecheck, lint, component tests, and focused browser verification pass.
- MCP server smoke tests pass for stdio and any supported HTTP transport.
- Event replay reconstructs workflow run summaries.
- Logs and metrics have focused tests or verification hooks.
- Example workflow suite passes with fake runners.
- Security review confirms sandbox/capability policy fails closed.
- Documentation covers CLI, API, UI, MCP setup, workflow authoring, budgets, and troubleshooting.

## Resolved Design Decisions

- JavaScript runtime: ship embedded `goja` first. Preserve the runtime interface so a supervised JS runtime can be added later if customer workflow compatibility requires it.
- Workflow storage: personal/global reusable workflows live under `~/.you-agent-factory/workflows`. Installed workflow packages may expose package-relative workflow directories. Claude-compatible `.claude/workflows` directories are supported as an import/resolution compatibility path, not as the primary native storage location.
- MCP serving: MCP should share the same session runtime and server instancing as the normal backend server whenever practical. A `you mcp serve` entrypoint can exist, but it should attach to or compose the same session service/runtime rather than run a divergent implementation.
- CLI vocabulary: CLI uses base concepts directly: factories, sessions, work, dispatches, events, and artifacts. MCP may expose workflow-shaped tools for host-agent ergonomics.
- Source exposure: "source" means the authored orchestrator source: for JavaScript this is the workflow JS/TS file or inline generated script; for Petri this is the factory config/topology source. API/UI/MCP should expose source refs, hashes, metadata, and safe previews by default. Raw source should be available only through explicit inspect/export operations that apply the same secret/artifact hygiene rules as other diagnostic content.
- Core loop reuse: the first implementation should share the existing session runtime loop as much as possible. The goal is to switch out the token/coordination subsystem, not duplicate worker/provider/event/session infrastructure.
- Raw VM state: do not expose or persist raw VM state directly as a first-class durable model. Persist source refs/hashes, args digests, phase summaries, checkpoints, dispatch/artifact refs, and sanitized replay/debug snapshots. If a low-level VM snapshot is ever needed, store it as an internal diagnostic artifact with strict versioning and hygiene, not as the canonical resume contract.
- Terminology generalization: generalize Petri-specific API/UI terminology to dispatch/session/orchestrator terminology as much as possible before JavaScript sessions ship. Petri terms remain in Petri-specific detail panels and docs.
- Claude compatibility lookup: `.claude/workflows` lookup is enabled by default for compatibility.
- Workflow-file factory persistence: running a workflow file materializes an ephemeral JavaScript factory by default. Persist only when an explicit save/import/package command is used.
- MCP compatibility testing: integration tests should include a mock MCP client that exercises tool discovery, start-session, status, artifacts, cancellation, and error paths. A real Cursor-worker smoke test is desirable when available, but external host tests should be additive rather than the only proof.

## Diagnostic Artifact Format Suggestions

Use three related artifact formats instead of one raw VM dump. All formats must be sanitized, versioned, and safe to expose as diagnostic artifacts. None of them is the canonical durable JavaScript VM state.

### `javascript_checkpoint_summary.v1`

Purpose: resume and progress inspection.

Recommended shape:

```json
{
  "schemaVersion": 1,
  "kind": "javascript_checkpoint_summary",
  "sessionId": "session-123",
  "checkpointId": "checkpoint-0042",
  "sourceHash": "sha256:...",
  "phase": "Review",
  "phaseIndex": 1,
  "createdAt": "2026-06-08T00:00:00Z",
  "argsDigest": "sha256:...",
  "policyHash": "sha256:...",
  "completedDispatchIds": ["dispatch-1"],
  "pendingDispatchIds": ["dispatch-2"],
  "artifactIds": ["artifact-final-draft"],
  "budget": {
    "maxAgents": 16,
    "concurrency": 4,
    "remainingAgents": 12
  },
  "warnings": [],
  "resumeStrategy": "replay_completed_dispatches_then_continue"
}
```

Notes:

- Store source refs/hashes and dispatch/artifact refs, not raw JS heap objects.
- Store sanitized script-visible state only when it is structured-cloneable and passes artifact hygiene.
- Make checkpoints useful for resume and UI projection, not for debugger-grade VM restoration.

### `javascript_replay_snapshot.v1`

Purpose: replay/debug inspection.

Recommended shape:

```json
{
  "schemaVersion": 1,
  "kind": "javascript_replay_snapshot",
  "sessionId": "session-123",
  "sourceHash": "sha256:...",
  "eventsRange": {
    "fromSequence": 1,
    "toSequence": 84
  },
  "phases": [
    {
      "title": "Edit",
      "status": "completed",
      "dispatchCounts": { "queued": 0, "running": 0, "completed": 8, "failed": 0 },
      "tokenUsage": { "total": 12000 },
      "durationMillis": 180000
    }
  ],
  "dispatches": [
    {
      "dispatchId": "dispatch-1",
      "label": "edit:file.rs",
      "status": "completed",
      "artifactIds": ["artifact-1"],
      "providerSessionId": "provider-session-1"
    }
  ],
  "finalResultArtifactId": "artifact-final",
  "hygiene": {
    "auditMode": "metadata-only",
    "secretFindingCount": 0,
    "truncated": false
  }
}
```

Notes:

- This should be derivable from events plus stored artifacts.
- It is the right format for CLI/API/UI/MCP replay and support bundles.
- It should be append/rebuild friendly, similar to the existing event-first runtime posture.

### `javascript_vm_diagnostic.v1`

Purpose: rare low-level support diagnostics.

Recommended shape:

```json
{
  "schemaVersion": 1,
  "kind": "javascript_vm_diagnostic",
  "sessionId": "session-123",
  "sourceHash": "sha256:...",
  "runtime": "goja",
  "runtimeVersion": "recorded-build-version",
  "capturedAt": "2026-06-08T00:00:00Z",
  "diagnostics": {
    "stack": "sanitized stack",
    "currentPhase": "Review",
    "pendingPromiseCount": 0,
    "hostCallInFlightCount": 1
  },
  "hygiene": {
    "auditMode": "metadata-only",
    "secretFindingCount": 0
  }
}
```

Notes:

- This is not a resume format.
- It should be opt-in, internal/diagnostic visibility by default, and safe to omit from normal exports.
- Do not store raw heap snapshots, closures, host handles, or unsanitized script variables.

## Petri Terminology Generalization Report

This is an inventory report for later implementation. The goal is to generalize as much UI/API vocabulary as practical before JavaScript sessions ship, while preserving compatibility aliases for existing Petri factory behavior.

### Keep As Petri-Specific

These fields are genuinely Petri concepts and should remain under Petri-specific projections or compatibility views:

- `tokenId`
- `placeId`
- `fromPlaceId`
- `toPlaceId`
- `placeTokenCounts`
- `currentWorkItemsByPlaceID`
- `place_occupancy_work_items_by_place_id`
- `inputPlaces`
- `outputPlaces`
- `inputPlaceIDs`
- `outputPlaceIDs`
- `viaPlaceID`
- `transitionId` when it means a Petri transition
- marking/mutation fields such as `FactoryWorldMutationView`

Recommendation:

- Move these under `runtime.orchestrator.petri` and `topology.petri` over time.
- Keep compatibility copies on existing `FactoryWorld*` schemas until generated clients and UI consumers migrate.

### Generalize Before JavaScript Sessions Ship

These concepts are already close to orchestrator-neutral and should be generalized first:

- `FactoryWorldWorkstationRequestView` -> `FactoryWorldDispatchView`
- `workstationRequestsByDispatchId` -> `dispatchesById` or `dispatchDetailsById`
- `WorkstationRequestPayload` -> `DispatchRequestPayload`
- `WorkstationResponsePayload` -> `DispatchResponsePayload`
- `workstationName` -> `dispatchTargetName` or keep as optional Petri metadata beside `label`
- `workstationKind` -> `dispatchKind` or `orchestratorDispatchKind`
- `activeWorkstationNodeIDs` -> `activeCoordinationNodeIDs` or Petri-only projection
- `WorkstationResult` -> `DispatchResult`
- UI "workstation request" copy -> "dispatch" in shared panels

Recommendation:

- Prioritize shared read models, event payload display names, and UI labels that JavaScript sessions will immediately use.
- Keep Petri-specific detail panels named "Workstation" where the selected object is truly a Petri workstation.

### Already General Enough

These should remain shared and should be reused by JavaScript sessions:

- `dispatchId`
- `inFlightDispatchCount`
- `activeDispatchIDs`
- `activeExecutionsByDispatchID`
- `DispatchHistory`
- `ProviderSessionMetadata`
- `FactoryEvent.context`
- work request/input/output content shapes
- artifact and staged-file references
- session lifecycle fields

### Migration Strategy

1. Add orchestrator-kind-aware fields without removing current fields.
2. Teach UI projections to prefer shared fields and fall back to Petri compatibility fields.
3. Move Petri-only counts/places/tokens into `runtime.orchestrator.petri`.
4. Add JavaScript projections under `runtime.orchestrator.javascript`.
5. Rename UI copy from "workstation request" to "dispatch" in shared history/detail panels.
6. Keep OpenAPI compatibility aliases through at least one release after JavaScript sessions ship.
7. Remove or de-emphasize Petri terms only after contract fixtures prove both Petri and JavaScript sessions render correctly.

### Report Follow-Up

Create a dedicated implementation report later under `docs/internal/development/` with:

- exact OpenAPI schema fields to add/deprecate
- UI component rename list
- generated TypeScript type impact
- event payload compatibility strategy
- test fixture changes for one Petri session and one JavaScript session

## Save/Import/Package Recommendations

Default behavior:

- Running a workflow file creates an ephemeral JavaScript factory/session.
- Explicit save should default to `~/.you-agent-factory/workflows` for personal/global reuse.
- Project-owned save should require an explicit project/package target.
- Package-relative workflow directories should be used by installed packages and export/import flows, not by default ad hoc saves.

Suggested commands:

- `you session create --workflow <file|name>` creates an ephemeral factory session.
- `you factory save --session <id> --target user-workflows` saves to `~/.you-agent-factory/workflows`.
- `you factory save --session <id> --target project --path <path>` saves into project-owned factory/workflow storage.
- package/export commands decide package-relative paths explicitly.

## Research Sources

- GitHub issue: <https://github.com/portpowered/you-agent-factory/issues/741>
- GitHub issue comments API used to read linked research: <https://api.github.com/repos/portpowered/you-agent-factory/issues/741/comments>
- AgentLoom implementation status: <https://github.com/vblanco20-1/AgentLoom/blob/main/IMPLEMENTATION_STATUS.md>
- Codex dynamic workflows lab MCP server: <https://github.com/pezzos/codex-dynamic-workflows-lab/blob/main/plugins/codex-dynamic-workflows-lab/dist/plugin/mcp-server.js>
- pi-dynamic-workflows adversarial review generator: <https://github.com/QuintinShaw/pi-dynamic-workflows/blob/b817f17ec957fd4a9c83bb4cef956b0c7d718d7b/src/adversarial-review.ts#L23>
- Bun `edit-round.workflow.ts`: <https://github.com/oven-sh/bun/blob/a9886150983872820ddf6db20433c6bf458c2f2f/scripts/clippy-loop/edit-round.workflow.ts>
- plugin-ultracode README and repository guide: <https://github.com/just-every/plugin-ultracode>
- Anthropic launch post, May 28, 2026: <https://claude.com/blog/introducing-dynamic-workflows-in-claude-code>
- Anthropic dynamic workflow patterns post, June 2026: <https://claude.com/blog/a-harness-for-every-task-dynamic-workflows-in-claude-code>
- Claude Code dynamic workflow docs: <https://code.claude.com/docs/en/workflows>
- Claude Code parallel agents docs: <https://code.claude.com/docs/en/agents>
