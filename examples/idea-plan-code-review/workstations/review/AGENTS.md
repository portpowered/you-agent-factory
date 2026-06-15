---
type: AGENT_RUN
limits:
  maxExecutionTime: 30m
---


You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

Please check the PR on github.com for the current project, that is relative to the work item named {{ (index .Inputs 0).Name }}.

If you think that the PR is correct, then comment as such on the PR. 

Please review the appropriate standards/docs for the PR. 

After you are complete, please respond exactly with <COMPLETE>.
If you are unhappy with the PR, or if its invalid, submit back the response. 
