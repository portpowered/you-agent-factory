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
	WorkerSessionID    string                   `json:"workerSessionId"`
	FactorySessionID   string                   `json:"factorySessionId,omitempty"`
	WorkIDs            []string                 `json:"workIds,omitempty"`
	AttemptID          string                   `json:"attemptId,omitempty"`
	Topic              events.Topic             `json:"topic,omitempty"`
	Status             WorkerRecordingStatus    `json:"status,omitempty"`
	LastPosition       events.AggregateSequence `json:"lastPosition,omitempty"`
	Failure            string                   `json:"failure,omitempty"`
	InterruptionReason string                   `json:"interruptionReason,omitempty"`
	ExecutionTerminal  *WorkerRecordingTerminal `json:"executionTerminal,omitempty"`
	Records            []events.Record          `json:"records"`
}

// WorkerRecordingStatus is the durable recording-health state derived from
// accepted Worker history and explicit durable loss evidence. It deliberately
// does not describe the Worker execution outcome: a failed or canceled Worker
// can still have COMPLETE recording health.
type WorkerRecordingStatus string

const (
	WorkerRecordingStatusComplete   WorkerRecordingStatus = "COMPLETE"
	WorkerRecordingStatusDegraded   WorkerRecordingStatus = "DEGRADED"
	WorkerRecordingStatusIncomplete WorkerRecordingStatus = "INCOMPLETE"

	// WorkerRecordingInterruptionProcessStopped is the stable reason assigned
	// during recovery when a durable prefix has no persisted capture failure or
	// terminal fact. It describes the recording evidence, not Worker execution.
	WorkerRecordingInterruptionProcessStopped = "PROCESS_INTERRUPTED"

	// The following values are retained solely so older durable sidecars and
	// callers can be recognized and mapped explicitly. New snapshots never
	// write them.
	WorkerRecordingStatusActive    WorkerRecordingStatus = "ACTIVE"
	WorkerRecordingStatusCompleted WorkerRecordingStatus = "COMPLETED"
	WorkerRecordingStatusFailed    WorkerRecordingStatus = "FAILED"
)

// WorkerRecordingHistory is the canonical, source-native input to the pure
// Worker recording reducer.
type WorkerRecordingHistory struct {
	RecordingID        string
	WorkerSessionID    string
	FactorySessionID   string
	WorkIDs            []string
	AttemptID          string
	Topic              events.Topic
	Failure            string
	InterruptionReason string
	ExecutionTerminal  *WorkerRecordingTerminal
	Records            []events.Record
}

// WorkerRecordingTerminal is the detached terminal lifecycle fact derived by
// the reducer. The complete source record remains in Projection.Records.
type WorkerRecordingTerminal struct {
	Position events.AggregateSequence
	Phase    workers.Phase
	Status   string
}

