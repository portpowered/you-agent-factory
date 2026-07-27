package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
)

func NewProjectionService() recordings.ProjectionService {
	return projectionquerywire.NewService()
}

type neutralReplayPlan struct {
	facts           recordings.ReplayPlanFacts
	events          []recordings.CanonicalEvent
	expectedThrough *recordings.CanonicalEventCursor
	selectedTick    int
	processed       int
}

type combinedService struct {
	recordings.Ledger
	recordings.ProjectionService
	recordinglifecycle.Service
	artifactsExport artifactsexport.Service

	appendMu       sync.Mutex
	lifecycleMu    sync.Mutex
	replayByKey    map[string]*factorydefinitions.ReplayArtifact
	replayPlans    map[recordings.ReplayPlanHandle]*neutralReplayPlan
	nextReplayPlan int
}

var _ recordings.Service = (*combinedService)(nil)

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	if !validAppendEvent(request.Event) {
		return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
	}
	service.appendMu.Lock()
	defer service.appendMu.Unlock()
	legacy := factoryEventFromCanonical(request.Event)
	if request.Event.Scope.FactorySessionID != "" {
		sequence := int(nextScopedSequence(service.CanonicalEvents(), request.Event.Scope))
		legacy.Context.SessionSequence = &sequence
	}
	service.AppendRecordedEvent(legacy)
	recorded := service.CanonicalEvents()
	for index := len(recorded) - 1; index >= 0; index-- {
		if recorded[index].Id == string(request.Event.ID) {
			return recordings.AppendRecordedEventResult{
				Event: canonicalEventFromFactory(recorded[index], service.StreamGenerationID()),
			}, nil
		}
	}
	return recordings.AppendRecordedEventResult{}, nil
}

func validAppendEvent(event recordings.CanonicalEvent) bool {
	return strings.TrimSpace(string(event.ID)) != "" &&
		strings.TrimSpace(string(event.Kind)) != "" &&
		!event.RecordedAt.IsZero() &&
		(event.Scope.FactorySessionID == "" ||
			strings.TrimSpace(event.Scope.FactorySessionID) != "") &&
		json.Valid([]byte(event.Payload)) &&
		(event.SourceContext == "" || json.Valid([]byte(event.SourceContext)))
}

func factoryEventFromCanonical(event recordings.CanonicalEvent) factorydefinitions.FactoryEvent {
	context := factorydefinitions.FactoryEventContext{
		EventTime: event.RecordedAt,
		Sequence:  int(event.Sequence),
		Tick:      event.FactoryTick,
	}
	hasSourceContext := json.Valid([]byte(event.SourceContext))
	if hasSourceContext {
		_ = json.Unmarshal([]byte(event.SourceContext), &context)
	}
	var sessionID *string
	if event.Scope.FactorySessionID != "" {
		value := event.Scope.FactorySessionID
		sessionID = &value
	}
	legacy := factorydefinitions.FactoryEvent{
		Context: context,
		Id:      string(event.ID),
		Payload: json.RawMessage(event.Payload),
		Type:    factorydefinitions.FactoryEventType(event.Kind),
	}
	if !hasSourceContext && legacy.Context.SessionID == nil {
		legacy.Context.SessionID = sessionID
	}
	legacy.SchemaVersion = factorydefinitions.FactoryEventSchemaVersionV1
	return legacy
}

func (service *combinedService) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
	}
	if request.Cursor != nil {
		if request.Cursor.StreamGenerationID == "" || request.Cursor.Sequence < 0 {
			return recordings.SubscribeResult{}, recordings.ErrInvalidReconnectCursor
		}
		if request.Cursor.StreamGenerationID != service.StreamGenerationID() {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorUnavailable
		}
	}
	streamContext, cancel := context.WithCancel(ctx)
	stream, err := service.Subscribe(streamContext, nil, factorydefinitions.FactoryEventReconnectScope{
		SessionID: request.Scope.FactorySessionID,
	})
	if err != nil {
		cancel()
		if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorExpired
		}
		return recordings.SubscribeResult{}, err
	}
	subscription, err := newEventSubscription(
		stream,
		request.Scope,
		request.Cursor,
		streamContext.Done(),
		cancel,
	)
	if err != nil {
		cancel()
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{
		Subscription: subscription,
	}, nil
}

