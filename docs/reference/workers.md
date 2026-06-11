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
- Current built-in `modelProvider` values are `CLAUDE` and `CODEX`.
- Runner selection is separate from `modelProvider`. Use factory or
  workstation `runner` fields to choose the built-in runner ID: `codex`,
  `gemini`, `kiro`, `cursor-cli`, or `opencode`.
- Optional `openCodeAgent` selects a named OpenCode agent profile when the
  resolved runner is `opencode`. Omit it to keep today's default `opencode run`
  behavior without `--agent`.
- The current public `executorProvider` value is `SCRIPT_WRAP`.
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
`YOU_DEFAULT_WORKER_MODEL`, or global `--default-worker-model-provider` and
`--default-worker-model`. See `you docs config` for precedence, `DEFAULT`
resolution, and failure modes.

Authored worker `modelProvider` and `model` values always win over operator
defaults. Script and hosted workers never receive operator model defaults.

## Worker-Owned Vs Workstation-Owned Fields

| Put it on the worker | Put it on the workstation |
|----------------------|---------------------------|
| `type`, `model`, `modelProvider`, `executorProvider`, `openCodeAgent` | `type`, `worker`, `promptFile`, prompt body, `openCodeAgent` |
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
| `openCodeAgent` | agent workers | Named OpenCode agent profile for dispatches that resolve to runner `opencode` |

## Provider Fields

Keep `modelProvider` and `executorProvider` separate:

- `modelProvider` names the model backend. Current built-in values are
  `CLAUDE` and `CODEX`. Omit it when operator defaults supply the provider for
  this run.
- `model` names the concrete model identifier such as `gpt-5-codex`. Omit it
  when operator defaults supply the model for this run.
- `executorProvider` names the execution wrapper around that worker. The
  current public built-in value is `SCRIPT_WRAP`.

For a normal model worker, both fields can appear on the same worker because
they answer different questions: which model backend to use, and which worker
execution adapter should run it.

Use `runner` when the operator needs to choose the execution family. Keep
`modelProvider` for worker-local provider compatibility, diagnostics, and the
worker provider compatibility fallback when no explicit runner is configured.

## OpenCode Agent Profiles

Use `openCodeAgent` on an `AGENT_WORKER` when the dispatch should run through
the OpenCode runner and you want OpenCode to apply a named agent profile
(system prompt, tool permissions, and other settings created with
`opencode agent create`) instead of duplicating that guidance in the worker
body.

```yaml
---
type: AGENT_WORKER
model: gpt-5
runner: opencode
openCodeAgent: implementer
---

Follow the workstation prompt. The OpenCode agent profile supplies shared
tooling defaults for this worker.
```

Requirements and behavior:

- `openCodeAgent` must be a non-empty string when set. Explicit empty or
  whitespace-only values fail validation.
- The field is honored only when runner selection resolves to `opencode`.
  Setting `openCodeAgent` while the resolved runner is something else fails
  during factory build with an error that names `openCodeAgent`, the agent
  name, and the resolved runner ID.
- When configured and the runner is `opencode`, dispatches invoke
  `opencode run --agent <name>` before the rendered user prompt. Inference
  diagnostics record the resolved value as `opencode_agent`.
- Omit `openCodeAgent` to keep the existing `opencode run` argument shape with
  no `--agent` flag.

Discover available agent names with the OpenCode CLI:

```bash
opencode agent list
```

See the [OpenCode CLI reference](https://opencode.ai/docs/cli/) for agent
create/list commands and profile management.

Workstations can override the worker default. See `you docs workstations`
for precedence and workstation-level examples.

## Related

- `docs/reference/README.md`
- `docs/README.md`
- `you docs models`
- `you docs workstations`
- `you docs config`
- `you docs work`
- `docs/reference/authoring-agents-md.md`
