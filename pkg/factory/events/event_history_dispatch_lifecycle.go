package events

import (
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const (
	eventIDDispatchQueuedPrefix      = "factory-event/dispatch-queued"
	eventIDDispatchInterruptedPrefix = "factory-event/dispatch-interrupted"
	eventIDDispatchReconciledPrefix  = "factory-event/dispatch-reconciled"
	eventIDArtifactCreatedPrefix     = "factory-event/artifact-created"
)

// DispatchQueuedInput carries replay-safe facts for DISPATCH_QUEUED.
type DispatchQueuedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	DispatchKind        factoryapi.FactoryDispatchKind
	Label               string
	CoordinationRef     string
	RunnerID            string
	Model               string
	Provider            string
	ParentDispatchID    string
	RetryOfDispatchID   string
	QueuePosition       *int
	PromptDigest        string
	SchemaDigest        string
	InputArtifactIDs    []string
	InputWorkIDs        []string
}

// DispatchInterruptedInput carries replay-safe facts for DISPATCH_INTERRUPTED.
type DispatchInterruptedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Reason              string
	ObservedStatus      factoryapi.FactoryDispatchStatus
	RetryPlanned        bool
	ProviderSessionRef  *factoryapi.LoadableProviderSessionRef
	CheckpointRef       *factoryapi.FactorySessionJavaScriptCheckpointRef
}

// DispatchReconciledInput carries replay-safe facts for DISPATCH_RECONCILED.
type DispatchReconciledInput struct {
	SessionID              string
	OrchestratorKind       factoryapi.FactoryOrchestratorKind
	OrchestratorDialect    string
	PhaseID                string
	PhaseName              string
	DispatchID             string
	Source                 string
	Tick                   int
	ReconciledStatus       factoryapi.FactoryDispatchStatus
	ReconciliationSource   factoryapi.DispatchReconciliationSource
	Replayed               bool
	Usage                  *factoryapi.FactoryDispatchUsage
	ResultArtifactRef      *factoryapi.FactoryArtifactRef
	ArtifactIDs            []string
	FailureDetail          *factoryapi.FactoryDispatchFailureDetail
}

// ArtifactCreatedInput carries replay-safe facts for ARTIFACT_CREATED.
type ArtifactCreatedInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
	OrchestratorDialect string
	PhaseID             string
	PhaseName           string
	DispatchID          string
	Source              string
	Tick                int
	Artifact            factoryapi.FactoryArtifact
	CapturedAt          *time.Time
}

// RecordDispatchQueued records a canonical dispatch queue marker.
func (h *FactoryEventHistory) RecordDispatchQueued(input DispatchQueuedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.DispatchKind == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchQueuedEventPayload{
		DispatchKind: input.DispatchKind,
	}
	if label := strings.TrimSpace(input.Label); label != "" {
		payload.Label = &label
	}
	if coordinationRef := strings.TrimSpace(input.CoordinationRef); coordinationRef != "" {
		payload.CoordinationRef = &coordinationRef
	}
	if runnerID := strings.TrimSpace(input.RunnerID); runnerID != "" {
		payload.RunnerId = &runnerID
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		payload.Model = &model
	}
	if provider := strings.TrimSpace(input.Provider); provider != "" {
		payload.Provider = &provider
	}
	if parentDispatchID := strings.TrimSpace(input.ParentDispatchID); parentDispatchID != "" {
		payload.ParentDispatchId = &parentDispatchID
	}
	if retryOfDispatchID := strings.TrimSpace(input.RetryOfDispatchID); retryOfDispatchID != "" {
		payload.RetryOfDispatchId = &retryOfDispatchID
	}
	if input.QueuePosition != nil {
		payload.QueuePosition = input.QueuePosition
	}
	if promptDigest := strings.TrimSpace(input.PromptDigest); promptDigest != "" {
		payload.PromptDigest = &promptDigest
	}
	if schemaDigest := strings.TrimSpace(input.SchemaDigest); schemaDigest != "" {
		payload.SchemaDigest = &schemaDigest
	}
	if len(input.InputArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.InputArtifactIDs...)
		payload.InputArtifactIds = &artifactIDs
	}
	if len(input.InputWorkIDs) > 0 {
		workIDs := append([]string(nil), input.InputWorkIDs...)
		payload.InputWorkIds = &workIDs
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchQueued,
		fmt.Sprintf("%s/%s", eventIDDispatchQueuedPrefix, input.DispatchID),
		context,
		payload,
	))
}

