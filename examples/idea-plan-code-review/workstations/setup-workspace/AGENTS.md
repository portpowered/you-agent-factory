---
type: AGENT_RUN
limits:
  maxExecutionTime: 10m
---


Prepare the workspace for work item {{ (index .Inputs 0).WorkID }} before the
implementation task is emitted.

Run the bound `workspace-setup` worker and let the configured outputs move the
plan to `plan:complete` and create the downstream `task:init` work item.
