package recordings_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type peerRecordingSession struct {
	recordPath string
	started    bool
	finished   bool
	stopped    bool
	flushFail  bool
	events     []interfaces.FactoryEvent
	retainedErr error
}

// peerRootServiceFake is a peer-shaped Recordings root Service that uses only
// the published Recordings root package (plus approved peer-root vocabulary
// already present in root signatures). It never imports recordings nested
// implementation packages (events/, projections/, replay/, artifacts/, service/).
type peerRootServiceFake struct {
	events              []interfaces.FactoryEvent
	streamGenerationID  string
	subscribeErr        error
	subscribeStream     recordings.EventStream
	reconstructState    interfaces.FactoryWorldState
	reconstructErr      error
	dashboardData       recordings.SimpleDashboardRenderData
	throttlePauses      []interfaces.FactoryWorldThrottlePause
	workstationRequests recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice
	validateReplayErr   error
	recordings          map[string]*peerRecordingSession
	nextRecordingID     int
}

var _ recordings.Service = (*peerRootServiceFake)(nil)

func (fake *peerRootServiceFake) CanonicalEvents() []interfaces.FactoryEvent {
	if fake.events == nil {
		return nil
	}
	out := make([]interfaces.FactoryEvent, len(fake.events))
	copy(out, fake.events)
	return out
}

func (fake *peerRootServiceFake) Subscribe(
	_ context.Context,
	cursor *interfaces.FactoryEventReconnectCursor,
	_ interfaces.FactoryEventReconnectScope,
) (interfaces.FactoryEventStream, error) {
	if fake.subscribeErr != nil {
		return interfaces.FactoryEventStream{}, fake.subscribeErr
	}
	if cursor != nil && cursor.AfterEventID != "" && len(fake.events) == 0 {
		return interfaces.FactoryEventStream{}, recordings.ErrReconnectCursorNotFound
	}
	return fake.subscribeStream, nil
}

func (fake *peerRootServiceFake) StreamGenerationID() string {
	return fake.streamGenerationID
}

func (fake *peerRootServiceFake) AddEventRecorder(func(interfaces.FactoryEvent)) {}

func (fake *peerRootServiceFake) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {}

func (fake *peerRootServiceFake) AppendRecordedEvent(event interfaces.FactoryEvent) {
	fake.events = append(fake.events, event)
}

func (fake *peerRootServiceFake) Append(
	request recordings.AppendRecordedEventRequest,
) recordings.AppendRecordedEventResult {
	fake.AppendRecordedEvent(request.Event)
	return recordings.AppendRecordedEventResult{}
}

func (fake *peerRootServiceFake) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if err := invalidSubscribeScope(request.Scope); err != nil {
		return recordings.SubscribeResult{}, err
	}
	if fake.subscribeErr != nil {
		return recordings.SubscribeResult{}, fake.subscribeErr
	}
	if request.Cursor != nil && strings.TrimSpace(request.Cursor.AfterEventID) != "" {
		ackIndex := -1
		for index, event := range fake.events {
			if event.Id == request.Cursor.AfterEventID {
				ackIndex = index
				break
			}
		}
		if ackIndex < 0 {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorNotFound
		}
		newer := append([]interfaces.FactoryEvent(nil), fake.events[ackIndex+1:]...)
		return recordings.SubscribeResult{
			Stream: recordings.EventStream{
				StreamGenerationID: fake.streamGenerationID,
				History:            newer,
			},
		}, nil
	}
	stream, err := fake.Subscribe(ctx, request.Cursor, interfaces.FactoryEventReconnectScope(request.Scope))
	if err != nil {
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{Stream: stream}, nil
}

func invalidSubscribeScope(scope recordings.EventReconnectScope) error {
	if scope.SessionID != "" && strings.TrimSpace(scope.SessionID) == "" {
		return recordings.ErrInvalidSubscribeScope
	}
	return nil
}

func (fake *peerRootServiceFake) ReconstructFactoryWorldState(
	_ []interfaces.FactoryEvent,
	_ int,
) (interfaces.FactoryWorldState, error) {
	return fake.reconstructState, fake.reconstructErr
}

func (fake *peerRootServiceFake) SimpleDashboardRenderData(
	_ interfaces.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return fake.dashboardData
}

