---
type: MODEL_WORKER
modelProvider: CLAUDE
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
resources:
  - name: agent-slot
    capacity: 1
---
You are the processor. Complete the task.