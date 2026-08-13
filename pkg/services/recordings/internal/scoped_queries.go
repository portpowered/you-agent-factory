package internal

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

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
