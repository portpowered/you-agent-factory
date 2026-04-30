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
| `pkg/config/exhaustion_rule_contract_guard_test.go` | `walkProductionPkgFiles` | `filepath.WalkDir` | `pkg/` via `filepath.Clean("..")` from `pkg/config` | `handwritten pkg subtree` | Explicitly skipped via shared hidden-dir policy | Explicitly skips `pkg/api/generated` | Broad handwritten `pkg/` scan hardened against hidden metadata and unrelated generated output. |
| `pkg/interfaces/runtime_lookup_contract_guard_test.go` | `scanRuntimeLookupContractViolations` | `filepath.WalkDir` | package-local `pkg/interfaces` via `..` in production mode; temp fixture root in unit tests | `handwritten package source` | Not intentionally scanned in production because the root is package-local | No generated-output paths under the package-local scope | Production scan is package-local rather than repo-wide. |
| `pkg/interfaces/world_view_contract_guard_test.go` | `TestFactoryWorldContractGuard_RetiredBoundaryMirrorNamesStayOutOfInterfacesGoFiles` | `filepath.Walk` | `.` from `pkg/interfaces` | `handwritten package source` | Not intentionally scanned in production because the root is package-local | No generated-output paths under the package-local scope | Package-local boundary-name guard for `pkg/interfaces` only. |
| `pkg/interfaces/world_view_contract_guard_test.go` | `TestFactoryWorldContractGuard_RetiredCanonicalMirrorNamesStayOutOfPkgGoFiles` | `filepath.Walk` | `pkg/` via `..` from `pkg/interfaces` | `handwritten pkg subtree` | Explicitly skipped via shared hidden-dir policy | Explicitly skips `pkg/api/generated` | Broader pkg-tree scan now matches the handwritten-source skip policy used by the other broad walkers. |
| `pkg/petri/transition_contract_guard_test.go` | `TestTransitionContractGuard_ProductionTransitionLiteralsStayTopologyOnly` | `filepath.WalkDir` | repository root via `filepath.Join("..", "..")` from `pkg/petri` | `handwritten repo source` | Explicitly skips hidden metadata/worktree directories via shared hidden-dir policy | Explicitly skips `pkg/api/generated`, `ui/dist`, `ui/node_modules`, and `ui/storybook-static` | Broadest active scan; hardened so repository metadata stays out of scope while preserving the existing build-output exclusions. |

## Hardening Status

- Every active handwritten-source broad walker in `pkg/*_contract_guard_test.go`
  now opts out of hidden metadata/worktree directories explicitly.
- `pkg/` subtree walkers skip `pkg/api/generated` unless a guard intentionally
  targets generated output.
- The repository-root `pkg/petri` walker preserves its existing UI build-output
  skips while adding the same hidden-directory policy used by the `pkg/` scans.

## Operator Contract

- Treat repository-root paths as canonical for this checkout. Run guard
  commands from the repository root and scope broad handwritten-source sweeps to
  live surfaces such as `pkg/`, `docs/`, `factory/`, and
  `tests/functional_test/`.
- Use this inventory as the checked-in cleanup reference for active broad
  walker scope, hidden-directory policy, and generated-output exclusions before
  changing any `pkg/*_contract_guard_test.go` scan roots.
- References to `libraries/agent-factory` in archival reports, replay
  artifacts, or historical notes are preserved evidence from older layouts, not
  the live operator contract for this repository.
