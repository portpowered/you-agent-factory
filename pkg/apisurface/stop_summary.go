package apisurface

import (
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/materialize"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type stopSummaryWork struct {
	id       string
	name     string
	workType string
	state    string
	token    *interfaces.Token
}

type stopSummaryDispatch struct {
	id              string
	status          factoryapi.FactoryDispatchStatus
	dispatchKind    factoryapi.FactoryDispatchKind
	workstationName string
	failureReason   string
	failureMessage  string
}

// BuildFactorySessionStopSummary derives the canonical stopped-state inspect
// summary for one live Factory Session read without introducing goal-specific
// public surfaces.
func BuildFactorySessionStopSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	javascript *interfaces.FactorySessionJavaScriptRuntimeState,
) *factoryapi.FactoryStopSummary {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	materialized := materializedPublicWork(snapshot)
	if pausedSummary := buildPausedStopSummary(sessionID, snapshot, materialized); pausedSummary != nil {
		return pausedSummary
	}
	if interruptedSummary := buildInterruptedStopSummary(sessionID, snapshot, javascript, materialized); interruptedSummary != nil {
		return interruptedSummary
	}
	if blockedSummary := buildStoppedWorkStateSummary(sessionID, snapshot, materialized, "blocked", factoryapi.FactoryStopKind("BLOCKED")); blockedSummary != nil {
		return blockedSummary
	}
	if needsHumanSummary := buildStoppedWorkStateSummary(sessionID, snapshot, materialized, "needs-human", factoryapi.FactoryStopKind("NEEDS_HUMAN")); needsHumanSummary != nil {
		return needsHumanSummary
	}
	return nil
}

// BuildWorkStopSummary derives the canonical stopped-state inspect summary for
// one work read when that work item explains the current stop condition.
func BuildWorkStopSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	token *interfaces.Token,
) *factoryapi.FactoryStopSummary {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || token == nil {
		return nil
	}

	target := workFromToken(token, snapshotTopology(snapshot))
	if target.id == "" {
		return nil
	}
	materialized := materializedPublicWork(snapshot)
	if lifecycleStatus := strings.TrimSpace(lifecycleControlStatus(snapshot)); lifecycleStatus == string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		status := factoryapi.FactorySessionDurableLifecycleStatusPaused
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("PAUSED"), &status, target, latestRelevantDispatch(target.id, snapshot), "")
	}

	switch target.state {
	case "blocked":
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("BLOCKED"), nil, target, latestRelevantDispatch(target.id, snapshot), "")
	case "needs-human":
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("NEEDS_HUMAN"), nil, target, latestRelevantDispatch(target.id, snapshot), "")
	}

	if matching := workByID(materialized, target.id); matching != nil && matching.state != "" {
		switch matching.state {
		case "blocked":
			return buildStopSummary(sessionID, factoryapi.FactoryStopKind("BLOCKED"), nil, *matching, latestRelevantDispatch(target.id, snapshot), "")
		case "needs-human":
			return buildStopSummary(sessionID, factoryapi.FactoryStopKind("NEEDS_HUMAN"), nil, *matching, latestRelevantDispatch(target.id, snapshot), "")
		}
	}
	return nil
}

func buildPausedStopSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	materialized materialize.PublicWorkTokens,
) *factoryapi.FactoryStopSummary {
	if strings.TrimSpace(lifecycleControlStatus(snapshot)) != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		return nil
	}
	work := latestRelevantWork(materialized, snapshotTopology(snapshot))
	status := factoryapi.FactorySessionDurableLifecycleStatusPaused
	if work == nil {
		return &factoryapi.FactoryStopSummary{
			SessionId:              sessionID,
			StopKind:               factoryapi.FactoryStopKind("PAUSED"),
			SessionLifecycleStatus: &status,
		}
	}
	return buildStopSummary(sessionID, factoryapi.FactoryStopKind("PAUSED"), &status, *work, latestRelevantDispatch(work.id, snapshot), "")
}

func buildInterruptedStopSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	javascript *interfaces.FactorySessionJavaScriptRuntimeState,
	materialized materialize.PublicWorkTokens,
) *factoryapi.FactoryStopSummary {
	if javascript == nil {
		return nil
	}
	var stoppedDispatch *stopSummaryDispatch
	var stoppedWork *stopSummaryWork
	for _, dispatch := range javascript.Dispatches {
		if strings.TrimSpace(dispatch.Status) != string(factoryapi.FactoryDispatchStatusINTERRUPTED) {
			continue
		}
		dispatchCopy := interruptDispatchSummary(dispatch)
		stoppedDispatch = &dispatchCopy
		for _, workID := range dispatch.RelatedWorkIDs {
			if work := workByID(materialized, workID); work != nil {
				stoppedWork = work
				break
			}
		}
		if stoppedWork == nil {
			stoppedWork = latestRelevantWork(materialized, snapshotTopology(snapshot))
		}
		break
	}
	if stoppedDispatch == nil {
		return nil
	}
	if stoppedWork == nil {
		return &factoryapi.FactoryStopSummary{
			SessionId:      sessionID,
			StopKind:       factoryapi.FactoryStopKind("INTERRUPTED"),
			LatestDispatch: projectedStopDispatch(*stoppedDispatch),
		}
	}
	return buildStopSummary(sessionID, factoryapi.FactoryStopKind("INTERRUPTED"), nil, *stoppedWork, stoppedDispatch, interruptedResultSummary(*stoppedDispatch))
}

func buildStoppedWorkStateSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	materialized materialize.PublicWorkTokens,
	stateName string,
	stopKind factoryapi.FactoryStopKind,
) *factoryapi.FactoryStopSummary {
	work := latestWorkInState(materialized, snapshotTopology(snapshot), stateName)
	if work == nil {
		return nil
	}
	return buildStopSummary(sessionID, stopKind, nil, *work, latestRelevantDispatch(work.id, snapshot), "")
}

func buildStopSummary(
	sessionID string,
	stopKind factoryapi.FactoryStopKind,
	lifecycleStatus *factoryapi.FactorySessionDurableLifecycleStatus,
	work stopSummaryWork,
	dispatch *stopSummaryDispatch,
	resultSummary string,
) *factoryapi.FactoryStopSummary {
	summary := &factoryapi.FactoryStopSummary{
		SessionId: sessionID,
		StopKind:  stopKind,
	}
	if lifecycleStatus != nil {
		summary.SessionLifecycleStatus = lifecycleStatus
	}
	if work.id != "" {
		summary.WorkId = stringPtr(work.id)
	}
	if work.name != "" {
		summary.WorkName = stringPtr(work.name)
	}
	if work.workType != "" {
		summary.WorkTypeName = stringPtr(work.workType)
	}
	if label := workStateLabel(work.workType, work.state); label != "" {
		summary.WorkState = stringPtr(label)
	}
	if dispatch != nil {
		summary.LatestDispatch = projectedStopDispatch(*dispatch)
	}
	if strings.TrimSpace(resultSummary) != "" {
		summary.LatestResultSummary = stringPtr(strings.TrimSpace(resultSummary))
	}
	return summary
}

func interruptDispatchSummary(dispatch interfaces.FactorySessionDispatchState) stopSummaryDispatch {
	return stopSummaryDispatch{
		id:              strings.TrimSpace(dispatch.ID),
		status:          factoryapi.FactoryDispatchStatusINTERRUPTED,
		dispatchKind:    factoryapi.FactoryDispatchKind(strings.TrimSpace(dispatch.DispatchKind)),
		workstationName: strings.TrimSpace(dispatch.Label),
		failureReason:   strings.TrimSpace(failureReasonFromDispatchState(dispatch)),
		failureMessage:  strings.TrimSpace(failureMessageFromDispatchState(dispatch)),
	}
}

func latestRelevantDispatch(
	workID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *stopSummaryDispatch {
	workID = strings.TrimSpace(workID)
	if workID == "" || snapshot == nil {
		return nil
	}
	var bestCompleted *interfaces.CompletedDispatch
	for i := range snapshot.DispatchHistory {
		completed := snapshot.DispatchHistory[i]
		if !dispatchTouchesWork(completed.ConsumedTokens, workID) {
			continue
		}
		if bestCompleted == nil || completed.EndTime.After(bestCompleted.EndTime) || (bestCompleted.EndTime.IsZero() && completed.StartTime.After(bestCompleted.StartTime)) {
			bestCompleted = &completed
		}
	}
	if bestCompleted != nil {
		dispatch := completedDispatchSummary(*bestCompleted)
		return &dispatch
	}

	activeIDs := make([]string, 0, len(snapshot.Dispatches))
	for dispatchID, entry := range snapshot.Dispatches {
		if entry == nil || !dispatchTouchesWork(entry.ConsumedTokens, workID) {
			continue
		}
		activeIDs = append(activeIDs, dispatchID)
	}
	sort.Strings(activeIDs)
	if len(activeIDs) == 0 {
		return nil
	}
	entry := snapshot.Dispatches[activeIDs[len(activeIDs)-1]]
	dispatch := activeDispatchSummary(activeIDs[len(activeIDs)-1], *entry)
	return &dispatch
}

func activeDispatchSummary(dispatchID string, entry interfaces.DispatchEntry) stopSummaryDispatch {
	return stopSummaryDispatch{
		id:              strings.TrimSpace(dispatchID),
		status:          factoryapi.FactoryDispatchStatusRUNNING,
		dispatchKind:    factoryapi.FactoryDispatchKindPETRITRANSITION,
		workstationName: strings.TrimSpace(entry.WorkstationName),
	}
}

func completedDispatchSummary(completed interfaces.CompletedDispatch) stopSummaryDispatch {
	status := factoryapi.FactoryDispatchStatusCOMPLETED
	switch completed.Outcome {
	case interfaces.OutcomeFailed, interfaces.OutcomeRejected:
		status = factoryapi.FactoryDispatchStatusFAILED
	}
	return stopSummaryDispatch{
		id:              strings.TrimSpace(completed.DispatchID),
		status:          status,
		dispatchKind:    factoryapi.FactoryDispatchKindPETRITRANSITION,
		workstationName: strings.TrimSpace(completed.WorkstationName),
		failureReason:   completedFailureReason(completed),
		failureMessage:  strings.TrimSpace(completed.Reason),
	}
}

func projectedStopDispatch(dispatch stopSummaryDispatch) *factoryapi.FactoryStopDispatchSummary {
	if dispatch.id == "" {
		return nil
	}
	projected := &factoryapi.FactoryStopDispatchSummary{
		DispatchId:   dispatch.id,
		Status:       dispatch.status,
		DispatchKind: dispatch.dispatchKind,
	}
	if dispatch.workstationName != "" {
		projected.WorkstationName = stringPtr(dispatch.workstationName)
	}
	if dispatch.failureReason != "" {
		projected.FailureReason = stringPtr(dispatch.failureReason)
	}
	if dispatch.failureMessage != "" {
		projected.FailureMessage = stringPtr(dispatch.failureMessage)
	}
	return projected
}

func latestRelevantWork(
	materialized materialize.PublicWorkTokens,
	topology *state.Net,
) *stopSummaryWork {
	var works []stopSummaryWork
	for _, token := range materialized.Tokens {
		work := workFromToken(token, topology)
		if work.id == "" {
			continue
		}
		works = append(works, work)
	}
	if len(works) == 0 {
		return nil
	}
	sort.SliceStable(works, func(i, j int) bool {
		return works[i].token.EnteredAt.After(works[j].token.EnteredAt)
	})
	return &works[0]
}

func latestWorkInState(
	materialized materialize.PublicWorkTokens,
	topology *state.Net,
	stateName string,
) *stopSummaryWork {
	var matches []stopSummaryWork
	for _, token := range materialized.Tokens {
		work := workFromToken(token, topology)
		if work.id == "" || work.state != stateName {
			continue
		}
		matches = append(matches, work)
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].token.EnteredAt.After(matches[j].token.EnteredAt)
	})
	return &matches[0]
}

