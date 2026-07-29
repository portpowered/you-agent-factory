# Workers

`you docs workers` prints this guide from the packaged CLI topic list. The
repository file is `docs/reference/workers.md`.

Use this page when you need the current worker contract, the canonical
`workers/<name>/AGENTS.md` placement, and the split between worker-owned and
workstation-owned runtime fields.

This is the canonical customer-facing guide for workers. Keep worker types,
worker-scoped runtime fields, model/script backend fields, and
`workers/<name>/AGENTS.md` placement here. Keep workstation routing and
prompt/runtime fields in `you docs workstations`, and keep top-level
`factory.json` work type and routing context in
`you docs config`.

## Canonical Placement

Keep worker runtime definitions in the split layout beside `factory.json`:

```text
factory/
  factory.json
  workers/
    swe/AGENTS.md
  workstations/
    execute-story/AGENTS.md
```

`factory.json` declares the worker by name. The worker directory supplies the
runtime backend details for that name.

## How Workers Fit The Workflow

Workers are runtime executors that workstation steps invoke. Keep the split
between the two concepts explicit:

- `factory.json` declares the workflow topology, states, routing, workers, and
  workstations.
- A workstation names one worker through `worker` and controls the
  step-specific prompt, routing, limits, output validation, and execution
  environment.
- The bound worker supplies the execution backend and shared system
  instructions for every workstation that references it.

Use this page when you need the backend-facing worker contract. Use
`you docs workstations` when you need to understand when a step runs,
what it renders, and where it routes results.

## Worker Taxonomy

Public factory config names workers by behavior class:

| Public type | Behavior | Use when |
|-------------|----------|----------|
| `INFERENCE_WORKER` | One-shot model operations | Harnessless inference such as TTS, ASR, or typed `INFERENCE_RUN` dispatches |
| `AGENT_WORKER` | Agent-loop model execution | Prompt-rendered `AGENT_RUN` workstations that iterate until acceptance, rejection, or failure |
| `SCRIPT_WORKER` | Script execution | Command-backed `SCRIPT_RUN` workstations |
| `POLLER_WORKER` | Hosted poller integration | Workstations with `behavior: "POLLER"` that ingest external work through built-in providers |

### Legacy compatibility aliases

During the migration window, existing factories may still use:

- `MODEL_WORKER` — loads and validates successfully. Projects to
  `INFERENCE_WORKER` when paired with inference workstations, or to
  `AGENT_WORKER` when paired with agent-loop workstations. Prefer the new names
  in new docs and configs.
- `HOSTED_WORKER` — loads and validates successfully. Projects to
  `POLLER_WORKER` poller behavior. Prefer `POLLER_WORKER` for new hosted
  poller workers.

See `you docs workstations` for the matching `INFERENCE_RUN`, `AGENT_RUN`,
`SCRIPT_RUN`, and `POLLER_RUN` workstation taxonomy and compatibility rules.

## Current Contract

- Workers define the execution backend and system instructions.
- Workstations define topology, routing, prompt templates, and per-step
  execution context.
- The current worker types are `INFERENCE_WORKER`, `AGENT_WORKER`,
  `SCRIPT_WORKER`, and `POLLER_WORKER`.
- `INFERENCE_WORKER` can declare provider-agnostic `operations`, named input
  and output slots, `modelLocality`, and concrete `model` identity so
  `INFERENCE_RUN` workstations can validate compatibility before dispatch.
- `AGENT_WORKER` supplies the model backend and shared system instructions for
  prompt-rendered `AGENT_RUN` workstations.
- Current built-in `modelProvider` values are `CLAUDE`, `CODEX`, `CURSOR`, and
  `ANTIGRAVITY`.
- Runner selection is separate from `modelProvider`. Use factory or
  workstation `runner` fields to choose the built-in runner ID: `codex`,
  `claude`, `cursor-cli`, or `antigravity`.
- `executorProvider` accepts the canonical mechanisms `SCRIPT_WRAP` and `ACP`.
  For ACP, `modelProvider` names an integration such as `cursor-acp`. See
  `you docs providers` for ACP setup and
  lifecycle commands.
