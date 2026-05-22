---
author: Agent Factory Team
last-modified: 2026-05-21
doc-id: agent-factory/reference/templates
---

# Templates

you-agent-factory renders workstation prompts, workstation runtime fields, and
script-worker arguments with Go `text/template` syntax. Template data comes
from consumed input tokens under `.Inputs` and execution context under
`.Context`.

Use `docs/reference/templates.md` as the maintained canonical guide for the
full template variable inventory, supported surfaces, and JSON-versus-Markdown
quoting rules.

## Common Variables

| Variable | Description |
|----------|-------------|
| `{{ (index .Inputs 0).WorkID }}` | Work ID of the first non-resource input token. |
| `{{ (index .Inputs 0).WorkTypeID }}` | Work type of the input token. |
| `{{ (index .Inputs 0).Payload }}` | Submitted payload content. |
| `{{ index (index .Inputs 0).Tags "branch" }}` | Work tag value. |
| `{{ .Context.Project }}` | Explicit project context, first work-input `project` tag, or `default-project`. |
| `{{ .Context.WorkDir }}` | Working directory for the execution. |
| `{{ .Context.ArtifactDir }}` | Artifact output directory. |
| `{{ .Context.Env }}` | Environment map visible to the executor. |

## Where Templates Apply

- Workstation prompt bodies in `workstations/<name>/AGENTS.md`
- Inline `promptTemplate` values
- Workstation `workingDirectory`, `worktree`, and `env` values
- Script-worker `args`

## Variable Roots

- `.Inputs` is the token-data root for payloads, tags, relations, retry
  history, and per-input metadata.
- `.Context` is the execution-context root for project, working directory,
  artifact directory, and environment values.

## Quoting Rules

In JSON strings, escape the quotes inside template expressions:

```json
{
  "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}"
}
```

In Markdown `AGENTS.md`, use normal quotes:

```text
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

## Retry-Aware Example

```text
This is attempt {{ (index .Inputs 0).History.AttemptNumber }}.

{{ if (index .Inputs 0).RejectionFeedback }}
Previous rejection feedback:
{{ (index .Inputs 0).RejectionFeedback }}
{{ end }}

Task:
{{ (index .Inputs 0).Payload }}
```

## Related

- `you docs workstation`
- `you docs workers`
- `you docs batch-work`
