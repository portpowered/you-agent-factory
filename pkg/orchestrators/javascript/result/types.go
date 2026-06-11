package workflowresult

import (
	"encoding/json"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	ArtifactURIScheme      = "you-artifact"
	DefaultMaxEmbeddedBytes = 64 * 1024
)

// Issue is one workflow result validation diagnostic.
type Issue struct {
	Code    string
	Message string
	Path    string
}

// Result aggregates workflow result validation issues.
type Result struct {
	Issues []Issue
}

// HasIssues reports whether validation found one or more issues.
func (r Result) HasIssues() bool {
	return len(r.Issues) > 0
}

// TypedValue carries one workflow return/final value at the contract boundary.
// Runtime adapters populate the non-JSON markers when a value cannot be
// structured-clone projected directly.
type TypedValue struct {
	JSON         json.RawMessage
	Unresolved   bool
	Function     bool
	HostHandle   string
	RawBinary    []byte
	Visited      map[uintptr]struct{}
	HostObject   any
}

// SessionResultInput supplies one terminal session result projection.
type SessionResultInput struct {
	SessionID      string
	Status         factoryapi.FactorySessionStatus
	PrimaryValue   TypedValue
	Artifacts      []interfaces.FactorySessionArtifactState
	CheckpointRefs []factoryapi.FactorySessionJavaScriptCheckpointRef
	ResultArtifact *factoryapi.FactoryArtifactRef
}
