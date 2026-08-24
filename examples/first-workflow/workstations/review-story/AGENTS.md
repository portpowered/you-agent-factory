---
type: AGENT_RUN
limits:
  maxExecutionTime: 30m
---

Review the story implementation.

Story payload:
{{ (index .Inputs 0).Payload }}

Return ACCEPTED when the story is ready.
Return REJECTED with concrete feedback when another pass is needed.
