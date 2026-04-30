# Authoring AGENTS.md Files

---
author: ralph (agent)
last-modified: 2026-04-21
doc-id: agent-factory/authoring-agents-md
---

This guide explains how to write `AGENTS.md` files for the agent factory. The
recommended layout keeps prompt-heavy worker and workstation runtime
configuration in split `AGENTS.md` files beside a canonical `factory.json`
topology.

## Overview

The agent factory uses two kinds of `AGENTS.md` files:

| Kind | Location | Purpose |
|------|----------|---------|
| **Worker** | `factory/workers/{name}/AGENTS.md` | Defines *what* does the work — model config, executor adapter, system prompt |
| **Workstation** | `factory/workstations/{name}/AGENTS.md` | Defines *how* work is done — prompt template, execution limits, output schema |

Workers and workstations compose at runtime. A workstation references a worker by name. The worker's body becomes the system prompt; the workstation's body (or `promptFile`) becomes the user message, rendered with Go template variables from the work token.

## File Format

Every `AGENTS.md` file has the same structure:

```
---
<YAML frontmatter>
---

<Markdown body>
```

- Frontmatter is delimited by `---` on its own line (opening and closing).
- The markdown body follows the closing `---`.
- Frontmatter fields vary by type (see sections below).

## Worker AGENTS.md

Workers live under `factory/workers/{worker-name}/AGENTS.md`. A worker defines the execution backend — which model or script performs the work.

### Worker Types

#### MODEL_WORKER

An LLM-backed worker. The markdown body is the **system prompt** sent to the model.

```yaml
---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
executorProvider: script_wrap
resources:
  - name: agent-slot
    capacity: 1
timeout: 1h
stopToken: "<result>ACCEPTED</result>"
---

You are a software engineer. Implement the requested changes,
write tests, and ensure all quality checks pass.
```

#### SCRIPT_WORKER

A shell-command worker. The markdown body is a description (not executed).

```yaml
---
type: SCRIPT_WORKER
command: ./scripts/deploy.sh
args: ["--env", "staging", "--work-id", "{{ (index .Inputs 0).WorkID }}"]
timeout: 5m
---

Deployment worker. Runs the staging deploy script.
```

### Worker Frontmatter Fields

| Field | Type | Required | Applies to | Description |
|-------|------|----------|------------|-------------|
| `type` | string | yes | all | `MODEL_WORKER` or `SCRIPT_WORKER` |
| `model` | string | no | MODEL_WORKER | LLM model identifier (e.g., `claude-sonnet-4-20250514`) |
| `modelProvider` | string | no | MODEL_WORKER | Model-provider identifier used for model routing and provider diagnostics (for example `claude`) |
| `executorProvider` | string | no | MODEL_WORKER | Executor adapter identifier used to select the worker execution wrapper (for example `script_wrap`) |
| `command` | string | no | SCRIPT_WORKER | Shell command to execute |
| `args` | string[] | no | SCRIPT_WORKER | Command arguments (supports Go template syntax) |
| `resources` | `{name, capacity}[]` | no | all | Resource requirements this worker declares (e.g., `[{ name: "agent-slot", capacity: 1 }]`) |
| `timeout` | duration | no | all | Max execution time (e.g., `1h`, `30m`, `5m`) |
| `stopToken` | string | no | MODEL_WORKER | Token that signals task completion in model output |

Current built-in `modelProvider` values are `claude` and `codex`. The current
public `executorProvider` value is `script_wrap`. The canonical source of truth
for these worker-contract values is the `Worker` schema in
[`api/openapi.yaml`](../api/openapi.yaml).

## Timeout And Failure Behavior

Use this section when you need to decide whether a failure should terminate work, retry, or pause a provider lane.

### Which Setting Controls Timeouts

Executable workers resolve their per-attempt timeout from the loaded runtime config in this order:

1. workstation `limits.maxExecutionTime`
2. worker `timeout`
3. default subprocess fallback `2h`

Use `limits.maxExecutionTime` for new workstation configuration and `timeout` for worker configuration. Legacy workstation top-level `timeout` and older snake_case execution-limit keys are accepted only as migration aliases at the load boundary, then normalized into `limits.maxExecutionTime`. If both workstation `timeout` and `limits.maxExecutionTime` are present, `limits.maxExecutionTime` wins.

Generated Agent Factory workers use `timeout: 1h` as the starter configuration. If no timeout is configured, or the configured timeout parses to `0`, the runtime still applies the bounded `2h` subprocess fallback instead of running without a deadline.

Example workstation override:

```yaml
---
type: MODEL_WORKSTATION
worker: swe
limits:
  maxExecutionTime: 30m
---
```

Example worker override:

```yaml
---
type: SCRIPT_WORKER
command: ./scripts/deploy.sh
timeout: 30m
---
```