// WorkerRecordingProjection is the deterministic observable result of
// reducing one Worker history. INCOMPLETE projections are valid live or
// durable prefixes; ReplayWorkerRecording returns them instead of hiding the
// readable history behind a generic replay error.
type WorkerRecordingProjection struct {
	RecordingID        string
	WorkerSessionID    string
	FactorySessionID   string
	WorkIDs            []string
	AttemptID          string
	Topic              events.Topic
	Status             WorkerRecordingStatus
	Complete           bool
	LastPosition       events.AggregateSequence
	Opening            events.Record
	Terminal           *WorkerRecordingTerminal
	ExecutionTerminal  *WorkerRecordingTerminal
	Degradation        string
	InterruptionReason string
	Records            []events.Record
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

// WorkerRecordingListRequest is the bounded catalog query used by the
// Worker-ID inspection surface. Continuation tokens are process-local read
// cursors; they are intentionally not persisted in the artifact.
type WorkerRecordingListRequest struct {
	FactorySessionID string
	WorkID           string
	MaxResults       int
	NextToken        string
}

// WorkerRecordingCatalogDiagnostic describes a file that could not be fully
// projected without hiding the other valid catalog entries.
type WorkerRecordingCatalogDiagnostic struct {
	RecordingID string
	Path        string
	Code        WorkerRecordingCatalogDiagnosticCode
	Message     string
}

// WorkerRecordingCatalogDiagnosticCode is a stable, safe catalog diagnostic
// classification. Raw decoder and filesystem details stay out of public
// responses.
type WorkerRecordingCatalogDiagnosticCode string

const (
	WorkerRecordingCatalogMalformedTail   WorkerRecordingCatalogDiagnosticCode = "MALFORMED_TAIL"
	WorkerRecordingCatalogUnsupported     WorkerRecordingCatalogDiagnosticCode = "UNSUPPORTED_SCHEMA"
	WorkerRecordingCatalogUnreadable      WorkerRecordingCatalogDiagnosticCode = "UNREADABLE"
	WorkerRecordingCatalogRetention       WorkerRecordingCatalogDiagnosticCode = "RETENTION_REMOVED"
	WorkerRecordingCatalogInvalidIdentity WorkerRecordingCatalogDiagnosticCode = "INVALID_IDENTITY"
)

// WorkerRecordingListResult is a bounded catalog page. Diagnostics are
// returned alongside readable projections so one damaged tail cannot erase a
// valid Worker from list results.
type WorkerRecordingListResult struct {
	Projections []WorkerRecordingProjection
	MaxResults  int
	NextToken   string
	Diagnostics []WorkerRecordingCatalogDiagnostic
}

var (
	ErrInvalidWorkerRecordingRequest = errors.New("recordings: invalid Worker recording request")
	ErrWorkerRecordingOpening        = errors.New("recordings: Worker recording opening is invalid")
	ErrWorkerRecordingDelivery       = errors.New("recordings: Worker recording delivery failed")
	ErrWorkerRecordingOrder          = errors.New("recordings: Worker recording order is invalid")
	ErrWorkerRecordingDuplicate      = errors.New("recordings: Worker recording duplicate conflicts")
	ErrWorkerRecordingTerminal       = errors.New("recordings: Worker recording terminal lifecycle is invalid")
	ErrWorkerRecordingIncomplete     = errors.New("recordings: Worker recording is incomplete")
	ErrWorkerRecordingCompatibility  = errors.New("recordings: Worker recording compatibility is unsupported")
	ErrWorkerRecordingReplay         = errors.New("recordings: Worker recording replay failed")
	ErrWorkerRecordingCorruptTail    = errors.New("recordings: Worker recording has a corrupt tail")
	ErrWorkerRecordingAppend         = errors.New("recordings: Worker recording append is unavailable")
	ErrWorkerRecordingRetention      = errors.New("recordings: Worker recording retention removed history")
	ErrWorkerRecordingCursor         = errors.New("recordings: Worker recording cursor is unavailable")
)

const (
	workerLifecycleSourceType = events.SourceType("worker_session_lifecycle")
	workerLineageSourceType   = events.SourceType("worker_session_lineage")
	workerLineageSourceEvent  = events.SourceEventID("successor")
	workerDraftSchemaID       = events.SchemaID("workers.draft.v1")
	openingSourceSequence     = events.SourceSequence(1)
	openingSourceEventID      = events.SourceEventID("started")
	terminalSourceSequence    = events.SourceSequence(2)
	terminalSourceEventID     = events.SourceEventID("terminal")
)

// ReduceWorkerRecording is the single deterministic reducer used by live
// capture and completed replay. It performs no I/O or provider lookup.
// WorkerRecordingCodec exposes the pure Worker history and portable-contract
// operations without adding package-level operational entry points to the
// Recordings service root.
type WorkerRecordingCodec struct{}

func (WorkerRecordingCodec) ReduceWorkerRecording(history WorkerRecordingHistory) (WorkerRecordingProjection, error) {
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
	projection := WorkerRecordingProjection{
		RecordingID:        history.RecordingID,
		WorkerSessionID:    history.WorkerSessionID,
		FactorySessionID:   history.FactorySessionID,
		WorkIDs:            append([]string(nil), history.WorkIDs...),
		AttemptID:          history.AttemptID,
		Topic:              topic,
		Status:             WorkerRecordingStatusIncomplete,
		Degradation:        strings.TrimSpace(history.Failure),
		InterruptionReason: strings.TrimSpace(history.InterruptionReason),
		Records:            make([]events.Record, 0, len(history.Records)),
	}
	identities := make(map[events.AppendIdentity]struct{}, len(history.Records))
	for index, record := range history.Records {
		if err := reduceWorkerRecord(&projection, identities, record, index, history.WorkerSessionID, topic); err != nil {
			return WorkerRecordingProjection{}, err
		}
	}
	if history.ExecutionTerminal != nil {
		if err := validateExecutionTerminal(*history.ExecutionTerminal); err != nil {
			return WorkerRecordingProjection{}, err
		}
		projection.ExecutionTerminal = cloneWorkerRecordingTerminal(history.ExecutionTerminal)
		if projection.Terminal != nil && !sameWorkerRecordingTerminal(projection.Terminal, history.ExecutionTerminal) {
			return WorkerRecordingProjection{}, fmt.Errorf("%w: durable execution terminal disagrees with recorded terminal", ErrWorkerRecordingTerminal)
		}
	} else if projection.Terminal != nil {
		projection.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.Terminal)
	}
	projection.Status = classifyWorkerRecordingStatus(projection, history.ExecutionTerminal != nil)
	projection.Complete = projection.Status == WorkerRecordingStatusComplete
	if projection.Status == WorkerRecordingStatusDegraded && projection.Degradation == "" {
		projection.Degradation = "DURABLE_CAPTURE_LOSS"
	}
	return projection, nil
}

