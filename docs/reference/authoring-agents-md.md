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

Use this guide for file shape, placement, prompt bodies, prompt files, and
authoring patterns. Use [Workers](workers.md) for the worker contract,
[Workstations](workstations.md) for workstation runtime fields and routing, and
[Factory JSON and work configuration](work.md) for the `factory.json` topology.

## Overview

The agent factory uses two kinds of `AGENTS.md` files:

| Kind | Location | Purpose |
|------|----------|---------|
| **Worker** | `factory/workers/{name}/AGENTS.md` | Holds worker-owned backend settings and the system prompt |
| **Workstation** | `factory/workstations/{name}/AGENTS.md` | Holds workstation-owned prompt instructions and optional prompt-file wiring |

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
- Frontmatter fields vary by type. See [Workers](workers.md) and
  [Workstations](workstations.md) for the canonical field lists.

## Worker AGENTS.md

Workers live under `factory/workers/{worker-name}/AGENTS.md`. The markdown
body is the system prompt for a model worker, or descriptive text for a script
worker. Keep the complete worker field contract in [Workers](workers.md).

Minimal model worker:

```yaml
---
type: MODEL_WORKER
---

You are a helpful assistant.
```

Script worker with a short descriptive body:

```yaml
---
type: SCRIPT_WORKER
command: ./scripts/deploy.sh
args: ["--env", "staging", "--work-id", "{{ (index .Inputs 0).WorkID }}"]
timeout: 5m
---

Deployment worker. Runs the staging deploy script.
```

Use [Workers](workers.md#worker-owned-vs-workstation-owned-fields) when you
need to decide whether a field belongs on the worker or the workstation.

## Workstation AGENTS.md

Workstations live under `factory/workstations/{workstation-name}/AGENTS.md`.
The markdown body is the prompt template that tells the bound worker what to do
with the current work token. Keep routing fields, runtime kinds, execution
limits, stop words, and logical moves in [Workstations](workstations.md).

Prompt-backed workstation:

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

Workstation prompt bodies and `promptFile` contents are rendered with Go's
`text/template` package. Use the common variables in examples, and use
[Templates](./templates.md) for the complete supported
surface.

Single-input workstations usually read from the first token:

```
Work ID: {{ (index .Inputs 0).WorkID }}
Task: {{ (index .Inputs 0).Payload }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

History and context values are useful when prompts need retry-aware or
environment-aware instructions:

```
Attempt: {{ (index .Inputs 0).History.AttemptNumber }}
Last error: {{ (index .Inputs 0).History.LastError }}
Repository: {{ .Context.WorkDir }}
```

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

For the full variable reference and template-surface contract, see
[Templates](./templates.md). Older snake_case
frontmatter aliases remain compatibility-only input during migration. Canonical
examples and preferred configs should use camelCase keys.

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

In `factory.json`, workstation entries map directly to workstation directories
and the `worker` field maps to the worker directory. The exact workstation
routing fields belong in [Workstations](workstations.md).

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

- [Workers](workers.md) - worker field contract and backend settings
- [Workstations](workstations.md) - workstation runtime fields, routing, and logical moves
- [Authoring Factories](./authoring-factories.md) - factory sequencing and mock-worker checks
- [Templates](./templates.md) - complete variable listing, supported surfaces, and quoting examples
- [Architecture](../internal/development/architecture.md) - engine design and subsystem details
