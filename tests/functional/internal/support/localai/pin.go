// Package localai provides reusable functional-test support for the pinned
// LocalAI managed backend protocol.
package localai

const (
	// PinnedLocalAIRevision identifies the LocalAI source used to define the
	// protocol fixture. It is deliberately separate from the backend artifact
	// pins: the fixture has no executable or model-weight dependency.
	PinnedLocalAIRevision = "b224c96db6f4b87306a33a808650bfce63b12588"
	// PinnedBackendProtocolRevision is the immutable blob revision recorded by
	// the repository's LocalAI artifact manifest.
	PinnedBackendProtocolRevision = "ad62c6df07ae1169eb14411a565a689cd996b19c"
	// PinnedBackendProtocolPath is the path within the pinned LocalAI source.
	PinnedBackendProtocolPath = "backend/backend.proto"
)
