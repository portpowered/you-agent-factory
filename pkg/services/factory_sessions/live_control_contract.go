package factorysessions

// Live-control root slice freezes open, list, get/snapshot, pause, resume, and
// close vocabulary on the singular Service. Peers consume these plain root
// contracts without importing private live-runtime registry or host types:
//
//   - Open: OpenRequest → *OpenResult
//   - List: []ReadProjection
//   - Get/snapshot: SessionProjection
//   - Pause/Resume: ControlRequest → LifecycleControlResult
//   - Close: session identity → error
//
// Typed failures peers distinguish with errors.Is / errors.As:
//   - ErrSessionNotFound for missing live sessions
//   - *ControlError for rejected lifecycle transitions (Outcome InvalidState or
//     TerminalSession), without nested live-runtime imports
//
// Live-control operations remain methods on Service; this file does not publish
// a separate peer-facing live-session interface.

// LiveControlOpenRequest is the plain root open request for live session control.
// It is the published name for OpenRequest on the live-control slice.
type LiveControlOpenRequest = OpenRequest

// LiveControlOpenResult is the plain root open result carrying stable session
// identity and discovered targets for the live-control slice.
type LiveControlOpenResult = OpenResult

// LiveControlListItem is one live session row returned by list through the
// live-control root vocabulary.
type LiveControlListItem = ReadProjection

// LiveControlSnapshot is the plain root get/snapshot result for one live
// session, including stable identity and live projection shape.
type LiveControlSnapshot = SessionProjection

// LiveControlRequest is the plain pause/resume request metadata published on
// the live-control root slice.
type LiveControlRequest = ControlRequest

// LiveControlResult is the plain pause/resume success shape published on the
// live-control root slice.
type LiveControlResult = LifecycleControlResult

// LiveControlError is the typed rejected-lifecycle-transition failure published
// on the live-control root slice. Peers match it with errors.As and inspect
// Outcome without importing nested live-runtime packages.
type LiveControlError = ControlError
