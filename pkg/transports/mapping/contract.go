package apisurface

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	modelassets "github.com/portpowered/infinite-you/pkg/models/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/models/catalog"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// ModelAPI is the model catalog and direct-invocation seam for API handlers and bounded test doubles.
type ModelAPI interface {
	ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error)
	GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error)
	InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (ModelInvocationResult, error)
	PullModel(ctx context.Context, modelName string) (ModelPullResult, error)
}

// FactorySaveAPI is the session-scoped factory definition read and persist seam.
type FactorySaveAPI interface {
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	SaveFactoryForSession(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
	SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error)
}

// SessionAPI owns legacy unscoped factory operations, factory-session
// inventory and lifecycle, and session-scoped work operations.
type SessionAPI interface {
	factory.APIFactory
	GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error)
	WorkAPI
	ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(ctx context.Context, sessionID string, options interfaces.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error)
	OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(ctx context.Context, sessionID string) error
	PauseLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	SubscribeFactoryResponseEventsForSession(ctx context.Context, sessionID string, afterSequence int64, dispatchID string) (FactoryResponseEventSubscription, error)
}

// WorkAPI is the session-scoped work submission, operator move, and runtime observability seam.
type WorkAPI interface {
	SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error)
	MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
}

// InvocationAPI is the session-scoped factory invocation seam used by the API
// transport to submit one logical input and return the selected primary result.
type InvocationAPI interface {
	InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (FactoryInvocationResult, error)
}

// DurableSessionLifecycleAPI is the shared durable session read and lifecycle
// control seam for pause, resume, cancel, terminate, approve, and retry-dispatch.
type DurableSessionLifecycleAPI interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
	PauseDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ApproveDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	RetryDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	InterruptDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factoryapi.FactorySessionInterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

// DurableSessionExecutionAPI is the shared durable factory-session execution start
// seam for async and sync dynamic workflow sessions. Live-session open and
// invocation remain on SessionAPI and InvocationAPI.
type DurableSessionExecutionAPI interface {
	StartDurableFactorySessionAsync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
	StartDurableFactorySessionSync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionSyncExecutionResponse, error)
}

// DurableSessionListingAPI is the shared scoped session listing seam for live and
// persisted durable execution sessions.
type DurableSessionListingAPI interface {
	ListDurableFactorySessions(ctx context.Context, params factoryapi.ListFactorySessionsParams) (factoryapi.ListFactorySessionsResponse, error)
}

// DurableSessionProjectionAPI is the shared durable session result, dispatch,
// artifact, and event replay seam for dynamic workflow inspection.
type DurableSessionProjectionAPI interface {
	GetDurableFactorySessionResult(ctx context.Context, sessionID string, params factoryapi.GetFactorySessionResultsParams) (factoryapi.FactorySessionResult, error)
	ListDurableFactorySessionDispatches(ctx context.Context, sessionID string, params factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error)
	ListDurableFactorySessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error)
	ReadDurableFactorySessionEvents(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) (*interfaces.FactoryEventStream, error)
}

// APISurface is the runtime seam consumed by the Agent Factory API server.
// It resolves requests against the service-owned current runtime so activation
// can swap the active runtime without leaving API reads pinned to startup
// state.
type APISurface interface {
	factory.APIFactory
	GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error)
	ModelAPI
}

// SessionAPISurface extends APISurface with explicit per-session routing while
// preserving the legacy unscoped compatibility behavior through APISurface.
type SessionAPISurface interface {
	APISurface
	SessionAPI
	FactorySaveAPI
	WorkAPI
	InvocationAPI
	DurableSessionExecutionAPI
}

// FactoryInvocationResult carries the runtime-owned outcome of one session
// invocation request after input resolution and primary-result selection.
type FactoryInvocationResult = interfaces.FactoryInvocationResult

