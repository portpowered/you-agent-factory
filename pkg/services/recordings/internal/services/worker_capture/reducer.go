// Package worker_capture contains the pure source-native Worker recording
// reducer. It has no Recordings service dependency so the public Recordings
// seam can re-export the value contract without creating an ownership cycle.
package worker_capture

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerRecordingSnapshot is the durable sidecar shape used by the default
// local writer. The schema is source-native so replay loads exact Events
// records rather than reconstructing them from a provider projection.
type WorkerRecordingSnapshot struct {
	RecordingID string                           `json:"recordingId"`
	Sessions    []WorkerSessionRecordingSnapshot `json:"sessions"`
}

// WorkerSessionRecordingSnapshot contains one detached Worker topic history
// in aggregate order.
type WorkerSessionRecordingSnapshot struct {
	WorkerSessionID string                   `json:"workerSessionId"`
	Topic           events.Topic             `json:"topic,omitempty"`
	Status          WorkerRecordingStatus    `json:"status,omitempty"`
	LastPosition    events.AggregateSequence `json:"lastPosition,omitempty"`
	Failure         string                   `json:"failure,omitempty"`
	Records         []events.Record          `json:"records"`
}

// WorkerRecordingStatus is the durable capture state derived from the
// accepted Worker history.
type WorkerRecordingStatus string

const (
	WorkerRecordingStatusActive    WorkerRecordingStatus = "ACTIVE"
	WorkerRecordingStatusCompleted WorkerRecordingStatus = "COMPLETED"
	WorkerRecordingStatusFailed    WorkerRecordingStatus = "FAILED"
)

// WorkerRecordingHistory is the canonical, source-native input to the pure
// Worker recording reducer.
type WorkerRecordingHistory struct {
	RecordingID     string
	WorkerSessionID string
	Topic           events.Topic
	Records         []events.Record
}

// WorkerRecordingTerminal is the detached terminal lifecycle fact derived by
// the reducer. The complete source record remains in Projection.Records.
type WorkerRecordingTerminal struct {
	Position events.AggregateSequence
	Phase    workers.Phase
	Status   string
}

// WorkerRecordingProjection is the deterministic observable result of
// reducing one Worker history. Active projections are valid live prefixes;
// ReplayWorkerRecording only returns a projection after a legal terminal.
type WorkerRecordingProjection struct {
	RecordingID     string
	WorkerSessionID string
	Topic           events.Topic
	Status          WorkerRecordingStatus
	Complete        bool
	LastPosition    events.AggregateSequence
	Opening         events.Record
	Terminal        *WorkerRecordingTerminal
	Records         []events.Record
}

// WorkerRecordingReplayRequest selects one Worker Session history from a
// durable source-native snapshot.
type WorkerRecordingReplayRequest struct {
	Snapshot        WorkerRecordingSnapshot
	WorkerSessionID string
}

// WorkerRecordingReplayResult contains the same projection shape produced by
// live reduction and no execution or provider handles.
type WorkerRecordingReplayResult struct {
	Projection WorkerRecordingProjection
}

var (
	ErrInvalidWorkerRecordingRequest = errors.New("recordings: invalid Worker recording request")
	ErrWorkerRecordingOpening        = errors.New("recordings: Worker recording opening is invalid")
	ErrWorkerRecordingDelivery       = errors.New("recordings: Worker recording delivery failed")
	ErrWorkerRecordingOrder          = errors.New("recordings: Worker recording order is invalid")
	ErrWorkerRecordingDuplicate      = errors.New("recordings: Worker recording duplicate conflicts")
	ErrWorkerRecordingTerminal       = errors.New("recordings: Worker recording terminal lifecycle is invalid")
	ErrWorkerRecordingIncomplete     = errors.New("recordings: Worker recording is incomplete")
	ErrWorkerRecordingReplay         = errors.New("recordings: Worker recording replay failed")
)

const (
	workerLifecycleSourceType = events.SourceType("worker_session_lifecycle")
	workerDraftSchemaID       = events.SchemaID("workers.draft.v1")
	openingSourceSequence     = events.SourceSequence(1)
	openingSourceEventID      = events.SourceEventID("started")
	terminalSourceSequence    = events.SourceSequence(2)
	terminalSourceEventID     = events.SourceEventID("terminal")
)

