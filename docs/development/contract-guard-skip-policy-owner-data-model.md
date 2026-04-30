# Contract-Guard Skip-Policy Owner Data Model

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/reduce-contract-guard-skip-helper-ownership-after-green-conformance-lane`)
- Owner: Codex branch `ralph/reduce-contract-guard-skip-helper-ownership-after-green-conformance-lane`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `pkg/api`, `pkg/petri`, `pkg/interfaces`, `pkg/config`
- Canonical architecture document to update before completion: this artifact

## Reconciled Starting State

- Verified on `2026-04-30`.
- The cleanup branch started from `main` at commit `d3ddedd`, so there were no landed-but-unmerged code edits in the targeted guard files to preserve.
- The only local worktree changes before implementation were untracked `prd.json` and `prd.md`, which are task-management artifacts and not part of the review diff.
- No open pull request or review comments existed for this branch when the cleanup started.

## Chosen Ownership Model

- The canonical owner for the shared handwritten-source directory skip default is `pkg/internal/contractguard.ShouldSkipRelativeDir`, with `ShouldSkipDir` as the root/path convenience wrapper for filesystem walks.
- Later stories must route the targeted broad handwritten-source contract-guard scans through that shared owner in:
  - `pkg/api/legacy_model_guard_test.go`
  - `pkg/petri/transition_contract_guard_test.go`
  - `pkg/interfaces/*contract_guard_test.go`
  - `pkg/config/exhaustion_rule_contract_guard_test.go`
- The helper answers only the shared default question: whether a relative directory should be skipped by the broad scan, including hidden metadata directories.
- Package-specific differences, especially generated-directory exceptions, must stay explicit at the call site as allowlist data or helper arguments rather than being hidden in package-local helper bodies.

## Review Constraints For Later Stories

- Hidden metadata directories must remain excluded from the broad handwritten-source scans covered by this cleanup.
- `pkg/api` and `pkg/petri` currently duplicate the same broad skip list, so they are the first consumers that should delegate to the shared owner.
- `pkg/config` currently has a narrower walker that skips `api/generated`; later consolidation must keep that difference explicit instead of silently inheriting the broader UI-oriented skips.
- `pkg/interfaces` currently uses package-local scan shapes instead of a shared directory policy owner; later stories should route any broad scan through the same owner rather than leave interfaces as a separate policy island.
- Keep the shared owner small enough that reviewers can read the default policy directly from one file.

## Implementation Update

- `pkg/config/exhaustion_rule_contract_guard_test.go` now delegates directory skipping to `contractguard.ShouldSkipDir("..", path, "api/generated")`, keeping `api/generated` reviewable as an explicit package-specific exception.
- `pkg/interfaces/runtime_lookup_contract_guard_test.go` and `pkg/interfaces/world_view_contract_guard_test.go` now use `contractguard.ShouldSkipDir(...)` for their broad filesystem scans, so hidden metadata directory policy no longer drifts separately inside `pkg/interfaces`.
