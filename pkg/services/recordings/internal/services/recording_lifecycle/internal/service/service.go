package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type recordingSession struct {
	artifact       recordings.RecordingArtifactReference
	serviceTarget  string
	selection      recordings.RecordingTargetRequest
	scope          recordings.CanonicalEventScope
	events         []recordings.CanonicalEvent
	flushedThrough *recordings.CanonicalEventCursor
	failures       []recordings.RecordingFailure
	finalizedAt    *time.Time
}

// Service serializes target binding and lifecycle state for Recordings.
type Service struct {
	mu              sync.Mutex
	targets         recordings.LiveRecordingTargetPlanner
	byID            map[string]*recordingSession
	nextRecordingID int
}

var _ recordinglifecycle.Service = (*Service)(nil)

// New constructs one private recording-lifecycle owner.
func New(targets recordings.LiveRecordingTargetPlanner) *Service {
	return &Service{
		targets: targets,
		byID:    make(map[string]*recordingSession),
	}
}

func (service *Service) StartRecording(
	request recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	if !request.Enabled {
		return recordings.StartRecordingResult{}, nil
	}
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.StartRecordingResult{}, recordings.ErrInvalidRecordingScope
	}
	if id := strings.TrimSpace(string(request.RecordingID)); id != "" {
		service.mu.Lock()
		existing, exists := service.byID[id]
		if exists && existing.scope == request.Scope && existing.selection == request.Target {
			status := recordingStatus(request.RecordingID, existing)
			service.mu.Unlock()
			return recordings.StartRecordingResult{Enabled: true, Status: status}, nil
		}
		service.mu.Unlock()
		if exists {
			return recordings.StartRecordingResult{}, recordings.ErrRecordingBindingConflict
		}
	}
	artifact := request.Target.Artifact
	serviceTarget := strings.TrimSpace(string(artifact))
	if serviceTarget == "" {
		if strings.TrimSpace(request.Target.HomeDir) == "" {
			return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
		}
		if service == nil || service.targets == nil {
			return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
		}
		target, err := service.targets.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
			HomeDir:           request.Target.HomeDir,
			ReportedSessionID: request.Target.ReportedSessionID,
		})
		if err != nil {
			return recordings.StartRecordingResult{}, err
		}
		serviceTarget = strings.TrimSpace(target.ServicePath)
		artifact = recordings.RecordingArtifactReference(strings.TrimSpace(target.ReportedPath))
		if serviceTarget == "" || strings.TrimSpace(string(artifact)) == "" {
			return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
		}
	}
	bound, err := service.bind(recordings.BindRecordingRequest{
		RecordingID: request.RecordingID,
		Artifact:    artifact,
		Scope:       request.Scope,
	}, serviceTarget, request.Target)
	if err != nil {
		return recordings.StartRecordingResult{}, err
	}
	return recordings.StartRecordingResult{Enabled: true, Status: bound.Status}, nil
}

func (service *Service) BindRecording(
	request recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	return service.bind(
		request,
		strings.TrimSpace(string(request.Artifact)),
		recordings.RecordingTargetRequest{Artifact: request.Artifact},
	)
}

func (service *Service) bind(
	request recordings.BindRecordingRequest,
	serviceTarget string,
	selection recordings.RecordingTargetRequest,
) (recordings.BindRecordingResult, error) {
	if strings.TrimSpace(string(request.Artifact)) == "" || serviceTarget == "" {
		return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
	}
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.BindRecordingResult{}, recordings.ErrInvalidRecordingScope
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	id := strings.TrimSpace(string(request.RecordingID))
	if id == "" {
		id = service.nextIDLocked()
	} else if existing, exists := service.byID[id]; exists {
		if existing.artifact != request.Artifact ||
			existing.serviceTarget != serviceTarget ||
			existing.scope != request.Scope {
			return recordings.BindRecordingResult{}, recordings.ErrRecordingBindingConflict
		}
		return recordings.BindRecordingResult{
			Status: recordingStatus(recordings.RecordingID(id), existing),
		}, nil
	}
	session := &recordingSession{
		artifact:      request.Artifact,
		serviceTarget: serviceTarget,
		selection:     selection,
		scope:         request.Scope,
	}
	service.byID[id] = session
	return recordings.BindRecordingResult{
		Status: recordingStatus(recordings.RecordingID(id), session),
	}, nil
}

func (service *Service) nextIDLocked() string {
	for {
		service.nextRecordingID++
		id := "recording-" + strconv.Itoa(service.nextRecordingID)
		if _, exists := service.byID[id]; !exists {
			return id
		}
	}
}

