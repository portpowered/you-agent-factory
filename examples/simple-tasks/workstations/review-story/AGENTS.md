---
type: AGENT_RUN
worker: reviewer
limits:
  maxExecutionTime: 30m
---


## Review: {{ (index .Inputs 0).WorkID }}

### Story Requirements

{{ (index .Inputs 0).Payload }}

### Previous Output

{{ (index .Inputs 0).PreviousOutput }}

## Working Directory

{{ .Context.WorkDir }}

## Instructions

Review the most recent commit(s) against the acceptance criteria above. Run quality checks. Accept or reject.
