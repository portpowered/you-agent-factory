---
type: SCRIPT_WORKER
command: bash
args: ["scripts/poll-github.sh"]
timeout: 2m
---

Poll GitHub and emit one canonical batch payload on stdout per run.
