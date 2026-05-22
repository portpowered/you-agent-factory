# Templates

Use this page when you need the supported Go-template surfaces, the full
template variable inventory, and the quoting rules that differ between
Markdown and JSON. This guide is the maintained public owner for template
authoring.

## Current Contract

- you-agent-factory renders templates with Go `text/template`.
- Workstation prompt bodies and files referenced by `promptFile` are template
  surfaces.
- Script-worker `args`, workstation `workingDirectory`, workstation `worktree`,
  and workstation `env` values are also template surfaces.
- `.Inputs` is the token-data root. `.Context` is the workflow and execution
  context root.
- Use canonical field names from the current prompt-template contract. Invalid
  template syntax or missing field names fail rendering.

## Supported Surfaces

| Surface | Where authors put it | What it is for |
|---------|----------------------|----------------|
| Prompt body | `workstations/<name>/AGENTS.md` markdown body | Main rendered user message |
| `promptFile` content | File referenced by `promptFile` | External prompt template instead of inline markdown body |
| Script `args` | `workers/<name>/AGENTS.md` frontmatter | Per-dispatch command arguments |
| `workingDirectory` | `factory.json` or workstation frontmatter | Execution working directory |
| `worktree` | `factory.json` or workstation frontmatter | CLI provider worktree path |
| `env` values | `factory.json` or workstation frontmatter | Per-dispatch environment values |

## Variable Roots

| Variable family | Use it for | Example |
|-----------------|------------|---------|
| `.Inputs` | Current work item data such as payload, tags, IDs, relations, and retry history | `{{ (index .Inputs 0).Payload }}` |
| `.Inputs[N].Tags` | Per-token metadata lookups | `{{ index (index .Inputs 0).Tags "branch" }}` |
| `.Inputs[N].History` | Attempt-aware prompts and retries | `{{ (index .Inputs 0).History.AttemptNumber }}` |
| `.Context` | Execution context such as working dir, artifact dir, project, and env | `{{ .Context.WorkDir }}` |

Use `index` for map lookups such as tags and environment values.

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

Each token's `.Relations` field is a slice of `Relation` structs with these
fields:

| Field | Description |
|-------|-------------|
| `.Type` | Relation type: `DEPENDS_ON`, `PARENT_CHILD`, or `SPAWNED_BY` |
| `.TargetWorkID` | WorkID of the related work item |
| `.RequiredState` | State the target must be in (for example `"complete"`) |

## Tags and Project Resolution

Tags are a `map[string]string`. Use `index` to access them safely:

```text
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

## Quoting Rules

Use normal quotes inside Markdown prompt templates and prompt files:

```text
Branch: {{ index (index .Inputs 0).Tags "branch" }}
Project: {{ .Context.Project }}
```

Escape inner quotes when the template appears inside a JSON string such as
`workingDirectory`, `worktree`, `env`, or script `args` examples:

```json
{
  "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}",
  "env": {
    "INPUT_BRANCH": "{{ index (index .Inputs 0).Tags \"branch\" }}"
  }
}
```

That escaping rule matters because JSON strings use double quotes. In Markdown
prompt bodies and prompt files, the template expression can use normal quotes.

## Example Prompt Snippets

### Prompt Body

```text
You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

Payload: {{ (index .Inputs 0).Payload }}
```

### Prompt File

```text
Repository: {{ .Context.WorkDir }}
Project: {{ .Context.Project }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

### Script Args

```yaml
type: SCRIPT_WORKER
command: ["sh", "-lc"]
args:
  - "echo"
  - "{{ (index .Inputs 0).WorkID }}"
  - "{{ index (index .Inputs 0).Tags \"branch\" }}"
```

### Rendered Workstation Fields

```json
{
  "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}",
  "worktree": "worktrees/{{ index (index .Inputs 0).Tags \"branch\" }}",
  "env": {
    "PROJECT_NAME": "{{ .Context.Project }}"
  }
}
```

### Tags and Multi-Input Access

```text
PRD task: {{ (index .Inputs 0).Payload }}
Review feedback: {{ (index .Inputs 1).Payload }}
Reviewer: {{ index (index .Inputs 1).Tags "reviewer" }}
```

### Retry-Aware Prompt

```text
This is attempt {{ (index .Inputs 0).History.AttemptNumber }} to complete this task.
{{ if gt (index .Inputs 0).History.AttemptNumber 1 }}
Previous attempt failed with: {{ (index .Inputs 0).History.LastError }}
Previous output was:
{{ (index .Inputs 0).PreviousOutput }}

Please fix the issues from the previous attempt.
{{ end }}
```

### Rejection Feedback

```text
{{ if (index .Inputs 0).RejectionFeedback }}
Your previous attempt was rejected with the following feedback:
{{ (index .Inputs 0).RejectionFeedback }}

Please address this feedback in your response.
{{ end }}

Task: {{ (index .Inputs 0).Payload }}
```

### Iterating Over Inputs

```text
{{ range $i, $input := .Inputs }}
Input {{ $i }}: {{ $input.WorkID }} - {{ $input.Payload }}
{{ end }}
```

## Minimal Authoring Checklist

- Use `.Inputs` for submitted work data and `.Context` for execution context.
- Use `index` for tag or env map lookups.
- Keep JSON template expressions escaped inside string literals.
- Keep Markdown prompt expressions unescaped and readable.
- Keep template examples on the supported variable roots instead of inventing
  new aliases.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Workstations](workstations.md)
- [Author AGENTS.md](authoring-agents-md.md)
- [Authoring factories](authoring-factories.md)
