package service

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	dashboardprojections "github.com/portpowered/infinite-you/pkg/services/recordings/projections/dashboard"
)

type projectionService struct{}

func NewProjectionService() recordings.ProjectionService {
	return projectionService{}
}

func (projectionService) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (factorydefinitions.FactoryWorldState, error) {
	return projections.ReconstructCanonicalFactoryWorldState(events, selectedTick)
}

func (projectionService) SimpleDashboardRenderData(
	state factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return dashboardprojections.SimpleDashboardRenderDataFromWorldState(state)
}

func (projectionService) ProjectActiveThrottlePauses(
	topology factorydefinitions.InitialStructurePayload,
	pauses []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return projections.ProjectActiveThrottlePauses(topology, pauses)
}

func (projectionService) ProjectWorkstationRequests(
	state factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (projectionService) ValidateReconnectReplay(
	recorded []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	_, err := recordingevents.BuildCanonicalReconnectReplay(recorded, cursor, scope)
	return err
}

type lifecycleRecordingSession struct {
	recordPath  string
	started     bool
	finished    bool
	stopped     bool
	flushFail   bool
	events      []factorydefinitions.FactoryEvent
	retainedErr error
}

type combinedService struct {
	recordings.Ledger
	recordings.ProjectionService

	lifecycleMu     sync.Mutex
	lifecycleByID   map[string]*lifecycleRecordingSession
	nextRecordingID int
	replayByKey     map[string]*factorydefinitions.ReplayArtifact
}

var _ recordings.Service = (*combinedService)(nil)

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) recordings.AppendRecordedEventResult {
	service.AppendRecordedEvent(request.Event)
	return recordings.AppendRecordedEventResult{}
}

func (service *combinedService) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if request.Scope.SessionID != "" && strings.TrimSpace(request.Scope.SessionID) == "" {
		return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
	}
	stream, err := service.Subscribe(ctx, request.Cursor, request.Scope)
	if err != nil {
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{Stream: stream}, nil
}

func (service *combinedService) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	state, err := service.ReconstructFactoryWorldState(request.Events, request.SelectedTick)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	return recordings.ReconstructWorldStateResult{WorldState: state}, nil
}

func (service *combinedService) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) recordings.SimpleDashboardQueryResult {
	return recordings.SimpleDashboardQueryResult{
		Data: service.SimpleDashboardRenderData(request.WorldState),
	}
}

func (service *combinedService) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) recordings.WorkstationRequestsQueryResult {
	return recordings.WorkstationRequestsQueryResult{
		Projection: service.ProjectWorkstationRequests(request.WorldState),
	}
}

func (service *combinedService) ValidateReconnectReplayFrom(
	request recordings.ValidateReconnectReplayRequest,
) error {
	return service.ValidateReconnectReplay(request.Events, request.Cursor, request.Scope)
}

func (service *combinedService) BindRecording(
	request recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	if strings.TrimSpace(request.RecordPath) == "" {
		return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if service.lifecycleByID == nil {
		service.lifecycleByID = make(map[string]*lifecycleRecordingSession)
	}
	id := strings.TrimSpace(request.RecordingID)
	if id == "" {
		service.nextRecordingID++
		id = "recording-" + strconv.Itoa(service.nextRecordingID)
	}
	service.lifecycleByID[id] = &lifecycleRecordingSession{recordPath: request.RecordPath}
	return recordings.BindRecordingResult{RecordingID: id}, nil
}

func (service *combinedService) lifecycleSession(id string) (*lifecycleRecordingSession, error) {
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if strings.TrimSpace(id) == "" || service.lifecycleByID == nil {
		return nil, recordings.ErrMissingRecordingTarget
	}
	session, ok := service.lifecycleByID[id]
	if !ok {
		return nil, recordings.ErrMissingRecordingTarget
	}
	return session, nil
}

func (service *combinedService) StartRecording(
	_ context.Context,
	request recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.StartRecordingResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	session.started = true
	session.stopped = false
	return recordings.StartRecordingResult{}, nil
}

func (service *combinedService) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingEventResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if session.finished {
		return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
	}
	session.events = append(session.events, request.Event)
	return recordings.RecordRecordingEventResult{}, nil
}

