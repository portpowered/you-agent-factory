package recordings_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

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