- Older snake_case and alias frontmatter keys are compatibility-only inputs.
  New docs and authored configs should use canonical camelCase fields.

## Minimal Worker

Only `type` is required for a split worker definition. A minimal agent worker
can be:

```yaml
---
type: AGENT_WORKER
---

You are a helpful assistant.
```

When operator defaults are configured, you can omit `modelProvider` and
`model` on `MODEL_WORKER` definitions and let the runtime fill them from
`~/.you-agent-factory/config.json`, `YOU_DEFAULT_WORKER_MODEL_PROVIDER`,
`YOU_DEFAULT_WORKER_MODEL`, or run-scoped `you run --provider` and `--model`.
See `you docs config` for precedence, `DEFAULT`
resolution, and failure modes.

Authored worker `modelProvider` and `model` values always win over operator
defaults. Script and hosted workers never receive operator model defaults.

## Worker-Owned Vs Workstation-Owned Fields

| Put it on the worker | Put it on the workstation |
|----------------------|---------------------------|
| `type`, `model`, `modelProvider`, `executorProvider` | `type`, `worker`, `promptFile`, prompt body |
| `command`, `args` | `behavior`, `outputs`, `onFailure`, `onContinue`, `onRejection` |
| `provider`, `auth.secretRef`, `linear.*` for hosted pollers | `outputSchema`, `limits.maxExecutionTime`, `limits.maxRetries` |
| `resources`, `timeout`, `stopToken`, `skipPermissions` | `stopWords`, `workingDirectory`, `worktree`, `env` |
| Worker body used as the model system prompt | Prompt template used as the rendered user message |

For hosted Linear pollers, keep `behavior: "POLLER"` and routing on the
workstation. Keep `auth.secretRef`, `linear.pollInterval`, `linear.teamIds`,
`linear.stateIds`, `linear.mapping`, and optional `linear.claim` on the bound
worker. Do not move hosted Linear provider config onto the workstation body.

Use the worker when the setting belongs to the execution backend or shared
worker identity. Use the workstation when the setting belongs to one workflow
step, prompt rendering, or per-step execution behavior.

## Worker Types

### `INFERENCE_WORKER`

Use an inference worker when the workstation should perform one bounded model
operation such as TTS or ASR through `INFERENCE_RUN`. The markdown body
describes the operation context; slot bindings and operation names live on the
workstation.

```yaml
---
type: INFERENCE_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: CODEX
modelLocality: LOCAL
resources:
  - name: omnivoice-cache
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
      - name: voice
        contentTypes:
          - JSON
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech from resolved text content.
```

For a cloud-backed worker, keep the same operation contract and change only the
worker identity and locality:

```yaml
---
type: INFERENCE_WORKER
model: gpt-4o-mini-tts
modelProvider: CODEX
modelLocality: CLOUD
resources:
  - name: cloud-tts-quota
    capacity: 1
  - name: cloud-tts-slot
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
      - name: voice
        contentTypes:
          - JSON
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech through the cloud-backed provider.
```

Validation rejects duplicate operation names on one worker, duplicate slot
names within one operation direction, lowercase or invalid uppercase enum
values, and incompatible content declarations.

Legacy `MODEL_WORKER` remains accepted for inference workers during the
migration window and projects to the same inference behavior.

### `AGENT_WORKER`

Use an agent worker when the workstation should render a prompt and dispatch to
a model-backed executor through `AGENT_RUN`. The markdown body is the system
prompt.

```yaml
---
type: AGENT_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are a software engineer. Follow the workstation instructions and keep
changes scoped to the current work item.
```

Legacy `MODEL_WORKER` remains accepted for agent-loop workers during the
migration window. When paired with `AGENT_RUN` (or legacy `MODEL_WORKSTATION`),
it projects to `AGENT_WORKER` behavior.

#### Agent loop runtime

`AGENT_WORKER` pairs with `AGENT_RUN` workstations to start or resume one
agent loop per dispatch. The factory runtime owns session cancellation,
timeout, and final-result selection. Agent-loop execution is implemented
inside the service through a library adapter; customers configure agent
behavior through factory vocabulary (`AGENT_WORKER`, `AGENT_RUN`, worker and
workstation fields) rather than by invoking an external harness CLI.