func classifyWorkerRecordingStatus(projection WorkerRecordingProjection, hasAuthoritativeTerminal bool) WorkerRecordingStatus {
	if projection.ExecutionTerminal == nil {
		return WorkerRecordingStatusIncomplete
	}
	if projection.Degradation != "" || (hasAuthoritativeTerminal && projection.Terminal == nil) {
		return WorkerRecordingStatusDegraded
	}
	return WorkerRecordingStatusComplete
}

func reduceWorkerRecord(
	projection *WorkerRecordingProjection,
	identities map[events.AppendIdentity]struct{},
	record events.Record,
	index int,
	sessionID string,
	topic events.Topic,
) error {
	if err := validateWorkerRecord(record, topic, identities, index == 0, sessionID); err != nil {
		return err
	}
	identities[record.Identity()] = struct{}{}
	draft, err := decodeWorkerDraft(record)
	if err != nil {
		if index == 0 {
			return fmt.Errorf("%w: %w", ErrWorkerRecordingOpening, err)
		}
		return err
	}
	lineageRecord := isWorkerLineageRecord(draft)
	if projection.Terminal != nil && (!lineageRecord || !isWorkerSuccessorLineageRecord(record, draft)) {
		return fmt.Errorf("%w: record follows terminal at position %d", ErrWorkerRecordingTerminal, record.ID.Position)
	}
	if index == 0 {
		if err := validateWorkerOpening(record, draft, sessionID); err != nil {
			return err
		}
		projection.Opening = record.Detached()
	} else if draft.Kind == workers.KindSession && draft.Phase == workers.PhaseStarted {
		return fmt.Errorf("%w: duplicate SESSION/STARTED record at position %d", ErrWorkerRecordingOpening, record.ID.Position)
	}
	if err := reduceWorkerTerminal(projection, record, draft, sessionID); err != nil {
		return err
	}
	projection.Records = append(projection.Records, record.Detached())
	projection.LastPosition = record.ID.Position
	return nil
}

func reduceWorkerTerminal(projection *WorkerRecordingProjection, record events.Record, draft workers.Draft, sessionID string) error {
	if draft.Kind != workers.KindSession || !isWorkerTerminalPhase(draft.Phase) {
		return nil
	}
	if err := validateWorkerTerminal(record, sessionID); err != nil {
		return err
	}
	if projection.Terminal != nil {
		return fmt.Errorf("%w: multiple terminal records", ErrWorkerRecordingTerminal)
	}
	status, err := workerTerminalStatus(draft)
	if err != nil {
		return err
	}
	projection.Terminal = &WorkerRecordingTerminal{Position: record.ID.Position, Phase: draft.Phase, Status: status}
	return nil
}

// ReplayWorkerRecording reduces one durable snapshot and returns every
// readable health state. Incomplete and degraded projections are intentional
// read results; callers must inspect Projection.Status instead of treating a
// missing terminal as a generic replay failure.
func (codec WorkerRecordingCodec) ReplayWorkerRecording(request WorkerRecordingReplayRequest) (WorkerRecordingReplayResult, error) {
	if err := validateReplayRequest(request); err != nil {
		return WorkerRecordingReplayResult{}, err
	}
	sessionID, err := replayWorkerSessionID(request.Snapshot, request.WorkerSessionID)
	if err != nil {
		return WorkerRecordingReplayResult{}, err
	}
	for _, session := range request.Snapshot.Sessions {
		if session.WorkerSessionID != sessionID {
			continue
		}
		return codec.replayWorkerRecordingSession(request.Snapshot, session)
	}
	return WorkerRecordingReplayResult{}, fmt.Errorf("%w: Worker Session %q was not found", ErrWorkerRecordingReplay, sessionID)
}

