package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// recordedWorkerSessionObservation is the runtime-facing observation adapter.
// Factory Runtime owns the per-session canonical ledger and Recordings owns
// its world-state projector; Worker Sessions remains the owner of the public
// detached observation vocabulary and Provider Sessions enrichment.
type recordedWorkerSessionObservation struct {
	live      workersessions.ObservationService
	ledger    recordings.RuntimeLedger
	projector factory.WorldStateProjector
	clock     factory.Clock
}

var _ workersessions.ObservationService = (*recordedWorkerSessionObservation)(nil)

func newRecordedWorkerSessionObservation(
	live workersessions.ObservationService,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
) workersessions.ObservationService {
	return &recordedWorkerSessionObservation{
		live: live, ledger: ledger, projector: projector, clock: clock,
	}
}

func (s *recordedWorkerSessionObservation) ListObservations(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if s == nil || s.ledger == nil || s.projector == nil {
		return s.listLive(ctx, req)
	}

	events := s.ledger.CanonicalEvents()
	recorded, knownWork, err := s.projectRecorded(ctx, events, req.WorkID)
	if err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	live, liveErr := s.listLive(ctx, req)
	if liveErr == nil {
		recorded = mergeRecordedObservations(recorded, live.Observations)
	}
	if !acceptableLiveObservationError(liveErr) {
		return workersessions.ListObservationsResult{}, liveErr
	}
	return recordedObservationListResult(recorded, knownWork, live, liveErr)
}

func acceptableLiveObservationError(err error) bool {
	return err == nil || isObservationNotFound(err) || isObservationProjectionUnavailable(err)
}

