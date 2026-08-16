package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
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

type runtimeWorkSubmitter interface {
	SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

// runtimeEventSubscriber is retained only for the P5B event-backbone seam.
// Recordings will become the sole event-read owner before this capability is
// removed from Factory Runtime.
type runtimeEventSubscriber interface {
	SubscribeFactoryEvents(
		context.Context,
		*interfaces.FactoryEventReconnectCursor,
		interfaces.FactoryEventReconnectScope,
	) (*interfaces.FactoryEventStream, error)
}

func (a workRuntimeAdapter) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	submitter, ok := a.runtime.(runtimeWorkSubmitter)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return submitter.SubmitWorkRequest(ctx, request)
}

func (a workRuntimeAdapter) MoveWork(ctx context.Context, workID, state string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	result, err := a.runtime.ControlMoveWork(ctx, factoryruntime.MoveWorkRequest{
		WorkID: workID, StateName: state, Source: factoryruntime.WorkMoveSource(source), RequestID: requestID,
	})
	if err != nil {
		if errors.Is(err, factoryruntime.ErrMoveWorkRequestConflict) {
			return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
		}
		return work.OperatorMoveResult{}, err
	}
	return work.OperatorMoveResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
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
		item := runtimeWorkItem(token, snapshot.Topology, inFlight, names, runtimeReadFacts{
			dispatches: snapshot.Dispatches, dispatchHistory: snapshot.DispatchHistory, results: snapshot.Results,
		})
		item.HumanApproval = runtimeHumanApprovalForWork(a.sessionID, token.Color.WorkID, snapshot.Dispatches, snapshot.Topology)
		item.StopSummary = runtimeWorkStopSummary(sessionprojection.ProjectWorkStopSummary(a.sessionID, snapshot, token, sessionSummary))
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func runtimeHumanApprovalForWork(
	sessionID string,
	workID string,
	dispatches map[string]*factoryruntime.DispatchEntry,
	topology *legacysnapshot.RuntimeTopology,
) *work.HumanApprovalReadModel {
	if workID == "" || topology == nil || len(dispatches) == 0 {
		return nil
	}
	dispatchIDs := make([]string, 0, len(dispatches))
	for dispatchID := range dispatches {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	for _, dispatchID := range dispatchIDs {
		entry := dispatches[dispatchID]
		if entry == nil {
			continue
		}
		transition := topology.Transitions[entry.TransitionID]
		if transition == nil || transition.Type != factoryruntime.PetriTransitionHumanApproval {
			continue
		}
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID != workID {
				continue
			}
			return &work.HumanApprovalReadModel{
				ApprovalID: approvalIDForDispatch(entry.DispatchID), SessionID: sessionID,
				DispatchID: entry.DispatchID, WorkstationID: entry.TransitionID,
				WorkstationName: entry.WorkstationName, Decisions: []string{"APPROVE", "REJECT"},
				Status: "PENDING",
			}
		}
	}
	return nil
}

func approvalIDForDispatch(dispatchID string) string {
	return "approval-" + dispatchID
}

type runtimeReadFacts struct {
	dispatches      map[string]*factoryruntime.DispatchEntry
	dispatchHistory []factoryruntime.CompletedDispatch
	results         []workers.WorkResult
}

func runtimeWorkItem(
	token *workers.Token,
	net *legacysnapshot.RuntimeTopology,
	inFlight bool,
	names map[string]string,
	facts ...runtimeReadFacts,
) work.ReadModel {
	var readFacts runtimeReadFacts
	if len(facts) > 0 {
		readFacts = facts[0]
	}
	name := runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
	item := work.ReadModel{CursorID: token.ID, Name: name, WorkID: token.Color.WorkID, WorkTypeName: token.Color.WorkTypeID, State: runtimeWorkState(token, net, inFlight), ChainingTraceDepth: token.Color.ChainingTraceDepth, CurrentChainingTraceID: runtimeFirstNonEmpty(token.Color.CurrentChainingTraceID, token.Color.TraceID), PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...), TraceID: token.Color.TraceID, Content: work.CloneWorkContentParts(token.Color.Content), StructuredResult: jsonvalue.Clone(token.Color.StructuredResult), StructuredResultPresent: jsonvalue.Present(token.Color.StructuredResult, token.Color.StructuredResultPresent), Tags: work.CloneTags(token.Color.Tags), ExpectedArtifacts: (factoryruntime.WorkArtifactProjection{}).Project(factoryruntime.WorkArtifactProjectionInput{Token: token, Topology: net, Dispatches: readFacts.dispatches, DispatchHistory: readFacts.dispatchHistory, Results: readFacts.results})}
	for _, relation := range token.Color.Relations {
		item.Relations = append(item.Relations, work.ReadRelation{Type: relation.Type, SourceWorkName: name, TargetWorkName: runtimeFirstNonEmpty(names[relation.TargetWorkID], relation.TargetWorkID), TargetWorkID: relation.TargetWorkID, RequiredState: relation.RequiredState})
	}
	return item
}

func runtimeWorkState(token *workers.Token, net *legacysnapshot.RuntimeTopology, inFlight bool) *work.State {
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

func runtimeWorkTypes(net *legacysnapshot.RuntimeTopology) map[string]*legacysnapshot.RuntimeWorkType {
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
