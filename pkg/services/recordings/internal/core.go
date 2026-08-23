package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	factoryeventkinds "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events/kinds"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
	"strings"
	"sync"
	"time"
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
	historicalQuery historicalquery.Service

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
var _ recordings.CompletedFlushWatermarkReader = (*combinedService)(nil)

// CompletedFlushWatermark publishes the lifecycle owner's completed durable
// position through the Recordings root without exposing that owner or its
// mutable state to peers.
func (service *combinedService) CompletedFlushWatermark(
	streamGenerationID string,
) (recordings.CanonicalEventCursor, bool) {
	if service == nil || service.Service == nil {
		return recordings.CanonicalEventCursor{}, false
	}
	reader, ok := service.Service.(recordinglifecycle.CompletedFlushWatermarkReader)
	if !ok {
		return recordings.CanonicalEventCursor{}, false
	}
	return reader.CompletedFlushWatermark(streamGenerationID)
}

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return service.canonicalLedger.Append(request)
}

func (service *combinedService) ValidateFactoryEventKindParity(openAPIYAML []byte) error {
	return factoryeventkinds.ValidateBundledFactoryEventKindParity(openAPIYAML)
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

func (service *combinedService) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	if service == nil || service.historicalQuery == nil {
		return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
			Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
			RecordingID: request.Recording.RecordingID,
		}
	}
	return service.historicalQuery.QueryHistoricalRecording(request)
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

func (service *combinedService) LoadReplayRecordingForResume(
	request recordings.LoadReplayRecordingForResumeRequest,
) (recordings.LoadReplayRecordingForResumeResult, error) {
	return service.replayService.LoadReplayRecordingForResume(request)
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
		nil,
		nil,
		nil,
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
		nil,
		nil,
		nil,
		clocks...,
	)
}

// NewServiceWithLifecycleEffectsAndHistoricalQuery constructs the Recordings
// root with a Wire-selected historical read capability and no-op logging.
func NewServiceWithLifecycleEffectsAndHistoricalQuery(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	publication portableArtifactPublication,
	historicalQuery historicalquery.Service,
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
		historicalQuery,
		nil,
		nil,
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
	historicalQuery historicalquery.Service,
	readFile recordings.RecordingReadFile,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
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
		replayService:     replaywire.NewService(lifecycle, projection, readFile, decodeFactorySnapshot),
		canonicalLedger:   canonicalledgerwire.NewService(ledger),
		historicalQuery:   historicalQuery,
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

// OpenRecordingScope opens finalized lifecycle state as a historical scope.
// Unlike BeginRecordingScope, it does not start a target, ticker, writer, or
// other live effect for the selected recording.
func (service *combinedService) OpenRecordingScope(
	ctx context.Context,
	request recordings.OpenRecordingScopeRequest,
) (result recordings.OpenRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.open", recordings.RecordingScopeRef{}, request.Scope)
	defer func() { operation.finish(err) }()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.OpenRecordingScopeResult{}, err
	}
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.OpenRecordingScopeResult{}, recordings.ErrInvalidRecordingScope
	}
	snapshot, err := service.Service.Snapshot(request.RecordingID)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return recordings.OpenRecordingScopeResult{}, recordings.ErrReplayRecordingNotFound
	}
	if err != nil {
		return recordings.OpenRecordingScopeResult{}, err
	}
	if snapshot.Status.FinalizedAt == nil {
		return recordings.OpenRecordingScopeResult{}, recordings.ErrReplayRecordingNotFinalized
	}
	if request.Scope != (recordings.CanonicalEventScope{}) &&
		request.Scope != snapshot.Status.Scope {
		return recordings.OpenRecordingScopeResult{}, recordings.ErrInvalidRecordingScope
	}
	ref := service.newRecordingScope()
	binding := historicalScopeBinding(
		request.RecordingID,
		snapshot.Status,
	)
	binding.terminal = scopeStatusFrom(ref, snapshot.Status)
	service.scopeMu.Lock()
	service.scopeByRef[ref] = binding
	service.scopeMu.Unlock()
	return recordings.OpenRecordingScopeResult{
		Scope:  ref,
		Status: scopeStatusFrom(ref, snapshot.Status),
	}, nil
}

func historicalScopeBinding(
	recordingID recordings.RecordingID,
	status recordings.RecordingStatusFacts,
) *recordingScopeBinding {
	binding := &recordingScopeBinding{
		recordingID: recordingID,
		eventScope:  status.Scope,
		historical:  true,
		finalized:   true,
		replayPlans: make(map[recordings.ReplayPlanHandle]struct{}),
	}
	return binding
}