func recordedObservationListResult(
	recorded []workersessions.Observation,
	knownWork bool,
	live workersessions.ListObservationsResult,
	liveErr error,
) (workersessions.ListObservationsResult, error) {
	if !knownWork && len(recorded) == 0 {
		if liveErr == nil && len(live.Observations) > 0 {
			return live, nil
		}
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	if len(recorded) == 0 && liveErr == nil && len(live.Observations) > 0 {
		return live, nil
	}
	sortObservationAttempts(recorded)
	return workersessions.ListObservationsResult{Observations: recorded}, nil
}

func (s *recordedWorkerSessionObservation) listLive(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if s == nil || s.live == nil {
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.live.ListObservations(ctx, req)
}

func (s *recordedWorkerSessionObservation) GetObservation(
	ctx context.Context,
	req workersessions.GetObservationRequest,
) (workersessions.Observation, error) {
	if s == nil || s.live == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.live.GetObservation(ctx, req)
}

func (s *recordedWorkerSessionObservation) ReadTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	if s == nil || s.live == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.live.ReadTranscript(ctx, req)
}

func (s *recordedWorkerSessionObservation) StreamObservations(
	ctx context.Context,
	req workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	if s == nil || s.live == nil {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	return s.live.StreamObservations(ctx, req)
}

type recordedDispatchObservation struct {
	workerSessionID string
	dispatchID      string
	turnID          string
	workIDs         []string
	startedAt       time.Time
	endedAt         *time.Time
	state           workersessions.State
	failure         *workersessions.FailureCause
	provider        *workers.ProviderSessionMetadata
}

type recordedDispatchAssociation struct {
	workerSessionID string
	turnID          string
	eventTime       time.Time
}

type recordedDispatchRequest struct {
	workIDs   []string
	startedAt time.Time
}

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
		fact.failure = recordedFailure(dispatch.Result.FailureDetail, dispatch.Result.FailureMetadata, fact.state)
		fact.provider = cloneProviderMetadata(dispatch.ProviderSession)
	}
	for _, provider := range providerSessions {
		if provider.DispatchID != dispatchID {
			continue
		}
		fact.provider = cloneProviderMetadata(&provider.ProviderSession)
		fact.workIDs = firstRecordedWorkIDs(provider.WorkItemIDs, fact.workIDs)
		fact.failure = firstRecordedFailure(fact.failure, recordedFailure(provider.FailureDetail, nil, fact.state))
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

func recordedWorkExists(world interfaces.FactoryWorldState, events []interfaces.FactoryEvent, workID string) bool {
	if _, ok := world.WorkItemsByID[workID]; ok {
		return true
	}
	if _, ok := world.ActiveWorkItemsByID[workID]; ok {
		return true
	}
	if _, ok := world.TerminalWorkByID[workID]; ok {
		return true
	}
	if _, ok := world.FailedWorkItemsByID[workID]; ok {
		return true
	}
	for _, event := range events {
		if containsRecordedWorkID(pointerStringSlice(event.Context.WorkIDs), workID) {
			return true
		}
		if event.Type != interfaces.FactoryEventTypeWorkRequest {
			continue
		}
	}
	return false
}

func recordedObservationFromFact(fact recordedDispatchObservation, clock factory.Clock) workersessions.Observation {
	state := fact.state
	if state == "" {
		state = workersessions.StateStarting
	}
	observation := workersessions.Observation{
		WorkerSessionID:          fact.workerSessionID,
		ProviderSessionAvailable: fact.provider != nil && fact.provider.ID != "",
		WorkIDs:                  append([]string(nil), fact.workIDs...),
		TurnID:                   fact.turnID,
		AttemptID:                fact.dispatchID,
		State:                    state,
		DurationBasis:            workersessions.DurationBasisUnavailable,
		Transcript:               workersessions.TranscriptAvailabilityUnavailable,
	}
	if fact.provider != nil {
		observation.ProviderSession = providerSessionRef(*fact.provider)
	}
	if !fact.startedAt.IsZero() {
		started := fact.startedAt.UTC()
		observation.StartedAt = &started
		if fact.endedAt != nil {
			ended := fact.endedAt.UTC()
			observation.EndedAt = &ended
			duration := ended.Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
		} else if !state.Terminal() && clock != nil {
			duration := clock.Now().Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisActiveClock
		}
	}
	if fact.failure != nil {
		failure := *fact.failure
		observation.Failure = &failure
	}
	return observation
}

func recordedObservationState(outcome string) workersessions.State {
	switch workers.WorkOutcome(outcome) {
	case workers.OutcomeAccepted, workers.OutcomeContinue:
		return workersessions.StateCompleted
	case workers.OutcomeFailed, workers.OutcomeRejected:
		return workersessions.StateFailed
	default:
		return workersessions.StateFailed
	}
}

func recordedFailure(
	detail *workers.FailureDetail,
	metadata *workers.WorkFailureMetadata,
	state workersessions.State,
) *workersessions.FailureCause {
	if !state.Terminal() || state == workersessions.StateCompleted {
		return nil
	}
	return &workersessions.FailureCause{
		Kind:                workersessions.FailureCauseWorkersExecutionFailure,
		Detail:              recordedFailureDetail(detail, metadata),
		ProviderFailureKind: recordedProviderFailureKind(detail),
	}
}

func recordedFailureDetail(detail *workers.FailureDetail, metadata *workers.WorkFailureMetadata) string {
	if metadata != nil {
		family, familyKnown := recordedFailureFamily(metadata.Family)
		typ, typeKnown := recordedFailureType(metadata.Type)
		if familyKnown && typeKnown && (family != "" || typ != "") {
			if family == "" {
				family = "unknown"
			}
			if typ == "" {
				typ = "unknown"
			}
			return "family=" + family + " type=" + typ
		}
	}
	if typ, ok := recordedFailureType(detailType(detail)); ok && typ != "" {
		return "type=" + typ
	}
	return "the Workers execution result was not successful"
}

func detailType(detail *workers.FailureDetail) workers.WorkFailureType {
	if detail == nil {
		return ""
	}
	return detail.Reason
}

func recordedFailureFamily(family workers.WorkFailureFamily) (string, bool) {
	switch family {
	case "":
		return "", true
	case workers.WorkFailureFamilyTerminal, workers.WorkFailureFamilyRetryable, workers.WorkFailureFamilyThrottle:
		return string(family), true
	default:
		return "", false
	}
}

func recordedFailureType(typ workers.WorkFailureType) (string, bool) {
	switch typ {
	case "":
		return "", true
	case workers.WorkFailureTypeAuthFailure,
		workers.WorkFailureTypePermanentBadRequest,
		workers.WorkFailureTypeThrottled,
		workers.WorkFailureTypeInternalServerError,
		workers.WorkFailureTypeTimeout,
		workers.WorkFailureTypeUnknown,
		workers.WorkFailureTypeMisconfigured,
		workers.WorkFailureTypeCommandLineTooLong,
		workers.WorkFailureTypeMissingExecutable:
		return string(typ), true
	default:
		return "", false
	}
}

func recordedProviderFailureKind(detail *workers.FailureDetail) providers.ExecuteFailureKind {
	if detail == nil {
		return ""
	}
	switch detail.Reason {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailureKindAuthentication
	case workers.WorkFailureTypeTimeout:
		return providers.ExecuteFailureKindTimeout
	case workers.WorkFailureTypeThrottled:
		return providers.ExecuteFailureKindThrottled
	case workers.WorkFailureTypeMisconfigured:
		return providers.ExecuteFailureKindMisconfigured
	default:
		return ""
	}
}

func mergeRecordedObservations(recorded, live []workersessions.Observation) []workersessions.Observation {
	if len(recorded) == 0 {
		return nil
	}
	bySession := make(map[string]workersessions.Observation, len(live))
	for _, observation := range live {
		bySession[observation.WorkerSessionID] = observation
	}
	for index := range recorded {
		liveObservation, ok := bySession[recorded[index].WorkerSessionID]
		if !ok {
			continue
		}
		if liveObservation.ProviderSessionAvailable {
			recorded[index].ProviderSession = liveObservation.ProviderSession.Clone()
			recorded[index].ProviderSessionAvailable = true
		}
		if liveObservation.TokenUsage != nil {
			clone := liveObservation.TokenUsage.Clone()
			recorded[index].TokenUsage = &clone
		}
		if liveObservation.Transcript != workersessions.TranscriptAvailabilityUnavailable {
			recorded[index].Transcript = liveObservation.Transcript
			recorded[index].Parse = liveObservation.Parse
		}
		if recorded[index].Failure == nil && liveObservation.Failure != nil {
			failure := *liveObservation.Failure
			recorded[index].Failure = &failure
		}
	}
	return recorded
}

func sortObservationAttempts(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}

func cloneAndSortFactoryEvents(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	ordered := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		ordered[index] = event.Clone()
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.Context.Tick != right.Context.Tick {
			return left.Context.Tick < right.Context.Tick
		}
		if left.Context.Sequence != right.Context.Sequence {
			return left.Context.Sequence < right.Context.Sequence
		}
		if !left.Context.EventTime.Equal(right.Context.EventTime) {
			return left.Context.EventTime.Before(right.Context.EventTime)
		}
		return left.Id < right.Id
	})
	return ordered
}

