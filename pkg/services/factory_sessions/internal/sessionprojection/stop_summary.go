package sessionprojection

import (
	"fmt"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type stopSummaryWork struct {
	id, name, workType, state string
	token                     *factorytoken.Token
}

type stopSummaryRecovery struct{ resultSummary, surface, action string }

type stopSummaryDispatch struct {
	id, workstationName, failureReason, failureMessage string
	status                                             StopDispatchStatus
	dispatchKind                                       StopDispatchKind
}

// ProjectFactorySessionStopSummary derives the canonical stopped-state inspect
// summary for one live Factory Session. Stop precedence is owner policy:
// paused, JavaScript interruption, interrupted Work, blocked Work, then Work
// awaiting human input.
func ProjectFactorySessionStopSummary(sessionID string, snapshot *legacysnapshot.Snapshot, javascript *interfaces.FactorySessionJavaScriptRuntimeState) *StopSummary {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	materialized := materializedPublicWork(snapshot)
	if summary := buildPausedStopSummary(sessionID, snapshot, materialized); summary != nil {
		return summary
	}
	if summary := buildInterruptedStopSummary(sessionID, snapshot, javascript, materialized); summary != nil {
		return summary
	}
	if summary := buildInterruptedWorkStateSummary(sessionID, snapshot, materialized); summary != nil {
		return summary
	}
	if summary := buildStoppedWorkStateSummary(sessionID, snapshot, materialized, "blocked", StopKindBlocked); summary != nil {
		return summary
	}
	return buildStoppedWorkStateSummary(sessionID, snapshot, materialized, "needs-human", StopKindNeedsHuman)
}

// ProjectWorkStopSummary derives the canonical stopped-state inspect summary
// for one Work read when that Work explains the current stop condition.
func ProjectWorkStopSummary(sessionID string, snapshot *legacysnapshot.Snapshot, token *factorytoken.Token, sessionStopSummary *StopSummary) *StopSummary {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || token == nil {
		return nil
	}
	target := workFromToken(token, snapshotTopology(snapshot))
	if target.id == "" {
		return nil
	}
	if lifecycleControlStatus(snapshot) == "PAUSED" {
		status := "PAUSED"
		return buildStopSummary(sessionID, StopKindPaused, &status, target, latestRelevantDispatch(target.id, snapshot), pausedRecoverySummary(sessionID))
	}
	interrupted := interruptedWorkStopSummary(target.id, sessionStopSummary)
	if summary := stopSummaryForWorkState(sessionID, target, snapshot, interrupted); summary != nil {
		return summary
	}
	if matching := workByID(materializedPublicWork(snapshot), target.id); matching != nil && matching.state != "" {
		if summary := stopSummaryForWorkState(sessionID, *matching, snapshot, interrupted); summary != nil {
			return summary
		}
	}
	return interrupted
}

func interruptedWorkStopSummary(workID string, summary *StopSummary) *StopSummary {
	if summary == nil || summary.StopKind != StopKindInterrupted || summary.WorkID == nil || strings.TrimSpace(*summary.WorkID) != strings.TrimSpace(workID) {
		return nil
	}
	copy := *summary
	return &copy
}

func stopSummaryForWorkState(sessionID string, work stopSummaryWork, snapshot *legacysnapshot.Snapshot, interrupted *StopSummary) *StopSummary {
	switch work.state {
	case "interrupted":
		if interrupted != nil {
			return interrupted
		}
		return buildStopSummary(sessionID, StopKindInterrupted, nil, work, interruptedDispatchFromWork(work, snapshot), interruptedWorkRecoverySummary(sessionID, work))
	case "blocked":
		return buildStopSummary(sessionID, StopKindBlocked, nil, work, latestRelevantDispatch(work.id, snapshot), blockedRecoverySummary(work))
	case "needs-human":
		return buildStopSummary(sessionID, StopKindNeedsHuman, nil, work, latestRelevantDispatch(work.id, snapshot), needsHumanRecoverySummary(work))
	default:
		return nil
	}
}

func buildPausedStopSummary(sessionID string, snapshot *legacysnapshot.Snapshot, materialized factory.PublicWorkTokens) *StopSummary {
	if lifecycleControlStatus(snapshot) != "PAUSED" {
		return nil
	}
	status := "PAUSED"
	recovery := pausedRecoverySummary(sessionID)
	work := latestRelevantWork(materialized, snapshotTopology(snapshot))
	if work == nil {
		return &StopSummary{SessionID: sessionID, StopKind: StopKindPaused, SessionLifecycleStatus: &status, SuggestedRecoverySurface: stringPtr(recovery.surface), SuggestedRecoveryAction: stringPtr(recovery.action)}
	}
	return buildStopSummary(sessionID, StopKindPaused, &status, *work, latestRelevantDispatch(work.id, snapshot), recovery)
}

func buildInterruptedStopSummary(sessionID string, snapshot *legacysnapshot.Snapshot, javascript *interfaces.FactorySessionJavaScriptRuntimeState, materialized factory.PublicWorkTokens) *StopSummary {
	if javascript == nil {
		return nil
	}
	var stoppedDispatch *stopSummaryDispatch
	var stoppedWork *stopSummaryWork
	for _, dispatch := range javascript.Dispatches {
		if strings.TrimSpace(dispatch.Status) != string(StopDispatchStatusInterrupted) {
			continue
		}
		copy := interruptDispatchSummary(dispatch)
		stoppedDispatch = &copy
		for _, workID := range dispatch.RelatedWorkIDs {
			if work := workByID(materialized, workID); work != nil {
				stoppedWork = work
				break
			}
		}
		break
	}
	if stoppedDispatch == nil {
		return nil
	}
	if stoppedWork == nil {
		recovery := interruptedRecoverySummary(sessionID)
		return &StopSummary{SessionID: sessionID, StopKind: StopKindInterrupted, LatestDispatch: projectedStopDispatch(*stoppedDispatch), LatestResultSummary: stringPtr(interruptedResultSummary(*stoppedDispatch)), SuggestedRecoverySurface: stringPtr(recovery.surface), SuggestedRecoveryAction: stringPtr(recovery.action)}
	}
	return buildStopSummary(sessionID, StopKindInterrupted, nil, *stoppedWork, stoppedDispatch, interruptedRecoverySummary(sessionID))
}

func buildInterruptedWorkStateSummary(sessionID string, snapshot *legacysnapshot.Snapshot, materialized factory.PublicWorkTokens) *StopSummary {
	work := latestWorkInState(materialized, snapshotTopology(snapshot), "interrupted")
	if work == nil {
		return nil
	}
	return buildStopSummary(sessionID, StopKindInterrupted, nil, *work, interruptedDispatchFromWork(*work, snapshot), interruptedWorkRecoverySummary(sessionID, *work))
}

func buildStoppedWorkStateSummary(sessionID string, snapshot *legacysnapshot.Snapshot, materialized factory.PublicWorkTokens, stateName string, stopKind StopKind) *StopSummary {
	work := latestWorkInState(materialized, snapshotTopology(snapshot), stateName)
	if work == nil {
		return nil
	}
	return buildStopSummary(sessionID, stopKind, nil, *work, latestRelevantDispatch(work.id, snapshot), stopKindRecoverySummary(stopKind, *work))
}

func buildStopSummary(sessionID string, stopKind StopKind, lifecycleStatus *string, work stopSummaryWork, dispatch *stopSummaryDispatch, recovery stopSummaryRecovery) *StopSummary {
	summary := &StopSummary{SessionID: sessionID, StopKind: stopKind, SessionLifecycleStatus: lifecycleStatus}
	summary.WorkID, summary.WorkName, summary.WorkTypeName = stringPtr(work.id), stringPtr(work.name), stringPtr(work.workType)
	summary.WorkState = stringPtr(workStateLabel(work.workType, work.state))
	if dispatch != nil {
		summary.LatestDispatch = projectedStopDispatch(*dispatch)
	}
	if strings.TrimSpace(recovery.resultSummary) == "" {
		recovery.resultSummary = defaultRecoveryResultSummary(stopKind, work, dispatch)
	}
	summary.LatestResultSummary = stringPtr(recovery.resultSummary)
	summary.SuggestedRecoverySurface = stringPtr(recovery.surface)
	summary.SuggestedRecoveryAction = stringPtr(recovery.action)
	return summary
}

func interruptDispatchSummary(dispatch interfaces.FactorySessionDispatchState) stopSummaryDispatch {
	return stopSummaryDispatch{id: strings.TrimSpace(dispatch.ID), status: StopDispatchStatusInterrupted, dispatchKind: StopDispatchKind(strings.TrimSpace(dispatch.DispatchKind)), workstationName: strings.TrimSpace(dispatch.Label), failureReason: strings.TrimSpace(failureReasonFromDispatchState(dispatch)), failureMessage: strings.TrimSpace(failureMessageFromDispatchState(dispatch))}
}

func latestRelevantDispatch(workID string, snapshot *legacysnapshot.Snapshot) *stopSummaryDispatch {
	workID = strings.TrimSpace(workID)
	if workID == "" || snapshot == nil {
		return nil
	}
	var best *interfaces.CompletedDispatch
	for i := range snapshot.DispatchHistory {
		completed := snapshot.DispatchHistory[i]
		if !dispatchTouchesWork(completed.ConsumedTokens, workID) {
			continue
		}
		if best == nil || completed.EndTime.After(best.EndTime) || (best.EndTime.IsZero() && completed.StartTime.After(best.StartTime)) {
			best = &completed
		}
	}
	if best != nil {
		dispatch := completedDispatchSummary(*best)
		return &dispatch
	}
	var activeIDs []string
	for id, entry := range snapshot.Dispatches {
		if entry != nil && dispatchTouchesWork(entry.ConsumedTokens, workID) {
			activeIDs = append(activeIDs, id)
		}
	}
	sort.Strings(activeIDs)
	if len(activeIDs) == 0 {
		return nil
	}
	id := activeIDs[len(activeIDs)-1]
	dispatch := activeDispatchSummary(id, *snapshot.Dispatches[id])
	return &dispatch
}

func interruptedDispatchFromWork(work stopSummaryWork, snapshot *legacysnapshot.Snapshot) *stopSummaryDispatch {
	dispatch := latestRelevantDispatch(work.id, snapshot)
	if dispatch == nil {
		return nil
	}
	interrupted := *dispatch
	interrupted.status = StopDispatchStatusInterrupted
	interrupted.failureMessage = firstNonEmpty(failureMessageFromWork(work), interrupted.failureMessage, interrupted.failureReason)
	interrupted.failureReason = firstNonEmpty(interrupted.failureReason, "work_interrupted")
	return &interrupted
}

func activeDispatchSummary(id string, entry interfaces.DispatchEntry) stopSummaryDispatch {
	return stopSummaryDispatch{id: strings.TrimSpace(id), status: StopDispatchStatusRunning, dispatchKind: StopDispatchKindPetriTransition, workstationName: strings.TrimSpace(entry.WorkstationName)}
}

func completedDispatchSummary(completed interfaces.CompletedDispatch) stopSummaryDispatch {
	status := StopDispatchStatusCompleted
	if completed.Outcome == workerexecution.OutcomeFailed || completed.Outcome == workerexecution.OutcomeRejected {
		status = StopDispatchStatusFailed
	}
	return stopSummaryDispatch{id: strings.TrimSpace(completed.DispatchID), status: status, dispatchKind: StopDispatchKindPetriTransition, workstationName: strings.TrimSpace(completed.WorkstationName), failureReason: completedFailureReason(completed), failureMessage: strings.TrimSpace(completed.Reason)}
}

func projectedStopDispatch(dispatch stopSummaryDispatch) *StopDispatchSummary {
	if dispatch.id == "" {
		return nil
	}
	projected := &StopDispatchSummary{DispatchID: dispatch.id, Status: dispatch.status, DispatchKind: dispatch.dispatchKind, WorkstationName: stringPtr(dispatch.workstationName)}
	if dispatch.failureReason != "" && dispatch.failureMessage != "" {
		projected.FailureDetail = &StopFailureDetail{Reason: publicFailureReason(dispatch.failureReason), Message: dispatch.failureMessage}
	}
	return projected
}

func publicFailureReason(reason string) StopFailureType {
	candidate := StopFailureType(strings.TrimSpace(reason))
	switch candidate {
	case "auth_failure", "permanent_bad_request", "throttled", "internal_server_error", "timeout", "misconfigured", "missing_executable", "command_line_too_long":
		return candidate
	default:
		return StopFailureTypeUnknown
	}
}

func latestRelevantWork(materialized factory.PublicWorkTokens, topology *factory.Net) *stopSummaryWork {
	var works []stopSummaryWork
	for _, token := range materialized.Tokens {
		if work := workFromToken(token, topology); work.id != "" {
			works = append(works, work)
		}
	}
	if len(works) == 0 {
		return nil
	}
	sort.SliceStable(works, func(i, j int) bool { return works[i].token.EnteredAt.After(works[j].token.EnteredAt) })
	return &works[0]
}

func latestWorkInState(materialized factory.PublicWorkTokens, topology *factory.Net, stateName string) *stopSummaryWork {
	var works []stopSummaryWork
	for _, token := range materialized.Tokens {
		if work := workFromToken(token, topology); work.id != "" && work.state == stateName {
			works = append(works, work)
		}
	}
	if len(works) == 0 {
		return nil
	}
	sort.SliceStable(works, func(i, j int) bool { return works[i].token.EnteredAt.After(works[j].token.EnteredAt) })
	return &works[0]
}

func workByID(materialized factory.PublicWorkTokens, workID string) *stopSummaryWork {
	for _, token := range materialized.Tokens {
		if work := workFromToken(token, nil); work.id == strings.TrimSpace(workID) {
			return &work
		}
	}
	return nil
}

func workFromToken(token *factorytoken.Token, topology *factory.Net) stopSummaryWork {
	if token == nil {
		return stopSummaryWork{}
	}
	workType, stateName := factory.SplitPlaceID(token.PlaceID)
	if token.Color.WorkTypeID != "" {
		workType = token.Color.WorkTypeID
	}
	if topology != nil {
		if place := topology.Places[token.PlaceID]; place != nil {
			if strings.TrimSpace(place.TypeID) != "" {
				workType = strings.TrimSpace(place.TypeID)
			}
			if strings.TrimSpace(place.State) != "" {
				stateName = strings.TrimSpace(place.State)
			}
		}
	}
	return stopSummaryWork{id: strings.TrimSpace(token.Color.WorkID), name: strings.TrimSpace(firstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)), workType: strings.TrimSpace(workType), state: strings.TrimSpace(stateName), token: token}
}