func (service *combinedService) RecordRecordingError(
	request recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingErrorResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if session.finished {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrRecordingWriteRejected
	}
	if request.Err != nil {
		session.retainedErr = request.Err
		session.flushFail = true
	}
	return recordings.RecordRecordingErrorResult{}, nil
}

func (service *combinedService) FlushRecording(
	request recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.FlushRecordingResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	if session.flushFail {
		if session.retainedErr == nil {
			session.retainedErr = recordings.ErrRecordingFlushFailed
		}
		return recordings.FlushRecordingResult{}, recordings.ErrRecordingFlushFailed
	}
	return recordings.FlushRecordingResult{}, nil
}

func (service *combinedService) FinishRecording(
	request recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.FinishRecordingResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	_ = request.FinishedAt
	session.finished = true
	return recordings.FinishRecordingResult{}, nil
}

func (service *combinedService) StopRecording(
	request recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.StopRecordingResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	session.stopped = true
	session.started = false
	return recordings.StopRecordingResult{}, nil
}

func (service *combinedService) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	session, err := service.lifecycleSession(request.RecordingID)
	if err != nil {
		return recordings.RecordingStatusResult{}, err
	}
	service.lifecycleMu.Lock()
	defer service.lifecycleMu.Unlock()
	return recordings.RecordingStatusResult{
		Started:  session.started,
		Finished: session.finished,
		Stopped:  session.stopped,
		Err:      session.retainedErr,
	}, nil
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

func mapPortableArtifactError(err error, decodePath bool) error {
	if err == nil {
		return nil
	}
	var diagnostic *recordings.PortableRecordingDiagnostic
	if errors.As(err, &diagnostic) && diagnostic != nil {
		switch diagnostic.Code {
		case recordings.PortableRecordingCodeInvalidDigest:
			return recordings.ErrInvalidRecordingDigest
		case recordings.PortableRecordingCodeInvalidSummary:
			return recordings.ErrInvalidRecordingSummary
		case "MALFORMED_RECORDING_CONTRACT":
			if decodePath {
				return recordings.ErrInvalidRecordingDecode
			}
			return recordings.ErrInvalidRecordingSummary
		default:
			// Identity/version and other validate failures surface as summary
			// typed outcomes on the published artifact-export slice.
			return recordings.ErrInvalidRecordingSummary
		}
	}
	if decodePath {
		return recordings.ErrInvalidRecordingDecode
	}
	return err
}

func (service *combinedService) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	recording, err := recordings.BuildPortableRecording(request.Facts)
	if err != nil {
		return recordings.BuildPortableArtifactResult{}, mapPortableArtifactError(err, false)
	}
	return recordings.BuildPortableArtifactResult{Recording: recording}, nil
}

func (service *combinedService) ValidatePortableArtifact(
	request recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	if err := recordings.ValidatePortableRecording(request.Recording); err != nil {
		return recordings.ValidatePortableArtifactResult{}, mapPortableArtifactError(err, false)
	}
	return recordings.ValidatePortableArtifactResult{}, nil
}

func (service *combinedService) DecodePortableArtifact(
	request recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	if len(request.Payload) == 0 {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidRecordingDecode
	}
	recording, err := recordings.DecodePortableRecording(bytes.NewReader(request.Payload))
	if err != nil {
		return recordings.DecodePortableArtifactResult{}, mapPortableArtifactError(err, true)
	}
	return recordings.DecodePortableArtifactResult{Recording: recording}, nil
}

func (service *combinedService) SummarizePortableArtifact(
	request recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	recording := request.Recording
	if strings.TrimSpace(recording.Session.ID) == "" {
		return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidRecordingSummary
	}
	result := recordings.SummarizePortableArtifactResult{
		SessionID: recording.Session.ID,
		Status:    recording.Session.Status,
		Artifacts: append([]recordings.PortableRecordingArtifactSummary{}, recording.Artifacts...),
	}
	if recording.Result != nil {
		result.Availability = recording.Result.Availability
		result.Failure = recording.Result.Failure
	}
	return result, nil
}

func NewService(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
) recordings.Service {
	if ledger == nil || projection == nil {
		return nil
	}
	return &combinedService{
		Ledger:            ledger,
		ProjectionService: projection,
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
