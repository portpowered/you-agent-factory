package factory_visualization_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/recordingsqueries"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type recordingsRootBoundaryFixture struct {
	events    []factorydefinitions.FactoryEvent
	sessionID string
	scope     factorydefinitions.FactoryEventReconnectScope
}

func newRecordingsRootBoundaryFixture() recordingsRootBoundaryFixture {
	sessionID := "session-visualization-boundary"
	return recordingsRootBoundaryFixture{
		events: []factorydefinitions.FactoryEvent{
			{
				Id:   "evt-history",
				Type: "WORK_REQUEST",
				Context: factorydefinitions.FactoryEventContext{
					Sequence: 3,
					Tick:     3,
				},
			},
			{
				Id:   "evt-live",
				Type: "WORK_STATE_CHANGE",
				Context: factorydefinitions.FactoryEventContext{
					Sequence: 4,
					Tick:     4,
				},
			},
		},
		sessionID: sessionID,
		scope:     factorydefinitions.FactoryEventReconnectScope{SessionID: sessionID},
	}
}

func newRecordingsRequestBoundaryStub() *recordingsRequestBoundaryStub {
	return &recordingsRequestBoundaryStub{
		reconstructResult: recordings.ReconstructWorldStateResult{
			WorldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				SelectedTick:  4,
				Payload:       `{"topology":{"workstations":[{"id":"ws-1","name":"review","worker_id":"worker-1"}],"workers":[{"id":"worker-1","model":"gpt","provider":"openai"}],"places":[]}}`,
			},
		},
		dashboardResult: recordings.SimpleDashboardQueryResult{
			Data: recordings.SimpleDashboardRenderData{InFlightDispatchCount: 2},
		},
	}
}

// TestVisualizationConstructsRecordingsRequestsThroughRoot proves CUT-VIS-REC story 003:
// Factory Visualization projection-query edges construct Recordings root requests
// and invoke recordings.Service with observable acceptance or typed rejection outcomes.
func TestVisualizationConstructsRecordingsRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	fixture := newRecordingsRootBoundaryFixture()
	stub := newRecordingsRequestBoundaryStub()
	worldView := runRecordingsReconstructThroughRootProof(t, stub, fixture)
	runRecordingsDashboardThroughRootProof(t, stub, worldView)
	runRecordingsValidateThroughRootProof(t, stub, fixture)
	runRecordingsNilServiceRejectionProof(t, fixture, worldView)
}

func runRecordingsReconstructThroughRootProof(
	t *testing.T,
	stub *recordingsRequestBoundaryStub,
	fixture recordingsRootBoundaryFixture,
) recordings.WorldStateView {
	t.Helper()

	worldView, err := recordingsqueries.ReconstructWorldState(stub, fixture.events, 4)
	if err != nil {
		t.Fatalf("ReconstructWorldState: %v", err)
	}
	if stub.lastReconstruct.SelectedTick != 4 {
		t.Fatalf("reconstruct selected tick = %d, want 4", stub.lastReconstruct.SelectedTick)
	}
	if len(stub.lastReconstruct.Events) != 2 {
		t.Fatalf("reconstruct events = %d, want 2", len(stub.lastReconstruct.Events))
	}
	if stub.lastReconstruct.Events[0].ID != "evt-history" ||
		stub.lastReconstruct.Events[0].Sequence != 3 ||
		stub.lastReconstruct.Events[0].Cursor.StreamGenerationID != "factory-visualization" {
		t.Fatalf("reconstruct first event = %#v, want canonical visualization stream", stub.lastReconstruct.Events[0])
	}
	if worldView.SelectedTick != 4 || worldView.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("world view = %#v, want selected tick 4 and schema v1", worldView)
	}
	return worldView
}

func runRecordingsDashboardThroughRootProof(
	t *testing.T,
	stub *recordingsRequestBoundaryStub,
	worldView recordings.WorldStateView,
) {
	t.Helper()

	renderData, err := recordingsqueries.QuerySimpleDashboard(stub, worldView)
	if err != nil {
		t.Fatalf("QuerySimpleDashboard: %v", err)
	}
	if stub.lastDashboard.WorldState.SelectedTick != 4 {
		t.Fatalf("dashboard world-state tick = %d, want 4", stub.lastDashboard.WorldState.SelectedTick)
	}
	if renderData.InFlightDispatchCount != 2 {
		t.Fatalf("dashboard in-flight count = %d, want 2", renderData.InFlightDispatchCount)
	}

	worldState, err := recordingsqueries.DecodeWorldStatePayload(worldView)
	if err != nil {
		t.Fatalf("DecodeWorldStatePayload: %v", err)
	}
	renderData.ActiveThrottlePauses = recordingsqueries.ProjectActiveThrottlePauses(
		worldState.Topology,
		[]factorydefinitions.ActiveThrottlePause{
			{Provider: "openai", Model: "gpt", LaneID: "lane-1"},
		},
	)
	if len(renderData.ActiveThrottlePauses) != 1 ||
		renderData.ActiveThrottlePauses[0].AffectedWorkstationNames[0] != "review" {
		t.Fatalf("active throttle pauses = %#v, want review workstation projection", renderData.ActiveThrottlePauses)
	}
}