// InvocationResponseFromResult maps a shared invocation result onto the public
// invocation response contract used by both API and CLI JSON surfaces.
func InvocationResponseFromResult(result FactoryInvocationResult) factoryapi.InvocationResponse {
	response := factoryapi.InvocationResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Status:    factoryapi.InvocationTerminalStatus(result.Status),
	}
	if content := contentcontract.GeneratedPtrFromParts(result.PrimaryResult); content != nil {
		response.PrimaryResult = content
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		value := factoryapi.InvocationResponseErrorCode(code)
		response.ErrorCode = &value
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		response.Message = &message
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		response.SessionId = &sessionID
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		response.WorkId = &workID
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		response.WorkName = &workName
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		response.WorkState = &workState
	}
	return response
}

type stopSummaryWork struct {
	id       string
	name     string
	workType string
	state    string
	token    *factorytoken.Token
}

type stopSummaryRecovery struct {
	resultSummary string
	surface       string
	action        string
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
	if interruptedWorkSummary := buildInterruptedWorkStateSummary(sessionID, snapshot, materialized); interruptedWorkSummary != nil {
		return interruptedWorkSummary
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
	token *factorytoken.Token,
	sessionStopSummary *factoryapi.FactoryStopSummary,
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
		recovery := pausedRecoverySummary(sessionID)
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("PAUSED"), &status, target, latestRelevantDispatch(target.id, snapshot), recovery)
	}
	sessionInterruptedSummary := interruptedWorkStopSummary(target.id, sessionStopSummary)
	if summary := stopSummaryForWorkState(sessionID, target, snapshot, sessionInterruptedSummary); summary != nil {
		return summary
	}

	if matching := workByID(materialized, target.id); matching != nil && matching.state != "" {
		if summary := stopSummaryForWorkState(sessionID, *matching, snapshot, sessionInterruptedSummary); summary != nil {
			return summary
		}
	}
	if sessionInterruptedSummary != nil {
		return sessionInterruptedSummary
	}
	return nil
}

func interruptedWorkStopSummary(workID string, sessionStopSummary *factoryapi.FactoryStopSummary) *factoryapi.FactoryStopSummary {
	if sessionStopSummary == nil || sessionStopSummary.StopKind != factoryapi.FactoryStopKind("INTERRUPTED") {
		return nil
	}
	if sessionStopSummary.WorkId == nil || strings.TrimSpace(*sessionStopSummary.WorkId) != strings.TrimSpace(workID) {
		return nil
	}
	summary := *sessionStopSummary
	return &summary
}

func stopSummaryForWorkState(sessionID string, work stopSummaryWork, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], sessionInterruptedSummary *factoryapi.FactoryStopSummary) *factoryapi.FactoryStopSummary {
	switch work.state {
	case "interrupted":
		if sessionInterruptedSummary != nil {
			return sessionInterruptedSummary
		}
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("INTERRUPTED"), nil, work, interruptedDispatchFromWork(work, snapshot), interruptedWorkRecoverySummary(sessionID, work))
	case "blocked":
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("BLOCKED"), nil, work, latestRelevantDispatch(work.id, snapshot), blockedRecoverySummary(work))
	case "needs-human":
		return buildStopSummary(sessionID, factoryapi.FactoryStopKind("NEEDS_HUMAN"), nil, work, latestRelevantDispatch(work.id, snapshot), needsHumanRecoverySummary(work))
	default:
		return nil
	}
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
	recovery := pausedRecoverySummary(sessionID)
	if work == nil {
		return &factoryapi.FactoryStopSummary{
			SessionId:                sessionID,
			StopKind:                 factoryapi.FactoryStopKind("PAUSED"),
			SessionLifecycleStatus:   &status,
			SuggestedRecoverySurface: stringPtr(recovery.surface),
			SuggestedRecoveryAction:  stringPtr(recovery.action),
		}
	}
	return buildStopSummary(sessionID, factoryapi.FactoryStopKind("PAUSED"), &status, *work, latestRelevantDispatch(work.id, snapshot), recovery)
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
		break
	}
	if stoppedDispatch == nil {
		return nil
	}
	if stoppedWork == nil {
		recovery := interruptedRecoverySummary(sessionID)
		return &factoryapi.FactoryStopSummary{
			SessionId:                sessionID,
			StopKind:                 factoryapi.FactoryStopKind("INTERRUPTED"),
			LatestDispatch:           projectedStopDispatch(*stoppedDispatch),
			LatestResultSummary:      stringPtr(interruptedResultSummary(*stoppedDispatch)),
			SuggestedRecoverySurface: stringPtr(recovery.surface),
			SuggestedRecoveryAction:  stringPtr(recovery.action),
		}
	}
	return buildStopSummary(sessionID, factoryapi.FactoryStopKind("INTERRUPTED"), nil, *stoppedWork, stoppedDispatch, interruptedRecoverySummary(sessionID))
}

