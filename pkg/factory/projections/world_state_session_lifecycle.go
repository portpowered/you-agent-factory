package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func (r *factoryWorldReducer) applySessionLifecycleEvent(event factoryapi.FactoryEvent) (bool, error) {
	switch event.Type {
	case factoryapi.FactoryEventTypeSessionStarted:
		return true, r.applySessionStartedEvent(event)
	case factoryapi.FactoryEventTypeSessionResultUpdated:
		return true, r.applySessionResultUpdatedEvent(event)
	case factoryapi.FactoryEventTypeSessionCompleted:
		return true, r.applySessionCompletedEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applySessionStartedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionStartedEventPayload()
	if err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	if sessionID := stringValue(event.Context.SessionId); sessionID != "" {
		bracket.SessionID = sessionID
	}
	if kind := event.Context.OrchestratorKind; kind != nil {
		bracket.OrchestratorKind = string(*kind)
	}
	bracket.OrchestratorDialect = stringValue(event.Context.OrchestratorDialect)
	bracket.FactoryID = stringValue(payload.FactoryId)
	bracket.SourceRef = stringValue(payload.SourceRef)
	bracket.SourceHash = stringValue(payload.SourceHash)
	bracket.PolicyHash = stringValue(payload.PolicyHash)
	bracket.ArgsDigest = stringValue(payload.ArgsDigest)
	bracket.StartedAt = payload.StartedAt.UTC()
	return nil
}

func (r *factoryWorldReducer) applySessionResultUpdatedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionResultUpdatedEventPayload()
	if err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.ResultStatus = string(payload.ResultStatus)
	bracket.ResultSummary = workcontent.PartsFromGenerated(payload.ResultSummary)
	bracket.ArtifactIDs = cloneStringSlice(sliceValue(payload.ArtifactIds))
	return nil
}

func (r *factoryWorldReducer) applySessionCompletedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionCompletedEventPayload()
	if err != nil {
		return err
	}
	bracket := r.ensureSessionBracket()
	mergeSessionBracketIdentity(bracket, event.Context)
	bracket.Terminal = true
	bracket.FinalStatus = string(payload.FinalStatus)
	bracket.CompletedAt = payload.CompletedAt.UTC()
	if payload.DurationMillis != nil {
		bracket.DurationMillis = *payload.DurationMillis
	}
	if payload.ResultStatus != nil {
		bracket.ResultStatus = string(*payload.ResultStatus)
	}
	bracket.ArtifactIDs = cloneStringSlice(sliceValue(payload.ArtifactIds))
	if payload.DispatchCounts != nil {
		bracket.DispatchCounts = &interfaces.FactoryWorldJavaScriptChildDispatchCounts{
			Queued:    payload.DispatchCounts.Queued,
			Running:   payload.DispatchCounts.Running,
			Completed: payload.DispatchCounts.Completed,
		}
	}
	if payload.FailureDetail != nil {
		bracket.FailureReason = stringValue(payload.FailureDetail.Reason)
		bracket.FailureMessage = stringValue(payload.FailureDetail.Message)
		bracket.FailureErrorClass = stringValue(payload.FailureDetail.ErrorClass)
	}
	return nil
}

func (r *factoryWorldReducer) ensureSessionBracket() *interfaces.FactoryWorldSessionBracketState {
	if r.stateValue.SessionBracket == nil {
		r.stateValue.SessionBracket = &interfaces.FactoryWorldSessionBracketState{}
	}
	return r.stateValue.SessionBracket
}

func mergeSessionBracketIdentity(bracket *interfaces.FactoryWorldSessionBracketState, context factoryapi.FactoryEventContext) {
	if bracket == nil {
		return
	}
	if sessionID := stringValue(context.SessionId); sessionID != "" {
		bracket.SessionID = sessionID
	}
	if kind := context.OrchestratorKind; kind != nil && bracket.OrchestratorKind == "" {
		bracket.OrchestratorKind = string(*kind)
	}
	if dialect := stringValue(context.OrchestratorDialect); dialect != "" && bracket.OrchestratorDialect == "" {
		bracket.OrchestratorDialect = dialect
	}
}

func buildFactoryWorldSessionBracketProjection(
	state interfaces.FactoryWorldState,
) *interfaces.FactoryWorldSessionBracketProjection {
	if state.SessionBracket == nil {
		return nil
	}
	bracket := state.SessionBracket
	if bracket.SessionID == "" && !bracket.Terminal && bracket.StartedAt.IsZero() {
		return nil
	}
	return &interfaces.FactoryWorldSessionBracketProjection{
		SessionID:           bracket.SessionID,
		OrchestratorKind:    bracket.OrchestratorKind,
		OrchestratorDialect: bracket.OrchestratorDialect,
		FactoryID:           bracket.FactoryID,
		SourceRef:           bracket.SourceRef,
		StartedAt:           bracket.StartedAt,
		ResultStatus:        bracket.ResultStatus,
		ResultSummary:       cloneWorkContentParts(bracket.ResultSummary),
		ArtifactIDs:         cloneStringSlice(bracket.ArtifactIDs),
		Terminal:            bracket.Terminal,
		FinalStatus:         bracket.FinalStatus,
		CompletedAt:         bracket.CompletedAt,
		DurationMillis:      bracket.DurationMillis,
		FailureReason:       bracket.FailureReason,
		FailureMessage:      bracket.FailureMessage,
	}
}

func cloneWorkContentParts(parts []interfaces.WorkContentPart) []interfaces.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]interfaces.WorkContentPart, len(parts))
	copy(cloned, parts)
	return cloned
}