func runRecordingsValidateThroughRootProof(
	t *testing.T,
	stub *recordingsRequestBoundaryStub,
	fixture recordingsRootBoundaryFixture,
) {
	t.Helper()

	afterSequence := 3
	if err := recordingsqueries.ValidateReconnectReplay(
		stub,
		fixture.events,
		factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &afterSequence},
		fixture.scope,
	); err != nil {
		t.Fatalf("ValidateReconnectReplay success path: %v", err)
	}
	if stub.lastValidate.Scope.FactorySessionID != fixture.sessionID {
		t.Fatalf("validate scope session = %q, want %q", stub.lastValidate.Scope.FactorySessionID, fixture.sessionID)
	}
	if stub.lastValidate.Cursor.StreamGenerationID != "factory-visualization" {
		t.Fatalf("validate cursor stream = %q, want factory-visualization", stub.lastValidate.Cursor.StreamGenerationID)
	}

	stub.validateErr = recordings.ErrReconnectCursorNotFound
	err := recordingsqueries.ValidateReconnectReplay(
		stub,
		fixture.events,
		factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "missing-event"},
		fixture.scope,
	)
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("missing cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func runRecordingsNilServiceRejectionProof(
	t *testing.T,
	fixture recordingsRootBoundaryFixture,
	worldView recordings.WorldStateView,
) {
	t.Helper()

	_, err := recordingsqueries.ReconstructWorldState(nil, fixture.events, 4)
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("nil service reconstruct error = %v, want ErrInvalidProjectionInput", err)
	}
	_, err = recordingsqueries.QuerySimpleDashboard(nil, worldView)
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("nil service dashboard error = %v, want ErrInvalidProjectionInput", err)
	}
	err = recordingsqueries.ValidateReconnectReplay(nil, fixture.events, factorydefinitions.FactoryEventReconnectCursor{}, fixture.scope)
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("nil service validate error = %v, want ErrInvalidProjectionInput", err)
	}
}

// TestVisualizationRecordingsRequestConstructionImportsRecordingsRootOnly seals the
// request-construction path: Visualization projection-query helpers may depend on
// Recordings only through the service root contract.

type recordingsRequestBoundaryStub struct {
	lastReconstruct recordings.ReconstructWorldStateRequest
	lastDashboard   recordings.SimpleDashboardQueryRequest
	lastValidate    recordings.ValidateReconnectReplayRequest

	reconstructResult recordings.ReconstructWorldStateResult
	dashboardResult   recordings.SimpleDashboardQueryResult
	validateErr       error
}

var _ recordings.Service = (*recordingsRequestBoundaryStub)(nil)

func (stub *recordingsRequestBoundaryStub) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	stub.lastReconstruct = request
	return stub.reconstructResult, nil
}

func (stub *recordingsRequestBoundaryStub) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	stub.lastDashboard = request
	return stub.dashboardResult, nil
}

func (stub *recordingsRequestBoundaryStub) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	stub.lastValidate = request
	return stub.validateErr
}

func (stub *recordingsRequestBoundaryStub) Append(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (stub *recordingsRequestBoundaryStub) SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
	return recordings.SubscribeResult{}, recordings.ErrReconnectCursorNotFound
}

func (stub *recordingsRequestBoundaryStub) QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, nil
}

func (stub *recordingsRequestBoundaryStub) BindRecording(recordings.BindRecordingRequest) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) StartRecording(recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, nil
}

func (stub *recordingsRequestBoundaryStub) RecordRecordingEvent(recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (stub *recordingsRequestBoundaryStub) RecordRecordingError(recordings.RecordRecordingErrorRequest) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (stub *recordingsRequestBoundaryStub) FlushRecording(recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) StopRecording(recordings.StopRecordingRequest) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) FinishRecording(recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) LoadReplayRecording(recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (stub *recordingsRequestBoundaryStub) CreateReplayPlan(recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *recordingsRequestBoundaryStub) ObserveReplay(recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (stub *recordingsRequestBoundaryStub) BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) EncodePortableArtifact(recordings.EncodePortableArtifactRequest) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) DecodePortableArtifact(recordings.DecodePortableArtifactRequest) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) ExportPortableArtifact(context.Context, recordings.ExportPortableArtifactRequest) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (stub *recordingsRequestBoundaryStub) ReadPortableArtifact(context.Context, recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}