func validateReplayRequest(request WorkerRecordingReplayRequest) error {
	if strings.TrimSpace(request.Snapshot.RecordingID) == "" {
		return fmt.Errorf("%w: recording identity is required", ErrWorkerRecordingReplay)
	}
	if len(request.Snapshot.Sessions) == 0 {
		return fmt.Errorf("%w: Worker Session history is missing", ErrWorkerRecordingReplay)
	}
	return validateWorkerRecordingSnapshot(request.Snapshot)
}

func replayWorkerSessionID(snapshot WorkerRecordingSnapshot, requested string) (string, error) {
	sessionID := strings.TrimSpace(requested)
	if sessionID != "" {
		return sessionID, nil
	}
	if len(snapshot.Sessions) != 1 {
		return "", fmt.Errorf("%w: Worker Session ID is required for a multi-session snapshot", ErrWorkerRecordingReplay)
	}
	return snapshot.Sessions[0].WorkerSessionID, nil
}

func (codec WorkerRecordingCodec) replayWorkerRecordingSession(
	snapshot WorkerRecordingSnapshot,
	session WorkerSessionRecordingSnapshot,
) (WorkerRecordingReplayResult, error) {
	legacyStatus, legacyFailure, err := normalizeWorkerRecordingStatus(session.Status)
	if err != nil {
		return WorkerRecordingReplayResult{}, err
	}
	failure := strings.TrimSpace(session.Failure)
	if legacyFailure && failure == "" {
		failure = "LEGACY_CAPTURE_FAILED"
	}
	projection, err := codec.ReduceWorkerRecording(WorkerRecordingHistory{
		RecordingID:        snapshot.RecordingID,
		WorkerSessionID:    session.WorkerSessionID,
		FactorySessionID:   session.FactorySessionID,
		WorkIDs:            session.WorkIDs,
		AttemptID:          session.AttemptID,
		Topic:              session.Topic,
		Failure:            failure,
		InterruptionReason: session.InterruptionReason,
		ExecutionTerminal:  session.ExecutionTerminal,
		Records:            session.Records,
	})
	if err != nil {
		return WorkerRecordingReplayResult{}, fmt.Errorf("%w: %w", ErrWorkerRecordingReplay, err)
	}
	if projection.Status != WorkerRecordingStatusIncomplete && strings.TrimSpace(session.InterruptionReason) != "" {
		return WorkerRecordingReplayResult{}, fmt.Errorf("%w: interruption reason is only valid for INCOMPLETE recordings", ErrWorkerRecordingCompatibility)
	}
	projection.InterruptionReason = recoveredInterruptionReason(projection, failure)
	if legacyStatus != "" && !isLegacyWorkerRecordingStatus(session.Status) && legacyStatus != projection.Status {
		return WorkerRecordingReplayResult{}, fmt.Errorf("%w: declared status %q disagrees with durable evidence %q", ErrWorkerRecordingCompatibility, session.Status, projection.Status)
	}
	return WorkerRecordingReplayResult{Projection: projection}, nil
}
func validateWorkerRecordingSnapshot(snapshot WorkerRecordingSnapshot) error {
	seen := make(map[string]struct{}, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		workerSessionID := strings.TrimSpace(session.WorkerSessionID)
		if workerSessionID == "" {
			return fmt.Errorf("%w: snapshot contains an unnamed Worker Session", ErrWorkerRecordingReplay)
		}
		if workerSessionID != session.WorkerSessionID {
			return fmt.Errorf("%w: Worker Session identity %q contains surrounding whitespace", ErrWorkerRecordingReplay, session.WorkerSessionID)
		}
		if _, exists := seen[workerSessionID]; exists {
			return fmt.Errorf("%w: Worker Session %q appears more than once", ErrWorkerRecordingDuplicate, workerSessionID)
		}
		seen[workerSessionID] = struct{}{}
		if session.LastPosition != 0 {
			if len(session.Records) == 0 || session.LastPosition != session.Records[len(session.Records)-1].ID.Position {
				return fmt.Errorf("%w: declared last position %d does not match the durable prefix", ErrWorkerRecordingOrder, session.LastPosition)
			}
		}
	}
	return nil
}

