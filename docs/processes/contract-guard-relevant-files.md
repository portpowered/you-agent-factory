# Contract Guard Relevant Files

This file inventories the broad contract guards that scan filesystem roots with `filepath.Walk` or `filepath.WalkDir`.

## Broad walker inventory

| Guard file | Walk API | Walk root | Surface guarded | Current skip policy | Status in this cleanup lane |
| --- | --- | --- | --- | --- | --- |
| `pkg/config/exhaustion_rule_contract_guard_test.go` | `filepath.WalkDir` | `pkg/` via `..` from `pkg/config` | Handwritten production Go source under `pkg/` | Package-local helper skips hidden metadata directories and `pkg/api/generated` | `US-002` now owns skip policy explicitly in-package; no broader helper introduced in this lane |
| `pkg/interfaces/runtime_lookup_contract_guard_test.go` | `filepath.WalkDir` | `pkg/` via `..` from `pkg/interfaces` | Handwritten Go source under `pkg/` for runtime lookup ownership | No directory skips; package-local scan of `pkg/` only | No additional cleanup required in this lane |
| `pkg/interfaces/world_view_contract_guard_test.go` | `filepath.Walk` | `pkg/interfaces` via `.` and `pkg/` via `..` | Handwritten Go source for retired world-view mirror names | No directory skips; one package-local scan and one broad `pkg/` scan | No additional cleanup required in this lane |
| `pkg/petri/transition_contract_guard_test.go` | `filepath.WalkDir` | Repository root via `../..` from `pkg/petri` | Handwritten production Go source across the repo | Skips `pkg/api/generated`, `ui/dist`, `ui/node_modules`, and `ui/storybook-static` | Already accounts for generated surfaces; no additional cleanup required in this lane |

## Notes for future iterations

- Broad handwritten-source guards in this lane keep skip-policy ownership package-local unless a shared helper would remove real duplication without hiding per-guard exclusions.
- The inventory separates handwritten-source scans from generated-output exclusions so future guard cleanup can document ownership decisions in code and in this file together.
