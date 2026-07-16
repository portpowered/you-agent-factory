package workflowresult

import (
	"encoding/json"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

const (
	ArtifactURIScheme       = "you-artifact"
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
	JSON       json.RawMessage
	Unresolved bool
	Function   bool
	HostHandle string
	RawBinary  []byte
	Visited    map[uintptr]struct{}
	HostObject any
}

// SessionResultInput supplies one terminal session result projection.
type SessionResultInput struct {
	SessionID      string
	Status         interfaces.RuntimeStatus
	PrimaryValue   TypedValue
	Artifacts      []interfaces.FactorySessionArtifactState
	CheckpointRefs []interfaces.FactorySessionJavaScriptCheckpointEventRef
	ResultArtifact *interfaces.FactoryArtifactRef
}

// LiveSessionResult is the transport-independent terminal read projection for
// one live JavaScript Factory Session.
type LiveSessionResult struct {
	SessionID         string
	Status            interfaces.RuntimeStatus
	CheckpointRefs    []interfaces.FactorySessionJavaScriptCheckpointEventRef
	ResultArtifactRef *interfaces.FactoryArtifactRef
}

// PartialSessionResult is the transport-independent checkpoint-backed read
// projection for one live JavaScript Factory Session.
type PartialSessionResult struct {
	SessionID                string
	Phase                    string
	CheckpointRefs           []interfaces.FactorySessionJavaScriptCheckpointEventRef
	PartialResultArtifactRef *interfaces.FactoryArtifactRef
}

// ResultStatus describes customer-visible result availability for one
// JavaScript Factory Session projection.
type ResultStatus string

const (
	ResultStatusFinal    ResultStatus = "FINAL"
	ResultStatusNotReady ResultStatus = "NOT_READY"
	ResultStatusPartial  ResultStatus = "PARTIAL"
)

// SessionResult is the transport-independent durable result projection shared
// by result reads and Factory event payload construction.
type SessionResult struct {
	SessionID     string
	ResultStatus  ResultStatus
	PrimaryResult []work.WorkContentPart
	ArtifactIDs   []string
	ArtifactRefs  []interfaces.FactoryArtifactRef
}

// SessionResultUpdatedPayload is the Factory-owned result fact emitted when a
// JavaScript session's observable result changes.
type SessionResultUpdatedPayload struct {
	ResultStatus  interfaces.FactorySessionResultStatus
	ResultSummary []work.WorkContentPart
	ArtifactIDs   []string
}
