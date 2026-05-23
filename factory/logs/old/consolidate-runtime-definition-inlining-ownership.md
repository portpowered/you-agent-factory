# Cleanup Idea: Consolidate Runtime Definition Inlining Ownership

## Why this cleanup exists

The backend still keeps the same runtime-definition inlining walk in multiple
active config assembly paths.

Today:

- `pkg/config/layout.go` `InlineRuntimeDefinitions(...)` clones a factory
  config, walks workers and workstations, loads runtime definitions, and
  applies the same worker and workstation merge semantics onto the cloned
  config.
- `pkg/config/layout.go` `FactoryConfigWithRuntimeDefinitions(...)` repeats the
  same clone and merge walk against a runtime-definition lookup instead of disk.
- `pkg/config/runtime_config.go` `NewLoadedFactoryConfig(...)` repeats the same
  merge ownership again while building the effective runtime config and lookup
  maps.

This is live backend duplication in one subsystem, not dead compatibility code.
Any future change to runtime-definition merge behavior currently has to be
threaded through multiple active owners.

## Requested change

Collapse runtime-definition application onto one canonical package-local helper
path in `pkg/config`.

Keep this cleanup narrow:

- preserve runtime behavior exactly
- do not broaden this into replay-specific merge semantics or runtime lookup
  redesign
- do not introduce a new abstraction layer beyond one private canonical helper
- prefer deleting repeated worker and workstation merge loops instead of adding
  wrappers around each copy
- keep tests behavioral around effective config outcomes rather than helper
  location assertions

Suggested shape:

- Extract one private helper that takes a cloned factory config plus a runtime
  definition lookup and applies worker and workstation runtime definitions in
  place.
- Reuse that helper from `InlineRuntimeDefinitions(...)` after the on-disk
  definitions are loaded into a lookup shape.
- Reuse the same helper from `FactoryConfigWithRuntimeDefinitions(...)`.
- Reuse the same helper inside `NewLoadedFactoryConfig(...)` before the loaded
  runtime lookup maps are populated.
- Preserve the current normalization path for workstation runtime semantics,
  including the existing `applyWorkstationRuntimeDefinition(...)` and
  `normalizeCanonicalWorkstationRuntime(...)` behavior.

## Relevant files

- `pkg/config/layout.go`
- `pkg/config/runtime_config.go`
- `pkg/replay/generated_factory.go`
- `pkg/config/layout_test.go`
- `pkg/config/runtime_config_test.go`

## Acceptance criteria

- Runtime-definition inlining no longer keeps separate worker and workstation
  merge walks across `InlineRuntimeDefinitions(...)`,
  `FactoryConfigWithRuntimeDefinitions(...)`, and `NewLoadedFactoryConfig(...)`.
- `pkg/config` has one canonical package-local owner for applying runtime
  definitions onto a cloned factory config.
- Flatten, expand, runtime-load, and replay-facing config assembly behavior
  remains unchanged.
- Worker and workstation runtime definitions still merge exactly as before,
  including prompt-template, body, limits, stop words, topology, env, and
  worker-type behavior.
- Verification stays behavioral and focused on effective config outcomes, for
  example:
  `go test ./pkg/config ./pkg/replay`

## Review guidance

Review this as a canonical-owner cleanup inside `pkg/config`. The main thing to
verify is that the repeated runtime-definition merge loops disappeared and that
effective runtime config behavior still matches current tests.