func eventTimeForDispatch(events []interfaces.FactoryEvent, dispatchID string) time.Time {
	for index := len(events) - 1; index >= 0; index-- {
		if stringPointerValue(events[index].Context.DispatchID) == dispatchID && events[index].Type == interfaces.FactoryEventTypeDispatchResponse {
			return events[index].Context.EventTime.UTC()
		}
	}
	return time.Time{}
}

func firstRecordedTime(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary.UTC()
	}
	return fallback.UTC()
}

func firstRecordedWorkIDs(primary, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	return append([]string(nil), fallback...)
}

func firstRecordedFailure(primary, fallback *workersessions.FailureCause) *workersessions.FailureCause {
	if primary != nil {
		return primary
	}
	return fallback
}

func containsRecordedWorkID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUniqueRecordedString(values []string, value string) []string {
	if value == "" || containsRecordedWorkID(values, value) {
		return values
	}
	return append(values, value)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pointerStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

func providerSessionRef(metadata workers.ProviderSessionMetadata) providers.SessionRef {
	return providers.SessionRef{
		Provider: providers.ID(metadata.Provider),
		Kind:     metadata.Kind,
		ID:       metadata.ID,
	}
}

func cloneProviderMetadata(metadata *workers.ProviderSessionMetadata) *workers.ProviderSessionMetadata {
	if metadata == nil {
		return nil
	}
	clone := *metadata
	return &clone
}

func isObservationNotFound(err error) bool {
	return err == workersessions.ErrObservationWorkNotFound
}

func isObservationProjectionUnavailable(err error) bool {
	return err == workersessions.ErrObservationProjectionUnavailable
}

func observationContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ErrObservationCanceled
	}
	return nil
}