func canonicalEventFromFactory(
	event factorydefinitions.FactoryEvent,
	generationID string,
) recordings.CanonicalEvent {
	sourceContext, _ := json.Marshal(event.Context)
	scope := recordings.CanonicalEventScope{}
	if event.Context.SessionID != nil {
		scope.FactorySessionID = *event.Context.SessionID
	}
	sequence := recordings.CanonicalEventSequence(event.Context.Sequence)
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(event.Id),
		Sequence:    sequence,
		FactoryTick: event.Context.Tick,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt:    event.Context.EventTime,
		Kind:          recordings.CanonicalEventKind(event.Type),
		Payload:       string(event.Payload),
		SourceContext: string(sourceContext),
	}
}

func (service *combinedService) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	if err := validateProjectionEvents(request.Scope, request.After, request.Events); err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	events := make([]factorydefinitions.FactoryEvent, len(request.Events))
	for index, event := range request.Events {
		events[index] = factoryEventFromCanonical(event)
	}
	state, err := service.ReconstructFactoryWorldState(events, request.SelectedTick)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	through := recordings.CanonicalEventCursor{}
	if request.After != nil {
		through = *request.After
	}
	if len(request.Events) > 0 {
		through = request.Events[len(request.Events)-1].Cursor
	}
	return recordings.ReconstructWorldStateResult{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         request.Scope,
			Through:       through,
			SelectedTick:  request.SelectedTick,
			Payload:       string(payload),
		},
	}, nil
}

func validateProjectionEvents(
	scope recordings.CanonicalEventScope,
	after *recordings.CanonicalEventCursor,
	events []recordings.CanonicalEvent,
) error {
	if scope.FactorySessionID != "" && strings.TrimSpace(scope.FactorySessionID) == "" {
		return recordings.ErrInvalidProjectionScope
	}
	expected := recordings.CanonicalEventSequence(0)
	generationID := ""
	if after != nil {
		if after.StreamGenerationID == "" || after.Sequence < 0 {
			return recordings.ErrMalformedProjectionOrder
		}
		expected = after.Sequence + 1
		generationID = after.StreamGenerationID
	}
	previous := expected - 1
	for _, event := range events {
		if err := validateProjectionEvent(
			scope,
			event,
			expected,
			previous,
			generationID,
		); err != nil {
			return err
		}
		generationID = event.Cursor.StreamGenerationID
		previous = event.Sequence
		expected++
	}
	return nil
}

func validateProjectionEvent(
	scope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
	expected recordings.CanonicalEventSequence,
	previous recordings.CanonicalEventSequence,
	generationID string,
) error {
	if event.Scope != scope {
		return recordings.ErrInvalidProjectionScope
	}
	if event.Cursor.Sequence != event.Sequence ||
		event.Cursor.StreamGenerationID == "" {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID == "" && event.Sequence != expected {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID != "" && event.Sequence <= previous {
		return recordings.ErrMalformedProjectionOrder
	}
	if generationID != "" && event.Cursor.StreamGenerationID != generationID {
		return recordings.ErrMalformedProjectionOrder
	}
	return nil
}

func (service *combinedService) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	state, err := decodeWorldStateView(request.WorldState)
	if err != nil {
		return recordings.SimpleDashboardQueryResult{}, err
	}
	return recordings.SimpleDashboardQueryResult{
		Data: service.SimpleDashboardRenderData(state),
	}, nil
}

func (service *combinedService) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	state, err := decodeWorldStateView(request.WorldState)
	if err != nil {
		return recordings.WorkstationRequestsQueryResult{}, err
	}
	return recordings.WorkstationRequestsQueryResult{
		Projection: service.ProjectWorkstationRequests(state),
	}, nil
}

func decodeWorldStateView(view recordings.WorldStateView) (factorydefinitions.FactoryWorldState, error) {
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 ||
		strings.TrimSpace(view.Payload) == "" {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrUnsupportedProjectionView
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
	}
	return state, nil
}