func workByID(materialized materialize.PublicWorkTokens, workID string) *stopSummaryWork {
	for _, token := range materialized.Tokens {
		work := workFromToken(token, nil)
		if work.id == strings.TrimSpace(workID) {
			return &work
		}
	}
	return nil
}

func workFromToken(token *interfaces.Token, topology *state.Net) stopSummaryWork {
	if token == nil {
		return stopSummaryWork{}
	}
	workTypeID, stateName := state.SplitPlaceID(token.PlaceID)
	if token.Color.WorkTypeID != "" {
		workTypeID = token.Color.WorkTypeID
	}
	if topology != nil {
		if place, ok := topology.Places[token.PlaceID]; ok && place != nil {
			if strings.TrimSpace(place.TypeID) != "" {
				workTypeID = strings.TrimSpace(place.TypeID)
			}
			if strings.TrimSpace(place.State) != "" {
				stateName = strings.TrimSpace(place.State)
			}
		}
	}
	return stopSummaryWork{
		id:       strings.TrimSpace(token.Color.WorkID),
		name:     strings.TrimSpace(firstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)),
		workType: strings.TrimSpace(workTypeID),
		state:    strings.TrimSpace(stateName),
		token:    token,
	}
}

func lifecycleControlStatus(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.LifecycleControlStatus
}

func materializedPublicWork(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) materialize.PublicWorkTokens {
	if snapshot == nil {
		return materialize.PublicWorkTokens{}
	}
	return materialize.CollectPublicWorkTokens(&snapshot.Marking, snapshot.Dispatches)
}

func snapshotTopology(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *state.Net {
	if snapshot == nil {
		return nil
	}
	return snapshot.Topology
}

func dispatchTouchesWork(tokens []interfaces.Token, workID string) bool {
	for _, token := range tokens {
		if strings.TrimSpace(token.Color.WorkID) == workID {
			return true
		}
	}
	return false
}

func workStateLabel(workTypeName, stateName string) string {
	workTypeName = strings.TrimSpace(workTypeName)
	stateName = strings.TrimSpace(stateName)
	switch {
	case workTypeName == "":
		return stateName
	case stateName == "":
		return workTypeName
	default:
		return workTypeName + ":" + stateName
	}
}

func failureReasonFromDispatchState(dispatch interfaces.FactorySessionDispatchState) string {
	if dispatch.FailureDetail == nil {
		return ""
	}
	return dispatch.FailureDetail.Reason
}

func failureMessageFromDispatchState(dispatch interfaces.FactorySessionDispatchState) string {
	if dispatch.FailureDetail == nil {
		return ""
	}
	return firstNonEmpty(dispatch.FailureDetail.Message, dispatch.FailureDetail.ErrorClass)
}

func interruptedResultSummary(dispatch stopSummaryDispatch) string {
	if dispatch.failureMessage != "" {
		return dispatch.failureMessage
	}
	if dispatch.failureReason != "" {
		return dispatch.failureReason
	}
	return "Latest relevant dispatch was interrupted before normal completion."
}

func completedFailureReason(completed interfaces.CompletedDispatch) string {
	if completed.FailureMetadata == nil {
		return ""
	}
	return strings.TrimSpace(string(completed.FailureMetadata.Type))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
