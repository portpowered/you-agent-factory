# Workers Reference

Use this page when you need the current worker contract, the canonical
`workers/<name>/AGENTS.md` placement, and the split between worker-owned and
workstation-owned runtime fields.

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

## Current Contract

- Workers define the execution backend and system instructions.
- Workstations define topology, routing, prompt templates, and per-step
  execution context.
- The current worker types are `MODEL_WORKER` and `SCRIPT_WORKER`.
- Current built-in `modelProvider` values are `CLAUDE` and `CODEX`.
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
| `type`, `model`, `modelProvider`, `executorProvider` | `type`, `worker`, `promptFile`, prompt body |
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

## Provider Fields

Keep `modelProvider` and `executorProvider` separate:

- `modelProvider` names the model backend. Current built-in values are
  `CLAUDE` and `CODEX`.
- `executorProvider` names the execution wrapper around that worker. The
  current public built-in value is `SCRIPT_WRAP`.

For a normal model worker, both fields can appear on the same worker because
they answer different questions: which model backend to use, and which worker
execution adapter should run it.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Author AGENTS.md](../authoring-agents-md.md)
- [Workstations reference](workstations.md)
- [Workstations and workers](../workstations.md)
- [Factory JSON and work configuration](../work.md)
