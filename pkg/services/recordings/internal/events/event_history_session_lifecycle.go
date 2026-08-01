package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	eventIDSessionStarted                = "factory-event/session-started"
	eventIDSessionPausedPrefix           = "factory-event/session-paused"
	eventIDSessionResumedPrefix          = "factory-event/session-resumed"
	eventIDSessionResultUpdatedPrefix    = "factory-event/session-result-updated"
	eventIDSessionCompleted              = "factory-event/session-completed"
	eventIDSessionLifecycleControlPrefix = "session-lifecycle-control"
)

// SessionLifecycleStartInput carries replay-safe facts for SESSION_STARTED.
type SessionLifecycleStartInput struct {
	SessionID           string
	OrchestratorKind    string
	OrchestratorDialect string
	Source              string
	FactoryID           string
	SourceRef           string
	SourceHash          string
	PolicyHash          string
	ArgsDigest          string
	Tick                int
}

// SessionLifecycleResultInput carries replay-safe facts for SESSION_RESULT_UPDATED.
type SessionLifecycleResultInput struct {
	SessionID        string
	OrchestratorKind string
	PhaseID          string
	PhaseName        string
	Source           string
	Tick             int
	ResultStatus     interfaces.FactorySessionResultStatus
	ResultSummary    []work.WorkContentPart
	ArtifactIDs      []string
}

// SessionLifecycleCompleteInput carries replay-safe facts for SESSION_COMPLETED.
type SessionLifecycleCompleteInput struct {
	SessionID        string
	OrchestratorKind string
	Source           string
	Tick             int
	FinalStatus      interfaces.FactorySessionLifecycleStatus
	ResultStatus     *interfaces.FactorySessionResultStatus
	ArtifactIDs      []string
	DispatchCounts   *interfaces.FactorySessionChildDispatchCounts
	FailureDetail    *workerexecution.FailureDetail
}

// SessionLifecycleControlInput remains an alias for source compatibility.
type SessionLifecycleControlInput = recordings.SessionLifecycleControlInput

// RecordSessionPaused records a successful Factory Session pause lifecycle transition.
func (h *FactoryEventHistory) RecordSessionPaused(input SessionLifecycleControlInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionPaused,
		fmt.Sprintf("%s/%d", eventIDSessionPausedPrefix, sequence),
		h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, input.OrchestratorDialect, input.Source, input.Tick, eventTime, sequence),
		interfaces.FactorySessionPausedEventPayload{
			Status:   interfaces.FactorySessionLifecycleStatusPaused,
			PausedAt: eventTime,
		},
	))
}

// RecordSessionResumed records a successful Factory Session resume lifecycle transition.
func (h *FactoryEventHistory) RecordSessionResumed(input SessionLifecycleControlInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionResumed,
		fmt.Sprintf("%s/%d", eventIDSessionResumedPrefix, sequence),
		h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, input.OrchestratorDialect, input.Source, input.Tick, eventTime, sequence),
		interfaces.FactorySessionResumedEventPayload{
			Status:    interfaces.FactorySessionLifecycleStatusRunning,
			ResumedAt: eventTime,
		},
	))
}

// RecordSessionStarted records the canonical session execution start marker.
func (h *FactoryEventHistory) RecordSessionStarted(input SessionLifecycleStartInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	h.mu.Lock()
	if h.hasSessionStarted {
		h.mu.Unlock()
		return
	}
	h.hasSessionStarted = true
	h.sessionStartedAt = interfaces.CanonicalEventTime(eventTime)
	h.mu.Unlock()

	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionStarted,
		eventIDSessionStarted,
		h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, input.OrchestratorDialect, input.Source, input.Tick, eventTime, sequence),
		interfaces.FactorySessionStartedEventPayload{
			FactoryID:  stringPtrIfNotEmpty(input.FactoryID),
			SourceRef:  stringPtrIfNotEmpty(input.SourceRef),
			SourceHash: stringPtrIfNotEmpty(input.SourceHash),
			PolicyHash: stringPtrIfNotEmpty(input.PolicyHash),
			ArgsDigest: stringPtrIfNotEmpty(input.ArgsDigest),
			StartedAt:  eventTime,
		},
	))
}