func recoveredInterruptionReason(projection WorkerRecordingProjection, failure string) string {
	if projection.Status != WorkerRecordingStatusIncomplete {
		return ""
	}
	if reason := strings.TrimSpace(projection.InterruptionReason); reason != "" {
		return reason
	}
	if reason := strings.TrimSpace(failure); reason != "" {
		return reason
	}
	return WorkerRecordingInterruptionProcessStopped
}

func normalizeWorkerRecordingStatus(status WorkerRecordingStatus) (WorkerRecordingStatus, bool, error) {
	switch status {
	case "":
		return "", false, nil
	case WorkerRecordingStatusComplete, WorkerRecordingStatusDegraded, WorkerRecordingStatusIncomplete:
		return status, false, nil
	case WorkerRecordingStatusActive:
		return WorkerRecordingStatusIncomplete, false, nil
	case WorkerRecordingStatusCompleted:
		return WorkerRecordingStatusComplete, false, nil
	case WorkerRecordingStatusFailed:
		return WorkerRecordingStatusIncomplete, true, nil
	default:
		return "", false, fmt.Errorf("%w: status %q is not recognized; rewrite the Worker recording with a supported schema", ErrWorkerRecordingCompatibility, status)
	}
}

func isLegacyWorkerRecordingStatus(status WorkerRecordingStatus) bool {
	return status == WorkerRecordingStatusActive || status == WorkerRecordingStatusCompleted || status == WorkerRecordingStatusFailed
}

func validateExecutionTerminal(terminal WorkerRecordingTerminal) error {
	if !isWorkerTerminalPhase(terminal.Phase) {
		return fmt.Errorf("%w: authoritative terminal phase %q is not terminal", ErrWorkerRecordingTerminal, terminal.Phase)
	}
	if strings.TrimSpace(terminal.Status) == "" {
		return fmt.Errorf("%w: authoritative terminal status is missing", ErrWorkerRecordingTerminal)
	}
	if terminal.Phase == workers.PhaseCanceled {
		if terminal.Status != "CANCELED" && terminal.Status != "TERMINATED" {
			return fmt.Errorf("%w: canceled authoritative terminal status %q is invalid", ErrWorkerRecordingTerminal, terminal.Status)
		}
	} else if terminal.Status != string(terminal.Phase) {
		return fmt.Errorf("%w: authoritative terminal phase %q and status %q disagree", ErrWorkerRecordingTerminal, terminal.Phase, terminal.Status)
	}
	return nil
}

func sameWorkerRecordingTerminal(left, right *WorkerRecordingTerminal) bool {
	if left == nil || right == nil || left.Phase != right.Phase || left.Status != right.Status {
		return false
	}
	return left.Position == 0 || right.Position == 0 || left.Position == right.Position
}

func cloneWorkerRecordingTerminal(terminal *WorkerRecordingTerminal) *WorkerRecordingTerminal {
	if terminal == nil {
		return nil
	}
	clone := *terminal
	return &clone
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
	if draft.Kind == workers.KindSession {
		var payload workers.SessionPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			return workers.Draft{}, fmt.Errorf("%w: Session payload JSON: %w", ErrWorkerRecordingDelivery, err)
		}
		if err := payload.ValidateLineage(); err != nil {
			return workers.Draft{}, fmt.Errorf("%w: Session lineage: %w", ErrWorkerRecordingDelivery, err)
		}
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
	if err := payload.ValidateLineage(); err != nil {
		return fmt.Errorf("%w: opening Session lineage is invalid", ErrWorkerRecordingOpening)
	}
	return nil
}

func isWorkerLineageRecord(draft workers.Draft) bool {
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseUpdated {
		return false
	}
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return false
	}
	return payload.Lineage != nil
}

func isWorkerSuccessorLineageRecord(record events.Record, draft workers.Draft) bool {
	if record.SourceType != workerLineageSourceType ||
		record.SourceSequence != 1 || record.SourceEventID != workerLineageSourceEvent {
		return false
	}
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil || payload.Lineage == nil {
		return false
	}
	expectedSourceID := events.SourceID(payload.WorkerSessionID + "/successor/" + payload.Lineage.SuccessorWorkerSessionID)
	return record.SourceID == expectedSourceID && payload.AttemptReason == "" && payload.Lineage.SuccessorWorkerSessionID != "" &&
		payload.Lineage.PredecessorWorkerSessionID == "" && payload.Lineage.PreviousDispatchID == "" &&
		payload.Lineage.PreviousAttemptID == ""
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

func cloneSessionLineage(value *workers.SessionLineage) *workers.SessionLineage {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}