// ReduceWorkerRecording is the single deterministic reducer used by live
// capture and completed replay. It performs no I/O or provider lookup.
func ReduceWorkerRecording(history WorkerRecordingHistory) (WorkerRecordingProjection, error) {
	if strings.TrimSpace(history.WorkerSessionID) == "" {
		return WorkerRecordingProjection{}, fmt.Errorf("%w: Worker Session ID is required", ErrInvalidWorkerRecordingRequest)
	}
	topic := history.Topic
	if topic == "" {
		topic = canonicalWorkerTopic(history.WorkerSessionID)
	}
	if err := validateWorkerTopic(topic, history.WorkerSessionID); err != nil {
		return WorkerRecordingProjection{}, err
	}
	if len(history.Records) == 0 {
		return WorkerRecordingProjection{}, fmt.Errorf("%w: opening record is missing", ErrWorkerRecordingIncomplete)
	}

	projection := WorkerRecordingProjection{
		RecordingID:     history.RecordingID,
		WorkerSessionID: history.WorkerSessionID,
		Topic:           topic,
		Status:          WorkerRecordingStatusActive,
		Records:         make([]events.Record, 0, len(history.Records)),
	}
	identities := make(map[events.AppendIdentity]struct{}, len(history.Records))
	for index, record := range history.Records {
		if err := validateWorkerRecord(record, topic, identities, index == 0, history.WorkerSessionID); err != nil {
			return WorkerRecordingProjection{}, err
		}
		identities[record.Identity()] = struct{}{}
		draft, err := decodeWorkerDraft(record)
		if err != nil {
			if index == 0 {
				return WorkerRecordingProjection{}, fmt.Errorf("%w: %w", ErrWorkerRecordingOpening, err)
			}
			return WorkerRecordingProjection{}, err
		}
		if index == 0 {
			if err := validateWorkerOpening(record, draft, history.WorkerSessionID); err != nil {
				return WorkerRecordingProjection{}, err
			}
			projection.Opening = record.Detached()
		} else if draft.Kind == workers.KindSession && draft.Phase == workers.PhaseStarted {
			return WorkerRecordingProjection{}, fmt.Errorf("%w: duplicate SESSION/STARTED record at position %d", ErrWorkerRecordingOpening, record.ID.Position)
		}

		if draft.Kind == workers.KindSession && isWorkerTerminalPhase(draft.Phase) {
			if err := validateWorkerTerminal(record, history.WorkerSessionID); err != nil {
				return WorkerRecordingProjection{}, err
			}
			if projection.Terminal != nil {
				return WorkerRecordingProjection{}, fmt.Errorf("%w: multiple terminal records", ErrWorkerRecordingTerminal)
			}
			if index != len(history.Records)-1 {
				return WorkerRecordingProjection{}, fmt.Errorf("%w: record follows terminal at position %d", ErrWorkerRecordingTerminal, record.ID.Position)
			}
			status, err := workerTerminalStatus(draft)
			if err != nil {
				return WorkerRecordingProjection{}, err
			}
			projection.Terminal = &WorkerRecordingTerminal{
				Position: record.ID.Position,
				Phase:    draft.Phase,
				Status:   status,
			}
		}

		projection.Records = append(projection.Records, record.Detached())
		projection.LastPosition = record.ID.Position
	}
	if projection.Terminal != nil {
		projection.Status = WorkerRecordingStatusCompleted
		projection.Complete = true
	}
	return projection, nil
}

// ReplayWorkerRecording reduces one durable snapshot and rejects an active
// or failed prefix so callers cannot mistake incomplete capture for replay.
func ReplayWorkerRecording(request WorkerRecordingReplayRequest) (WorkerRecordingReplayResult, error) {
	if strings.TrimSpace(request.Snapshot.RecordingID) == "" {
		return WorkerRecordingReplayResult{}, fmt.Errorf("%w: recording identity is required", ErrWorkerRecordingReplay)
	}
	if len(request.Snapshot.Sessions) == 0 {
		return WorkerRecordingReplayResult{}, fmt.Errorf("%w: Worker Session history is missing", ErrWorkerRecordingReplay)
	}
	sessionID := strings.TrimSpace(request.WorkerSessionID)
	if sessionID == "" {
		if len(request.Snapshot.Sessions) != 1 {
			return WorkerRecordingReplayResult{}, fmt.Errorf("%w: Worker Session ID is required for a multi-session snapshot", ErrWorkerRecordingReplay)
		}
		sessionID = request.Snapshot.Sessions[0].WorkerSessionID
	}
	for _, session := range request.Snapshot.Sessions {
		if session.WorkerSessionID != sessionID {
			continue
		}
		if session.Status == WorkerRecordingStatusFailed {
			return WorkerRecordingReplayResult{}, fmt.Errorf("%w: durable capture failed", ErrWorkerRecordingIncomplete)
		}
		projection, err := ReduceWorkerRecording(WorkerRecordingHistory{
			RecordingID:     request.Snapshot.RecordingID,
			WorkerSessionID: session.WorkerSessionID,
			Topic:           session.Topic,
			Records:         session.Records,
		})
		if err != nil {
			return WorkerRecordingReplayResult{}, fmt.Errorf("%w: %w", ErrWorkerRecordingReplay, err)
		}
		if !projection.Complete {
			return WorkerRecordingReplayResult{}, fmt.Errorf("%w: no durable terminal record", ErrWorkerRecordingIncomplete)
		}
		return WorkerRecordingReplayResult{Projection: projection}, nil
	}
	return WorkerRecordingReplayResult{}, fmt.Errorf("%w: Worker Session %q was not found", ErrWorkerRecordingReplay, sessionID)
}