// RecordSessionResultUpdated records partial, final, or failed-with-partial result availability.
func (h *FactoryEventHistory) RecordSessionResultUpdated(input SessionLifecycleResultInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" || input.ResultStatus == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	context := h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence)
	if phaseID := strings.TrimSpace(input.PhaseID); phaseID != "" {
		context.PhaseID = &phaseID
	}
	if phaseName := strings.TrimSpace(input.PhaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	payload := interfaces.FactorySessionResultUpdatedEventPayload{
		ResultStatus: input.ResultStatus,
	}
	if len(input.ResultSummary) > 0 {
		payload.ResultSummary = append([]work.WorkContentPart(nil), input.ResultSummary...)
	}
	if len(input.ArtifactIDs) > 0 {
		payload.ArtifactIDs = append([]string(nil), input.ArtifactIDs...)
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionResultUpdated,
		fmt.Sprintf("%s/%d", eventIDSessionResultUpdatedPrefix, sequence),
		context,
		payload,
	))
}

// RecordSessionCompleted records the authoritative terminal session lifecycle marker.
func (h *FactoryEventHistory) RecordSessionCompleted(input SessionLifecycleCompleteInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	h.mu.Lock()
	if h.hasSessionCompleted {
		h.mu.Unlock()
		return
	}
	startedAt := h.sessionStartedAt
	h.hasSessionCompleted = true
	h.mu.Unlock()

	eventTime = interfaces.CanonicalEventTime(eventTime)
	durationMillis := int64(0)
	if !startedAt.IsZero() {
		durationMillis = eventTime.Sub(startedAt).Milliseconds()
		if durationMillis < 0 {
			durationMillis = 0
		}
	}
	payload := interfaces.FactorySessionCompletedEventPayload{
		FinalStatus:    input.FinalStatus,
		CompletedAt:    eventTime,
		DurationMillis: int64Ptr(durationMillis),
		ResultStatus:   input.ResultStatus,
		FailureDetail:  input.FailureDetail,
	}
	if len(input.ArtifactIDs) > 0 {
		payload.ArtifactIDs = append([]string(nil), input.ArtifactIDs...)
	}
	if input.DispatchCounts != nil {
		payload.DispatchCounts = input.DispatchCounts
	}
	sequence := h.allocateSessionLifecycleSequence()
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionCompleted,
		eventIDSessionCompleted,
		h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence),
		payload,
	))
}

// RecordSessionLifecycleFromFactoryConfig records SESSION_STARTED using runtime wiring inputs.
func (h *FactoryEventHistory) RecordSessionLifecycleFromFactoryConfig(
	sessionID string,
	factoryCfg *interfaces.FactoryConfig,
	tick int,
	eventTime time.Time,
) {
	if h == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = workerexecution.DefaultSessionID
	}
	input := SessionLifecycleStartInput{
		SessionID:        sessionID,
		OrchestratorKind: interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryCfg)),
		Source:           "runtime",
		Tick:             tick,
	}
	if factoryCfg != nil {
		if name := strings.TrimSpace(factoryCfg.Name); name != "" {
			input.FactoryID = name
		} else if project := strings.TrimSpace(factoryCfg.Project); project != "" {
			input.FactoryID = project
		}
		if factoryCfg.Orchestrator != nil && factoryCfg.Orchestrator.JavaScript != nil {
			js := factoryCfg.Orchestrator.JavaScript
			input.OrchestratorDialect = strings.TrimSpace(js.Dialect)
			input.SourceRef = strings.TrimSpace(js.SourceRef)
			input.SourceHash = strings.TrimSpace(js.SourceHash)
			input.PolicyHash = sessionLifecycleDigestJSON(js.DefaultPolicy)
			input.ArgsDigest = sessionLifecycleDigestJSON(js.ArgsSchema)
		}
	}
	h.RecordSessionStarted(input, eventTime)
}

