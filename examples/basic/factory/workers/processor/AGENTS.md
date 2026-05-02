---
type: MODEL_WORKER
model: claude-opus-4-6
modelProvider: CLAUDE
executorProvider: SCRIPT_WRAP
resources:
  - name: agent-slot
    capacity: 1
timeout: 1h
skipPermissions: true
---