When a deadline expires, the running worker is cancelled and the result is classified as `execution timeout` with normalized timeout metadata.

### Minimal Worker Example

Only `type` is strictly required. All other fields have defaults or are optional:

```yaml
---
type: MODEL_WORKER
---

You are a helpful assistant.
```

## Workstation AGENTS.md

Workstations live under `factory/workstations/{workstation-name}/AGENTS.md`. A workstation defines the task instructions — the prompt template that tells a worker what to do with a specific work item.

### Runtime Workstation Types

#### MODEL_WORKSTATION

Pairs with a worker to execute LLM tasks. The markdown body is the **prompt template**, rendered with Go template variables from the input work token.

```yaml
---
type: MODEL_WORKSTATION
worker: swe
limits:
  maxExecutionTime: 30m
  maxRetries: 3
---

You are reviewing a code change.

## Work Item

Work ID: {{ (index .Inputs 0).WorkID }}
Type: {{ (index .Inputs 0).WorkTypeID }}

## Request

{{ (index .Inputs 0).Payload }}

{{ if gt (index .Inputs 0).History.AttemptNumber 1 }}
## Previous Attempt

This is attempt {{ (index .Inputs 0).History.AttemptNumber }}.
Last error: {{ (index .Inputs 0).History.LastError }}

Previous output:
{{ (index .Inputs 0).PreviousOutput }}
{{ end }}
```

#### LOGICAL_MOVE

A passthrough that moves tokens without calling any worker. Useful for aggregation points or routing.

```yaml
---
type: LOGICAL_MOVE
---

Aggregation point. Collects completed work items.
```

### Workstation Frontmatter Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `MODEL_WORKSTATION` or `LOGICAL_MOVE` |
| `worker` | string | no | Name of the worker directory under `factory/workers/` |
| `promptFile` | string | no | Path to an external prompt template file (relative to workstation dir) |
| `outputSchema` | string | no | JSON schema string for structured output parsing |
| `limits.maxRetries` | int | no | Maximum retry attempts |
| `limits.maxExecutionTime` | duration | no | Maximum execution time per attempt. Legacy top-level `timeout` and snake_case aliases are migration-only inputs |
| `stopWords` | string[] | no | Ordered workstation stop markers for accept-or-fail handling. Missing markers currently follow the failure path. Legacy singular workstation stop aliases are migration-only inputs and canonical serialization rewrites them here |

### External Prompt Files

For large or shared prompts, use `promptFile` instead of inlining the template in the body:

```yaml
---
type: MODEL_WORKSTATION
worker: swe
promptFile: prompt.md
---

This body is ignored when `promptFile` is set.
```

The file `prompt.md` (in the same directory as `AGENTS.md`) contains the Go template.

## Prompt Template Variables

Workstation prompt bodies (and `promptFile` contents) are rendered using Go's `text/template` package. Variables come from the input work token(s) and workflow context.

### Input Token Fields

Read token data through `.Inputs`. Single-input workstations usually read
`(index .Inputs 0)`. Multi-input workstations should choose the required input
position explicitly.

| Variable | Type | Description |
|----------|------|-------------|
| `{{ (index .Inputs 0).WorkID }}` | string | Unique work item identifier |
| `{{ (index .Inputs 0).WorkTypeID }}` | string | Work type (e.g., `request`, `story`) |
| `{{ (index .Inputs 0).TraceID }}` | string | Trace correlation ID |
| `{{ (index .Inputs 0).ParentID }}` | string | Parent work ID (if spawned) |
| `{{ (index .Inputs 0).Payload }}` | string | Raw payload content |
| `{{ (index .Inputs 0).Tags }}` | map | Metadata key-value pairs; access a key with `{{ index (index .Inputs 0).Tags "key" }}` |
| `{{ (index .Inputs 0).PreviousOutput }}` | string | Output from previous attempt (from `Tags["_last_output"]`) |
| `{{ (index .Inputs 0).RejectionFeedback }}` | string | Feedback from rejection (from `Tags["_rejection_feedback"]`) |

### History Fields

Access history through the input token:

| Variable | Type | Description |
|----------|------|-------------|
| `{{ (index .Inputs 0).History.AttemptNumber }}` | int | Current attempt (1-indexed) |
| `{{ (index .Inputs 0).History.TotalVisits }}` | int | Total transitions this token has fired |
| `{{ (index .Inputs 0).History.FailureCount }}` | int | Total failures across all transitions |
| `{{ (index .Inputs 0).History.LastError }}` | string | Error from most recent failure |
| `{{ (index .Inputs 0).History.FailureLog }}` | []FailureRecord | Ordered log of all failures |

### Context Fields

Access via `{{ .Context.FieldName }}`:

| Variable | Type | Description |
|----------|------|-------------|
| `{{ .Context.WorkDir }}` | string | Working directory for execution |
| `{{ .Context.Project }}` | string | Explicit dispatch/factory project context, first work-input project tag, or `default-project` |
| `{{ .Context.ArtifactDir }}` | string | Output artifacts directory |
| `{{ .Context.Env }}` | map | Environment variables — access with `{{ index .Context.Env "VAR" }}` |

### Multi-Input Access

When a transition consumes multiple tokens, use `.Inputs`:

```
{{ range $i, $input := .Inputs }}
Input {{ $i }}: {{ $input.WorkID }} — {{ $input.Payload }}
{{ end }}
```

Or access by index:

```
First input: {{ (index .Inputs 0).Payload }}
Second input: {{ (index .Inputs 1).Payload }}
```

For the full variable reference, see [prompt-variables.md](./prompt-variables.md).

Older snake_case frontmatter aliases remain compatibility-only input during migration. Canonical examples and preferred configs should use the camelCase keys above.

## How Workers and Workstations Compose

At runtime, when a transition fires:

1. The factory loads the **workstation** `AGENTS.md` from the transition's `workstation` field.
2. The workstation's `worker` field identifies which **worker** `AGENTS.md` to load.
3. The worker's body becomes the **system prompt**.
4. The workstation's prompt template is rendered with token data to become the **user message**.
5. Both are sent to the worker's configured model/provider.

```
┌─────────────────────┐     ┌──────────────────────────┐
│  Worker AGENTS.md   │     │  Workstation AGENTS.md   │
│  (system prompt)    │     │  (prompt template)       │
│                     │     │                          │
│  "You are a code    │     │  "Review this change:    │
│   reviewer..."      │     │   {{ (index .Inputs 0).Payload }}"        │
└────────┬────────────┘     └────────────┬─────────────┘
         │                               │
         │    ┌──────────────────┐       │
         └───>│  LLM Request     │<──────┘
              │  system: worker  │
              │  user: rendered  │
              │       template   │
              └──────────────────┘
```

If no workstation prompt template is configured, the first input token's payload is used directly as the user message.

## Factory Mapping

In the current `factory.json` contract, workstation entries map directly to
workstation directories and the `worker` field maps to the worker directory:

```json
{
  "name": "review-story",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

In a review-loop factory such as `examples/simple-tasks/`, this maps to
`examples/simple-tasks/workstations/review-story/AGENTS.md`. The workstation's
`worker` field maps to `examples/simple-tasks/workers/reviewer/AGENTS.md`.

## Common Patterns

### Retry-Aware Prompts

Use history fields to give the model context about previous failures:

```
{{ if gt (index .Inputs 0).History.AttemptNumber 1 }}
This is retry attempt {{ (index .Inputs 0).History.AttemptNumber }}.
The previous attempt failed: {{ (index .Inputs 0).History.LastError }}

Previous output (for reference):
{{ (index .Inputs 0).PreviousOutput }}

Please fix the issues from the previous attempt.
{{ end }}
```

### Rejection Feedback Loop

When a reviewer rejects work, feedback flows through `RejectionFeedback`:

```
{{ if (index .Inputs 0).RejectionFeedback }}
Your previous submission was rejected:
{{ (index .Inputs 0).RejectionFeedback }}

Please address this feedback.
{{ end }}

Task: {{ (index .Inputs 0).Payload }}
```

### Environment-Aware Instructions

Use context and tags for dynamic behavior:

```
Repository: {{ .Context.WorkDir }}
{{ if .Context.WorkDir }}
Working in directory: {{ .Context.WorkDir }}
{{ end }}

Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

## Existing Examples

| File | Type | Description |
|------|------|-------------|
| `examples/write-code-review/workers/executor/AGENTS.md` | MODEL_WORKER | Review-loop executor worker with structured output requirements |
| `examples/basic/factory/workers/processor/AGENTS.md` | MODEL_WORKER | Minimal single-step example worker |
| `factory/workstations/process/AGENTS.md` | MODEL_WORKSTATION | Checked-in repository-maintainer workstation prompt |
| `examples/simple-tasks/workstations/execute-story/AGENTS.md` | MODEL_WORKSTATION | Review-loop workstation with rejection feedback |

For complete current examples that include workers and workstations, see:
- `factory/` — checked-in repository-maintainer workflow with plan, process, and review stages
- `examples/write-code-review/` — split review loop with canonical camelCase config
- `tests/functional_test/testdata/service_simple/` — checked-in smoke fixture using the public config contract

## See Also

- [Authoring Workflows](./authoring-workflows.md#failure-routing-and-provider-behavior) — how normalized worker failures interact with workflow arcs and retry loops
- [Authoring Workflows](./authoring-workflows.md) — current `factory.json` workflow topology and mock-worker workflow checks
- [Prompt Variables Reference](./prompt-variables.md) — complete variable listing with examples
- [Architecture](./development/architecture.md) — engine design and subsystem details
