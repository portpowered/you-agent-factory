package recordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type peerRecordingSession struct {
	recordPath  string
	started     bool
	finished    bool
	stopped     bool
	flushFail   bool
	events      []interfaces.FactoryEvent
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
	replayByKey         map[string]*interfaces.ReplayArtifact
	replayCorruptKeys   map[string]bool
	replayBindingHooks  []recordings.ReplayHook
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

func (fake *peerRootServiceFake) replayKey(request recordings.LoadReplayArtifactRequest) string {
	if id := strings.TrimSpace(request.ArtifactID); id != "" {
		return id
	}
	return strings.TrimSpace(request.Path)
}

func (fake *peerRootServiceFake) LoadReplayArtifact(
	request recordings.LoadReplayArtifactRequest,
) (recordings.LoadReplayArtifactResult, error) {
	key := fake.replayKey(request)
	if key == "" {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrMissingReplayArtifact
	}
	if fake.replayCorruptKeys[key] {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrInvalidReplayArtifact
	}
	artifact, ok := fake.replayByKey[key]
	if !ok || artifact == nil {
		return recordings.LoadReplayArtifactResult{}, recordings.ErrInvalidReplayArtifact
	}
	detached := *artifact
	return recordings.LoadReplayArtifactResult{Artifact: &detached}, nil
}

func (fake *peerRootServiceFake) BindReplayExecution(
	request recordings.BindReplayExecutionRequest,
) (recordings.BindReplayExecutionResult, error) {
	if request.Artifact == nil {
		return recordings.BindReplayExecutionResult{}, recordings.ErrUnsupportedReplayBinding
	}
	if strings.TrimSpace(request.Artifact.SchemaVersion) == "" {
		return recordings.BindReplayExecutionResult{}, recordings.ErrUnsupportedReplayBinding
	}
	hooks := append([]recordings.ReplayHook{}, fake.replayBindingHooks...)
	return recordings.BindReplayExecutionResult{
		Provider:           nil,
		CommandRunner:      nil,
		Hooks:              hooks,
		CompletionDelivery: nil,
	}, nil
}

func peerArtifactDigestOK(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	hexPart := value[len(prefix):]
	if len(hexPart) != 64 {
		return false
	}
	for _, r := range hexPart {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (fake *peerRootServiceFake) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	facts := request.Facts
	if strings.TrimSpace(facts.SessionID) == "" {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidRecordingSummary
	}
	if !peerArtifactDigestOK(facts.SourceHash) || !peerArtifactDigestOK(facts.PolicyHash) {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
	}
	artifacts := make([]recordings.PortableRecordingArtifactSummary, 0, len(facts.Artifacts))
	for _, artifact := range facts.Artifacts {
		if !peerArtifactDigestOK(artifact.ContentHash) {
			return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
		}
		artifacts = append(artifacts, recordings.PortableRecordingArtifactSummary{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, CreatedAt: artifact.CreatedAt.UTC(),
		})
	}
	argumentsDigest := strings.TrimSpace(facts.ArgumentsDigest)
	if argumentsDigest == "" {
		argumentsDigest = facts.SourceHash
	} else if !peerArtifactDigestOK(argumentsDigest) {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
	}
	recording := recordings.PortableRecording{
		RecordingKind:              recordings.KindJavaScriptFactorySession,
		SchemaVersion:              "2",
		ReplayCompatibilityVersion: "1",
		ArgumentsDigest:            argumentsDigest,
		PolicyHash:                 facts.PolicyHash,
		Artifacts:                  artifacts,
	}
	recording.Session.ID = facts.SessionID
	recording.Session.Status = facts.Status
	recording.Session.OrchestratorKind = facts.OrchestratorKind
	recording.Source.Ref = facts.SourceRef
	recording.Source.Hash = facts.SourceHash
	if facts.Result != nil {
		recording.Result = &recordings.PortableRecordingResult{
			Status:        facts.Result.Status,
			Mode:          facts.Result.Mode,
			PrimaryResult: append(json.RawMessage{}, facts.Result.PrimaryResult...),
			ArtifactIDs:   append([]string{}, facts.Result.ArtifactIDs...),
			Failure:       facts.Result.Failure,
			Availability:  facts.Result.Availability,
		}
	}
	return recordings.BuildPortableArtifactResult{Recording: recording}, nil
}

func (fake *peerRootServiceFake) ValidatePortableArtifact(
	request recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	recording := request.Recording
	if recording.ArgumentsDigest != "" && !peerArtifactDigestOK(recording.ArgumentsDigest) {
		return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
	}
	if recording.Source.Hash != "" && !peerArtifactDigestOK(recording.Source.Hash) {
		return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
	}
	if recording.PolicyHash != "" && !peerArtifactDigestOK(recording.PolicyHash) {
		return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
	}
	for _, artifact := range recording.Artifacts {
		if artifact.ContentHash != "" && !peerArtifactDigestOK(artifact.ContentHash) {
			return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidRecordingDigest
		}
	}
	if strings.TrimSpace(recording.Session.ID) == "" {
		return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidRecordingSummary
	}
	return recordings.ValidatePortableArtifactResult{}, nil
}

func (fake *peerRootServiceFake) DecodePortableArtifact(
	request recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	if len(request.Payload) == 0 {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidRecordingDecode
	}
	var recording recordings.PortableRecording
	if err := json.Unmarshal(request.Payload, &recording); err != nil {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidRecordingDecode
	}
	if _, err := fake.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: recording,
	}); err != nil {
		return recordings.DecodePortableArtifactResult{}, err
	}
	return recordings.DecodePortableArtifactResult{Recording: recording}, nil
}