func (service *Service) sessionLocked(
	id recordings.RecordingID,
) (*recordingSession, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, recordings.ErrMissingRecordingTarget
	}
	session, ok := service.byID[string(id)]
	if !ok {
		return nil, recordings.ErrMissingRecordingTarget
	}
	return session, nil
}

func (service *Service) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, err := service.sessionLocked(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingEventResult{}, err
	}
	if session.finalizedAt != nil {
		return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
	}
	if !validRecordingEvent(session, request.Event) {
		return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
	}
	session.events = append(session.events, request.Event)
	return recordings.RecordRecordingEventResult{
		Status: recordingStatus(request.RecordingID, session),
	}, nil
}

func (service *Service) RecordRecordingError(
	request recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, err := service.sessionLocked(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingErrorResult{}, err
	}
	if session.finalizedAt != nil {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrRecordingWriteRejected
	}
	if strings.TrimSpace(request.Failure.Code) == "" ||
		strings.TrimSpace(request.Failure.Message) == "" {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
	}
	session.failures = append(session.failures, request.Failure)
	return recordings.RecordRecordingErrorResult{
		Status: recordingStatus(request.RecordingID, session),
	}, nil
}

func (service *Service) FlushRecording(
	request recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, err := service.sessionLocked(request.RecordingID)
	if err != nil {
		return recordings.FlushRecordingResult{}, err
	}
	if len(session.events) > 0 {
		cursor := session.events[len(session.events)-1].Cursor
		session.flushedThrough = &cursor
	}
	return recordings.FlushRecordingResult{
		Status: recordingStatus(request.RecordingID, session),
	}, nil
}

func (service *Service) FinishRecording(
	request recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, err := service.sessionLocked(request.RecordingID)
	if err != nil {
		return recordings.FinishRecordingResult{}, err
	}
	if session.finalizedAt == nil {
		finishedAt := request.FinishedAt.UTC()
		session.finalizedAt = &finishedAt
	}
	return recordings.FinishRecordingResult{
		Status: recordingStatus(request.RecordingID, session),
	}, nil
}

func (service *Service) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	snapshot, err := service.Snapshot(request.RecordingID)
	if err != nil {
		return recordings.RecordingStatusResult{}, err
	}
	return recordings.RecordingStatusResult{Status: snapshot.Status}, nil
}

func (service *Service) Snapshot(
	id recordings.RecordingID,
) (recordinglifecycle.Snapshot, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, err := service.sessionLocked(id)
	if err != nil {
		return recordinglifecycle.Snapshot{}, err
	}
	return recordinglifecycle.Snapshot{
		Status: recordingStatus(id, session),
		Events: append([]recordings.CanonicalEvent(nil), session.events...),
	}, nil
}

func validRecordingEvent(session *recordingSession, event recordings.CanonicalEvent) bool {
	if strings.TrimSpace(string(event.ID)) == "" ||
		strings.TrimSpace(string(event.Kind)) == "" ||
		event.RecordedAt.IsZero() ||
		!json.Valid([]byte(event.Payload)) ||
		event.Scope != session.scope ||
		event.Sequence < 0 ||
		event.Cursor.StreamGenerationID == "" ||
		event.Cursor.Sequence != event.Sequence {
		return false
	}
	if len(session.events) == 0 {
		return true
	}
	previous := session.events[len(session.events)-1]
	return event.Cursor.StreamGenerationID == previous.Cursor.StreamGenerationID &&
		event.Sequence > previous.Sequence
}

func recordingStatus(
	id recordings.RecordingID,
	session *recordingSession,
) recordings.RecordingStatusFacts {
	state := recordings.RecordingActive
	if len(session.failures) > 0 {
		state = recordings.RecordingFailed
	} else if session.finalizedAt != nil {
		state = recordings.RecordingFinalized
	}
	status := recordings.RecordingStatusFacts{
		RecordingID:    id,
		Artifact:       session.artifact,
		Scope:          session.scope,
		State:          state,
		AcceptedEvents: len(session.events),
		Failures:       append([]recordings.RecordingFailure(nil), session.failures...),
	}
	if len(session.events) > 0 {
		cursor := session.events[len(session.events)-1].Cursor
		status.LastEvent = &cursor
	}
	if session.flushedThrough != nil {
		cursor := *session.flushedThrough
		status.FlushedThrough = &cursor
	}
	if session.finalizedAt != nil {
		finalizedAt := *session.finalizedAt
		status.FinalizedAt = &finalizedAt
	}
	return status
}
