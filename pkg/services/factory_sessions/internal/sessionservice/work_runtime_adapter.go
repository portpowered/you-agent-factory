package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
	submitter, ok := a.runtime.(factoryruntime.APIFactory)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("legacy Factory Runtime submission is required")
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
		item.StopSummary = runtimeWorkStopSummary(sessionprojection.ProjectWorkStopSummary(a.sessionID, snapshot, token, sessionSummary))
		result.Items = append(result.Items, item)
	}
	return result, nil
}

type runtimeReadFacts struct {
	dispatches      map[string]*factoryruntime.DispatchEntry
	dispatchHistory []factoryruntime.CompletedDispatch
	results         []workers.WorkResult
}

func runtimeWorkItem(
	token *workers.Token,
	net *factoryruntime.Net,
	inFlight bool,
	names map[string]string,
	facts ...runtimeReadFacts,
) work.ReadModel {
	var readFacts runtimeReadFacts
	if len(facts) > 0 {
		readFacts = facts[0]
	}
	name := runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
	item := work.ReadModel{CursorID: token.ID, Name: name, WorkID: token.Color.WorkID, WorkTypeName: token.Color.WorkTypeID, State: runtimeWorkState(token, net, inFlight), ChainingTraceDepth: token.Color.ChainingTraceDepth, CurrentChainingTraceID: runtimeFirstNonEmpty(token.Color.CurrentChainingTraceID, token.Color.TraceID), PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...), TraceID: token.Color.TraceID, Content: work.CloneWorkContentParts(token.Color.Content), Tags: work.CloneTags(token.Color.Tags), ExpectedArtifacts: runtimeExpectedArtifacts(token, net, readFacts.dispatches, readFacts.dispatchHistory, readFacts.results)}
	for _, relation := range token.Color.Relations {
		item.Relations = append(item.Relations, work.ReadRelation{Type: relation.Type, SourceWorkName: name, TargetWorkName: runtimeFirstNonEmpty(names[relation.TargetWorkID], relation.TargetWorkID), TargetWorkID: relation.TargetWorkID, RequiredState: relation.RequiredState})
	}
	return item
}

func runtimeExpectedArtifacts(
	token *workers.Token,
	net *factoryruntime.Net,
	dispatches map[string]*factoryruntime.DispatchEntry,
	dispatchHistory []factoryruntime.CompletedDispatch,
	results []workers.WorkResult,
) []work.ExpectedArtifactReadModel {
	if token == nil {
		return nil
	}
	var workTypeDeclarations []work.ExpectedArtifactDeclaration
	if net != nil {
		if workType := net.WorkTypes[token.Color.WorkTypeID]; workType != nil {
			workTypeDeclarations = append(workTypeDeclarations, workType.ExpectedArtifacts...)
		}
	}

	if dispatch, ok := runtimeActiveArtifactDispatch(token, dispatches); ok {
		return work.ExpectedArtifactReadModelProjector{}.Project(
			workTypeDeclarations,
			runtimeWorkstationArtifactDeclarations(net, dispatch.transitionID, dispatch.workstationName),
			runtimeExpectedArtifactInputs(dispatch.inputs, token),
			work.ExpectedArtifactObservation{},
			runtimeExpectedArtifactTemplateContext(dispatch.templateContext)...,
		)
	}
	if dispatch, ok := runtimeCompletedArtifactDispatch(token, dispatchHistory, results); ok {
		return work.ExpectedArtifactReadModelProjector{}.Project(
			workTypeDeclarations,
			runtimeWorkstationArtifactDeclarations(net, dispatch.transitionID, dispatch.workstationName),
			runtimeExpectedArtifactInputs(dispatch.inputs, token),
			dispatch.observation,
			runtimeExpectedArtifactTemplateContext(dispatch.templateContext)...,
		)
	}

	var workstationDeclarations []work.ExpectedArtifactDeclaration
	if net != nil {
		transitionIDs := make([]string, 0, len(net.Transitions))
		for transitionID := range net.Transitions {
			transitionIDs = append(transitionIDs, transitionID)
		}
		sort.Strings(transitionIDs)
		for _, transitionID := range transitionIDs {
			transition := net.Transitions[transitionID]
			if transition == nil {
				continue
			}
			consumesPlace := false
			for _, arc := range transition.InputArcs {
				if arc.PlaceID == token.PlaceID {
					consumesPlace = true
					break
				}
			}
			if !consumesPlace {
				continue
			}
			workstationDeclarations = append(workstationDeclarations, transition.ExpectedArtifacts...)
		}
	}
	return work.ExpectedArtifactReadModelProjector{}.Project(
		workTypeDeclarations,
		workstationDeclarations,
		[]work.ExpectedArtifactInput{runtimeExpectedArtifactInput(*token)},
		work.ExpectedArtifactObservation{},
	)
}

type runtimeArtifactDispatchFacts struct {
	transitionID    string
	workstationName string
	inputs          []workers.Token
	observation     work.ExpectedArtifactObservation
	templateContext *work.ExpectedArtifactTemplateContext
}