func (fake *peerRootServiceFake) SummarizePortableArtifact(
	request recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	recording := request.Recording
	if strings.TrimSpace(recording.Session.ID) == "" {
		return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidRecordingSummary
	}
	artifacts := append([]recordings.PortableRecordingArtifactSummary{}, recording.Artifacts...)
	result := recordings.SummarizePortableArtifactResult{
		SessionID: recording.Session.ID,
		Status:    recording.Session.Status,
		Artifacts: artifacts,
	}
	if recording.Result != nil {
		result.Availability = recording.Result.Availability
		result.Failure = recording.Result.Failure
	}
	return result, nil
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

func TestReplayRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	seed := &interfaces.ReplayArtifact{
		SchemaVersion: "factory-event-log/v1",
		RecordedAt:    time.Unix(1_700_000_000, 0).UTC(),
		Events: []interfaces.FactoryEvent{
			{Id: "event-1", Type: "WORK_REQUEST"},
		},
	}
	fake := &peerRootServiceFake{
		replayByKey: map[string]*interfaces.ReplayArtifact{
			"session.replay.json": seed,
			"artifact-42":         seed,
		},
		replayCorruptKeys: map[string]bool{
			"corrupt.replay.json": true,
		},
		replayBindingHooks: []recordings.ReplayHook{},
	}
	var service recordings.Service = fake

	loaded, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "session.replay.json",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact success path: %v", err)
	}
	if loaded.Artifact == nil {
		t.Fatal("LoadReplayArtifact Artifact nil, want detached replay artifact")
	}
	if loaded.Artifact.SchemaVersion != seed.SchemaVersion {
		t.Fatalf("LoadReplayArtifact SchemaVersion = %q, want %q", loaded.Artifact.SchemaVersion, seed.SchemaVersion)
	}
	if loaded.Artifact == seed {
		t.Fatal("LoadReplayArtifact must return a detached artifact, not the seeded pointer")
	}

	byID, err := service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		ArtifactID: "artifact-42",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact by id: %v", err)
	}
	if byID.Artifact == nil || byID.Artifact.SchemaVersion != seed.SchemaVersion {
		t.Fatalf("LoadReplayArtifact by id = %#v, want seeded schema", byID.Artifact)
	}

	bound, err := service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: loaded.Artifact,
	})
	if err != nil {
		t.Fatalf("BindReplayExecution success path: %v", err)
	}
	if bound.Hooks == nil {
		t.Fatal("BindReplayExecution Hooks nil, want published success shape with non-nil hooks slice")
	}

	_, err = service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{})
	if !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("missing artifact path/id error = %v, want ErrMissingReplayArtifact", err)
	}

	_, err = service.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "corrupt.replay.json",
	})
	if !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("corrupt artifact error = %v, want ErrInvalidReplayArtifact", err)
	}
	if errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("corrupt artifact must remain distinct from ErrMissingReplayArtifact")
	}

	_, err = service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: nil,
	})
	if !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("unsupported binding error = %v, want ErrUnsupportedReplayBinding", err)
	}
	if errors.Is(err, recordings.ErrMissingReplayArtifact) || errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("unsupported binding must remain distinct from load typed errors")
	}

	_, err = service.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &interfaces.ReplayArtifact{},
	})
	if !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("empty-schema binding error = %v, want ErrUnsupportedReplayBinding", err)
	}
}

