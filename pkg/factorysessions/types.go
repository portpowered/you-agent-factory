package factorysessions

import (
	"github.com/google/uuid"
)

// DefaultSessionID is the stable alias for the primary live factory session.
const DefaultSessionID = "~default"

// TargetKind identifies whether a session target is the default factory or a named layout.
type TargetKind string

const (
	TargetKindDefault TargetKind = "default"
	TargetKindNamed   TargetKind = "named"
)

// TargetRef selects a discovered factory session target.
type TargetRef struct {
	Kind TargetKind
	Name string
}

// Target describes a runnable factory directory discovered under a session folder.
type Target struct {
	Ref        TargetRef
	Label      string
	FolderPath string
	FactoryDir string
	Project    string
}

// OpenResult is the internal outcome of opening or validating a session folder.
type OpenResult struct {
	SessionID       string
	Targets         []Target
	InitsNewFactory bool
	FolderPath      string
}

// LiveSession tracks one live factory session and its runtime handle.
// Handle is typed by the composition root (for example *service.liveRuntimeHandle).
type LiveSession struct {
	ID         string
	FactoryDir string
	FolderPath string
	Handle     any
	IsDefault  bool
	Project    string
	Target     TargetRef
}

// NewSessionID allocates a unique live session identifier.
func NewSessionID() string {
	return uuid.NewString()
}