Do not treat `INFERENCE_WORKER` or `operations` as a substitute for agent
loops. Inference workers declare one-shot model operations for
`INFERENCE_RUN` workstations. Agent workers do not declare `operations`;
validation rejects model capability declarations on `AGENT_WORKER`.

#### Local model capacity

Local `AGENT_WORKER` definitions borrow ready model capacity from the
process-wide model host through inferencer leases. Factory sessions acquire
and release those leases around agent model calls; they do not own supervised
local model subprocess lifecycle. Use `you docs models` for managed-runtime
readiness, pull, and `/models` inspection. Modelhost surfaces report
readiness, lifecycle, and lease state only; they do not own agent transcript
metadata.

#### Explicit tool policy

Agent tool use is disabled unless the worker declares an explicit
`agentTools.policy`. Omit `agentTools` to keep the default `DISABLED`
behavior.

| Policy | Behavior |
|--------|----------|
| `DISABLED` | Default. The agent loop runs with tool execution disabled. Tool calls are denied with stable diagnostics instead of being silently ignored. |
| `READ_ONLY` | Allows bounded filesystem inspection tools: `read_file` and `list_directory` under the dispatch working directory. Write tools are denied. |
| `ENABLED` | Allows `read_file`, `list_directory`, and `write_file` under the dispatch working directory. Paths outside the working directory are rejected. |

`agentTools` is valid only on `AGENT_WORKER`. Validation fails when tool
configuration is present without a policy, when the policy is unsupported,
or when `agentTools` appears on non-agent worker types.

```yaml
---
type: AGENT_WORKER
model: gpt-5-codex
modelProvider: CODEX
agentTools:
  policy: READ_ONLY
---

You are a software engineer. Use read-only filesystem tools only when the
workstation prompt requires repository inspection.
```

Allowed tool execution records bounded `tool_diagnostics` summaries for tool
start, success, and failure. Diagnostics do not expose raw process output,
secrets, or unrestricted host paths as primary customer results.

#### Agent-run failure classes

Agent dispatches surface stable `failure_class` values in work and session
inspection metadata. These classes identify agent execution separately from
generic inference failures:

| `failure_class` | Typical cause | Recovery hint |
|-----------------|---------------|---------------|
| `agent_run_lease_denied` | Managed runtime capacity exhausted | Retry later or increase `MODEL` resource capacity |
| `agent_run_model_not_ready` | Managed runtime missing or still loading | Pull or wait for readiness before retrying |
| `agent_run_model_runtime_failure` | Managed runtime failed or is unsupported | Resolve runtime failure or adjust worker configuration |
| `agent_run_tool_policy_violation` | Tool call denied by `agentTools.policy` | Change tool policy or workstation prompt expectations |
| `agent_run_tool_denied` | Unsupported tool name requested | Use only the supported tool set for the configured policy |
| `agent_run_tool_failure` | Allowed tool execution failed | Inspect bounded tool diagnostics and fix the underlying path or content issue |
| `agent_run_harness_failure` | Agent loop runtime failure | Inspect dispatch failure details and factory logs |
| `agent_run_canceled` | Factory session or dispatch canceled | Resubmit or resume according to workflow policy |
| `agent_run_timeout` | Dispatch or worker timeout exceeded | Increase limits or narrow workstation scope |

See `you docs sessions` for dispatch inspection fields that separate final
output from tool diagnostics and transcript metadata.

### `SCRIPT_WORKER`

Use a script worker when the workstation should run a command instead of a
model. The markdown body is descriptive only; the executed fields are
`command` and `args`.

```yaml
---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
timeout: 10m
---

Runs the Go test suite.
```

### `POLLER_WORKER`

Use a poller worker when Infinite You should run a built-in provider
integration instead of a custom script or model backend. V1 poller workers are
poller-only and pair with a workstation that uses `behavior: "POLLER"`.

Legacy `HOSTED_WORKER` remains accepted during the migration window and
projects to the same poller behavior.

