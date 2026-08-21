package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	// ingress is the Work-submission boundary declared when Factory Sessions
	// bound the runtime. It retires with factoryruntime.APIFactory.
	ingress factoryruntime.APIFactory
}

func (a workRuntimeAdapter) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if a.ingress == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return a.ingress.SubmitWorkRequest(ctx, request)
}

func (a workRuntimeAdapter) MoveWork(ctx context.Context, workID, state string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	if a.runtime == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Factory Runtime work move is required")
	}
	result, err := a.runtime.ControlMoveWork(ctx, factoryruntime.MoveWorkRequest{
		WorkID: workID, StateName: state, Source: factoryruntime.WorkMoveSource(source), RequestID: requestID,
	})
	if err != nil {
		return work.OperatorMoveResult{}, translateMoveWorkFailure(err)
	}
	return work.OperatorMoveResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

// translateMoveWorkFailure detaches engine-owned operator-move failures into
// the Work-owned sentinels Work's own surfaces branch on. Engine error identity
// ends at this adapter, exactly like engine result identity does. Failures the
// engine does not classify pass through unchanged.
func translateMoveWorkFailure(err error) error {
	switch {
	case errors.Is(err, factoryruntime.ErrMoveWorkRequestConflict):
		return work.ErrMoveWorkRequestAlreadyApplied
	case errors.Is(err, factoryruntime.ErrMoveWorkNotFound):
		return work.ErrMoveWorkNotFound
	case errors.Is(err, factoryruntime.ErrMoveWorkInvalidState):
		return work.ErrMoveWorkInvalidState
	case errors.Is(err, factoryruntime.ErrMoveWorkInFlightDispatch):
		return work.ErrMoveWorkInFlightDispatch
	case errors.Is(err, factoryruntime.ErrMoveWorkEngineTerminated):
		return work.ErrMoveWorkEngineTerminated
	default:
		return err
	}
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
	admissions, err := a.readWorkAdmissions(ctx)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	result.Admissions = admissions
	return result, nil
}

func (a workRuntimeAdapter) readWorkAdmissions(ctx context.Context) ([]work.WorkAdmission, error) {
	if a.ingress == nil {
		return nil, errors.New("Factory Runtime Work admission history is required")
	}
	historyContext := ctx
	if historyContext == nil {
		historyContext = context.Background()
	}
	historyContext, cancelHistory := context.WithCancel(historyContext)
	defer cancelHistory()
	stream, err := a.ingress.SubscribeFactoryEvents(
		historyContext,
		nil,
		factorydefinitions.FactoryEventReconnectScope{SessionID: a.sessionID},
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe Work admission history: %w", err)
	}
	if stream == nil {
		return nil, errors.New("subscribe Work admission history: stream is unavailable")
	}
	return workAdmissionsFromFactoryEvents(a.sessionID, stream.History), nil
}

func workAdmissionsFromFactoryEvents(
	sessionID string,
	events []factorydefinitions.FactoryEvent,
) []work.WorkAdmission {
	admissions := make([]work.WorkAdmission, 0)
	for _, event := range events {
		if event.Type != factorydefinitions.FactoryEventTypeWorkRequest {
			continue
		}
		if sessionID != "" && event.Context.SessionID != nil && *event.Context.SessionID != sessionID {
			continue
		}
		var payload work.WorkRequestEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		for index, item := range payload.Works {
			workID := item.WorkID
			if workID == "" && event.Context.WorkIDs != nil && index < len(*event.Context.WorkIDs) {
				workID = (*event.Context.WorkIDs)[index]
			}
			if workID == "" || item.Name == "" {
				continue
			}
			admissions = append(admissions, work.WorkAdmission{
				WorkID: workID,
				Name:   item.Name,
				Order:  len(admissions),
			})
		}
	}
	return admissions
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
	workType, stateName := token.Color.WorkTypeID, token.State
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
