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
- The current worker types are `MODEL_WORKER`, `SCRIPT_WORKER`, and
  `HOSTED_WORKER`.
- Current built-in `modelProvider` values are `CLAUDE` and `CODEX`.
- Runner selection is separate from `modelProvider`. Use factory or
  workstation `runner` fields to choose the built-in runner ID: `codex`,
  `gemini`, `kiro`, `cursor-cli`, or `opencode`.
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

### `HOSTED_WORKER`

Use a hosted worker when the repository owns the integration code and the
workstation should bind to a built-in provider rather than a subprocess.

V1 hosted workers currently support only `provider: LINEAR`, and the intended
shape is for a `POLLER` workstation to bind that worker so service mode can
supervise the provider loop as a first-class sidecar.

```yaml
---
type: HOSTED_WORKER
provider: LINEAR
auth:
  secretRef: secrets/linear-api-key
linear:
  pollInterval: 2m
  teams: ["ENG"]
  states: ["unstarted", "started"]
  mapping:
    workType: task
    state: init
  claim:
    assigneeField: linearAssignee
---

Hosted Linear intake poller.
```

Hosted-worker rules:

- Use `auth.secretRef` for the provider API key. Inline secret values are not
  part of the v1 contract.
- OAuth-oriented fields such as `clientId`, `clientSecret`, `refreshToken`,
  or token URLs are rejected in v1.
- Provider-specific config lives on the worker, not on the workstation.
- The current hosted Linear poller supports deterministic issue-to-work
  mapping, bounded checkpointed resume behavior, team or issue-state filters,
  and safe diagnostics that do not log secret material.

## Core Fields

| Field | Applies to | What it controls |
|-------|------------|------------------|
| `type` | all workers | `MODEL_WORKER`, `SCRIPT_WORKER`, or `HOSTED_WORKER` |
| `model` | model workers | Concrete model identifier such as `gpt-5-codex` |
| `modelProvider` | model workers | Model-routing provider identity used for provider selection and diagnostics |
| `executorProvider` | model workers | Execution wrapper or adapter used to run the worker |
| `command` | script workers | Executable to run |
| `args` | script workers | Command arguments; values can use Go template expressions |
| `resources` | all workers | Worker-level resource requirements |
| `timeout` | all workers | Per-attempt worker timeout |
| `stopToken` | model workers | Output marker for accepted completion when configured |
| `skipPermissions` | model workers | Provider-specific permission shortcut |
| `provider` | hosted workers | Built-in hosted integration provider. V1 supports `LINEAR`. |
| `auth.secretRef` | hosted workers | Secret reference for provider authentication. |
| `linear` | hosted workers | Provider-specific Linear poller configuration such as poll interval, filters, mapping, and optional claim settings. |

## Provider Fields

Keep `modelProvider` and `executorProvider` separate:

- `modelProvider` names the model backend. Current built-in values are
  `CLAUDE` and `CODEX`.
- `executorProvider` names the execution wrapper around that worker. The
  current public built-in value is `SCRIPT_WRAP`.

For a normal model worker, both fields can appear on the same worker because
they answer different questions: which model backend to use, and which worker
execution adapter should run it.

Hosted workers do not use `modelProvider` or `executorProvider` to reach
provider APIs. Keep hosted provider identity under `provider`, keep
authentication under `auth.secretRef`, and keep provider-specific polling
settings under the provider block such as `linear`.

## When To Use Script Versus Hosted Pollers

- Use a `SCRIPT_WORKER` poller when you already own the external integration
  logic and want the service to supervise that script as a poller sidecar.
- Use a `HOSTED_WORKER` poller when the repository-owned provider code already
  implements the integration and you want the public contract instead of a
  subprocess dependency.
- Use a hosted `LINEAR` worker when you need the built-in GraphQL client,
  checkpointed resume behavior, and deterministic issue mapping without writing
  custom script code.

V1 non-goals for hosted workers:

- OAuth flows are not supported.
- Inline API key fields are not supported.
- Providers other than `LINEAR` are not yet part of the public contract.

Use `runner` when the operator needs to choose the execution family. Keep
`modelProvider` for worker-local provider compatibility, diagnostics, and the
legacy fallback path when no explicit runner is configured.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Workstations reference](workstations.md)
- [Factory JSON and work configuration](work.md)
- [Author AGENTS.md](authoring-agents-md.md)