func (fake *peerRootServiceFake) ProjectActiveThrottlePauses(
	_ interfaces.InitialStructurePayload,
	_ []interfaces.ActiveThrottlePause,
) []interfaces.FactoryWorldThrottlePause {
	return fake.throttlePauses
}

func (fake *peerRootServiceFake) ProjectWorkstationRequests(
	_ interfaces.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return fake.workstationRequests
}

func (fake *peerRootServiceFake) ValidateReconnectReplay(
	_ []interfaces.FactoryEvent,
	_ interfaces.FactoryEventReconnectCursor,
	_ interfaces.FactoryEventReconnectScope,
) error {
	return fake.validateReplayErr
}

func (fake *peerRootServiceFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	state, err := fake.ReconstructFactoryWorldState(request.Events, request.SelectedTick)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	return recordings.ReconstructWorldStateResult{WorldState: state}, nil
}

func (fake *peerRootServiceFake) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) recordings.SimpleDashboardQueryResult {
	return recordings.SimpleDashboardQueryResult{
		Data: fake.SimpleDashboardRenderData(request.WorldState),
	}
}

func (fake *peerRootServiceFake) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) recordings.WorkstationRequestsQueryResult {
	return recordings.WorkstationRequestsQueryResult{
		Projection: fake.ProjectWorkstationRequests(request.WorldState),
	}
}

func (fake *peerRootServiceFake) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	return fake.ValidateReconnectReplay(
		request.Events,
		interfaces.FactoryEventReconnectCursor(request.Cursor),
		interfaces.FactoryEventReconnectScope(request.Scope),
	)
}

func (fake *peerRootServiceFake) ensureRecordings() {
	if fake.recordings == nil {
		fake.recordings = make(map[string]*peerRecordingSession)
	}
}

func (fake *peerRootServiceFake) recordingSession(id string) (*peerRecordingSession, error) {
	fake.ensureRecordings()
	if strings.TrimSpace(id) == "" {
		return nil, recordings.ErrMissingRecordingTarget
	}
	session, ok := fake.recordings[id]
	if !ok {
		return nil, recordings.ErrMissingRecordingTarget
	}
	return session, nil
}

func (fake *peerRootServiceFake) BindRecording(
	request recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	if strings.TrimSpace(request.RecordPath) == "" {
		return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
	}
	fake.ensureRecordings()
	id := strings.TrimSpace(request.RecordingID)
	if id == "" {
		fake.nextRecordingID++
		id = "recording-" + strconv.Itoa(fake.nextRecordingID)
	}
	fake.recordings[id] = &peerRecordingSession{recordPath: request.RecordPath}
	return recordings.BindRecordingResult{RecordingID: id}, nil
}

func (fake *peerRootServiceFake) StartRecording(
	_ context.Context,
	request recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.StartRecordingResult{}, err
	}
	session.started = true
	session.stopped = false
	return recordings.StartRecordingResult{}, nil
}

func (fake *peerRootServiceFake) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingEventResult{}, err
	}
	if session.finished {
		return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
	}
	session.events = append(session.events, request.Event)
	return recordings.RecordRecordingEventResult{}, nil
}

func (fake *peerRootServiceFake) RecordRecordingError(
	request recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingErrorResult{}, err
	}
	if session.finished {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrRecordingWriteRejected
	}
	if request.Err != nil {
		session.retainedErr = request.Err
		session.flushFail = true
	}
	return recordings.RecordRecordingErrorResult{}, nil
}

func (fake *peerRootServiceFake) FlushRecording(
	request recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.FlushRecordingResult{}, err
	}
	if session.flushFail {
		if session.retainedErr == nil {
			session.retainedErr = recordings.ErrRecordingFlushFailed
		}
		return recordings.FlushRecordingResult{}, recordings.ErrRecordingFlushFailed
	}
	return recordings.FlushRecordingResult{}, nil
}

func (fake *peerRootServiceFake) FinishRecording(
	request recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.FinishRecordingResult{}, err
	}
	_ = request.FinishedAt
	session.finished = true
	return recordings.FinishRecordingResult{}, nil
}

func (fake *peerRootServiceFake) StopRecording(
	request recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.StopRecordingResult{}, err
	}
	session.stopped = true
	session.started = false
	return recordings.StopRecordingResult{}, nil
}