func lifecycleControlStatus(snapshot *legacysnapshot.Snapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.LifecycleControlStatus)
}

func materializedPublicWork(snapshot *legacysnapshot.Snapshot) factory.PublicWorkTokens {
	if snapshot == nil {
		return factory.PublicWorkTokens{}
	}
	return factory.CollectPublicWorkTokens(snapshot.Marking.Tokens, snapshot.Dispatches)
}

func snapshotTopology(snapshot *legacysnapshot.Snapshot) *factory.Net {
	if snapshot == nil {
		return nil
	}
	return snapshot.Topology
}

func dispatchTouchesWork(tokens []factorytoken.Token, workID string) bool {
	for _, token := range tokens {
		if strings.TrimSpace(token.Color.WorkID) == workID {
			return true
		}
	}
	return false
}

func workStateLabel(workType, stateName string) string {
	workType, stateName = strings.TrimSpace(workType), strings.TrimSpace(stateName)
	if workType == "" {
		return stateName
	}
	if stateName == "" {
		return workType
	}
	return workType + ":" + stateName
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
	return strings.TrimSpace(dispatch.FailureDetail.Message)
}

func interruptedResultSummary(dispatch stopSummaryDispatch) string {
	return firstNonEmpty(dispatch.failureMessage, dispatch.failureReason, "Latest relevant dispatch was interrupted before normal completion.")
}

