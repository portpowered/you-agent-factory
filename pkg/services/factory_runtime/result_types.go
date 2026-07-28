package factory

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	ArtifactURIScheme       = orchestratorcontract.ArtifactURIScheme
	DefaultMaxEmbeddedBytes = orchestratorcontract.DefaultMaxEmbeddedBytes
)

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

// SessionResultProjection contains the detached result facts derived from one
// canonical Factory Runtime result input. Consumers select the read or event
// shape they need without independently re-running result policy.
type SessionResultProjection struct {
	Live    LiveSessionResult
	Durable SessionResult
	Updated SessionResultUpdatedPayload
}

// SessionResultProjectionOperation owns canonical Factory Runtime result
// assembly. Wire injects this exact role into Factory Sessions.
type SessionResultProjectionOperation interface {
	ProjectSessionResults(SessionResultInput) SessionResultProjection
}
