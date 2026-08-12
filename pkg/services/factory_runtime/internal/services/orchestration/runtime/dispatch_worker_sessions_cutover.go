package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// startThroughWorkerSessions is the W4 Runtime dispatch cutover seam. For
// every resolved dispatch it: boots the underlying Workers pool if needed,
// reserves one stable, control-addressable Worker Session identity, commits
// that association to canonical Factory Events before Start can publish Worker
// Session lifecycle records, then hands the resolved request to
// worker_sessions.Service.Start (which drives the existing Workers
// workstation-pool boundary underneath).
// The Worker Sessions terminal outcome is translated back into the exact
// workers.WorkstationDispatchResult shape the pre-cutover accept callback
// expects, so existing Work materialization and Factory result behavior is
// unchanged.
func startThroughWorkerSessions(
	ctx context.Context,
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	workersBoundary workers.WorkstationPoolBoundary,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if strings.TrimSpace(request.Execution.RecordingID) == "" && cfg != nil {
		request.Execution.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if err := workersBoundary.Start(ctx); err != nil {
		return err
	}
	dispatchID := request.Execution.Dispatch.DispatchID
	sessionID := dispatchID
	if _, err := cfg.workerSessions.Reserve(
		context.WithoutCancel(ctx),
		workersessions.ReserveRequest{ID: sessionID},
	); err != nil {
		return err
	}
	eventHistory.RecordDispatchWorkerSessionAssociation(
		request.Execution.Dispatch.Execution.DispatchCreatedTick,
		dispatchID,
		sessionID,
		request.Execution.Dispatch.Execution.RequestID,
		cfg.clock.Now(),
	)
	execute := func() {
		// Retry is left at its zero value on purpose: Petri dispatch has always
		// been one attempt, with retryability classified and handed outward for
		// the graph to act on. Converging JavaScript children onto this call
		// must not quietly give every Petri Worker attempt-level retry it never
		// had.
		startResult, startErr := cfg.workerSessions.InvokeSession(
			context.WithoutCancel(ctx),
			workersessions.InvokeSessionRequest{ID: sessionID, Execution: request},
		)
		result, dispatchErr := workerSessionDispatchOutcome(request, startResult, startErr)
		accept(context.Background(), request, result, dispatchErr)
	}
	async := !cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil
	if async {
		go execute()
		return nil
	}
	execute()
	return nil
}

// workerSessionDispatchOutcome translates one Worker Sessions Start result
// into the exact workers.WorkstationDispatchResult/error shape the Runtime
// accept callback expects. When Start handed the attempt off to Workers, the
// raw StartResult.Dispatch/DispatchErr already carry that exact shape and are
// returned unchanged. This includes a canceled terminal result when a control
// won after identity reservation but before Workers admission. When Start
// rejected the request before any Workers call (invalid request, conflicting
// start, or a before-handoff Events publication failure), a synthesized FAILED
// result is returned instead of fabricating a Workers payload that never
// existed.
func workerSessionDispatchOutcome(
	request workers.WorkstationDispatchRequest,
	startResult workersessions.InvokeSessionResult,
	startErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	transitionID := request.Execution.Dispatch.TransitionID
	if startErr != nil {
		return workers.WorkstationDispatchResult{
			DispatchID:      dispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result: workerexecution.WorkResult{
				DispatchID:   dispatchID,
				TransitionID: transitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        startErr.Error(),
			},
		}, startErr
	}
	if handedOffToWorkers(startResult) {
		return startResult.Dispatch, startResult.DispatchErr
	}
	errText := "worker session start failed before Workers handoff"
	if startResult.Session.Result != nil && startResult.Session.Result.Cause != nil {
		errText = string(startResult.Session.Result.Cause.Kind)
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workerexecution.WorkResult{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        errText,
		},
	}, nil
}

// handedOffToWorkers reports whether Start actually reached the Workers
// DispatchWorkstation call. The only FAILED terminal Start commits without a
// Workers handoff is FailureCauseEventPublicationFailure. A canceled terminal
// Session with no cause also carries a synthesized canceled dispatch when
// control won before admission; all other terminal causes (start failure,
// adapter failure, executor panic, or Workers execution failure) are produced
// from within the handoff itself.
func handedOffToWorkers(startResult workersessions.InvokeSessionResult) bool {
	result := startResult.Session.Result
	if result == nil || result.Cause == nil {
		return true
	}
	return result.Cause.Kind != workersessions.FailureCauseEventPublicationFailure
}

// WorkerSessionsObservation returns the runtime-bound detached Worker Session
// observation projection for the current Factory Session.
func (f *factoryImpl) WorkerSessionsObservation() workersessions.ObservationService {
	if f == nil || f.cfg == nil {
		return nil
	}
	var workerRecordingReader recordings.WorkerRecordingReader
	if reader, ok := f.cfg.workerSessions.(recordings.WorkerRecordingReader); ok {
		workerRecordingReader = reader
	}
	return newRecordedWorkerSessionObservationWithRecording(
		f.cfg.workerSessions,
		f.eventHistory,
		f.cfg.worldStateProjector,
		f.clock,
		f.cfg.providerSessions,
		f.cfg.recordingID,
		workerRecordingReader,
	)
}

// recordedWorkerSessionObservation is the runtime-facing observation adapter.
// Factory Runtime owns the per-session canonical ledger and Recordings owns
// its world-state projector; Worker Sessions remains the owner of the public
// detached observation vocabulary and Provider Sessions enrichment.
type recordedWorkerSessionObservation struct {
	workersessions.Service
	ledger           recordings.RuntimeLedger
	projector        factory.WorldStateProjector
	clock            factory.Clock
	providerSessions providersessions.Service
	recordingID      string
	recordingReader  recordings.WorkerRecordingReader
}

var _ workersessions.Service = (*recordedWorkerSessionObservation)(nil)

func (s *recordedWorkerSessionObservation) projectRecorded(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	workID string,
) ([]workersessions.Observation, bool, error) {
	if err := observationContextError(ctx); err != nil {
		return nil, false, err
	}
	ordered := cloneAndSortFactoryEvents(events)
	selectedTick := latestFactoryEventTick(ordered)
	world, err := s.projector(ordered, selectedTick)
	if err != nil {
		return nil, false, workersessions.ErrObservationProjectionUnavailable
	}

	knownWork := recordedWorkExists(world, ordered, workID)
	associations, requests := recordedDispatchFacts(ordered)
	if len(associations) == 0 {
		return make([]workersessions.Observation, 0), knownWork, nil
	}

	completed := recordedDispatchStateMaps(world)
	result := make([]workersessions.Observation, 0, len(associations))
	for dispatchID, association := range associations {
		fact := recordedDispatchFact(dispatchID, association, requests, completed, world.ProviderSessions, world.ActiveDispatches, ordered)
		if !containsRecordedWorkID(fact.workIDs, workID) {
			continue
		}
		observation := recordedObservationFromFact(fact, s.clock)
		result = append(result, observation)
	}
	return result, knownWork, nil
}

func latestFactoryEventTick(events []interfaces.FactoryEvent) int {
	selectedTick := 0
	for _, event := range events {
		if event.Context.Tick > selectedTick {
			selectedTick = event.Context.Tick
		}
	}
	return selectedTick
}

func recordedDispatchStateMaps(
	world interfaces.FactoryWorldState,
) map[string]interfaces.FactoryWorldDispatchCompletion {
	completed := make(map[string]interfaces.FactoryWorldDispatchCompletion, len(world.CompletedDispatches))
	for _, dispatch := range world.CompletedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	for _, dispatch := range world.FailedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	return completed
}

func recordedDispatchFact(
	dispatchID string,
	association recordedDispatchAssociation,
	requests map[string]recordedDispatchRequest,
	completed map[string]interfaces.FactoryWorldDispatchCompletion,
	providerSessions []interfaces.FactoryWorldProviderSessionRecord,
	active map[string]interfaces.FactoryWorldDispatch,
	events []interfaces.FactoryEvent,
) recordedDispatchObservation {
	fact := recordedDispatchObservation{
		workerSessionID: association.workerSessionID,
		dispatchID:      dispatchID,
		turnID:          association.turnID,
		startedAt:       association.eventTime,
		state:           workersessions.StateStarting,
	}
	if request, ok := requests[dispatchID]; ok {
		fact.workIDs = append([]string(nil), request.workIDs...)
		fact.startedAt = request.startedAt
	}
	if dispatch, ok := active[dispatchID]; ok {
		fact.state = workersessions.StateRunning
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
	}
	if dispatch, ok := completed[dispatchID]; ok {
		fact.state = recordedObservationState(dispatch.Result.Outcome)
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.endedAt = recordedDispatchEnd(dispatch, events, dispatchID)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
		fact.failure = recordedFailureWithDiagnostics(
			workers.WorkOutcome(dispatch.Result.Outcome),
			dispatch.Result.FailureDetail,
			dispatch.Result.FailureMetadata,
			fact.state,
			dispatch.Diagnostics,
		)
		fact.provider = cloneProviderMetadata(dispatch.ProviderSession)
	}
	for _, provider := range providerSessions {
		if provider.DispatchID != dispatchID {
			continue
		}
		fact.provider = cloneProviderMetadata(&provider.ProviderSession)
		fact.workIDs = firstRecordedWorkIDs(provider.WorkItemIDs, fact.workIDs)
		fact.failure = firstRecordedFailure(fact.failure, recordedFailureWithDiagnostics(
			workers.OutcomeFailed,
			provider.FailureDetail,
			nil,
			fact.state,
			provider.Diagnostics,
		))
		break
	}
	return fact
}

func recordedDispatchEnd(
	dispatch interfaces.FactoryWorldDispatchCompletion,
	events []interfaces.FactoryEvent,
	dispatchID string,
) *time.Time {
	ended := dispatch.CompletedAt
	if ended.IsZero() {
		ended = eventTimeForDispatch(events, dispatchID)
	}
	if ended.IsZero() {
		return nil
	}
	ended = ended.UTC()
	return &ended
}

func recordedDispatchFacts(events []interfaces.FactoryEvent) (map[string]recordedDispatchAssociation, map[string]recordedDispatchRequest) {
	associations := make(map[string]recordedDispatchAssociation)
	requests := make(map[string]recordedDispatchRequest)
	for _, event := range events {
		dispatchID := stringPointerValue(event.Context.DispatchID)
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			if dispatchID == "" {
				continue
			}
			var payload interfaces.DispatchWorkerSessionAssociationEventPayload
			if json.Unmarshal(event.Payload, &payload) != nil || payload.WorkerSessionID == "" {
				continue
			}
			associations[dispatchID] = recordedDispatchAssociation{
				workerSessionID: payload.WorkerSessionID,
				turnID:          stringPointerValue(event.Context.RequestID),
				eventTime:       event.Context.EventTime.UTC(),
			}
		case interfaces.FactoryEventTypeDispatchRequest:
			if dispatchID == "" {
				continue
			}
			var payload interfaces.DispatchRequestEventPayload
			if json.Unmarshal(event.Payload, &payload) != nil {
				continue
			}
			workIDs := append([]string(nil), pointerStringSlice(event.Context.WorkIDs)...)
			if len(workIDs) == 0 {
				for _, input := range payload.Inputs {
					if input.WorkID != "" {
						workIDs = appendUniqueRecordedString(workIDs, input.WorkID)
					}
				}
			}
			requests[dispatchID] = recordedDispatchRequest{
				workIDs:   workIDs,
				startedAt: event.Context.EventTime.UTC(),
			}
		}
	}
	return associations, requests
}