func stopKindRecoverySummary(kind StopKind, work stopSummaryWork) stopSummaryRecovery {
	switch kind {
	case StopKindBlocked:
		return blockedRecoverySummary(work)
	case StopKindNeedsHuman:
		return needsHumanRecoverySummary(work)
	case StopKindInterrupted:
		return interruptedRecoverySummary("")
	case StopKindPaused:
		return pausedRecoverySummary("")
	default:
		return stopSummaryRecovery{}
	}
}

func blockedRecoverySummary(work stopSummaryWork) stopSummaryRecovery {
	label := workReferenceLabel(work)
	return stopSummaryRecovery{resultSummary: firstNonEmpty(failureMessageFromWork(work), rejectionFeedbackFromWork(work)), surface: "existing work repair, work move, or follow-up submission controls", action: "Inspect the blocked work " + label + ", then use the existing work repair, work move, or follow-up submission controls to unblock it."}
}

func needsHumanRecoverySummary(work stopSummaryWork) stopSummaryRecovery {
	label := workReferenceLabel(work)
	return stopSummaryRecovery{resultSummary: firstNonEmpty(rejectionFeedbackFromWork(work), failureMessageFromWork(work)), surface: "existing work follow-up submission, work move, or session workflow controls", action: "Provide the requested human input, approval, or artifact review for work " + label + " through the existing workflow controls, then continue the normal session flow."}
}