func (fake *peerRootServiceFake) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordingStatusResult{}, err
	}
	return recordings.RecordingStatusResult{
		Started:  session.started,
		Finished: session.finished,
		Stopped:  session.stopped,
		Err:      session.retainedErr,
	}, nil
}

func TestServiceRootContract_FakeImplementsAndExercisesSeam(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{
		streamGenerationID: "generation-empty",
		subscribeErr:       recordings.ErrReconnectCursorNotFound,
		validateReplayErr:  recordings.ErrReconnectCursorNotFound,
	}

	// Peers consume only the singular root Service seam.
	var service recordings.Service = fake
	ctx := context.Background()

	events := service.CanonicalEvents()
	if len(events) != 0 {
		t.Fatalf("CanonicalEvents = %#v, want empty read through singular root", events)
	}
	if got := service.StreamGenerationID(); got != "generation-empty" {
		t.Fatalf("StreamGenerationID = %q, want generation-empty", got)
	}

	_, err := service.Subscribe(
		ctx,
		&interfaces.FactoryEventReconnectCursor{AfterEventID: "missing-cursor"},
		interfaces.FactoryEventReconnectScope{},
	)
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("Subscribe error = %v, want ErrReconnectCursorNotFound", err)
	}

	state, err := service.ReconstructFactoryWorldState(nil, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if state.Tick != 0 {
		t.Fatalf("ReconstructFactoryWorldState state = %#v, want empty detached world state", state)
	}

	dashboard := service.SimpleDashboardRenderData(state)
	if dashboard.InFlightDispatchCount != 0 {
		t.Fatalf("SimpleDashboardRenderData = %#v, want empty dashboard projection", dashboard)
	}

	workstation := service.ProjectWorkstationRequests(state)
	if workstation.WorkstationRequestsByDispatchId != nil && len(*workstation.WorkstationRequestsByDispatchId) != 0 {
		t.Fatalf("ProjectWorkstationRequests = %#v, want empty workstation projection", workstation)
	}

	if err := service.ValidateReconnectReplay(
		nil,
		interfaces.FactoryEventReconnectCursor{AfterEventID: "missing-cursor"},
		interfaces.FactoryEventReconnectScope{},
	); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("ValidateReconnectReplay error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestAppendSubscribeRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{streamGenerationID: "generation-append-subscribe"}
	var service recordings.Service = fake
	ctx := context.Background()

	first := interfaces.FactoryEvent{Id: "event-1", Type: "WORK_REQUEST"}
	second := interfaces.FactoryEvent{Id: "event-2", Type: "WORK_STATE_CHANGE"}
	_ = service.Append(recordings.AppendRecordedEventRequest{Event: first})
	_ = service.Append(recordings.AppendRecordedEventRequest{Event: second})

	cursor := recordings.EventReconnectCursor{AfterEventID: "event-1"}
	result, err := service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.EventReconnectScope{SessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom success path: %v", err)
	}
	if got := len(result.Stream.History); got != 1 {
		t.Fatalf("SubscribeFrom history len = %d, want 1 event newer than cursor", got)
	}
	if got := result.Stream.History[0].Id; got != "event-2" {
		t.Fatalf("SubscribeFrom newer event id = %q, want event-2", got)
	}
	if got := result.Stream.StreamGenerationID; got != "generation-append-subscribe" {
		t.Fatalf("SubscribeFrom stream generation = %q, want generation-append-subscribe", got)
	}

	stale := recordings.EventReconnectCursor{AfterEventID: "missing-cursor"}
	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &stale,
		Scope:  recordings.EventReconnectScope{SessionID: "session-1"},
	})
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("stale cursor error = %v, want ErrReconnectCursorNotFound", err)
	}

	_, err = service.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.EventReconnectScope{SessionID: "   "},
	})
	if !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("invalid scope error = %v, want ErrInvalidSubscribeScope", err)
	}
	if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("invalid scope must remain distinct from ErrReconnectCursorNotFound")
	}
}

func TestProjectionQueryRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{
		reconstructState: interfaces.FactoryWorldState{Tick: 7},
		dashboardData: recordings.SimpleDashboardRenderData{
			InFlightDispatchCount: 2,
		},
		workstationRequests: recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{},
		validateReplayErr:   recordings.ErrReconnectCursorNotFound,
	}
	var service recordings.Service = fake

	events := []interfaces.FactoryEvent{
		{Id: "event-1", Type: "WORK_REQUEST", Context: interfaces.FactoryEventContext{Tick: 7}},
		{Id: "event-2", Type: "WORK_STATE_CHANGE", Context: interfaces.FactoryEventContext{Tick: 7}},
	}
	world, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       events,
		SelectedTick: 7,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState success path: %v", err)
	}
	if world.WorldState.Tick != 7 {
		t.Fatalf("ReconstructWorldState tick = %d, want 7", world.WorldState.Tick)
	}

	dashboard := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	})
	if dashboard.Data.InFlightDispatchCount != 2 {
		t.Fatalf("QuerySimpleDashboard = %#v, want InFlightDispatchCount 2", dashboard.Data)
	}

	workstation := service.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: world.WorldState,
	})
	if workstation.Projection.WorkstationRequestsByDispatchId != nil &&
		len(*workstation.Projection.WorkstationRequestsByDispatchId) != 0 {
		t.Fatalf("QueryWorkstationRequests = %#v, want empty detached projection", workstation.Projection)
	}

	if err := service.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: nil,
		Cursor: recordings.EventReconnectCursor{AfterEventID: "missing-cursor"},
		Scope:  recordings.EventReconnectScope{SessionID: "session-1"},
	}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("invalid reconnect validation error = %v, want ErrReconnectCursorNotFound", err)
	}

	_, err = service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       events,
		SelectedTick: -1,
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("malformed projection input error = %v, want ErrInvalidProjectionInput", err)
	}
	if errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("malformed projection input must remain distinct from ErrReconnectCursorNotFound")
	}
}

func TestRecordingLifecycleRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{}
	var service recordings.Service = fake
	ctx := context.Background()

	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordPath:    "session.replay.json",
		FlushInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("BindRecording success path: %v", err)
	}
	if bound.RecordingID == "" {
		t.Fatal("BindRecording RecordingID empty, want bound handle")
	}

	if _, err := service.StartRecording(ctx, recordings.StartRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       interfaces.FactoryEvent{Id: "event-1", Type: "WORK_REQUEST"},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}

	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording success path: %v", err)
	}

	finishedAt := time.Unix(1_700_000_000, 0).UTC()
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.RecordingID,
		FinishedAt:  finishedAt,
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}

	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: bound.RecordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if !status.Finished {
		t.Fatalf("QueryRecordingStatus = %#v, want Finished true after finish", status)
	}

	_, err = service.BindRecording(recordings.BindRecordingRequest{RecordPath: ""})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing recording target error = %v, want ErrMissingRecordingTarget", err)
	}

	_, err = service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: "missing-id"})
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing recording id error = %v, want ErrMissingRecordingTarget", err)
	}

	failing, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordPath: "failing.replay.json",
	})
	if err != nil {
		t.Fatalf("BindRecording for flush failure: %v", err)
	}
	if _, err := service.StartRecording(ctx, recordings.StartRecordingRequest{
		RecordingID: failing.RecordingID,
	}); err != nil {
		t.Fatalf("StartRecording for flush failure: %v", err)
	}
	if _, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: failing.RecordingID,
		Err:         errors.New("producer boundary failure"),
	}); err != nil {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	_, err = service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: failing.RecordingID,
	})
	if !errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("flush failure error = %v, want ErrRecordingFlushFailed", err)
	}
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("flush failure must remain distinct from ErrMissingRecordingTarget")
	}

	_, err = service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.RecordingID,
		Event:       interfaces.FactoryEvent{Id: "event-after-finish", Type: "WORK_STATE_CHANGE"},
	})
	if !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write error = %v, want ErrRecordingWriteRejected", err)
	}
	if errors.Is(err, recordings.ErrMissingRecordingTarget) || errors.Is(err, recordings.ErrRecordingFlushFailed) {
		t.Fatalf("post-finish write rejection must remain distinct from other lifecycle typed errors")
	}

	if _, err := service.StopRecording(recordings.StopRecordingRequest{
		RecordingID: bound.RecordingID,
	}); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
}
