# Cleanup Idea: Consolidate Generated Work Content Translation

## Why this cleanup exists

The backend still keeps the same generated work-content decoding logic in
multiple active boundaries.

Today:

- `pkg/api/handlers.go` decodes generated `WorkContent` into
  `interfaces.WorkContentPart` for public submit and upsert flows.
- `pkg/factory/projections/world_state.go` repeats the same generated
  work-content decoding while reconstructing projected world state from event
  history.
- `pkg/replay/event_artifact.go` repeats the same generated work-content
  decoding again while rebuilding replay artifacts and work-item views.

That leaves three owners for one boundary translation rule. If content-part
support changes, drift now has to be fixed in three places.

## Requested change

Collapse generated work-content decoding onto one canonical backend owner and
reuse it from API, projection, and replay code.

Keep this cleanup narrow:

- preserve current runtime behavior
- do not broaden this into replay schema redesign or projection-model changes
- do not add a new abstraction layer beyond one shared translation owner
- prefer deleting duplicate helpers instead of preserving wrappers
- keep verification behavioral around API, replay, and projection outcomes

Suggested shape:

- Extract one canonical helper for translating generated `factoryapi.WorkContent`
  into `[]interfaces.WorkContentPart`.
- Reuse that helper from `pkg/api/handlers.go`,
  `pkg/factory/projections/world_state.go`, and
  `pkg/replay/event_artifact.go`.
- Keep API-specific validation behavior local where it needs path-aware bad
  request errors, but remove duplicate happy-path translation ownership.
- If practical within the same narrow change, also collapse adjacent generated
  work-to-domain item mapping duplication in replay and projection code where
  it is only carrying the same content translation seam forward.

## Relevant files

- `pkg/api/handlers.go`
- `pkg/factory/projections/world_state.go`
- `pkg/replay/event_artifact.go`
- `pkg/interfaces/`
- `pkg/api/server_test.go`
- `pkg/factory/projections/`
- `pkg/replay/`

## Acceptance criteria

- Generated work-content decoding no longer exists as three separate helper
  implementations across API, projection, and replay code.
- API submit or upsert handling, world-state reconstruction, and replay
  artifact hydration all reuse one canonical generated work-content decoder for
  supported `text` and `image` parts.
- API-only field-path validation semantics remain unchanged where they are
  currently exposed as bad-request messages.
- Observable runtime behavior stays the same for supported content parts across
  API, replay, and projected world-state flows.
- Focused verification remains behavioral, for example:
  `go test ./pkg/api ./pkg/factory/projections ./pkg/replay`

## Review guidance

Review this as a duplication-removal change around one translation seam. The
main thing to verify is that the repeated generated content decoders
disappeared while API parsing, replay hydration, and world-state reconstruction
still produce the same observable work content.
