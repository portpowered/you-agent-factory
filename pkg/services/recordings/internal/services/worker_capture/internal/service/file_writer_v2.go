package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type workerRecordingV2Header struct {
	SchemaVersion    string                           `json:"schemaVersion"`
	Kind             string                           `json:"kind"`
	RecordingID      string                           `json:"recordingId"`
	WorkerSessionID  string                           `json:"workerSessionId"`
	FactorySessionID string                           `json:"factorySessionId,omitempty"`
	Topic            events.Topic                     `json:"topic"`
	WorkIDs          []string                         `json:"workIds,omitempty"`
	AttemptID        string                           `json:"attemptId,omitempty"`
	Status           recordings.WorkerRecordingStatus `json:"status"`
}

type workerRecordingV2Record struct {
	SchemaVersion   string        `json:"schemaVersion"`
	Kind            string        `json:"kind"`
	WorkerSessionID string        `json:"workerSessionId,omitempty"`
	Record          events.Record `json:"record"`
}

type workerRecordingV2Health struct {
	SchemaVersion     string                              `json:"schemaVersion"`
	Kind              string                              `json:"kind"`
	WorkerSessionID   string                              `json:"workerSessionId,omitempty"`
	Status            recordings.WorkerRecordingStatus    `json:"status"`
	Reason            string                              `json:"reason,omitempty"`
	LastPosition      events.AggregateSequence            `json:"lastPosition"`
	ExecutionTerminal *recordings.WorkerRecordingTerminal `json:"executionTerminal,omitempty"`
}

type workerRecordingSessionReadState struct {
	session      *recordings.WorkerSessionRecordingSnapshot
	healthStatus recordings.WorkerRecordingStatus
	healthReason string
	tailReason   string
}

func (writer *FileWriter) readV2Snapshot(data []byte, expectedRecordingID string) (recordings.WorkerRecordingSnapshot, error) {
	var snapshot recordings.WorkerRecordingSnapshot
	states := make(map[string]*workerRecordingSessionReadState)
	currentSessionID, tailReason, tailSessionID := "", "", ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), workerRecordingV2MaxLineBytes)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		nextSessionID, reason, reasonSessionID, err := readV2Line(
			scanner.Bytes(), lineNumber, expectedRecordingID, currentSessionID, &snapshot, states,
		)
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, err
		}
		currentSessionID = nextSessionID
		if reason != "" {
			tailReason, tailSessionID = reason, reasonSessionID
			break
		}
	}
	if scanner.Err() != nil {
		tailReason = "OVERSIZE_LINE"
		if tailSessionID == "" {
			tailSessionID = currentSessionID
		}
	}
	if snapshot.RecordingID == "" {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: v2 recording header is missing", recordings.ErrWorkerRecordingReplay)
	}
	if len(snapshot.Sessions) == 0 {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: v2 Worker Session header is missing", recordings.ErrWorkerRecordingReplay)
	}
	if err := finalizeV2Snapshot(&snapshot, states, tailReason, tailSessionID); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	return snapshot, nil
}