// recordingSnapshot is the small detached handoff needed by scope queries.
// It keeps lifecycle's private Snapshot type out of this file's public
// surface while making the prefix-selection invariant explicit.
type recordingSnapshot struct {
	status recordings.RecordingStatusFacts
	events []recordings.CanonicalEvent
}

func (service *combinedService) snapshotScopeLocked(
	ctx context.Context,
	binding *recordingScopeBinding,
	requireFinalized bool,
	through *recordings.CanonicalEventCursor,
) (recordingSnapshot, error) {
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordingSnapshot{}, err
	}
	if binding.closed {
		return recordingSnapshot{}, recordings.ErrRecordingScopeClosed
	}
	snapshot, err := service.Service.Snapshot(binding.recordingID)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return recordingSnapshot{}, recordings.ErrRecordingScopeStale
	}
	if err != nil {
		return recordingSnapshot{}, err
	}
	if requireFinalized && snapshot.Status.FinalizedAt == nil {
		return recordingSnapshot{}, recordings.ErrReplayRecordingNotFinalized
	}
	events, err := scopeEventPrefix(snapshot.Events, through)
	if err != nil {
		return recordingSnapshot{}, err
	}
	return recordingSnapshot{
		status: snapshot.Status,
		events: events,
	}, nil
}

func scopeEventPrefix(
	events []recordings.CanonicalEvent,
	through *recordings.CanonicalEventCursor,
) ([]recordings.CanonicalEvent, error) {
	if through == nil {
		return append([]recordings.CanonicalEvent(nil), events...), nil
	}
	if through.StreamGenerationID == "" || through.Sequence < 0 {
		return nil, recordings.ErrInvalidReconnectCursor
	}
	if len(events) > 0 && through.StreamGenerationID != events[0].Cursor.StreamGenerationID {
		return nil, recordings.ErrReconnectCursorUnavailable
	}
	for index, event := range events {
		if event.Cursor == *through {
			return append([]recordings.CanonicalEvent(nil), events[:index+1]...), nil
		}
	}
	return nil, recordings.ErrReconnectCursorNotFound
}

func (service *combinedService) SubscribeRecordingScope(
	ctx context.Context,
	request recordings.SubscribeRecordingScopeRequest,
) (result recordings.SubscribeRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.subscribe", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.SubscribeRecordingScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	snapshot, err := service.snapshotScopeLocked(ctx, binding, false, nil)
	if err != nil {
		return recordings.SubscribeRecordingScopeResult{}, err
	}
	if request.Cursor != nil {
		if err := service.validateScopeCursor(binding, snapshot.events, *request.Cursor); err != nil {
			return recordings.SubscribeRecordingScopeResult{}, err
		}
	}
	if binding.historical {
		subscription, err := newHistoricalScopeSubscription(ctx, snapshot.events, request.Cursor)
		if err != nil {
			return recordings.SubscribeRecordingScopeResult{}, err
		}
		return recordings.SubscribeRecordingScopeResult{Subscription: subscription}, nil
	}
	subscribed, err := service.canonicalLedger.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: request.Cursor,
		Scope:  binding.eventScope,
	})
	if err != nil {
		return recordings.SubscribeRecordingScopeResult{}, err
	}
	return recordings.SubscribeRecordingScopeResult{
		Subscription: subscribed.Subscription,
	}, nil
}

