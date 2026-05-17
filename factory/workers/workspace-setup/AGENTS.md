---
type: SCRIPT_WORKER
command: python3
args:
  - "factory/scripts/setup-workspace.py"
  - "{{ (index .Inputs 0).Name }}"
---
