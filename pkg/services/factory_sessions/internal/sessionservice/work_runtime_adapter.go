package service

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

// workRuntimeAdapter is the Factory Sessions-owned adapter from one live
// engine into Work's consumer-owned runtime port. Engine identities end here.
type workRuntimeAdapter struct {
	sessionID string
	runtime   factoryruntime.Service
}

func (a workRuntimeAdapter) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return a.runtime.SubmitWorkRequest(ctx, request)
}

func (a workRuntimeAdapter) MoveWork(ctx context.Context, workID, state string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	return a.runtime.MoveWork(ctx, workID, state, source, requestID)
}

func (a workRuntimeAdapter) ReadWorkSnapshot(ctx context.Context) (work.ReadSnapshot, error) {
	legacyObservation, err := runtimebinding.LegacyObservationForService(a.runtime)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	snapshot, err := legacyObservation.GetEngineStateSnapshot(ctx)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	if snapshot == nil {
		return work.ReadSnapshot{}, nil
	}
	materialized := factoryruntime.CollectPublicWorkTokens(snapshot.Marking.Tokens, snapshot.Dispatches)
	names := runtimeWorkNames(materialized.Tokens)
	sessionSummary := sessionprojection.ProjectFactorySessionStopSummary(a.sessionID, snapshot, nil)
	result := work.ReadSnapshot{Items: make([]work.ReadModel, 0, len(materialized.Tokens))}
	for _, token := range materialized.Tokens {
		_, inFlight := materialized.InFlightOnlyByID[token.ID]
		item := runtimeWorkItem(token, snapshot.Topology, inFlight, names)
		item.StopSummary = runtimeWorkStopSummary(sessionprojection.ProjectWorkStopSummary(a.sessionID, snapshot, token, sessionSummary))
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func runtimeWorkItem(token *workers.Token, net *factoryruntime.Net, inFlight bool, names map[string]string) work.ReadModel {
	name := runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
	item := work.ReadModel{CursorID: token.ID, Name: name, WorkID: token.Color.WorkID, WorkTypeName: token.Color.WorkTypeID, State: runtimeWorkState(token, net, inFlight), ChainingTraceDepth: token.Color.ChainingTraceDepth, CurrentChainingTraceID: runtimeFirstNonEmpty(token.Color.CurrentChainingTraceID, token.Color.TraceID), PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...), TraceID: token.Color.TraceID, Content: work.CloneWorkContentParts(token.Color.Content), Tags: work.CloneTags(token.Color.Tags)}
	for _, relation := range token.Color.Relations {
		item.Relations = append(item.Relations, work.ReadRelation{Type: relation.Type, SourceWorkName: name, TargetWorkName: runtimeFirstNonEmpty(names[relation.TargetWorkID], relation.TargetWorkID), TargetWorkID: relation.TargetWorkID, RequiredState: relation.RequiredState})
	}
	return item
}

func runtimeWorkState(token *workers.Token, net *factoryruntime.Net, inFlight bool) *work.State {
	if token == nil {
		return nil
	}
	workType, stateName := factoryruntime.SplitPlaceID(token.PlaceID)
	if token.Color.WorkTypeID != "" {
		workType = token.Color.WorkTypeID
	}
	if net != nil {
		if place, ok := net.Places[token.PlaceID]; ok {
			workType, stateName = place.TypeID, place.State
		}
	}
	if stateName == "" {
		return nil
	}
	category := string(factoryruntime.CategoryForState(runtimeWorkTypes(net), workType, stateName))
	if inFlight {
		category = work.StateTypeProcessing
	}
	return &work.State{Name: stateName, Type: category}
}

func runtimeWorkTypes(net *factoryruntime.Net) map[string]*factoryruntime.WorkType {
	if net == nil {
		return nil
	}
	return net.WorkTypes
}
func runtimeWorkNames(tokens []*workers.Token) map[string]string {
	result := make(map[string]string, len(tokens))
	for _, token := range tokens {
		if token != nil && token.Color.WorkID != "" {
			result[token.Color.WorkID] = runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
		}
	}
	return result
}
func runtimeFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func runtimeWorkStopSummary(summary *factorysessions.StopSummary) *work.StopSummary {
	if summary == nil {
		return nil
	}
	result := &work.StopSummary{SessionID: summary.SessionID, StopKind: string(summary.StopKind), SessionLifecycleStatus: summary.SessionLifecycleStatus, WorkID: summary.WorkID, WorkName: summary.WorkName, WorkTypeName: summary.WorkTypeName, WorkState: summary.WorkState, LatestResultSummary: summary.LatestResultSummary, SuggestedRecoverySurface: summary.SuggestedRecoverySurface, SuggestedRecoveryAction: summary.SuggestedRecoveryAction}
	if summary.LatestDispatch != nil {
		result.LatestDispatch = &work.StopDispatchSummary{DispatchID: summary.LatestDispatch.DispatchID, Status: string(summary.LatestDispatch.Status), DispatchKind: string(summary.LatestDispatch.DispatchKind), WorkstationName: summary.LatestDispatch.WorkstationName}
		if summary.LatestDispatch.FailureDetail != nil {
			result.LatestDispatch.FailureDetail = &work.StopFailureDetail{Reason: string(summary.LatestDispatch.FailureDetail.Reason), Message: summary.LatestDispatch.FailureDetail.Message}
		}
	}
	return result
}
