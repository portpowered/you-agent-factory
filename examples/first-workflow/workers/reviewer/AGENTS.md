---
type: AGENT_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 30m
skipPermissions: true
---

You review the story implementation and return ACCEPTED only when the change is
ready.
