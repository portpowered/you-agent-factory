---
behavior: standard
type: MODEL_WORKSTATION
worker: reviewer
---


## Review: {{ (index .Inputs 0).WorkID }}

### Story Requirements

{{ (index .Inputs 0).Payload }}

### Previous Output

{{ (index .Inputs 0).PreviousOutput }}

{{ if .Context.WorkDir }}
## Git Worktree

Reviewing changes in worktree: {{ .Context.WorkDir }}
{{ end }}

## Working Directory

{{ .Context.WorkDir }}

## Instructions

Review the most recent commit(s) against the acceptance criteria above. Run quality checks. Accept or reject.
