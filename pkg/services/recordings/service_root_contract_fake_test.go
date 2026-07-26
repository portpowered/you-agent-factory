package recordings_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

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
