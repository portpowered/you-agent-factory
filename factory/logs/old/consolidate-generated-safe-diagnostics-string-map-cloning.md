# Cleanup Idea: Consolidate Generated Safe-Diagnostics String-Map Cloning

## Why this cleanup exists

Current `main` already consolidated most safe-diagnostics clone ownership in
`pkg/interfaces`, including the recent merge that routed
`safeDiagnosticsStringMapPtr` through the canonical `cloneStringMap` helper.

One narrow duplicate path still remains in the same package:

- `pkg/interfaces/safe_diagnostics.go` `safeDiagnosticsStringMapValue`
  hand-copies `factoryapi.StringMap`
- `pkg/interfaces/world_state_clones.go` already owns the canonical
  nil-preserving detached `map[string]string` clone helper through
  `cloneStringMap`

This leaves one more parallel map-copy implementation in an area that has
otherwise been consolidating around the package-level clone owner.

## Requested change

Remove the remaining handwritten generated-safe-diagnostics string-map copy and
route that conversion path through the existing canonical clone helper.

Keep the cleanup narrow:

- do not change safe-diagnostics filtering behavior
- do not change exported interfaces or generated API contracts
- do not broaden this into unrelated diagnostics or event-history refactors
- preserve nil-in, nil-out semantics and detached map copies for generated map
  values

## Relevant files

- `pkg/interfaces/safe_diagnostics.go`
- `pkg/interfaces/safe_diagnostics_test.go`
- `pkg/interfaces/world_state_clones.go`

## Acceptance criteria

- `safeDiagnosticsStringMapValue` no longer maintains a handwritten loop for
  cloning generated safe-diagnostics maps.
- The generated-to-canonical safe-diagnostics path reuses the existing
  package-level clone owner.
- Generated safe-diagnostics map conversion still preserves detached-copy
  behavior and nil handling.
- Existing `pkg/interfaces` tests continue to prove the observable behavior
  without any public behavior changes.

## Review guidance

Prefer behavioral proof in `pkg/interfaces/safe_diagnostics_test.go` rather
than meta tests. The key regression risk is accidental aliasing or changed nil
handling when converting generated safe-diagnostics maps back into canonical
structures.