// RecordDispatchInterrupted records a canonical dispatch interruption marker.
func (h *FactoryEventHistory) RecordDispatchInterrupted(input DispatchInterruptedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.ObservedStatus == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchInterruptedEventPayload{
		Reason:         strings.TrimSpace(input.Reason),
		ObservedStatus: input.ObservedStatus,
		InterruptedAt:  eventTime,
		RetryPlanned:   input.RetryPlanned,
	}
	if input.ProviderSessionRef != nil {
		providerSessionRef := *input.ProviderSessionRef
		payload.ProviderSessionRef = &providerSessionRef
	}
	if input.CheckpointRef != nil {
		checkpointRef := *input.CheckpointRef
		payload.CheckpointRef = &checkpointRef
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchInterrupted,
		fmt.Sprintf("%s/%d", eventIDDispatchInterruptedPrefix, sequence),
		context,
		payload,
	))
}

// RecordDispatchReconciled records a canonical dispatch reconciliation marker.
func (h *FactoryEventHistory) RecordDispatchReconciled(input DispatchReconciledInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.DispatchID) == "" || input.ReconciledStatus == "" || input.ReconciliationSource == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.DispatchReconciledEventPayload{
		ReconciledStatus:     input.ReconciledStatus,
		ReconciliationSource: input.ReconciliationSource,
		Replayed:             input.Replayed,
	}
	if input.Usage != nil {
		usage := *input.Usage
		payload.Usage = &usage
	}
	if input.ResultArtifactRef != nil {
		resultArtifactRef := *input.ResultArtifactRef
		payload.ResultArtifactRef = &resultArtifactRef
	}
	if len(input.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.ArtifactIDs...)
		payload.ArtifactIds = &artifactIDs
	}
	if input.FailureDetail != nil {
		failureDetail := *input.FailureDetail
		payload.FailureDetail = &failureDetail
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeDispatchReconciled,
		fmt.Sprintf("%s/%s", eventIDDispatchReconciledPrefix, input.DispatchID),
		context,
		payload,
	))
}

// RecordArtifactCreated records a canonical artifact creation marker.
func (h *FactoryEventHistory) RecordArtifactCreated(input ArtifactCreatedInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Artifact.Id) == "" {
		return
	}
	eventTime = canonicalDispatchLifecycleEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.dispatchLifecycleContext(
		input.SessionID,
		input.OrchestratorKind,
		input.OrchestratorDialect,
		input.PhaseID,
		input.PhaseName,
		input.DispatchID,
		input.Source,
		input.Tick,
		eventTime,
		sequence,
	)
	payload := factoryapi.ArtifactCreatedEventPayload{
		Artifact: input.Artifact,
	}
	if input.CapturedAt != nil {
		capturedAt := canonicalDispatchLifecycleEventTime(*input.CapturedAt)
		payload.CapturedAt = &capturedAt
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeArtifactCreated,
		fmt.Sprintf("%s/%s", eventIDArtifactCreatedPrefix, input.Artifact.Id),
		context,
		payload,
	))
}

func (h *FactoryEventHistory) dispatchLifecycleContext(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	orchestratorDialect string,
	phaseID string,
	phaseName string,
	dispatchID string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) factoryapi.FactoryEventContext {
	context := h.sessionLifecycleContext(sessionID, orchestratorKind, orchestratorDialect, source, tick, eventTime, sessionSequence)
	if phaseID := strings.TrimSpace(phaseID); phaseID != "" {
		context.PhaseId = &phaseID
	}
	if phaseName := strings.TrimSpace(phaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	if dispatchID := strings.TrimSpace(dispatchID); dispatchID != "" {
		context.DispatchId = &dispatchID
	}
	return context
}

func canonicalDispatchLifecycleEventTime(eventTime time.Time) time.Time {
	if eventTime.IsZero() {
		return eventTime
	}
	return eventTime.UTC()
}