func newHistoricalScopeSubscription(
	ctx context.Context,
	events []recordings.CanonicalEvent,
	cursor *recordings.CanonicalEventCursor,
) (recordings.EventSubscription, error) {
	start := 0
	if cursor != nil {
		for index, event := range events {
			if event.Cursor == *cursor {
				start = index + 1
				break
			}
		}
		if start == 0 {
			return nil, recordings.ErrReconnectCursorExpired
		}
	}
	remaining := append([]recordings.CanonicalEvent(nil), events[start:]...)
	return func(nextContext context.Context) recordings.SubscriptionOutcome {
		if nextContext == nil {
			nextContext = ctx
		}
		select {
		case <-nextContext.Done():
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		default:
		}
		if len(remaining) == 0 {
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
		event := remaining[0]
		remaining = remaining[1:]
		return recordings.SubscriptionOutcome{
			Kind:  recordings.SubscriptionEvent,
			Event: event,
		}
	}, nil
}

func cursorInEvents(
	events []recordings.CanonicalEvent,
	cursor recordings.CanonicalEventCursor,
) bool {
	if cursor.StreamGenerationID == "" || cursor.Sequence < 0 {
		return false
	}
	for _, event := range events {
		if event.Cursor == cursor {
			return true
		}
	}
	return false
}

func (service *combinedService) validateScopeCursor(
	binding *recordingScopeBinding,
	events []recordings.CanonicalEvent,
	cursor recordings.CanonicalEventCursor,
) error {
	if cursor.StreamGenerationID == "" || cursor.Sequence < 0 {
		return recordings.ErrInvalidReconnectCursor
	}
	generationID := ""
	if len(events) > 0 {
		generationID = events[0].Cursor.StreamGenerationID
	} else if !binding.historical && service.Ledger != nil {
		generationID = service.Ledger.StreamGenerationID()
	}
	if generationID != "" && cursor.StreamGenerationID != generationID {
		return recordings.ErrReconnectCursorUnavailable
	}
	if !cursorInEvents(events, cursor) {
		return recordings.ErrReconnectCursorExpired
	}
	return nil
}

func (service *combinedService) LoadReplayRecordingScope(
	ctx context.Context,
	request recordings.LoadReplayRecordingScopeRequest,
) (result recordings.LoadReplayRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.replay_load", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, snapshot, err := service.scopeSnapshot(
		ctx,
		request.Scope,
		true,
		nil,
	)
	if err != nil {
		return recordings.LoadReplayRecordingScopeResult{}, err
	}
	return recordings.LoadReplayRecordingScopeResult{
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: binding.recordingID,
			Scope:       snapshot.status.Scope,
			Events:      snapshot.events,
		},
		Status: scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) CreateReplayPlanScope(
	ctx context.Context,
	request recordings.CreateReplayPlanScopeRequest,
) (result recordings.CreateReplayPlanScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.replay_plan", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.CreateReplayPlanScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	snapshot, err := service.snapshotScopeLocked(ctx, binding, true, nil)
	if err != nil {
		return recordings.CreateReplayPlanScopeResult{}, err
	}
	planned, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: request.SchemaVersion,
		Timing:        request.Timing,
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: binding.recordingID,
			Scope:       snapshot.status.Scope,
			Events:      snapshot.events,
		},
		ExpectedThrough: request.ExpectedThrough,
		SelectedTick:    request.SelectedTick,
	})
	if err != nil {
		return recordings.CreateReplayPlanScopeResult{}, err
	}
	binding.replayPlans[planned.Plan.Handle] = struct{}{}
	return recordings.CreateReplayPlanScopeResult{
		Plan:   planned.Plan,
		Status: scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) ObserveReplayScope(
	ctx context.Context,
	request recordings.ObserveReplayScopeRequest,
) (result recordings.ObserveReplayScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.replay_observe", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.ObserveReplayScopeResult{}, err
	}
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.ObserveReplayScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return recordings.ObserveReplayScopeResult{}, recordings.ErrRecordingScopeClosed
	}
	if _, ok := binding.replayPlans[request.Plan]; !ok {
		return recordings.ObserveReplayScopeResult{}, recordings.ErrReplayPlanNotFound
	}
	observed, err := service.ObserveReplay(recordings.ObserveReplayRequest{Plan: request.Plan})
	if err != nil {
		return recordings.ObserveReplayScopeResult{}, err
	}
	snapshot, err := service.snapshotScopeLocked(ctx, binding, true, nil)
	if err != nil {
		return recordings.ObserveReplayScopeResult{}, err
	}
	return recordings.ObserveReplayScopeResult{
		Observation: observed.Observation,
		Status:      scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) scopeSnapshot(
	ctx context.Context,
	ref recordings.RecordingScopeRef,
	requireFinalized bool,
	through *recordings.CanonicalEventCursor,
) (*recordingScopeBinding, recordingSnapshot, error) {
	binding, err := service.recordingScope(ref)
	if err != nil {
		return nil, recordingSnapshot{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	snapshot, err := service.snapshotScopeLocked(
		ctx,
		binding,
		requireFinalized,
		through,
	)
	if err != nil {
		return nil, recordingSnapshot{}, err
	}
	return binding, snapshot, nil
}

func (service *combinedService) ReconstructRecordingScope(
	ctx context.Context,
	request recordings.ReconstructRecordingScopeRequest,
) (result recordings.ReconstructRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.project", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	_, snapshot, err := service.scopeSnapshot(ctx, request.Scope, false, request.Through)
	if err != nil {
		return recordings.ReconstructRecordingScopeResult{}, err
	}
	reconstructed, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        snapshot.status.Scope,
		Events:       snapshot.events,
		SelectedTick: request.SelectedTick,
	})
	if err != nil {
		return recordings.ReconstructRecordingScopeResult{}, err
	}
	return recordings.ReconstructRecordingScopeResult{
		WorldState: reconstructed.WorldState,
		Status:     scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) QuerySimpleDashboardScope(
	ctx context.Context,
	request recordings.QuerySimpleDashboardScopeRequest,
) (result recordings.QuerySimpleDashboardScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.dashboard", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	projected, err := service.ReconstructRecordingScope(ctx, recordings.ReconstructRecordingScopeRequest{
		Scope:        request.Scope,
		Through:      request.Through,
		SelectedTick: request.SelectedTick,
	})
	if err != nil {
		return recordings.QuerySimpleDashboardScopeResult{}, err
	}
	dashboard, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: projected.WorldState,
	})
	if err != nil {
		return recordings.QuerySimpleDashboardScopeResult{}, err
	}
	return recordings.QuerySimpleDashboardScopeResult{
		Data:       dashboard.Data,
		WorldState: projected.WorldState,
		Status:     projected.Status,
	}, nil
}