func (service *combinedService) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	if err := validateReconnectReplayHistory(
		request.Scope,
		request.Cursor,
		request.Events,
	); err != nil {
		return err
	}
	afterSequence := int(request.Cursor.Sequence)
	events := make([]factorydefinitions.FactoryEvent, len(request.Events))
	for index, event := range request.Events {
		events[index] = factoryEventFromCanonical(event)
	}
	return service.ValidateReconnectReplay(
		events,
		factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &afterSequence},
		factorydefinitions.FactoryEventReconnectScope{SessionID: request.Scope.FactorySessionID},
	)
}

func validateReconnectReplayHistory(
	scope recordings.CanonicalEventScope,
	cursor recordings.CanonicalEventCursor,
	events []recordings.CanonicalEvent,
) error {
	if cursor.StreamGenerationID == "" || cursor.Sequence < 0 {
		return recordings.ErrMalformedProjectionOrder
	}
	if err := validateProjectionEvents(scope, nil, events); err != nil {
		return err
	}
	for _, event := range events {
		if event.Cursor == cursor {
			return nil
		}
	}
	return recordings.ErrReconnectCursorNotFound
}

func (service *combinedService) replayArtifactKey(request recordings.LoadReplayArtifactRequest) string {
	if id := strings.TrimSpace(request.ArtifactID); id != "" {
		return id
	}
	return strings.TrimSpace(request.Path)
}

func (service *combinedService) LoadReplayArtifact(
	request recordings.LoadReplayArtifactRequest,
) (recordings.LoadReplayArtifactResult, error) {
	key := service.replayArtifactKey(request)
	if key == "" {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrMissingReplayArtifact
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if service.replayByKey == nil {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrInvalidReplayArtifact
	}
	artifact, ok := service.replayByKey[key]
	if !ok || artifact == nil {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrInvalidReplayArtifact
	}
	detached := *artifact
	return recordings.LoadReplayArtifactResult{Artifact: &detached}, nil
}

func (service *combinedService) BindReplayExecution(
	request recordings.BindReplayExecutionRequest,
) (recordings.BindReplayExecutionResult, error) {
	if request.Artifact == nil || strings.TrimSpace(request.Artifact.SchemaVersion) == "" {
		return recordings.BindReplayExecutionResult{}, recordings.ErrUnsupportedReplayBinding
	}
	// Additive root publication returns the published success shape without
	// completing nested IMP-REC replay-execution wiring. Nested packets still
	// own binding real provider/command-runner/hooks behind this contract.
	return recordings.BindReplayExecutionResult{
		Hooks: []recordings.ReplayHook{},
	}, nil
}

func (service *combinedService) LoadReplayRecording(
	request recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	snapshot, err := service.Snapshot(request.RecordingID)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFound
	}
	if err != nil {
		return recordings.LoadReplayRecordingResult{}, err
	}
	if snapshot.Status.FinalizedAt == nil {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFinalized
	}
	return recordings.LoadReplayRecordingResult{
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: request.RecordingID,
			Scope:       snapshot.Status.Scope,
			Events:      append([]recordings.CanonicalEvent(nil), snapshot.Events...),
		},
	}, nil
}

func validateNeutralReplayPlan(request recordings.CreateReplayPlanRequest) error {
	if request.SchemaVersion != recordings.ReplayPlanSchemaV1 ||
		request.Timing != recordings.ReplayTimingOrderOnly ||
		request.SelectedTick < 0 {
		return recordings.ErrUnsupportedReplayPlan
	}
	if strings.TrimSpace(string(request.Recording.RecordingID)) == "" {
		return recordings.ErrCorruptReplayInput
	}
	for _, event := range request.Recording.Events {
		if !validAppendEvent(event) {
			return recordings.ErrCorruptReplayInput
		}
	}
	if err := validateProjectionEvents(
		request.Recording.Scope,
		nil,
		request.Recording.Events,
	); err != nil {
		return recordings.ErrCorruptReplayInput
	}
	return nil
}

