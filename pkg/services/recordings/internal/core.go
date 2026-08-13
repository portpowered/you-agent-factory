package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
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
	replayService   recordingsreplay.Service
	canonicalLedger canonicalledger.Service

	lifecycleMu sync.Mutex
	recordingMu sync.Mutex
	replayByKey map[string]*recordings.ReplayArtifact
	clock       recordings.RecordingClock

	scopeMu     sync.RWMutex
	scopeIssuer string
	nextScopeID uint64
	scopeByRef  map[recordings.RecordingScopeRef]*recordingScopeBinding

	runtimeRouter          *runtimeLedgerRouter
	runtimeSnapshotCapture factorydefinitions.LoadedFactorySnapshotCapturer
	replaySnapshotDecoder  factorydefinitions.FactorySnapshotJSONDecoder
	replayConfigDecoder    factorydefinitions.ReplayRuntimeConfigDecoder
	replayInputs           recordings.ReplayInputLoader
	logger                 logging.Logger
}

var _ recordings.Service = (*combinedService)(nil)

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return service.canonicalLedger.Append(request)
}

func (service *combinedService) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	return service.canonicalLedger.SubscribeFrom(ctx, request)
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

func decodeWorldStateView(view recordings.WorldStateView) (recordings.FactoryWorldState, error) {
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 ||
		strings.TrimSpace(view.Payload) == "" {
		return recordings.FactoryWorldState{}, recordings.ErrUnsupportedProjectionView
	}
	var state recordings.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return recordings.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
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
	publication portableArtifactPublication,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return newServiceWithLifecycleEffects(
		ledger,
		projection,
		targetPlanner,
		writer,
		tickers,
		publication,
		logging.NoopLogger{},
		clocks...,
	)
}

// NewServiceWithLifecycleEffectsAndLogger constructs the Recordings root with
// the process logger selected by canonical Wire. The logger is intentionally
// separate from the legacy test-friendly constructor so existing owner tests
// continue to exercise a no-op service without manufacturing an application
// logging graph.
func NewServiceWithLifecycleEffectsAndLogger(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	publication portableArtifactPublication,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return newServiceWithLifecycleEffects(
		ledger,
		projection,
		targetPlanner,
		writer,
		tickers,
		publication,
		logger,
		clocks...,
	)
}

func newServiceWithLifecycleEffects(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	publication portableArtifactPublication,
	logger logging.Logger,
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
	service := &combinedService{
		Ledger:            ledger,
		ProjectionService: projection,
		Service:           lifecycle,
		artifactsExport:   artifactsexportwire.NewService(lifecycle, publication),
		replayService:     replaywire.NewService(lifecycle, projection),
		canonicalLedger:   canonicalledgerwire.NewService(ledger),
		replayByKey:       make(map[string]*recordings.ReplayArtifact),
		clock:             firstRecordingClock(clocks),
		logger:            logging.EnsureLogger(logger),
	}
	service.scopeIssuer = recordingScopeIssuer(service)
	service.scopeByRef = make(map[recordings.RecordingScopeRef]*recordingScopeBinding)
	return service
}

type recordingOperationLog struct {
	logger logging.Logger
	fields []any
}

func (service *combinedService) startOperationLog(
	name string,
	ref recordings.RecordingScopeRef,
	scope recordings.CanonicalEventScope,
) recordingOperationLog {
	var logger logging.Logger = logging.NoopLogger{}
	if service != nil {
		logger = logging.EnsureLogger(service.logger)
	}
	fields := []any{"operation", name}
	if !ref.IsZero() {
		fields = append(fields, "scope_ref", ref.String())
	}
	if scope.FactorySessionID != "" {
		fields = append(fields, "factory_session_id", scope.FactorySessionID)
	}
	logger.Info("recordings operation started", fields...)
	return recordingOperationLog{logger: logger, fields: fields}
}

func (operation recordingOperationLog) finish(err error) {
	fields := append([]any(nil), operation.fields...)
	fields = append(fields, "outcome", "success")
	if err != nil {
		fields[len(fields)-1] = "error"
		fields = append(fields, "error_type", fmt.Sprintf("%T", err))
	}
	operation.logger.Info("recordings operation finished", fields...)
}

func firstRecordingClock(clocks []recordings.RecordingClock) recordings.RecordingClock {
	for _, clock := range clocks {
		if clock != nil {
			return clock
		}
	}
	return nil
}

func recordingClockNow(clocks ...recordings.RecordingClock) func() time.Time {
	clock := firstRecordingClock(clocks)
	if clock == nil {
		return nil
	}
	return func() time.Time { return clock.Now() }
}

func (service *combinedService) recordingFinishedAt() time.Time {
	if service == nil || service.clock == nil {
		return time.Time{}
	}
	return service.clock.Now().UTC()
}

func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingevents.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}