func pausedRecoverySummary(sessionID string) stopSummaryRecovery {
	return stopSummaryRecovery{surface: "existing Factory Session resume control", action: "Resume the paused Factory Session " + sessionIDReference(sessionID) + " with the existing session resume control, then re-check queued or buffered work."}
}

func interruptedRecoverySummary(sessionID string) stopSummaryRecovery {
	return stopSummaryRecovery{surface: "existing dispatch retry, work repair, or session workflow controls", action: "Inspect the interrupted dispatch in Factory Session " + sessionIDReference(sessionID) + ", then use the existing retry, repair, or session workflow controls to continue recovery."}
}

func interruptedWorkRecoverySummary(sessionID string, work stopSummaryWork) stopSummaryRecovery {
	recovery := interruptedRecoverySummary(sessionID)
	recovery.resultSummary = firstNonEmpty(failureMessageFromWork(work), rejectionFeedbackFromWork(work))
	return recovery
}

func defaultRecoveryResultSummary(kind StopKind, _ stopSummaryWork, dispatch *stopSummaryDispatch) string {
	if dispatch == nil {
		return ""
	}
	if kind == StopKindInterrupted {
		return interruptedResultSummary(*dispatch)
	}
	return firstNonEmpty(dispatch.failureMessage, dispatch.failureReason)
}

