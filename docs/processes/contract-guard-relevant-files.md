# Contract Guard Relevant Files

This file inventories the broad contract guards that scan filesystem roots with `filepath.Walk` or `filepath.WalkDir`.

## Broad walker inventory

| Guard file | Walk API | Walk root | Surface guarded | Current skip policy | Status in this cleanup lane |
| --- | --- | --- | --- | --- | --- |
| `pkg/config/exhaustion_rule_contract_guard_test.go` | `filepath.WalkDir` | `pkg/` via `..` from `pkg/config` | Handwritten production Go source under `pkg/` | Skips `pkg/api/generated` only | Unresolved first-class case for `US-002`; hidden metadata skips are still missing |
| `pkg/interfaces/runtime_lookup_contract_guard_test.go` | `filepath.WalkDir` | `pkg/` via `..` from `pkg/interfaces` | Handwritten Go source under `pkg/` for runtime lookup ownership | No directory skips; package-local scan of `pkg/` only | No additional cleanup required in this lane |
| `pkg/interfaces/world_view_contract_guard_test.go` | `filepath.Walk` | `pkg/interfaces` via `.` and `pkg/` via `..` | Handwritten Go source for retired world-view mirror names | No directory skips; one package-local scan and one broad `pkg/` scan | No additional cleanup required in this lane |
| `pkg/petri/transition_contract_guard_test.go` | `filepath.WalkDir` | Repository root via `../..` from `pkg/petri` | Handwritten production Go source across the repo | Skips `pkg/api/generated`, `ui/dist`, `ui/node_modules`, and `ui/storybook-static` | Already accounts for generated surfaces; no additional cleanup required in this lane |

## Notes for future iterations

- `pkg/config/exhaustion_rule_contract_guard_test.go` is the only broad handwritten-source guard in this inventory that still needs repository-model hardening in the current PRD.
- The inventory separates handwritten-source scans from generated-output exclusions so the next story can decide skip-policy ownership explicitly in code instead of by maintainer memory.