func canonicalWorkerTopic(sessionID string) events.Topic {
	return events.Topic("worker-session/" + strings.TrimSpace(sessionID) + "/events")
}

func validateWorkerTopic(topic events.Topic, sessionID string) error {
	if err := topic.Validate(); err != nil {
		return fmt.Errorf("%w: topic: %w", ErrWorkerRecordingReplay, err)
	}
	if topic != canonicalWorkerTopic(sessionID) {
		return fmt.Errorf("%w: topic %q is not the canonical Worker Session topic", ErrWorkerRecordingOrder, topic)
	}
	return nil
}

func validateWorkerRecord(
	record events.Record,
	topic events.Topic,
	identities map[events.AppendIdentity]struct{},
	opening bool,
	sessionID string,
) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("%w: malformed record: %w", ErrWorkerRecordingDelivery, err)
	}
	if opening && (record.ID.Topic != topic || record.ID.Position != 1 ||
		record.SourceType != workerLifecycleSourceType ||
		record.SourceID != events.SourceID(sessionID) ||
		record.SourceSequence != openingSourceSequence ||
		record.SourceEventID != openingSourceEventID ||
		record.SchemaID != workerDraftSchemaID) {
		return fmt.Errorf("%w: expected correlated position-1 SESSION/STARTED for Worker Session %q", ErrWorkerRecordingOpening, sessionID)
	}
	if record.ID.Topic != topic {
		return fmt.Errorf("%w: record topic %q does not match %q", ErrWorkerRecordingOrder, record.ID.Topic, topic)
	}
	if _, exists := identities[record.Identity()]; exists {
		return fmt.Errorf("%w: source identity %q/%q/%d/%q repeated", ErrWorkerRecordingDuplicate, record.SourceType, record.SourceID, record.SourceSequence, record.SourceEventID)
	}
	wantPosition := events.AggregateSequence(len(identities) + 1)
	if record.ID.Position != wantPosition {
		return fmt.Errorf("%w: expected aggregate position %d, got %d", ErrWorkerRecordingOrder, wantPosition, record.ID.Position)
	}
	return nil
}

func decodeWorkerDraft(record events.Record) (workers.Draft, error) {
	if record.SchemaID != workerDraftSchemaID {
		return workers.Draft{}, fmt.Errorf("%w: record schema %q is not %q", ErrWorkerRecordingDelivery, record.SchemaID, workerDraftSchemaID)
	}
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		return workers.Draft{}, fmt.Errorf("%w: Worker draft JSON: %w", ErrWorkerRecordingDelivery, err)
	}
	if err := workers.ValidateDraft(draft); err != nil {
		return workers.Draft{}, fmt.Errorf("%w: Worker draft: %w", ErrWorkerRecordingDelivery, err)
	}
	return draft, nil
}

func validateWorkerOpening(record events.Record, draft workers.Draft, sessionID string) error {
	if record.SourceType != workerLifecycleSourceType ||
		record.SourceID != events.SourceID(sessionID) ||
		record.SourceSequence != openingSourceSequence ||
		record.SourceEventID != openingSourceEventID ||
		record.ID.Position != 1 ||
		draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted {
		return fmt.Errorf("%w: expected correlated position-1 SESSION/STARTED for Worker Session %q", ErrWorkerRecordingOpening, sessionID)
	}
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil ||
		payload.Status != "STARTING" || payload.WorkerSessionID != sessionID {
		return fmt.Errorf("%w: opening payload does not match STARTING/%q", ErrWorkerRecordingOpening, sessionID)
	}
	return nil
}

func validateWorkerTerminal(record events.Record, sessionID string) error {
	if record.SourceType != workerLifecycleSourceType ||
		record.SourceID != events.SourceID(sessionID) ||
		record.SourceSequence != terminalSourceSequence ||
		record.SourceEventID != terminalSourceEventID {
		return fmt.Errorf("%w: terminal identity is not correlated to Worker Session %q", ErrWorkerRecordingTerminal, sessionID)
	}
	return nil
}

func workerTerminalStatus(draft workers.Draft) (string, error) {
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(draft.Payload, &payload); err != nil || strings.TrimSpace(payload.Status) == "" {
		return "", fmt.Errorf("%w: terminal payload status is missing", ErrWorkerRecordingTerminal)
	}
	want := string(draft.Phase)
	if draft.Phase == workers.PhaseCanceled && payload.Status != "CANCELED" && payload.Status != "TERMINATED" {
		return "", fmt.Errorf("%w: canceled terminal status %q is invalid", ErrWorkerRecordingTerminal, payload.Status)
	}
	if draft.Phase != workers.PhaseCanceled && payload.Status != want {
		return "", fmt.Errorf("%w: terminal phase %q and status %q disagree", ErrWorkerRecordingTerminal, draft.Phase, payload.Status)
	}
	return payload.Status, nil
}

func isWorkerTerminalPhase(phase workers.Phase) bool {
	return phase == workers.PhaseCompleted || phase == workers.PhaseFailed || phase == workers.PhaseCanceled
}
