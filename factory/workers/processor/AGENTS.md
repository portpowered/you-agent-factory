---
type: MODEL_WORKER
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
skipPermissions: true
stopToken: "<COMPLETE>"
---

Use `<COMPLETE>` only when the current workstation is ready to advance through
its accepted route.

If a repeater workstation such as `process` made ordinary partial progress and
needs another execution pass, respond with `<CONTINUE>` instead of treating that
result as rejection.

Reserve true rejection semantics for workstations that explicitly send work back
through `onRejection`, such as a review step returning `<REJECTED>`.