func (service *combinedService) QueryWorkstationRequestsScope(
	ctx context.Context,
	request recordings.QueryWorkstationRequestsScopeRequest,
) (result recordings.QueryWorkstationRequestsScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.workstation_requests", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	projected, err := service.ReconstructRecordingScope(ctx, recordings.ReconstructRecordingScopeRequest{
		Scope:        request.Scope,
		Through:      request.Through,
		SelectedTick: request.SelectedTick,
	})
	if err != nil {
		return recordings.QueryWorkstationRequestsScopeResult{}, err
	}
	workstation, err := service.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: projected.WorldState,
	})
	if err != nil {
		return recordings.QueryWorkstationRequestsScopeResult{}, err
	}
	return recordings.QueryWorkstationRequestsScopeResult{
		Projection: workstation.Projection,
		WorldState: projected.WorldState,
		Status:     projected.Status,
	}, nil
}

func (service *combinedService) BuildPortableArtifactScope(
	ctx context.Context,
	request recordings.BuildPortableArtifactScopeRequest,
) (result recordings.BuildPortableArtifactScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.artifact_build", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, snapshot, err := service.scopeSnapshot(ctx, request.Scope, false, nil)
	if err != nil {
		return recordings.BuildPortableArtifactScopeResult{}, err
	}
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.BuildPortableArtifactScopeResult{}, err
	}
	if built.Artifact.Summary.Scope != snapshot.status.Scope {
		return recordings.BuildPortableArtifactScopeResult{}, recordings.ErrForeignPortableArtifact
	}
	return recordings.BuildPortableArtifactScopeResult{
		Artifact: built.Artifact,
		Status:   scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) ExportPortableArtifactScope(
	ctx context.Context,
	request recordings.ExportPortableArtifactScopeRequest,
) (result recordings.ExportPortableArtifactScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.artifact_export", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, snapshot, err := service.scopeSnapshot(ctx, request.Scope, false, nil)
	if err != nil {
		return recordings.ExportPortableArtifactScopeResult{}, err
	}
	exported, err := service.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.ExportPortableArtifactScopeResult{}, err
	}
	if exported.Artifact.Summary.Scope != snapshot.status.Scope {
		return recordings.ExportPortableArtifactScopeResult{}, recordings.ErrForeignPortableArtifact
	}
	return recordings.ExportPortableArtifactScopeResult{
		Reference: exported.Reference,
		Artifact:  exported.Artifact,
		Status:    scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}

func (service *combinedService) ReadPortableArtifactScope(
	ctx context.Context,
	request recordings.ReadPortableArtifactScopeRequest,
) (result recordings.ReadPortableArtifactScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.artifact_read", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, snapshot, err := service.scopeSnapshot(ctx, request.Scope, false, nil)
	if err != nil {
		return recordings.ReadPortableArtifactScopeResult{}, err
	}
	read, err := service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: binding.recordingID,
		Reference:   request.Reference,
	})
	if err != nil {
		return recordings.ReadPortableArtifactScopeResult{}, err
	}
	if read.Artifact.Summary.Scope != snapshot.status.Scope {
		return recordings.ReadPortableArtifactScopeResult{}, recordings.ErrForeignPortableArtifact
	}
	return recordings.ReadPortableArtifactScopeResult{
		Artifact: read.Artifact,
		Status:   scopeStatusFrom(request.Scope, snapshot.status),
	}, nil
}