The current built-in hosted provider is `LINEAR`. Hosted workers authenticate
through `auth.secretRef` only. Do not put inline API keys, OAuth tokens, or
other credential fields on the worker body.

#### Hosted Linear authentication and secrets

Set `auth.secretRef` to the secret name the runtime should resolve before
service mode starts polling. The common path is `secrets/linear-api-key`.

At runtime, Infinite You resolves the referenced secret in this order:

1. A non-empty environment variable derived from the secret reference. For
   `secrets/linear-api-key`, set `INFINITE_YOU_SECRET_SECRETS_LINEAR_API_KEY`.
2. A file relative to the factory runtime base directory. For
   `secrets/linear-api-key`, create `secrets/linear-api-key` beside
   `factory.json` with the Linear API key as the file contents.

Prepare one of those sources before starting service mode. The website and
split `workers/<name>/AGENTS.md` files store only the secret reference, never
the raw key value.

#### Hosted Linear poller fields

| Field | Required | What it controls |
|-------|----------|------------------|
| `provider` | Yes | Built-in hosted provider. Use `LINEAR` for the Linear poller. |
| `auth.secretRef` | Yes | Referenced secret name resolved at runtime. |
| `linear.pollInterval` | No | Go duration between poll cycles, such as `30s` or `2m`. |
| `linear.teamIds` | No | Linear team identifiers that bound the poll source. |
| `linear.stateIds` | No | Linear issue-state identifiers that bound the poll source. |
| `linear.mapping.workType` | Yes | Canonical submitted work type for matched issues. |
| `linear.mapping.state` | Yes | Canonical submitted work state for matched issues. |
| `linear.claim.assigneeField` | No | Linear issue field used for optional assignee claim metadata. |

`linear.teamIds` and `linear.stateIds` are optional scope filters. When
omitted, the hosted Linear poller uses its default source bounds for the
configured credentials.

#### Hosted Linear poller example

`workers/linear-poller/AGENTS.md`:

```yaml
---
type: POLLER_WORKER
provider: LINEAR
auth:
  secretRef: secrets/linear-api-key
linear:
  pollInterval: 2m
  teamIds: ["team-a"]
  stateIds: ["state-b"]
  mapping:
    workType: task
    state: init
  claim:
    assigneeField: assignee.email
---

Repository-owned Linear poller.
```

Bind it from a poller workstation in `factory.json`:

```json
{
  "name": "linear-intake",
  "behavior": "POLLER",
  "worker": "linear-poller",
  "outputs": [{ "workType": "task", "state": "init" }],
  "onFailure": { "workType": "task", "state": "failed" }
}
```

Use `you docs workstations` for `behavior: "POLLER"` lifecycle semantics and
`you docs authoring-factories` for a fuller poller walkthrough.

## Core Fields

| Field | Applies to | What it controls |
|-------|------------|------------------|
| `type` | all workers | `INFERENCE_WORKER`, `AGENT_WORKER`, `SCRIPT_WORKER`, or `POLLER_WORKER` |
| `provider` | poller workers | Built-in hosted provider such as `LINEAR` |
| `auth.secretRef` | poller workers | Referenced secret name resolved at runtime |
| `linear` | LINEAR poller workers | Poll interval, scope filters, mapping, and optional claim config |
| `model` | inference and agent workers | Concrete model identifier such as `gpt-5-codex` |
| `modelProvider` | inference and agent workers | Model-routing provider identity used for provider selection and diagnostics |
| `executorProvider` | inference and agent workers | Execution wrapper or adapter used to run the worker |
| `command` | script workers | Executable to run |
| `args` | script workers | Command arguments; values can use Go template expressions |
| `resources` | all workers | Worker-level resource requirements |
| `timeout` | all workers | Per-attempt worker timeout |
| `stopToken` | agent workers | Output marker for accepted completion when configured |
| `skipPermissions` | inference and agent workers | Provider-specific permission shortcut |
| `modelLocality` | inference workers | `LOCAL` or `CLOUD` execution locality for model operations and diagnostics |
| `operations` | inference workers | Provider-agnostic capability declarations with uppercase operation names and typed slots |
| `agentTools.policy` | agent workers | Explicit tool policy: `DISABLED`, `READ_ONLY`, or `ENABLED` |

