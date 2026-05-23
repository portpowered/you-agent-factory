# Cleanup Idea: Consolidate Provider Metadata Clone Ownership

## Why this cleanup exists

The backend still keeps identical package-local clone helpers for provider
metadata even though the canonical detached-copy owner already exists in
`pkg/interfaces/world_state_clones.go`.

Today:

- `pkg/workers/provider_errors.go` clones provider-session metadata through a
  local `cloneProviderSession(...)` helper.
- `pkg/factory/subsystems/subsystem_transitioner.go` clones provider-failure
  metadata through a local `cloneProviderFailureMetadata(...)` helper.
- `pkg/replay/delivery.go` repeats the same local
  `cloneProviderFailureMetadata(...)` helper for replay result delivery.
- `pkg/interfaces/world_state_clones.go` already provides the canonical shared
  ownership points:
  - `CloneProviderSessionMetadata(...)`
  - `CloneProviderFailureMetadata(...)`

This is unnecessary backend duplication around a small canonical boundary. It
creates drift risk for no product value and leaves package-local ownership in
places where the repository already established a shared clone owner.

## Requested change

Collapse the remaining package-local provider metadata clone helpers onto the
canonical `pkg/interfaces` clone helpers.

Keep this cleanup narrow:

- preserve runtime behavior exactly
- do not broaden this into general work-item or content clone refactors
- do not add wrappers or compatibility aliases
- prefer deleting the duplicate helpers instead of keeping parallel owners

Suggested shape:

- Replace local provider-session cloning in `pkg/workers/provider_errors.go`
  with `interfaces.CloneProviderSessionMetadata(...)`.
- Replace local provider-failure cloning in
  `pkg/factory/subsystems/subsystem_transitioner.go` with
  `interfaces.CloneProviderFailureMetadata(...)`.
- Replace local provider-failure cloning in `pkg/replay/delivery.go` with
  `interfaces.CloneProviderFailureMetadata(...)`.
- Delete the redundant package-local clone helpers once all call sites use the
  canonical owner.

## Relevant files

- `pkg/interfaces/world_state_clones.go`
- `pkg/interfaces/world_state_clones_test.go`
- `pkg/workers/provider_errors.go`
- `pkg/factory/subsystems/subsystem_transitioner.go`
- `pkg/replay/delivery.go`
- `docs/internal/processes/development-guide-relevant-files.md`

## Acceptance criteria

- Provider-session and provider-failure metadata clone ownership exists in one
  canonical backend location instead of three package-local helper copies.
- `pkg/workers/provider_errors.go`,
  `pkg/factory/subsystems/subsystem_transitioner.go`, and
  `pkg/replay/delivery.go` call the shared `pkg/interfaces` clone helpers
  directly.
- The local helper functions for provider metadata cloning are deleted from the
  worker, subsystem-transitioner, and replay packages.
- Existing runtime behavior remains unchanged for provider-session and
  provider-failure propagation.
- Verification stays behavioral and focused on runtime/package outcomes rather
  than helper-location-only assertions.
- Run focused backend verification for the touched surfaces, for example:
  `go test ./pkg/interfaces ./pkg/workers ./pkg/factory/subsystems ./pkg/replay`

## Review guidance

Review this as a canonical-owner cleanup, not as a behavior change. The main
thing to verify is that the duplicate helper implementations disappeared and
that provider session/failure metadata still propagates unchanged through
worker errors, completed dispatch records, and replay result delivery.
