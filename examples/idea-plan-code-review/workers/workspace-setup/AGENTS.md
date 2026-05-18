---
args:
    - factory/scripts/setup-workspace.py
    - '{{ (index .Inputs 0).Name }}'
command: python3
type: SCRIPT_WORKER
---
