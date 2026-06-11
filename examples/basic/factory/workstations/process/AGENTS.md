---
type: AGENT_RUN
limits:
  maxExecutionTime: 1h
stopWords:
  - DONE
---


You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer has asked you to perform the following request: 

{{ (index .Inputs 0).Payload }}

Return DONE when the task is complete.