func buildInterruptedWorkStateSummary(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	materialized materialize.PublicWorkTokens,
) *factoryapi.FactoryStopSummary {
	work := latestWorkInState(materialized, snapshotTopology(snapshot), "interrupted")
	if work == nil {
		return nil
	}
	return buildStopSummary(
		sessionID,
		factoryapi.FactoryStopKind("INTERRUPTED"),
		nil,
		*work,
		interruptedDispatchFromWork(*work, snapshot),
		interruptedWorkRecoverySummary(sessionID, *work),
	)
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
	recovery := stopKindRecoverySummary(stopKind, *work)
	return buildStopSummary(sessionID, stopKind, nil, *work, latestRelevantDispatch(work.id, snapshot), recovery)
}

func buildStopSummary(
	sessionID string,
	stopKind factoryapi.FactoryStopKind,
	lifecycleStatus *factoryapi.FactorySessionDurableLifecycleStatus,
	work stopSummaryWork,
	dispatch *stopSummaryDispatch,
	recovery stopSummaryRecovery,
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
	if strings.TrimSpace(recovery.resultSummary) == "" {
		recovery.resultSummary = defaultRecoveryResultSummary(stopKind, work, dispatch)
	}
	if strings.TrimSpace(recovery.resultSummary) != "" {
		summary.LatestResultSummary = stringPtr(strings.TrimSpace(recovery.resultSummary))
	}
	if strings.TrimSpace(recovery.surface) != "" {
		summary.SuggestedRecoverySurface = stringPtr(recovery.surface)
	}
	if strings.TrimSpace(recovery.action) != "" {
		summary.SuggestedRecoveryAction = stringPtr(recovery.action)
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

func interruptedDispatchFromWork(
	work stopSummaryWork,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *stopSummaryDispatch {
	dispatch := latestRelevantDispatch(work.id, snapshot)
	if dispatch == nil {
		return nil
	}
	interrupted := *dispatch
	interrupted.status = factoryapi.FactoryDispatchStatusINTERRUPTED
	interrupted.failureMessage = firstNonEmpty(
		failureMessageFromWork(work),
		interrupted.failureMessage,
		interrupted.failureReason,
	)
	interrupted.failureReason = firstNonEmpty(
		interrupted.failureReason,
		"work_interrupted",
	)
	return &interrupted
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
	case workerexecution.OutcomeFailed, workerexecution.OutcomeRejected:
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
	if dispatch.failureReason != "" && dispatch.failureMessage != "" {
		projected.FailureDetail = &factoryapi.FailureDetail{
			Reason:  publicFailureReason(dispatch.failureReason),
			Message: dispatch.failureMessage,
		}
	}
	return projected
}

func publicFailureReason(reason string) factoryapi.WorkFailureType {
	candidate := factoryapi.WorkFailureType(strings.TrimSpace(reason))
	switch candidate {
	case factoryapi.WorkFailureTypeAuthFailure,
		factoryapi.WorkFailureTypePermanentBadRequest,
		factoryapi.WorkFailureTypeThrottled,
		factoryapi.WorkFailureTypeInternalServerError,
		factoryapi.WorkFailureTypeTimeout,
		factoryapi.WorkFailureTypeMisconfigured,
		factoryapi.WorkFailureTypeMissingExecutable,
		factoryapi.WorkFailureTypeCommandLineTooLong:
		return candidate
	default:
		return factoryapi.WorkFailureTypeUnknown
	}
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

func workFromToken(token *factorytoken.Token, topology *state.Net) stopSummaryWork {
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

func dispatchTouchesWork(tokens []factorytoken.Token, workID string) bool {
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
	return strings.TrimSpace(dispatch.FailureDetail.Message)
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

func stopKindRecoverySummary(stopKind factoryapi.FactoryStopKind, work stopSummaryWork) stopSummaryRecovery {
	switch stopKind {
	case factoryapi.FactoryStopKind("BLOCKED"):
		return blockedRecoverySummary(work)
	case factoryapi.FactoryStopKind("NEEDS_HUMAN"):
		return needsHumanRecoverySummary(work)
	case factoryapi.FactoryStopKind("INTERRUPTED"):
		return interruptedRecoverySummary("")
	case factoryapi.FactoryStopKind("PAUSED"):
		return pausedRecoverySummary("")
	default:
		return stopSummaryRecovery{}
	}
}

func blockedRecoverySummary(work stopSummaryWork) stopSummaryRecovery {
	workLabel := workReferenceLabel(work)
	return stopSummaryRecovery{
		resultSummary: blockedReasonSummary(work),
		surface:       "existing work repair, work move, or follow-up submission controls",
		action:        "Inspect the blocked work " + workLabel + ", then use the existing work repair, work move, or follow-up submission controls to unblock it.",
	}
}

func needsHumanRecoverySummary(work stopSummaryWork) stopSummaryRecovery {
	workLabel := workReferenceLabel(work)
	return stopSummaryRecovery{
		resultSummary: needsHumanReasonSummary(work),
		surface:       "existing work follow-up submission, work move, or session workflow controls",
		action:        "Provide the requested human input, approval, or artifact review for work " + workLabel + " through the existing workflow controls, then continue the normal session flow.",
	}
}

func pausedRecoverySummary(sessionID string) stopSummaryRecovery {
	sessionLabel := sessionIDReference(sessionID)
	return stopSummaryRecovery{
		surface: "existing Factory Session resume control",
		action:  "Resume the paused Factory Session " + sessionLabel + " with the existing session resume control, then re-check queued or buffered work.",
	}
}

func interruptedRecoverySummary(sessionID string) stopSummaryRecovery {
	sessionLabel := sessionIDReference(sessionID)
	return stopSummaryRecovery{
		surface: "existing dispatch retry, work repair, or session workflow controls",
		action:  "Inspect the interrupted dispatch in Factory Session " + sessionLabel + ", then use the existing retry, repair, or session workflow controls to continue recovery.",
	}
}

func interruptedWorkRecoverySummary(sessionID string, work stopSummaryWork) stopSummaryRecovery {
	recovery := interruptedRecoverySummary(sessionID)
	recovery.resultSummary = firstNonEmpty(
		failureMessageFromWork(work),
		rejectionFeedbackFromWork(work),
	)
	return recovery
}

func blockedReasonSummary(work stopSummaryWork) string {
	return firstNonEmpty(
		failureMessageFromWork(work),
		rejectionFeedbackFromWork(work),
	)
}

func needsHumanReasonSummary(work stopSummaryWork) string {
	return firstNonEmpty(
		rejectionFeedbackFromWork(work),
		failureMessageFromWork(work),
	)
}

func defaultRecoveryResultSummary(
	stopKind factoryapi.FactoryStopKind,
	work stopSummaryWork,
	dispatch *stopSummaryDispatch,
) string {
	if dispatch == nil {
		return ""
	}
	if stopKind == factoryapi.FactoryStopKind("INTERRUPTED") {
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
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		return fmt.Sprintf("%q", trimmed)
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

// RequestValidationError reports a stable client-side validation failure that
// should map to HTTP 400 at the transport boundary.
type RequestValidationError = interfaces.RequestValidationError

// ErrFactoryActivationRequiresIdle reports that runtime replacement was
// attempted while the current runtime still had active work.
var ErrFactoryActivationRequiresIdle = errors.New("factory activation requires idle runtime")

// ErrInvalidNamedFactory retains the public compatibility identity while
// invalid persisted Factory definitions are classified by the config owner.
var ErrInvalidNamedFactory = factoryconfig.ErrInvalidNamedFactory

// ErrCurrentFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentFactoryNotFound = interfaces.ErrCurrentFactoryNotFound

// ErrFactorySessionNotFound reports that no live session matched the requested
// public session identifier.
var ErrFactorySessionNotFound = errors.New("factory session not found")

// ErrInvalidEventReconnectCursor reports that the reconnect cursor did not
// match any recorded event in the targeted stream.
var ErrInvalidEventReconnectCursor = errors.New("invalid event reconnect cursor")

// ErrFactorySessionResultUnavailable reports that the requested session does not
// expose JavaScript result or partial-result reads.
var ErrFactorySessionResultUnavailable = errors.New("factory session result unavailable")

// ErrFactoryVersionStale retains the public compatibility error identity while
// Factory definition version policy is owned by the Factory domain.
var ErrFactoryVersionStale = interfaces.ErrFactoryVersionStale

// ErrModelNotFound reports that the requested discovered model identifier is
// not present in the current runtime configuration.
var ErrModelNotFound = managedruntime.ErrNotFound

// ErrModelNotAvailable reports that a discovered local model exists but its
// required local assets are not present in the managed cache.
var ErrModelNotAvailable = modelassets.ErrNotAvailable

// ErrModelPullUnsupported reports that the requested model does not support
// managed local asset pulls in the current runtime or platform.
var ErrModelPullUnsupported = modelassets.ErrPullUnsupported

// ErrManagedRuntimeSourceFetchFailed reports that required managed runtime
// assets could not be fetched from the configured backend source.
var ErrManagedRuntimeSourceFetchFailed = modelassets.ErrSourceFetchFailed

// ErrModelInvocationUnsupportedMode reports that the requested direct
// invocation response mode is not valid for the selected operation output.
var ErrModelInvocationUnsupportedMode = errors.New("model invocation response mode is not supported")

// ErrModelInvocationUnsupportedOperation reports that the targeted model does
// not expose the requested provider-agnostic operation.
var ErrModelInvocationUnsupportedOperation = modelcatalog.ErrUnsupportedOperation

// ModelInvocationResult carries the backend-owned direct invocation result used
// by the API transport for either JSON metadata or streamed audio responses.
type ModelInvocationResult struct {
	ModelName         string
	Worker            string
	Operation         string
	ProviderLocality  string
	Content           []work.WorkContentPart
	Bindings          []workerexecution.ResolvedModelOperationBinding
	StreamFile        string
	StreamContentType string
}

// ModelPullDownloadedFile describes one cached artifact materialized by a
// managed local-model asset pull.
type ModelPullDownloadedFile = modelassets.DownloadedFile

// ModelPullResult carries the service-owned result of pulling one model into
// the managed local cache.
type ModelPullResult = modelassets.PullResult

// TopologyValidationError carries validation targets that the graph editor can
// map back to form fields, nodes, edges, or save-level messages.
type TopologyValidationError struct {
	Message string
	Targets []factoryapi.FactoryValidationTarget
}

func (e *TopologyValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "factory topology validation failed"
}

func (e *TopologyValidationError) Is(target error) bool {
	return target == ErrInvalidNamedFactory
}

func NewTopologyValidationError(message string, targets []factoryapi.FactoryValidationTarget) *TopologyValidationError {
	if message == "" {
		message = "factory topology validation failed"
	}
	return &TopologyValidationError{
		Message: message,
		Targets: append([]factoryapi.FactoryValidationTarget(nil), targets...),
	}
}

// DefaultCurrentFactoryName is the reserved current-factory identifier used
// when the active runtime is the root factory and no named-factory pointer
// exists.
const DefaultCurrentFactoryName factoryapi.FactoryName = "UNDEFINED"
