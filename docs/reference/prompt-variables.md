# Prompt Template Variables

---
author: ralph (agent)
last-modified: 2026-03-19
doc-id: agent-factory/prompt-variables
---

This document lists all variables available in workstation prompt templates.
Prompts are rendered using Go's `text/template` package. Variables are accessed
through two roots:

- `.Inputs` for token data consumed by the transition.
- `.Context` for workflow and execution context.

## Input Token Fields

Each consumed token is available by position in `.Inputs`. For a single-input
transition, use `{{ (index .Inputs 0).FieldName }}`. For multi-input
transitions, choose the input position intentionally.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `.Inputs` | `[]TokenData` | Per-input-token data (see below) | `{{ (index .Inputs 0).Payload }}` |
| `(index .Inputs N).Name` | `string` | Human-readable work name for the Nth input token | `US-001` |
| `(index .Inputs N).WorkID` | `string` | Unique identifier for the Nth input token | `work-task-42` |
| `(index .Inputs N).WorkTypeID` | `string` | Work type for the Nth input token | `task` |
| `(index .Inputs N).DataType` | `string` | Token data type, such as `work` or `resource` | `work` |
| `(index .Inputs N).TraceID` | `string` | Trace correlation ID across transitions | `api-001` |
| `(index .Inputs N).ParentID` | `string` | Work ID of the parent token, when spawned | `work-chapter-1` |
| `(index .Inputs N).Project` | `string` | Project resolved for the token | `billing-api` |
| `(index .Inputs N).Payload` | `string` | Raw payload content as a string | `{"title":"review PR"}` |
| `(index .Inputs N).Tags` | `map[string]string` | Arbitrary metadata attached to the token | `{"env":"prod"}` |
| `(index .Inputs N).Relations` | `[]Relation` | Dependency and parent-child relations | see Relations section |
| `(index .Inputs N).PreviousOutput` | `string` | Output from the previous execution attempt | `partial result...` |
| `(index .Inputs N).RejectionFeedback` | `string` | Feedback from the previous rejection | `Missing section X` |

## History Fields

Access history through the input token that owns the attempt history, such as
`{{ (index .Inputs 0).History.AttemptNumber }}`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `(index .Inputs N).History.AttemptNumber` | `int` | Current attempt number, 1-indexed | `1` (first), `2` (retry) |
| `(index .Inputs N).History.TotalVisits` | `int` | Total number of transitions this token has fired | `3` |
| `(index .Inputs N).History.FailureCount` | `int` | Total number of failures across all transitions | `2` |
| `(index .Inputs N).History.LastError` | `string` | Error message from the most recent failure | `execution timeout` |
| `(index .Inputs N).History.FailureLog` | `[]FailureRecord` | Ordered log of all failures | see FailureLog section |

## Context Fields

Access via `{{ .Context.FieldName }}`.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `.Context.WorkDir` | `string` | Working directory for the execution | `/workspace/project` |
| `.Context.ArtifactDir` | `string` | Directory for output artifacts | `/workspace/.artifacts` |
| `.Context.Project` | `string` | Explicit dispatch/factory project context, first work-input project tag, or `default-project` | `billing-api` |
| `.Context.Env` | `map[string]string` | Environment variables available to the executor | `{"API_KEY":"..."}` |

## Relations

Each token's `.Relations` field is a slice of `Relation` structs with these fields:

| Field | Description |
|-------|-------------|
| `.Type` | Relation type: `DEPENDS_ON`, `PARENT_CHILD`, or `SPAWNED_BY` |
| `.TargetWorkID` | WorkID of the related work item |
| `.RequiredState` | State the target must be in (e.g., `"complete"`) |

## Tags Access

Tags are a `map[string]string`. Use `index` to access them safely:

```
{{ index (index .Inputs 0).Tags "my_key" }}
```

`(index .Inputs N).Project` resolves the project for that specific token: the
token's `project` tag wins, then explicit context, then the neutral
`default-project` value.

`.Context.Project` resolves dispatch-level project context. An explicit
dispatch, factory, or workflow project wins. If no explicit context exists, the
renderer falls back to the first non-resource input token with a `project` tag.
Resource tokens never supply `.Context.Project`. If neither source exists,
templates use `default-project`.

Reserved tag keys used internally:

| Key | Description |
|-----|-------------|
| `_last_output` | Stored output from the previous attempt, exposed as `(index .Inputs N).PreviousOutput` |
| `_rejection_feedback` | Feedback from rejection, exposed as `(index .Inputs N).RejectionFeedback` |

## Example Prompt Snippets

### Basic work item reference

```
You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

Payload: {{ (index .Inputs 0).Payload }}
```

### Retry-aware prompt using attempt number

```
This is attempt {{ (index .Inputs 0).History.AttemptNumber }} to complete this task.
{{ if gt (index .Inputs 0).History.AttemptNumber 1 }}
Previous attempt failed with: {{ (index .Inputs 0).History.LastError }}
Previous output was:
{{ (index .Inputs 0).PreviousOutput }}

Please fix the issues from the previous attempt.
{{ end }}
```

### Using tags for dynamic context

```
Environment: {{ index (index .Inputs 0).Tags "env" }}
Project: {{ .Context.Project }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}

Process the following task:
{{ (index .Inputs 0).Payload }}
```

### Referencing working directory

```
The repository is located at: {{ .Context.WorkDir }}
Project: {{ .Context.Project }}
```

### Fan-out child with parent reference

```
Parent work item: {{ (index .Inputs 0).ParentID }}
Your task: {{ (index .Inputs 0).Payload }}
```

### Using rejection feedback to improve output

```
{{ if (index .Inputs 0).RejectionFeedback }}
Your previous attempt was rejected with the following feedback:
{{ (index .Inputs 0).RejectionFeedback }}

Please address this feedback in your response.
{{ end }}

Task: {{ (index .Inputs 0).Payload }}
```

## Per-Input Token Access

When a transition consumes multiple input tokens, each token's data is available
via the `.Inputs` slice. Each entry is a `TokenData` with token fields such as
`WorkID`, `Payload`, `Tags`, and `History`.

| Field | Type | Description |
|-------|------|-------------|
| `.Inputs` | `[]TokenData` | Slice of per-input-token data objects |
| `(index .Inputs N).WorkID` | `string` | WorkID of the Nth input token |
| `(index .Inputs N).Payload` | `string` | Payload of the Nth input token |
| `(index .Inputs N).Tags` | `map[string]string` | Tags of the Nth input token |
| `(index .Inputs N).History` | `PromptHistory` | History of the Nth input token |

### Example: Multi-input transition

```
PRD task: {{ (index .Inputs 0).Payload }}
Review feedback: {{ (index .Inputs 1).Payload }}
Reviewer: {{ index (index .Inputs 1).Tags "reviewer" }}
```

### Example: Iterating over all inputs

```
{{ range $i, $input := .Inputs }}
Input {{ $i }}: {{ $input.WorkID }} — {{ $input.Payload }}
{{ end }}
```

## See Also

- `pkg/workers/prompt.go` — `PromptData`, `PromptHistory`, `PromptContext` struct definitions
- `pkg/workers/workstation_executor.go` — how prompts are rendered and dispatched
- `docs/reference/authoring-workflows.md` — workflow and workstation authoring guide