func failureMessageFromWork(work stopSummaryWork) string {
	if work.token == nil {
		return ""
	}
	return firstNonEmpty(work.token.History.LastError, latestFailureLogMessage(work.token.History))
}

func latestFailureLogMessage(history factorytoken.History) string {
	if len(history.FailureLog) == 0 {
		return ""
	}
	return strings.TrimSpace(history.FailureLog[len(history.FailureLog)-1].Error)
}

func rejectionFeedbackFromWork(work stopSummaryWork) string {
	if work.token == nil || work.token.Color.Tags == nil {
		return ""
	}
	return strings.TrimSpace(work.token.Color.Tags[interfaces.RejectionFeedback])
}

func workReferenceLabel(work stopSummaryWork) string {
	if work.name != "" && work.id != "" {
		return fmt.Sprintf("%q [%s]", work.name, work.id)
	}
	return fmt.Sprintf("%q", firstNonEmpty(work.name, work.id, "unknown work"))
}

func sessionIDReference(sessionID string) string {
	if id := strings.TrimSpace(sessionID); id != "" {
		return fmt.Sprintf("%q", id)
	}
	return `"unknown session"`
}

func completedFailureReason(completed interfaces.CompletedDispatch) string {
	if completed.FailureMetadata == nil {
		return ""
	}
	return strings.TrimSpace(string(completed.FailureMetadata.Type))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stringPtr(value string) *string {
	if value = strings.TrimSpace(value); value != "" {
		return &value
	}
	return nil
}
