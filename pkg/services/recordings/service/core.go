package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
)

func NewProjectionService() recordings.ProjectionService {
	return projectionquerywire.NewService()
}

type combinedService struct {
	recordings.Ledger
	recordings.ProjectionService
	recordinglifecycle.Service
	artifactsExport artifactsexport.Service
	replayService recordingsreplay.Service

	appendMu    sync.Mutex
	lifecycleMu sync.Mutex
	replayByKey map[string]*factorydefinitions.ReplayArtifact
}

var _ recordings.Service = (*combinedService)(nil)

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	if !canonical.ValidAppendEvent(request.Event) {
		return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
	}
	service.appendMu.Lock()
	defer service.appendMu.Unlock()
	legacy := canonical.FactoryEventFromCanonical(request.Event)
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
	return canonical.ReconstructWorldState(service.ProjectionService, request)
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
		events[index] = canonical.FactoryEventFromCanonical(event)
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
	if err := canonical.ValidateProjectionEvents(scope, nil, events); err != nil {
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
	return service.replayService.LoadReplayRecording(request)
}

func (service *combinedService) CreateReplayPlan(
	request recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	return service.replayService.CreateReplayPlan(request)
}

func (service *combinedService) ObserveReplay(
	request recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	return service.replayService.ObserveReplay(request)
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
	publication artifactsexport.PortableArtifactPublication,
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
	if publication == nil {
		var err error
		publication, err = NewPortableArtifactPublication()
		if err != nil {
			return nil
		}
	}
	return &combinedService{
		Ledger:            ledger,
		ProjectionService: projection,
		Service:           lifecycle,
		artifactsExport:   artifactsexportwire.NewService(lifecycle, publication),
		replayService:     replaywire.NewService(lifecycle, projection),
		replayByKey:       make(map[string]*factorydefinitions.ReplayArtifact),
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