func TestArtifactExportRootContract_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	fake := &peerRootServiceFake{}
	var service recordings.Service = fake
	createdAt := time.Unix(1_700_000_000, 0).UTC()
	digest := func(character byte) string {
		return "sha256:" + strings.Repeat(string(character), 64)
	}

	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		Facts: recordings.PortableRecordingCanonicalFacts{
			SessionID:        "session-export-1",
			Status:           "COMPLETED",
			OrchestratorKind: "JAVASCRIPT",
			SourceRef:        "workflow/export.js",
			SourceHash:       digest('1'),
			PolicyHash:       digest('2'),
			Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
				ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC",
				Label: "Result", ContentHash: digest('3'), SizeBytes: 21, CreatedAt: createdAt,
			}},
			Result: &recordings.PortableRecordingCanonicalResult{
				Status: "FINAL", Mode: "final",
				PrimaryResult: json.RawMessage(`{"answer":"ok"}`),
				ArtifactIDs:   []string{"artifact-result"},
				Availability: &recordings.PortableRecordingAvailability{
					Reason: "READY", Message: "available", Retryable: false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact success path: %v", err)
	}
	if built.Recording.Session.ID != "session-export-1" {
		t.Fatalf("BuildPortableArtifact session = %#v, want session-export-1", built.Recording.Session)
	}
	if len(built.Recording.Artifacts) != 1 || built.Recording.Artifacts[0].ID != "artifact-result" {
		t.Fatalf("BuildPortableArtifact artifacts = %#v, want detached canonical artifact result", built.Recording.Artifacts)
	}

	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: built.Recording,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact success path: %v", err)
	}

	summary, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Recording: built.Recording,
	})
	if err != nil {
		t.Fatalf("SummarizePortableArtifact success path: %v", err)
	}
	if summary.SessionID != "session-export-1" || summary.Status != "COMPLETED" {
		t.Fatalf("SummarizePortableArtifact = %#v, want session/status summary", summary)
	}
	if summary.Availability == nil || summary.Availability.Reason != "READY" {
		t.Fatalf("SummarizePortableArtifact availability = %#v, want READY", summary.Availability)
	}
	if len(summary.Artifacts) != 1 || summary.Artifacts[0].ID != "artifact-result" {
		t.Fatalf("SummarizePortableArtifact artifacts = %#v, want detached artifact summaries", summary.Artifacts)
	}

	encoded, err := json.Marshal(built.Recording)
	if err != nil {
		t.Fatalf("marshal portable recording: %v", err)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact success path: %v", err)
	}
	if decoded.Recording.Session.ID != built.Recording.Session.ID {
		t.Fatalf("DecodePortableArtifact = %#v, want detached recording", decoded.Recording.Session)
	}

	_, err = service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: recordings.PortableRecording{
			Session:         built.Recording.Session,
			Source:          built.Recording.Source,
			ArgumentsDigest: "not-a-digest",
			PolicyHash:      built.Recording.PolicyHash,
		},
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("invalid digest error = %v, want ErrInvalidRecordingDigest", err)
	}

	invalidSummary := built.Recording
	invalidSummary.Session.ID = ""
	_, err = service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Recording: invalidSummary,
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("invalid summary error = %v, want ErrInvalidRecordingSummary", err)
	}
	if errors.Is(err, recordings.ErrInvalidRecordingDigest) {
		t.Fatalf("invalid summary must remain distinct from ErrInvalidRecordingDigest")
	}

	_, err = service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	})
	if !errors.Is(err, recordings.ErrInvalidRecordingDecode) {
		t.Fatalf("decode failure error = %v, want ErrInvalidRecordingDecode", err)
	}
	if errors.Is(err, recordings.ErrInvalidRecordingDigest) || errors.Is(err, recordings.ErrInvalidRecordingSummary) {
		t.Fatalf("decode failure must remain distinct from digest/summary typed errors")
	}
}