func runtimeActiveArtifactDispatch(
	token *workers.Token,
	dispatches map[string]*factoryruntime.DispatchEntry,
) (runtimeArtifactDispatchFacts, bool) {
	ids := make([]string, 0, len(dispatches))
	for id := range dispatches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		dispatch := dispatches[id]
		if dispatch == nil || !runtimeDispatchContainsWork(dispatch.ConsumedTokens, token.Color.WorkID) {
			continue
		}
		return runtimeArtifactDispatchFacts{
			transitionID:    dispatch.TransitionID,
			workstationName: dispatch.WorkstationName,
			inputs:          append([]workers.Token(nil), dispatch.ConsumedTokens...),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return runtimeArtifactDispatchFacts{}, false
}

func runtimeCompletedArtifactDispatch(
	token *workers.Token,
	dispatches []factoryruntime.CompletedDispatch,
	results []workers.WorkResult,
) (runtimeArtifactDispatchFacts, bool) {
	for index := len(dispatches) - 1; index >= 0; index-- {
		dispatch := dispatches[index]
		if !runtimeDispatchContainsWork(dispatch.ConsumedTokens, token.Color.WorkID) {
			continue
		}
		return runtimeArtifactDispatchFacts{
			transitionID:    dispatch.TransitionID,
			workstationName: dispatch.WorkstationName,
			inputs:          append([]workers.Token(nil), dispatch.ConsumedTokens...),
			observation:     runtimeArtifactObservation(dispatch, results),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return runtimeArtifactDispatchFacts{}, false
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	if context == nil {
		return nil
	}
	clone := *context
	return &clone
}

func runtimeDispatchContainsWork(tokens []workers.Token, workID string) bool {
	for _, token := range tokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func runtimeArtifactObservation(
	dispatch factoryruntime.CompletedDispatch,
	results []workers.WorkResult,
) work.ExpectedArtifactObservation {
	if dispatch.ArtifactVerification != nil {
		return runtimeExpectedArtifactObservation(dispatch.ArtifactVerification)
	}
	for _, result := range results {
		if result.DispatchID != dispatch.DispatchID {
			continue
		}
		if result.ArtifactVerification == nil {
			if result.Outcome == workers.OutcomeAccepted {
				return work.ExpectedArtifactObservation{Verified: true}
			}
			return work.ExpectedArtifactObservation{}
		}
		return runtimeExpectedArtifactObservation(result.ArtifactVerification)
	}
	if dispatch.Outcome == workers.OutcomeAccepted {
		return work.ExpectedArtifactObservation{Verified: true}
	}
	return work.ExpectedArtifactObservation{}
}

func runtimeExpectedArtifactObservation(
	verification *workers.ExpectedArtifactVerification,
) work.ExpectedArtifactObservation {
	if verification == nil {
		return work.ExpectedArtifactObservation{}
	}
	observation := work.ExpectedArtifactObservation{Verified: true}
	for _, entry := range verification.Entries {
		observation.Entries = append(observation.Entries, work.ExpectedArtifactVerificationEntry{
			DeclarationIndex: entry.DeclarationIndex,
			Name:             entry.Name, Pattern: entry.Pattern, Reason: work.ExpectedArtifactVerificationReason(entry.Reason),
		})
	}
	return observation
}

func runtimeWorkstationArtifactDeclarations(
	net *factoryruntime.Net,
	transitionID string,
	workstationName string,
) []work.ExpectedArtifactDeclaration {
	if net == nil {
		return nil
	}
	for _, transition := range net.Transitions {
		if transition == nil {
			continue
		}
		if transition.ID == transitionID || transition.Name == transitionID || transition.Name == workstationName {
			return append([]work.ExpectedArtifactDeclaration(nil), transition.ExpectedArtifacts...)
		}
	}
	return nil
}

func runtimeExpectedArtifactInputs(tokens []workers.Token, fallback *workers.Token) []work.ExpectedArtifactInput {
	if len(tokens) == 0 && fallback != nil {
		tokens = []workers.Token{*fallback}
	}
	inputs := make([]work.ExpectedArtifactInput, 0, len(tokens))
	for _, token := range tokens {
		inputs = append(inputs, runtimeExpectedArtifactInput(token))
	}
	return inputs
}

func runtimeExpectedArtifactInput(token workers.Token) work.ExpectedArtifactInput {
	return work.ExpectedArtifactInput{
		Name:       runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID),
		WorkID:     token.Color.WorkID,
		WorkTypeID: token.Color.WorkTypeID,
		DataType:   string(token.Color.DataType),
		TraceID:    token.Color.TraceID,
		ParentID:   token.Color.ParentID,
		Project:    token.Color.Tags[workers.ProjectTagKey],
		Tags:       work.CloneTags(token.Color.Tags),
		Payload:    string(token.Color.Payload),
	}
}

func runtimeExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) []work.ExpectedArtifactTemplateContext {
	if context == nil {
		return nil
	}
	return []work.ExpectedArtifactTemplateContext{*context}
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
