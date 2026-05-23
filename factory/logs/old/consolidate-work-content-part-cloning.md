# Cleanup Idea: Consolidate Work Content Part Cloning

## Why this cleanup exists

Recent work-content cleanup moved generated/domain conversion ownership into
`pkg/interfaces`, but detached cloning of canonical `[]WorkContentPart` values
is still repeated across several runtime boundaries.

Current `origin/main` has a private `cloneWorkContentParts` helper in
`pkg/interfaces/world_state_clones.go`, a separate local `cloneWorkContent`
helper in `pkg/factory/token_transformer/transformer.go`, and multiple direct
copies using `append([]interfaces.WorkContentPart(nil), value...)` across
factory request normalization, projections, replay, delivery, and prompt
rendering.

The duplicated logic is small, but it appears at exactly the boundaries where
aliasing mistakes are expensive: submit normalization, token construction,
event replay, world-state projection, and worker prompt preparation.

## Requested change

Expose one canonical clone helper for work-content parts from `pkg/interfaces`
and reuse it where packages need detached copies of canonical work content.

Keep the cleanup narrow:

- do not change the `WorkContentPart` shape
- do not change generated API conversion behavior
- do not change nil/empty omission behavior at public API boundaries
- do not change payload normalization, image handling, or runner arguments
- do not broaden this into a work-content model redesign

Suggested shape:

- Rename or wrap the existing private helper in
  `pkg/interfaces/world_state_clones.go` as an exported helper such as
  `CloneWorkContentParts`.
- Keep the same behavior: nil or empty input returns nil; non-empty input
  returns a detached slice with the same ordered part values.
- Replace local duplicate clone logic and direct work-content slice appends
  where the caller is cloning canonical `[]interfaces.WorkContentPart`.
- Leave generated `factoryapi.WorkContent` conversion ownership in
  `pkg/interfaces/work_content_adapter.go`; this cleanup is only for canonical
  domain slice cloning.

## Relevant files

- `pkg/interfaces/world_state_clones.go`
- `pkg/interfaces/work_content_adapter_test.go` or a focused interfaces clone
  test file
- `pkg/factory/token_transformer/transformer.go`
- `pkg/factory/work_request.go`
- `pkg/factory/projections/world_state.go`
- `pkg/factory/subsystems/subsystem_transitioner.go`
- `pkg/replay/delivery.go`
- `pkg/replay/event_artifact.go`
- `pkg/replay/event_reducer.go`
- `pkg/workers/prompt.go`

## Acceptance criteria

- `pkg/interfaces` exposes one canonical helper for detached cloning of
  `[]WorkContentPart`.
- The private token-transformer work-content clone helper is removed.
- Repeated `append([]interfaces.WorkContentPart(nil), ... )` copies for
  canonical work content are replaced with the shared helper where appropriate.
- Nil and empty work-content slices still clone to nil where current callers
  rely on omit-empty behavior.
- Ordered text and image content parts remain unchanged through submit
  normalization, token construction, projection, replay, and prompt rendering.
- Focused behavioral tests continue to pass; add a small clone-helper test only
  if existing coverage does not already prove detached-copy and nil/empty
  behavior.

## Review guidance

Review this as an aliasing cleanup, not a content feature change. The main risk
is accidentally changing nil/empty behavior or introducing shared slices at
runtime boundaries, so prefer behavioral assertions that mutate the original
slice after cloning and verify the clone remains detached.
