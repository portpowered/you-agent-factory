package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	eventIDSessionStarted             = "factory-event/session-started"
	eventIDSessionPausedPrefix        = "factory-event/session-paused"
	eventIDSessionResumedPrefix       = "factory-event/session-resumed"
	eventIDSessionResultUpdatedPrefix = "factory-event/session-result-updated"
	eventIDSessionCompleted           = "factory-event/session-completed"
)

// SessionLifecycleStartInput carries replay-safe facts for SESSION_STARTED.
type SessionLifecycleStartInput struct {
	SessionID           string
	OrchestratorKind    factoryapi.FactoryOrchestratorKind
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
	OrchestratorKind factoryapi.FactoryOrchestratorKind
	PhaseID          string
	PhaseName        string
	Source           string
	Tick             int
	ResultStatus     factoryapi.FactoryEventSessionResultStatus
	ResultSummary    []interfaces.WorkContentPart
	ArtifactIDs      []string
}

// SessionLifecycleCompleteInput carries replay-safe facts for SESSION_COMPLETED.
type SessionLifecycleCompleteInput struct {
	SessionID        string
	OrchestratorKind factoryapi.FactoryOrchestratorKind
	Source           string
	Tick             int
	FinalStatus      factoryapi.FactorySessionDurableLifecycleStatus
	ResultStatus     *factoryapi.FactoryEventSessionResultStatus
	ArtifactIDs      []string
	DispatchCounts   *factoryapi.FactorySessionJavaScriptChildDispatchCounts
	FailureDetail    *factoryapi.FactoryDispatchFailureDetail
}

// SessionLifecycleControlInput carries replay-safe facts for SESSION_PAUSED and SESSION_RESUMED.
type SessionLifecycleControlInput struct {
	SessionID        string
	OrchestratorKind factoryapi.FactoryOrchestratorKind
	Source           string
	Tick             int
}

// RecordSessionPaused records a successful Factory Session pause lifecycle transition.
func (h *FactoryEventHistory) RecordSessionPaused(input SessionLifecycleControlInput, eventTime time.Time) {
	if h == nil || strings.TrimSpace(input.SessionID) == "" {
		return
	}
	eventTime = interfaces.CanonicalEventTime(eventTime)
	sequence := h.allocateSessionLifecycleSequence()
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeSessionPaused,
		fmt.Sprintf("%s/%d", eventIDSessionPausedPrefix, sequence),
		h.sessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence),
		factoryapi.SessionPausedEventPayload{
			Status:   factoryapi.FactorySessionDurableLifecycleStatusPaused,
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
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeSessionResumed,
		fmt.Sprintf("%s/%d", eventIDSessionResumedPrefix, sequence),
		h.sessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence),
		factoryapi.SessionResumedEventPayload{
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
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
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeSessionStarted,
		eventIDSessionStarted,
		h.sessionLifecycleContext(input.SessionID, input.OrchestratorKind, input.OrchestratorDialect, input.Source, input.Tick, eventTime, sequence),
		factoryapi.SessionStartedEventPayload{
			FactoryId:  stringPtrIfNotEmpty(input.FactoryID),
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
	context := h.sessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence)
	if phaseID := strings.TrimSpace(input.PhaseID); phaseID != "" {
		context.PhaseId = &phaseID
	}
	if phaseName := strings.TrimSpace(input.PhaseName); phaseName != "" {
		context.PhaseName = &phaseName
	}
	payload := factoryapi.SessionResultUpdatedEventPayload{
		ResultStatus: input.ResultStatus,
	}
	if summary := generatedWorkContentPtr(input.ResultSummary); summary != nil {
		payload.ResultSummary = summary
	}
	if len(input.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.ArtifactIDs...)
		payload.ArtifactIds = &artifactIDs
	}
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeSessionResultUpdated,
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
	payload := factoryapi.SessionCompletedEventPayload{
		FinalStatus:    input.FinalStatus,
		CompletedAt:    eventTime,
		DurationMillis: int64Ptr(durationMillis),
		ResultStatus:   input.ResultStatus,
		FailureDetail:  input.FailureDetail,
	}
	if len(input.ArtifactIDs) > 0 {
		artifactIDs := append([]string(nil), input.ArtifactIDs...)
		payload.ArtifactIds = &artifactIDs
	}
	if input.DispatchCounts != nil {
		payload.DispatchCounts = input.DispatchCounts
	}
	sequence := h.allocateSessionLifecycleSequence()
	h.appendGenerated(factoryEvent(
		factoryapi.FactoryEventTypeSessionCompleted,
		eventIDSessionCompleted,
		h.sessionLifecycleContext(input.SessionID, input.OrchestratorKind, "", input.Source, input.Tick, eventTime, sequence),
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
		sessionID = factory_context.DefaultSessionID
	}
	input := SessionLifecycleStartInput{
		SessionID:        sessionID,
		OrchestratorKind: interfaces.GeneratedPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryCfg)),
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
		sessionID = factory_context.DefaultSessionID
	}
	orchestratorKind := interfaces.GeneratedPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryCfg))
	finalStatus := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
	resultStatus := factoryapi.FactoryEventSessionResultStatusFinal
	var failureDetail *factoryapi.FactoryDispatchFailureDetail
	if factoryState == interfaces.FactoryStateFailed {
		finalStatus = factoryapi.FactorySessionDurableLifecycleStatusFailed
		resultStatus = factoryapi.FactoryEventSessionResultStatusFailedWithPartial
		if strings.TrimSpace(reason) != "" {
			failureDetail = &factoryapi.FactoryDispatchFailureDetail{
				Reason:  stringPtrIfNotEmpty("session_failed"),
				Message: stringPtrIfNotEmpty(reason),
			}
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

func (h *FactoryEventHistory) sessionLifecycleContext(
	sessionID string,
	orchestratorKind factoryapi.FactoryOrchestratorKind,
	orchestratorDialect string,
	source string,
	tick int,
	eventTime time.Time,
	sessionSequence int,
) factoryapi.FactoryEventContext {
	context := factoryapi.FactoryEventContext{
		Tick:      tick,
		EventTime: eventTime,
		SessionId: stringPtr(sessionID),
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

func generatedWorkContentPtr(parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	if len(parts) == 0 {
		return nil
	}
	generated := make([]factoryapi.WorkContentPart, 0, len(parts))
	for _, part := range parts {
		item := factoryapi.WorkContentPart{}
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			text := part.Text
			if err := item.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
			Type: factoryapi.WorkContentPartTypeText,
			Text: text,
		}); err != nil {
				continue
			}
		case interfaces.WorkContentPartTypeImage:
			if err := item.FromWorkImageContentPart(factoryapi.WorkImageContentPart{Url: part.URL}); err != nil {
				continue
			}
		default:
			continue
		}
		generated = append(generated, item)
	}
	if len(generated) == 0 {
		return nil
	}
	return &generated
}
