---
type: AGENT_RUN
worker: executor
limits:
  maxExecutionTime: 1h
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

## Instructions

Implement this story following the project standards. Run all quality checks before committing.