## Provider Fields

Keep `modelProvider` and `executorProvider` separate:

- `modelProvider` names the selected provider integration. It may be a model
  backend such as `CLAUDE` or `CODEX`, or an ACP integration such as
  `cursor-acp`. Omit it only when operator defaults supply the provider.
- `model` names the concrete model identifier such as `gpt-5-codex`. Omit it
  when operator defaults supply the model for this run.
- `executorProvider` names the execution mechanism around that worker. Use
  `SCRIPT_WRAP` for command wrappers and `ACP` for Agent Client Protocol.

For a normal model worker, both fields can appear on the same worker because
they answer different questions: which model backend to use, and which worker
execution adapter should run it.

Use `runner` when the operator needs to choose the execution family. Keep
`modelProvider` for worker-local provider compatibility, diagnostics, and the
worker provider compatibility fallback when no explicit runner is configured.

## Response-stream provider fidelity

Model-backed workers run through a **Provider Session** while a **Factory
Session** owns the public observation stream. Response-stream fidelity varies by
provider capability. Do not assume every provider emits the same incremental
`FactoryResponseEvent` shape or survives retention pressure without gaps.

### Native streaming vs final-only providers

| Fidelity class | What consumers observe on the public stream | When it applies |
|----------------|---------------------------------------------|-----------------|
| Native streaming | Incremental public `FactoryResponseEvent` records such as message snapshots, text deltas, tool lifecycle updates, and related progress while work is running | Providers whose runner and adapter expose structured native streaming |
| Final-only | Terminal semantic snapshots only — for example one completed message snapshot and run completion or failure records — without incremental deltas between start and finish | Providers whose adapter declares final-only output, including headless Agy execution and OpenCode runs that select final-only mode |

Provider-native transcript formats, spinner repaint bytes, and private parser
fields stay inside the Provider Session boundary. The Factory Session publishes
only the neutral public `FactoryResponseEvent` vocabulary on CLI stdout and on
`GET /factory-sessions/{session_id}/response-events`.

### Observable degradation consumers must handle

Plan for sparse or interrupted observation even when invocation succeeds:

| Case | What you see | What stays authoritative |
|------|--------------|--------------------------|
| Retention pressure | A `STREAM_GAP` record with `fromSequence`, `toSequence`, and `firstAvailableSequence` instead of silently skipped sequences | Reconnect from `firstAvailableSequence` or omit `after_sequence`; reconcile consumer state against the gap payload. See `you docs sessions`. |
| Final-only providers | No incremental message or tool deltas between dispatch start and the terminal snapshot | `primaryResult` on invocation responses and terminal work or session facts on canonical `FactoryEvent` history |
| Slow or lossy stdout or SSE consumers | Live records may lag publication; retained history can expire | Invocation success still comes from `primaryResult` and canonical Factory events, not from observing every intermediate record |

Authoritative invocation success does **not** require byte-for-byte replay of
every intermediate provider event. A final-only provider can complete
successfully while the public stream shows only terminal semantic snapshots.

### Non-promises

The runtime does **not** promise:

- unsupported provider-native streaming fidelity beyond the public
  `FactoryResponseEvent` contract,
- byte-identical provider transcripts on the public response stream,
- or durable process-restart replay of ephemeral response events beyond the
  Factory Session retention window described in `you docs sessions`.

### Related invocation and session surfaces

- `you docs run` — CLI primary-result, human response-stream, and NDJSON
  automation output modes for one-shot invocations
- `you docs sessions` — ephemeral `GET /factory-sessions/{session_id}/response-events`
  SSE reconnect, `after_sequence`, retention limits, and separation from
  canonical `GET /factory-sessions/{session_id}/events` Factory event replay

## Related

- `docs/reference/README.md`
- `docs/README.md`
- `you docs models`
- `you docs workstations`
- `you docs config`
- `you docs work`
- `you docs run`
- `you docs sessions`
- `docs/reference/authoring-agents-md.md`