func readV2Line(
	line []byte,
	lineNumber int,
	expectedRecordingID string,
	currentSessionID string,
	snapshot *recordings.WorkerRecordingSnapshot,
	states map[string]*workerRecordingSessionReadState,
) (string, string, string, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return currentSessionID, "EMPTY_TAIL", currentSessionID, nil
	}
	var envelope struct {
		SchemaVersion   string `json:"schemaVersion"`
		Kind            string `json:"kind"`
		WorkerSessionID string `json:"workerSessionId"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return currentSessionID, "MALFORMED_TAIL", currentSessionID, nil
	}
	if envelope.SchemaVersion != workerRecordingV2SchemaVersion {
		if lineNumber == 1 {
			return "", "", "", fmt.Errorf("%w: schema %q is not %q", recordings.ErrWorkerRecordingCompatibility, envelope.SchemaVersion, workerRecordingV2SchemaVersion)
		}
		return currentSessionID, "UNSUPPORTED_SCHEMA", currentSessionID, nil
	}
	switch envelope.Kind {
	case "header":
		return readV2HeaderLine(line, expectedRecordingID, snapshot, states)
	case "record":
		return readV2RecordLine(line, currentSessionID, snapshot, states)
	case "health":
		return readV2HealthLine(line, currentSessionID, snapshot, states)
	default:
		tailSessionID := envelope.WorkerSessionID
		if tailSessionID == "" {
			tailSessionID = currentSessionID
		}
		return currentSessionID, "UNKNOWN_ENVELOPE", tailSessionID, nil
	}
}

func readV2HeaderLine(
	line []byte,
	expectedRecordingID string,
	snapshot *recordings.WorkerRecordingSnapshot,
	states map[string]*workerRecordingSessionReadState,
) (string, string, string, error) {
	var header workerRecordingV2Header
	if err := json.Unmarshal(line, &header); err != nil || strings.TrimSpace(header.RecordingID) == "" || strings.TrimSpace(header.WorkerSessionID) == "" {
		return "", "INVALID_HEADER", header.WorkerSessionID, nil
	}
	if expectedRecordingID != "" && header.RecordingID != expectedRecordingID {
		return "", "", "", fmt.Errorf("%w: artifact recording identity %q does not match %q", recordings.ErrWorkerRecordingReplay, header.RecordingID, expectedRecordingID)
	}
	if snapshot.RecordingID == "" {
		snapshot.RecordingID = header.RecordingID
	} else if snapshot.RecordingID != header.RecordingID {
		return "", "", "", fmt.Errorf("%w: v2 artifact contains multiple recording identities", recordings.ErrWorkerRecordingDuplicate)
	}
	if _, exists := states[header.WorkerSessionID]; exists {
		return "", "DUPLICATE_HEADER", header.WorkerSessionID, nil
	}
	topic := header.Topic
	if topic == "" {
		topic = events.Topic("worker-session/" + header.WorkerSessionID + "/events")
	}
	snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{
		WorkerSessionID:  header.WorkerSessionID,
		FactorySessionID: header.FactorySessionID,
		WorkIDs:          append([]string(nil), header.WorkIDs...),
		AttemptID:        header.AttemptID,
		Topic:            topic,
		Records:          []events.Record{},
	})
	session := &snapshot.Sessions[len(snapshot.Sessions)-1]
	states[session.WorkerSessionID] = &workerRecordingSessionReadState{session: session, healthStatus: header.Status}
	return header.WorkerSessionID, "", "", nil
}

func readV2RecordLine(
	line []byte,
	currentSessionID string,
	snapshot *recordings.WorkerRecordingSnapshot,
	states map[string]*workerRecordingSessionReadState,
) (string, string, string, error) {
	var recordLine workerRecordingV2Record
	if err := json.Unmarshal(line, &recordLine); err != nil || recordLine.Record.IsZero() {
		tailSessionID := recordLine.WorkerSessionID
		if tailSessionID == "" {
			tailSessionID = currentSessionID
		}
		return currentSessionID, "MALFORMED_TAIL", tailSessionID, nil
	}
	if err := recordLine.Record.Validate(); err != nil {
		tailSessionID := recordLine.WorkerSessionID
		if tailSessionID == "" {
			tailSessionID = currentSessionID
		}
		return currentSessionID, "INVALID_RECORD", tailSessionID, nil
	}
	sessionID := recordLine.WorkerSessionID
	if sessionID == "" {
		sessionID = findSessionByTopic(snapshot.Sessions, recordLine.Record.ID.Topic, currentSessionID)
	}
	state := states[sessionID]
	if state == nil {
		return currentSessionID, "RECORD_WITHOUT_HEADER", sessionID, nil
	}
	state.session.Records = append(state.session.Records, recordLine.Record.Detached())
	return sessionID, "", "", nil
}

func readV2HealthLine(
	line []byte,
	currentSessionID string,
	_ *recordings.WorkerRecordingSnapshot,
	states map[string]*workerRecordingSessionReadState,
) (string, string, string, error) {
	var health workerRecordingV2Health
	if err := json.Unmarshal(line, &health); err != nil {
		return currentSessionID, "MALFORMED_TAIL", "", nil
	}
	sessionID := health.WorkerSessionID
	if sessionID == "" {
		sessionID = currentSessionID
	}
	state := states[sessionID]
	if state == nil {
		return currentSessionID, "HEALTH_WITHOUT_HEADER", sessionID, nil
	}
	if !validWorkerRecordingHealthStatus(health.Status) {
		return currentSessionID, "INVALID_HEALTH", sessionID, nil
	}
	state.healthStatus = health.Status
	state.healthReason = strings.TrimSpace(health.Reason)
	if health.ExecutionTerminal != nil {
		state.session.ExecutionTerminal = cloneWorkerRecordingTerminal(health.ExecutionTerminal)
	}
	return sessionID, "", "", nil
}

func finalizeV2Snapshot(
	snapshot *recordings.WorkerRecordingSnapshot,
	states map[string]*workerRecordingSessionReadState,
	tailReason string,
	tailSessionID string,
) error {
	for _, state := range states {
		if err := finalizeV2Session(snapshot.RecordingID, state, tailReason, tailSessionID); err != nil {
			return err
		}
	}
	return nil
}

func finalizeV2Session(
	recordingID string,
	state *workerRecordingSessionReadState,
	tailReason string,
	tailSessionID string,
) error {
	if state == nil || state.session == nil {
		return nil
	}
	if state.session.FactorySessionID == "" || state.session.AttemptID == "" || len(state.session.WorkIDs) == 0 {
		mergeSessionMetadata(state.session, metadataFromRecords(state.session.Records))
	}
	if state.session.WorkerSessionID == tailSessionID && tailReason != "" {
		state.tailReason = tailReason
	}
	applyV2HealthReason(state)
	if state.healthReason == "" && (state.healthStatus == "" || state.healthStatus == recordings.WorkerRecordingStatusActive) {
		if err := markV2Interrupted(recordingID, state); err != nil {
			return err
		}
	}
	if state.tailReason != "" {
		if err := classifyV2Tail(recordingID, state); err != nil {
			return err
		}
	}
	projection, err := reduceWorkerSession(recordingID, *state.session, state.session.Records)
	if err != nil {
		return fmt.Errorf("%w: reduce v2 Worker Session %q: %v", recordings.ErrWorkerRecordingReplay, state.session.WorkerSessionID, err)
	}
	applyProjectionToSession(state.session, projection)
	return nil
}

func applyV2HealthReason(state *workerRecordingSessionReadState) {
	if state == nil || state.healthReason == "" {
		return
	}
	switch state.healthStatus {
	case recordings.WorkerRecordingStatusDegraded:
		state.session.Failure = state.healthReason
	case recordings.WorkerRecordingStatusIncomplete:
		state.session.InterruptionReason = state.healthReason
	}
}

func markV2Interrupted(recordingID string, state *workerRecordingSessionReadState) error {
	base, err := reduceWorkerSession(recordingID, *state.session, state.session.Records)
	if err != nil {
		return fmt.Errorf("%w: reduce v2 Worker Session prefix: %v", recordings.ErrWorkerRecordingReplay, err)
	}
	if base.Terminal == nil && state.session.InterruptionReason == "" {
		state.session.InterruptionReason = recordings.WorkerRecordingInterruptionProcessStopped
	}
	return nil
}

func classifyV2Tail(recordingID string, state *workerRecordingSessionReadState) error {
	base, err := reduceWorkerSession(recordingID, *state.session, state.session.Records)
	if err != nil {
		return fmt.Errorf("%w: reduce v2 valid prefix: %v", recordings.ErrWorkerRecordingCorruptTail, err)
	}
	if base.Terminal != nil {
		state.session.Failure = state.tailReason
		state.session.InterruptionReason = ""
	} else if state.session.Failure == "" {
		state.session.InterruptionReason = state.tailReason
	}
	return nil
}