func (service *combinedService) CreateReplayPlan(
	request recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	if err := validateNeutralReplayPlan(request); err != nil {
		return recordings.CreateReplayPlanResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	service.nextReplayPlan++
	handle := recordings.ReplayPlanHandle("replay-plan-" + strconv.Itoa(service.nextReplayPlan))
	facts := recordings.ReplayPlanFacts{
		Handle:        handle,
		RecordingID:   request.Recording.RecordingID,
		Scope:         request.Recording.Scope,
		TotalEvents:   len(request.Recording.Events),
		SchemaVersion: request.SchemaVersion,
		Timing:        request.Timing,
	}
	var expected *recordings.CanonicalEventCursor
	if request.ExpectedThrough != nil {
		cursor := *request.ExpectedThrough
		expected = &cursor
	}
	service.replayPlans[handle] = &neutralReplayPlan{
		facts:           facts,
		events:          append([]recordings.CanonicalEvent(nil), request.Recording.Events...),
		expectedThrough: expected,
		selectedTick:    request.SelectedTick,
	}
	return recordings.CreateReplayPlanResult{Plan: facts}, nil
}

func (service *combinedService) ObserveReplay(
	request recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	plan, ok := service.replayPlans[request.Plan]
	if !ok {
		return recordings.ObserveReplayResult{}, recordings.ErrReplayPlanNotFound
	}
	if plan.processed < len(plan.events) {
		plan.processed++
	}
	reduced, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        plan.facts.Scope,
		Events:       append([]recordings.CanonicalEvent(nil), plan.events[:plan.processed]...),
		SelectedTick: plan.selectedTick,
	})
	if err != nil {
		return recordings.ObserveReplayResult{}, err
	}
	observation := recordings.ReplayObservation{
		Kind:            recordings.ReplayProgress,
		Plan:            request.Plan,
		ProcessedEvents: plan.processed,
		TotalEvents:     len(plan.events),
		WorldState:      reduced.WorldState,
	}
	if plan.processed > 0 {
		cursor := plan.events[plan.processed-1].Cursor
		observation.Through = &cursor
	}
	if plan.processed == len(plan.events) {
		observation.Kind = recordings.ReplayCompleted
		if replayDivergence := replayPlanDivergence(plan, observation.Through); replayDivergence != nil {
			observation.Kind = recordings.ReplayDiverged
			observation.Divergence = replayDivergence
		}
	}
	return recordings.ObserveReplayResult{Observation: observation}, nil
}

func replayPlanDivergence(
	plan *neutralReplayPlan,
	through *recordings.CanonicalEventCursor,
) *recordings.ReplayDivergenceFacts {
	if plan.expectedThrough == nil {
		return nil
	}
	actual := recordings.CanonicalEventCursor{}
	if through != nil {
		actual = *through
	}
	if actual == *plan.expectedThrough {
		return nil
	}
	return &recordings.ReplayDivergenceFacts{
		Expected: *plan.expectedThrough,
		Observed: actual,
	}
}

func NewService(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets ...recordings.LiveRecordingTargetPlanner,
) recordings.Service {
	return NewServiceWithLifecycleEffects(
		ledger,
		projection,
		firstTargetPlanner(targets),
		nil,
		nil,
	)
}

func firstTargetPlanner(
	targets []recordings.LiveRecordingTargetPlanner,
) recordings.LiveRecordingTargetPlanner {
	if len(targets) == 0 {
		return nil
	}
	return targets[0]
}

// NewServiceWithLifecycleEffects constructs the Recordings root with the exact
// active-flush persistence and scheduling effects selected by Wire.
func NewServiceWithLifecycleEffects(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	if ledger == nil || projection == nil {
		return nil
	}
	lifecycle := recordinglifecyclewire.NewService(
		targetPlanner,
		writer,
		tickers,
		clocks...,
	)
	publication, _ := artifactsexportwire.NewOSPublication()
	return &combinedService{
		Ledger:            ledger,
		ProjectionService: projection,
		Service:           lifecycle,
		artifactsExport:   artifactsexportwire.NewService(lifecycle, publication),
		replayByKey:       make(map[string]*factorydefinitions.ReplayArtifact),
		replayPlans:       make(map[recordings.ReplayPlanHandle]*neutralReplayPlan),
	}
}

func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingevents.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}
