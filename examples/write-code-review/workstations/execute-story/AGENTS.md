---
behavior: repeater
type: MODEL_WORKSTATION
worker: executor
---


## Story: {{ (index .Inputs 0).WorkID }}

{{ (index .Inputs 0).Payload }}

{{ if (index .Inputs 0).RejectionFeedback }}
## Reviewer Feedback (Previous Attempt)

Your previous implementation was rejected with the following feedback:

{{ (index .Inputs 0).RejectionFeedback }}

This is attempt {{ (index .Inputs 0).History.AttemptNumber }}. Address the reviewer's feedback in your implementation.
{{ end }}

## Working Directory

{{ .Context.WorkDir }}

{{ if .Context.WorkDir }}
## Git Worktree

You are working in an isolated worktree at: {{ .Context.WorkDir }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}
{{ end }}

## Tags

{{ range $key, $value := (index .Inputs 0).Tags }}
- {{ $key }}: {{ $value }}
{{ end }}

## Instructions

Implement this story following the project standards. Run all quality checks before committing.

This is a **repeater workstation** — you will be re-invoked until your implementation passes all checks
and you output `<result>ACCEPTED</result>`. Each invocation is a fresh attempt. Fix any issues from
previous attempts (visible in rejection feedback above) before proceeding.
