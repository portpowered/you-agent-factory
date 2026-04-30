# Contract Guard Walker Inventory

This inventory records every active broad filesystem walk in
`pkg/*_contract_guard_test.go`. It exists so cleanup work can distinguish
package-local scans from broader repository walks and make remaining
hardening targets explicit before changing skip policy.

## Classification Rules

- `handwritten package source`: scans maintained Go source in one package
  directory.
- `handwritten pkg subtree`: scans maintained Go source across `pkg/`.
- `handwritten repo source`: scans maintained Go source across the repository
  root.
- `generated output`: intentionally scans generated artifacts.
- `hidden metadata/worktree state`: intentionally scans hidden directories such
  as `.git`, `.claude`, or nested worktree metadata.

## Active Walker Inventory

| Guard file | Test or helper | Walk API | Scan root | Intended surface | Hidden metadata/worktree state | Generated output policy | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `pkg/config/exhaustion_rule_contract_guard_test.go` | `walkProductionPkgFiles` | `filepath.WalkDir` | `pkg/` via `filepath.Clean("..")` from `pkg/config` | `handwritten pkg subtree` | Not intended, but not yet skipped explicitly | Explicitly skips `pkg/api/generated` only | Remaining hardening target for hidden-dir policy and broader generated-output policy alignment. |
| `pkg/interfaces/runtime_lookup_contract_guard_test.go` | `scanRuntimeLookupContractViolations` | `filepath.WalkDir` | package-local `pkg/interfaces` via `..` in production mode; temp fixture root in unit tests | `handwritten package source` | Not intentionally scanned in production because the root is package-local | No generated-output paths under the package-local scope | Production scan is package-local rather than repo-wide. |
| `pkg/interfaces/world_view_contract_guard_test.go` | `TestFactoryWorldContractGuard_RetiredBoundaryMirrorNamesStayOutOfInterfacesGoFiles` | `filepath.Walk` | `.` from `pkg/interfaces` | `handwritten package source` | Not intentionally scanned in production because the root is package-local | No generated-output paths under the package-local scope | Package-local boundary-name guard for `pkg/interfaces` only. |
| `pkg/interfaces/world_view_contract_guard_test.go` | `TestFactoryWorldContractGuard_RetiredCanonicalMirrorNamesStayOutOfPkgGoFiles` | `filepath.Walk` | `pkg/` via `..` from `pkg/interfaces` | `handwritten pkg subtree` | Not intended, and not yet skipped explicitly | No explicit generated-output skip; `pkg/api/generated` is in scope unless otherwise filtered by file contents | Broader pkg-tree scan that should stay visible to the hardening follow-up. |
| `pkg/petri/transition_contract_guard_test.go` | `TestTransitionContractGuard_ProductionTransitionLiteralsStayTopologyOnly` | `filepath.WalkDir` | repository root via `filepath.Join("..", "..")` from `pkg/petri` | `handwritten repo source` | Not intended, but not yet skipped explicitly | Explicitly skips `pkg/api/generated`, `ui/dist`, `ui/node_modules`, and `ui/storybook-static` | Broadest active scan; already treats selected generated/build outputs as out of scope. |

## Hardening Targets

- `pkg/config/exhaustion_rule_contract_guard_test.go` is the clearest remaining
  gap because it walks `pkg/` and only skips `pkg/api/generated`.
- `pkg/interfaces/world_view_contract_guard_test.go` also contains a broad
  `pkg/` walk whose skip policy is still implicit.
- `pkg/petri/transition_contract_guard_test.go` already skips known generated
  and build-output directories, but it still relies on the repository layout
  rather than an explicit hidden-directory guard.

Future hardening work should keep package-local scans simple and make broad
walkers opt out of hidden metadata and unrelated generated output explicitly.