// RecordSessionLifecycleCompletion records terminal session result and completion events.
func (h *FactoryEventHistory) RecordSessionLifecycleCompletion(
	sessionID string,
	factoryCfg *interfaces.FactoryConfig,
	tick int,
	factoryState interfaces.FactoryState,
	reason string,
	eventTime time.Time,
) {
	if h == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = workerexecution.DefaultSessionID
	}
	orchestratorKind := interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryCfg))
	finalStatus := interfaces.FactorySessionLifecycleStatusSucceeded
	resultStatus := interfaces.FactorySessionResultStatusFinal
	var failureDetail *workerexecution.FailureDetail
	if factoryState == interfaces.FactoryStateFailed {
		finalStatus = interfaces.FactorySessionLifecycleStatusFailed
		resultStatus = interfaces.FactorySessionResultStatusFailedWithPartial
		if strings.TrimSpace(reason) != "" {
			failureDetail = &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: reason}
		}
	}
	result := resultStatus
	h.RecordSessionResultUpdated(SessionLifecycleResultInput{
		SessionID:        sessionID,
		OrchestratorKind: orchestratorKind,
		Source:           "runtime",
		Tick:             tick,
		ResultStatus:     resultStatus,
	}, eventTime)
	h.RecordSessionCompleted(SessionLifecycleCompleteInput{
		SessionID:        sessionID,
		OrchestratorKind: orchestratorKind,
		Source:           "runtime",
		Tick:             tick,
		FinalStatus:      finalStatus,
		ResultStatus:     &result,
		FailureDetail:    failureDetail,
	}, eventTime)
}

// RecordSessionLifecycleControl records one accepted pause or resume control on the
// canonical factory event stream for live runtime sessions.
func (h *FactoryEventHistory) RecordSessionLifecycleControl(input SessionLifecycleControlInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	if input.Outcome != interfaces.FactorySessionLifecycleControlOutcomeAccepted {
		return
	}
	if input.Operation != interfaces.FactorySessionLifecycleControlPause &&
		input.Operation != interfaces.FactorySessionLifecycleControlResume {
		return
	}
	if input.PreviousStatus == input.NewStatus {
		return
	}

	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	payload := interfaces.FactorySessionLifecycleControlEventPayload{
		Operation:      input.Operation,
		Outcome:        input.Outcome,
		PreviousStatus: input.PreviousStatus,
		NewStatus:      input.NewStatus,
		OccurredAt:     eventTime,
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		payload.Reason = stringPtrIfNotEmpty(reason)
	}
	h.appendEvent(domainFactoryEvent(
		interfaces.FactoryEventTypeSessionLifecycleControl,
		fmt.Sprintf("%s/%s/%d", eventIDSessionLifecycleControlPrefix, input.SessionID, sequence),
		h.domainSessionLifecycleContext(input.SessionID, input.OrchestratorKind, input.OrchestratorDialect, input.Source, input.Tick, eventTime, sequence),
		payload,
	))
}

func FactoryStateToDurableLifecycleStatus(state interfaces.FactoryState) interfaces.FactorySessionLifecycleStatus {
	switch state {
	case interfaces.FactoryStatePaused:
		return interfaces.FactorySessionLifecycleStatusPaused
	case interfaces.FactoryStateCompleted:
		return interfaces.FactorySessionLifecycleStatusSucceeded
	case interfaces.FactoryStateFailed:
		return interfaces.FactorySessionLifecycleStatusFailed
	default:
		return interfaces.FactorySessionLifecycleStatusRunning
	}
}

func (h *FactoryEventHistory) domainSessionLifecycleContext(
	sessionID string,
	orchestratorKind string,
	orchestratorDialect string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) interfaces.FactoryEventContext {
	context := interfaces.FactoryEventContext{
		Tick:      tick,
		EventTime: eventTime,
		SessionID: stringPtr(sessionID),
	}
	if orchestratorKind != "" {
		kind := orchestratorKind
		context.OrchestratorKind = &kind
	}
	if dialect := strings.TrimSpace(orchestratorDialect); dialect != "" {
		context.OrchestratorDialect = &dialect
	}
	if source := strings.TrimSpace(source); source != "" {
		context.Source = &source
	}
	context.SessionSequence = &sessionSequence
	return context
}

func (h *FactoryEventHistory) allocateSessionLifecycleSequence() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.nextSessionSequence
	h.nextSessionSequence++
	return current
}

func sessionLifecycleDigestJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
