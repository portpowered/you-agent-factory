# Cleanup Idea: Consolidate Work Content Generated-Domain Adapters

## Why this cleanup exists

The recent canonical work-content implementation added text and image content
parts across the API, runtime, event history, replay, and world-state
projection surfaces. The behavior is now present, but the generated API
contract conversion code is repeated in several places:

- `pkg/api/handlers.go` converts generated `WorkContent` to
  `interfaces.WorkContentPart` and back.
- `pkg/factory/event_history.go` converts domain work content to generated
  event work content.
- `pkg/replay/event_artifact.go` converts generated replay work content back to
  domain content.
- `pkg/factory/projections/world_state.go` has another generated-to-domain
  conversion loop.

Each copy switches on the same first-slice `text` and `image` variants. That is
already duplicate code, and it will become a change-amplification point when the
next content part type is added.

## Requested change

Create one canonical generated/domain adapter for work content and reuse it
from the existing boundary packages.

Keep the cleanup narrow:

- do not change the public OpenAPI schema
- do not change accepted or emitted JSON shapes
- do not change legacy payload compatibility behavior
- do not broaden this into content normalization, prompt rendering, runner
  argument shaping, or replay schema redesign
- preserve current nil/empty behavior where API responses omit empty content
- preserve validation behavior for request ingress, including clear errors for
  invalid content part shapes

`pkg/interfaces` already owns canonical runtime content shapes and already has
generated-contract adapters for safe diagnostics, so it is a reasonable home for
small helpers such as:

- generated `WorkContent` pointer to `[]interfaces.WorkContentPart`
- `[]interfaces.WorkContentPart` to generated `WorkContent` pointer
- an ingress variant that can return a caller-owned validation error message or
  enough structured information for `pkg/api` to preserve its current
  `requestFieldValidationError` behavior

If another package-local home fits the existing dependency direction better,
use it, but the end state should have one conversion owner rather than four
handwritten loops.

## Relevant files

- `pkg/interfaces/factory_runtime.go`
- `pkg/interfaces/safe_diagnostics.go`
- `pkg/api/handlers.go`
- `pkg/factory/event_history.go`
- `pkg/replay/event_artifact.go`
- `pkg/factory/projections/world_state.go`
- `pkg/api/server_test.go`
- `pkg/factory/event_history_test.go`
- `pkg/replay/delivery_test.go`

## Acceptance criteria

- There is one canonical implementation for converting between generated
  `factoryapi.WorkContent` values and domain `interfaces.WorkContentPart`
  slices.
- `pkg/api/handlers.go`, `pkg/factory/event_history.go`,
  `pkg/replay/event_artifact.go`, and `pkg/factory/projections/world_state.go`
  no longer each maintain independent `text`/`image` switch loops for the same
  conversion.
- API request validation still rejects malformed content parts with field-path
  useful errors such as `content[0].type` or `works[0].content[0].type`.
- API work readback, event-history emission, replay delivery, and world-state
  projection preserve text/image content part order and values.
- Existing work-content tests continue to pass. Add or adjust behavioral tests
  only where needed to prove the shared adapter preserves observable behavior.

## Review guidance

Prefer behavioral assertions at the API, event-history, replay, and projection
boundaries over tests that merely inventory helper locations. The cleanup is
successful when the repeated conversion logic is gone and the observable
text/image content behavior remains unchanged.
