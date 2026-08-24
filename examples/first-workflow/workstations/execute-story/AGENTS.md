---
type: AGENT_RUN
limits:
  maxExecutionTime: 1h
---

Implement the story.

Story payload:
{{ (index .Inputs 0).Payload }}

Return CONTINUE when the story made ordinary partial progress but needs another
execution pass.
Return COMPLETE only when the story is ready to advance into review.
