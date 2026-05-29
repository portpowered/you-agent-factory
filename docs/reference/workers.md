# Workers Reference

Use this page when you need the current worker contract, the canonical
`workers/<name>/AGENTS.md` placement, and the split between worker-owned and
workstation-owned runtime fields.

This is the canonical customer-facing guide for workers. Keep worker types,
worker-scoped runtime fields, model/script backend fields, and
`workers/<name>/AGENTS.md` placement here. Keep workstation routing and
prompt/runtime fields in [Workstations](workstations.md), and keep top-level
`factory.json` work type and routing context in
[Factory JSON and work configuration](work.md).

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
[Workstations](workstations.md) when you need to understand when a step runs,
what it renders, and where it routes results.

## Current Contract

- Workers define the execution backend and system instructions.
- Workstations define topology, routing, prompt templates, and per-step
  execution context.
- The current worker types are `MODEL_WORKER` and `SCRIPT_WORKER`.
- `MODEL_WORKER` can declare provider-agnostic `operations`, named input and
  output slots, `modelLocality`, and concrete `model` identity so
  `MODEL_INVOKE` workstations can validate compatibility before dispatch.
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

Only `type` is required for a split worker definition. A minimal model worker
can be:

```yaml
---
type: MODEL_WORKER
---

You are a helpful assistant.
```

## Worker-Owned Vs Workstation-Owned Fields

| Put it on the worker | Put it on the workstation |
|----------------------|---------------------------|
| `type`, `model`, `modelProvider`, `executorProvider`, `openCodeAgent` | `type`, `worker`, `promptFile`, prompt body, `openCodeAgent` |
| `command`, `args` | `outputSchema`, `limits.maxExecutionTime`, `limits.maxRetries` |
| `resources`, `timeout`, `stopToken`, `skipPermissions` | `stopWords`, `workingDirectory`, `worktree`, `env` |
| Worker body used as the model system prompt | Prompt template used as the rendered user message |

Use the worker when the setting belongs to the execution backend or shared
worker identity. Use the workstation when the setting belongs to one workflow
step, prompt rendering, or per-step execution behavior.

## Worker Types

### `MODEL_WORKER`

Use a model worker when the workstation should call a model-backed executor.
The markdown body is the system prompt.

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are a software engineer. Follow the workstation instructions and keep
changes scoped to the current work item.
```

When the worker should support direct model operations such as TTS, add the
capability contract directly to the worker:

```yaml
---
type: MODEL_WORKER
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
type: MODEL_WORKER
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

## Core Fields

| Field | Applies to | What it controls |
|-------|------------|------------------|
| `type` | all workers | `MODEL_WORKER` or `SCRIPT_WORKER` |
| `model` | model workers | Concrete model identifier such as `gpt-5-codex` |
| `modelProvider` | model workers | Model-routing provider identity used for provider selection and diagnostics |
| `executorProvider` | model workers | Execution wrapper or adapter used to run the worker |
| `command` | script workers | Executable to run |
| `args` | script workers | Command arguments; values can use Go template expressions |
| `resources` | all workers | Worker-level resource requirements |
| `timeout` | all workers | Per-attempt worker timeout |
| `stopToken` | model workers | Output marker for accepted completion when configured |
| `skipPermissions` | model workers | Provider-specific permission shortcut |
| `modelLocality` | model workers | `LOCAL` or `CLOUD` execution locality for model operations and diagnostics |
| `operations` | model workers | Provider-agnostic capability declarations with uppercase operation names and typed slots |
| `openCodeAgent` | model workers | Named OpenCode agent profile for dispatches that resolve to runner `opencode` |

## Provider Fields

Keep `modelProvider` and `executorProvider` separate:

- `modelProvider` names the model backend. Current built-in values are
  `CLAUDE` and `CODEX`.
- `executorProvider` names the execution wrapper around that worker. The
  current public built-in value is `SCRIPT_WRAP`.

For a normal model worker, both fields can appear on the same worker because
they answer different questions: which model backend to use, and which worker
execution adapter should run it.

Use `runner` when the operator needs to choose the execution family. Keep
`modelProvider` for worker-local provider compatibility, diagnostics, and the
legacy fallback path when no explicit runner is configured.

## OpenCode Agent Profiles

Use `openCodeAgent` on a `MODEL_WORKER` when the dispatch should run through
the OpenCode runner and you want OpenCode to apply a named agent profile
(system prompt, tool permissions, and other settings created with
`opencode agent create`) instead of duplicating that guidance in the worker
body.

```yaml
---
type: MODEL_WORKER
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

Workstations can override the worker default. See [Workstations](workstations.md)
for precedence and workstation-level examples.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Models and model operations](models.md)
- [Workstations reference](workstations.md)
- [Factory JSON and work configuration](work.md)
- [Author AGENTS.md](authoring-agents-md.md)
