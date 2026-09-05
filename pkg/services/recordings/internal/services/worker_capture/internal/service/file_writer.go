package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	workerRecordingV2SchemaVersion = "worker-session-recording.v2"
	workerRecordingV2Suffix        = ".worker.jsonl"
	workerRecordingV2MaxLineBytes  = 2 << 20
	defaultWorkerRecordingPageSize = 50
	maxWorkerRecordingDiagnostics  = 64
)

// FileWriter owns the local Worker recording artifact. New records are
// appended as individually synced v2 JSONL envelopes. The old .worker.json
// path remains a read-only compatibility source for snapshots written before
// the v2 cutover.
type FileWriter struct {
	storage platformreplay.Storage
	root    string

	mu        sync.Mutex
	snapshots map[string]recordings.WorkerRecordingSnapshot
}

var _ recordings.WorkerRecordingWriter = (*FileWriter)(nil)
var _ recordings.WorkerRecordingReader = (*FileWriter)(nil)
var _ recordings.WorkerRecordingHistoryReader = (*FileWriter)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*FileWriter)(nil)

// NewFileWriter constructs the default local durable Worker-record writer.
// The storage port owns atomic replacement and append/sync filesystem
// mechanics; construction itself does not touch the configured home.
func NewFileWriter(storage platformreplay.Storage, root string) (recordings.WorkerRecordingWriter, error) {
	if storage == nil {
		return nil, fmt.Errorf("Worker recording file writer: storage is required")
	}
	if root == "" {
		return nil, fmt.Errorf("Worker recording file writer: root is required")
	}
	return &FileWriter{
		storage:   storage,
		root:      root,
		snapshots: make(map[string]recordings.WorkerRecordingSnapshot),
	}, nil
}

// PersistWorkerRecord appends one source-native record only after its JSONL
// suffix has been synchronized. A cache is updated after the append so a
// reopened writer and the live writer reduce the same accepted prefix.
func (writer *FileWriter) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	ctx = workerRecordingContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(record.RecordingID) == "" || strings.TrimSpace(record.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}
	if err := record.Record.Validate(); err != nil {
		return fmt.Errorf("validate Worker record: %w", err)
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.persistWorkerRecordLocked(ctx, record)
}

func (writer *FileWriter) persistWorkerRecordLocked(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	snapshot, err := writer.snapshotForWriteLocked(ctx, record.RecordingID)
	if err != nil {
		return err
	}
	metadata := recordingMetadata(record)
	session := findOrCreateSession(&snapshot, record.WorkerSessionID)
	mergeSessionMetadata(session, metadata)
	snapshot.RecordingID = record.RecordingID

	for _, accepted := range session.Records {
		if accepted.Identity() != record.Record.Identity() {
			continue
		}
		if sameRecord(accepted, record.Record) {
			return nil
		}
		return fmt.Errorf("%w: source identity %q/%q/%d/%q changed", recordings.ErrWorkerRecordingDuplicate, record.Record.SourceType, record.Record.SourceID, record.Record.SourceSequence, record.Record.SourceEventID)
	}

	history := cloneEventRecords(session.Records)
	history = append(history, record.Record.Detached())
	projection, err := reduceWorkerSession(snapshot.RecordingID, *session, history)
	if err != nil {
		return err
	}

	if err := writer.ensureV2ArtifactLocked(ctx, snapshot); err != nil {
		return err
	}
	path := writer.v2Path(record.RecordingID)
	if !writer.v2SessionHeaderPresentLocked(ctx, record.RecordingID, record.WorkerSessionID) {
		if err := writer.appendV2Line(ctx, path, workerRecordingV2Header{
			SchemaVersion:    workerRecordingV2SchemaVersion,
			Kind:             "header",
			RecordingID:      snapshot.RecordingID,
			WorkerSessionID:  record.WorkerSessionID,
			FactorySessionID: session.FactorySessionID,
			Topic:            session.Topic,
			WorkIDs:          append([]string(nil), session.WorkIDs...),
			AttemptID:        session.AttemptID,
			Status:           recordings.WorkerRecordingStatusActive,
		}); err != nil {
			return err
		}
	}
	if err := writer.appendV2Line(ctx, path, workerRecordingV2Record{
		SchemaVersion:   workerRecordingV2SchemaVersion,
		Kind:            "record",
		WorkerSessionID: record.WorkerSessionID,
		Record:          record.Record.Detached(),
	}); err != nil {
		return err
	}
	if projection.Complete || projection.Status == recordings.WorkerRecordingStatusDegraded {
		if err := writer.appendV2Line(ctx, path, workerRecordingV2Health{
			SchemaVersion:     workerRecordingV2SchemaVersion,
			Kind:              "health",
			WorkerSessionID:   record.WorkerSessionID,
			Status:            projection.Status,
			Reason:            healthReason(projection),
			LastPosition:      projection.LastPosition,
			ExecutionTerminal: cloneWorkerRecordingTerminal(projection.ExecutionTerminal),
		}); err != nil {
			return err
		}
	}

	applyProjectionToSession(session, projection)
	writer.cacheSnapshot(snapshot)
	return nil
}

// PersistWorkerRecordingFailure appends a health envelope. The terminal fact
// is carried in the health line when execution committed a terminal but the
// source record itself was not durably accepted, preserving DEGRADED rather
// than fabricating a terminal event on replay.
func (writer *FileWriter) PersistWorkerRecordingFailure(ctx context.Context, failure recordings.WorkerRecordingFailure) error {
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	ctx = workerRecordingContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(failure.RecordingID) == "" || strings.TrimSpace(failure.WorkerSessionID) == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	snapshot, err := writer.snapshotForWriteLocked(ctx, failure.RecordingID)
	if err != nil {
		return err
	}
	snapshot.RecordingID = failure.RecordingID
	session := findOrCreateSession(&snapshot, failure.WorkerSessionID)
	if failure.Topic != "" {
		session.Topic = failure.Topic
	}
	mergeSessionMetadata(session, workerRecordingSessionMetadata{
		FactorySessionID: failure.FactorySessionID,
		WorkIDs:          failure.WorkIDs,
		AttemptID:        failure.AttemptID,
	})
	session.Failure = strings.TrimSpace(failure.Code)
	session.ExecutionTerminal = cloneWorkerRecordingTerminal(failure.ExecutionTerminal)

	projection, err := reduceWorkerSession(snapshot.RecordingID, *session, session.Records)
	if err != nil {
		return fmt.Errorf("classify failed Worker recording: %w", err)
	}
	if err := writer.ensureV2ArtifactLocked(ctx, snapshot); err != nil {
		return err
	}
	if err := writer.appendV2Line(ctx, writer.v2Path(failure.RecordingID), workerRecordingV2Health{
		SchemaVersion:     workerRecordingV2SchemaVersion,
		Kind:              "health",
		WorkerSessionID:   failure.WorkerSessionID,
		Status:            projection.Status,
		Reason:            healthReason(projection),
		LastPosition:      projection.LastPosition,
		ExecutionTerminal: cloneWorkerRecordingTerminal(projection.ExecutionTerminal),
	}); err != nil {
		return err
	}
	applyProjectionToSession(session, projection)
	writer.cacheSnapshot(snapshot)
	return nil
}

// LoadWorkerRecording reads v2 first and then the legacy v1 snapshot path.
// A syntactically damaged v2 tail returns its valid prefix with an explicit
// health classification; it does not erase the readable history behind a
// generic decode error.
func (writer *FileWriter) LoadWorkerRecording(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	if writer == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingWriter
	}
	ctx = workerRecordingContext(ctx)
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	if strings.TrimSpace(recordingID) == "" {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrInvalidWorkerRecordingRequest
	}

	writer.mu.Lock()
	if snapshot, ok := writer.snapshots[recordingID]; ok {
		clone := cloneSnapshot(snapshot)
		writer.mu.Unlock()
		return clone, nil
	}
	writer.mu.Unlock()

	snapshot, err := writer.readDurableSnapshot(ctx, recordingID)
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	writer.mu.Lock()
	writer.cacheSnapshot(snapshot)
	clone := cloneSnapshot(snapshot)
	writer.mu.Unlock()
	return clone, nil
}

// ListWorkerRecordingProjections scans only the configured canonical home,
// orders by stable Worker identity, and returns a bounded page. Catalog
// diagnostics are additive: one malformed artifact cannot hide valid peers.
func (writer *FileWriter) ListWorkerRecordingProjections(ctx context.Context, request recordings.WorkerRecordingListRequest) (recordings.WorkerRecordingListResult, error) {
	if writer == nil {
		return recordings.WorkerRecordingListResult{}, recordings.ErrMissingWorkerRecordingWriter
	}
	ctx = workerRecordingContext(ctx)
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingListResult{}, err
	}
	if request.MaxResults < 0 {
		return recordings.WorkerRecordingListResult{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	limit := request.MaxResults
	if limit == 0 {
		limit = defaultWorkerRecordingPageSize
	}
	cursor, err := decodeRecordingCatalogCursor(request.NextToken)
	if err != nil {
		return recordings.WorkerRecordingListResult{}, err
	}

	entries, err := os.ReadDir(writer.root)
	if errors.Is(err, os.ErrNotExist) {
		return recordings.WorkerRecordingListResult{MaxResults: limit}, nil
	}
	if err != nil {
		return recordings.WorkerRecordingListResult{}, fmt.Errorf("list Worker recording home: %w", err)
	}

	all := make([]recordingCatalogEntry, 0)
	diagnostics := make([]recordings.WorkerRecordingCatalogDiagnostic, 0)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return recordings.WorkerRecordingListResult{}, err
		}
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, workerRecordingV2Suffix) && !strings.HasSuffix(name, ".worker.json")) {
			continue
		}
		path := filepath.Join(writer.root, name)
		snapshot, readErr := writer.readArtifactPath(ctx, path, "")
		if readErr != nil {
			if len(diagnostics) < maxWorkerRecordingDiagnostics {
				diagnostics = append(diagnostics, recordings.WorkerRecordingCatalogDiagnostic{
					Path:    path,
					Code:    catalogDiagnosticCode(readErr),
					Message: catalogDiagnosticMessage(readErr),
				})
			}
			continue
		}
		for _, session := range snapshot.Sessions {
			result, replayErr := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
				Snapshot:        snapshot,
				WorkerSessionID: session.WorkerSessionID,
			})
			if replayErr != nil {
				if len(diagnostics) < maxWorkerRecordingDiagnostics {
					diagnostics = append(diagnostics, recordings.WorkerRecordingCatalogDiagnostic{
						RecordingID: snapshot.RecordingID,
						Path:        path,
						Code:        catalogDiagnosticCode(replayErr),
						Message:     catalogDiagnosticMessage(replayErr),
					})
				}
				continue
			}
			value := result.Projection
			if diagnostic, ok := catalogProjectionDiagnostic(value, path); ok && len(diagnostics) < maxWorkerRecordingDiagnostics {
				diagnostics = append(diagnostics, diagnostic)
			}
			if request.FactorySessionID != "" && value.FactorySessionID != request.FactorySessionID {
				continue
			}
			if request.WorkID != "" && !containsString(value.WorkIDs, request.WorkID) {
				continue
			}
			key := recordingCatalogKey(value)
			if key <= cursor {
				continue
			}
			all = append(all, recordingCatalogEntry{key: key, projection: value})
		}
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].key < all[j].key })
	if len(all) > limit {
		page := all[:limit]
		return recordings.WorkerRecordingListResult{
			Projections: catalogProjectionPage(page),
			MaxResults:  limit,
			NextToken:   encodeRecordingCatalogCursor(page[len(page)-1].key),
			Diagnostics: diagnostics,
		}, nil
	}
	return recordings.WorkerRecordingListResult{
		Projections: catalogProjectionPage(all),
		MaxResults:  limit,
		Diagnostics: diagnostics,
	}, nil
}

// LoadWorkerRecordingByWorkerSessionID resolves the durable Worker-ID index
// without consulting Provider Sessions. A duplicate identity is surfaced as
// a catalog error instead of arbitrarily returning one path.
func (writer *FileWriter) LoadWorkerRecordingByWorkerSessionID(ctx context.Context, workerSessionID string) (recordings.WorkerRecordingSnapshot, error) {
	if writer == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingWriter
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	const maxCatalogPages = 256
	nextToken := ""
	var recordingID string
	for page := 0; page < maxCatalogPages; page++ {
		result, err := writer.ListWorkerRecordingProjections(ctx, recordings.WorkerRecordingListRequest{
			MaxResults: defaultWorkerRecordingPageSize,
			NextToken:  nextToken,
		})
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, err
		}
		for _, projection := range result.Projections {
			if projection.WorkerSessionID != workerSessionID {
				continue
			}
			if recordingID != "" && recordingID != projection.RecordingID {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: Worker Session %q has multiple durable recordings", recordings.ErrWorkerRecordingDuplicate, workerSessionID)
			}
			recordingID = projection.RecordingID
		}
		if result.NextToken == "" {
			break
		}
		if result.NextToken == nextToken {
			return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: catalog cursor did not advance", recordings.ErrWorkerRecordingCursor)
		}
		nextToken = result.NextToken
		if page == maxCatalogPages-1 {
			return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: catalog exceeds %d pages", recordings.ErrWorkerRecordingCursor, maxCatalogPages)
		}
	}
	if recordingID == "" {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: Worker Session %q was not found", recordings.ErrWorkerRecordingReplay, workerSessionID)
	}
	return writer.LoadWorkerRecording(ctx, recordingID)
}

func (writer *FileWriter) snapshotForWriteLocked(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	if snapshot, ok := writer.snapshots[recordingID]; ok {
		return cloneSnapshot(snapshot), nil
	}
	snapshot, err := writer.readDurableSnapshot(ctx, recordingID)
	if errors.Is(err, os.ErrNotExist) {
		return recordings.WorkerRecordingSnapshot{RecordingID: recordingID}, nil
	}
	return snapshot, err
}

func (writer *FileWriter) readDurableSnapshot(ctx context.Context, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	data, err := writer.storage.ReadFile(writer.v2Path(recordingID))
	if err == nil {
		return writer.readV2Snapshot(data, recordingID)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("load Worker recording v2 artifact: %w", err)
	}
	data, err = writer.storage.ReadFile(writer.path(recordingID))
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("load Worker recording snapshot: %w", err)
	}
	return writer.readV1Snapshot(data, recordingID)
}

func (writer *FileWriter) readArtifactPath(ctx context.Context, path, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	data, err := writer.storage.ReadFile(path)
	if err != nil {
		return recordings.WorkerRecordingSnapshot{}, err
	}
	if strings.HasSuffix(path, workerRecordingV2Suffix) {
		return writer.readV2Snapshot(data, recordingID)
	}
	return writer.readV1Snapshot(data, recordingID)
}

func (writer *FileWriter) readV1Snapshot(data []byte, recordingID string) (recordings.WorkerRecordingSnapshot, error) {
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: decode Worker recording snapshot: %v", recordings.ErrWorkerRecordingReplay, err)
	}
	if snapshot.RecordingID != recordingID && recordingID != "" {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: snapshot identity %q does not match %q", recordings.ErrWorkerRecordingReplay, snapshot.RecordingID, recordingID)
	}
	if strings.TrimSpace(snapshot.RecordingID) == "" {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: snapshot recording identity is missing", recordings.ErrWorkerRecordingReplay)
	}
	if len(snapshot.Sessions) == 0 {
		return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: Worker Session history is missing", recordings.ErrWorkerRecordingReplay)
	}
	for index := range snapshot.Sessions {
		session := &snapshot.Sessions[index]
		if metadata := metadataFromRecords(session.Records); metadata.hasValues() {
			mergeSessionMetadata(session, metadata)
		}
		result, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
			Snapshot:        snapshot,
			WorkerSessionID: session.WorkerSessionID,
		})
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("classify Worker recording snapshot: %w", err)
		}
		applyProjectionToSession(session, result.Projection)
	}
	return snapshot, nil
}

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
	currentSessionID := ""
	var tailReason string
	var tailSessionID string
	lineNumber := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), workerRecordingV2MaxLineBytes)
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			tailReason, tailSessionID = "EMPTY_TAIL", currentSessionID
			break
		}
		var envelope struct {
			SchemaVersion   string `json:"schemaVersion"`
			Kind            string `json:"kind"`
			WorkerSessionID string `json:"workerSessionId"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			tailReason, tailSessionID = "MALFORMED_TAIL", currentSessionID
			break
		}
		if envelope.SchemaVersion != workerRecordingV2SchemaVersion {
			if lineNumber == 1 {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: schema %q is not %q", recordings.ErrWorkerRecordingCompatibility, envelope.SchemaVersion, workerRecordingV2SchemaVersion)
			}
			tailReason, tailSessionID = "UNSUPPORTED_SCHEMA", currentSessionID
			break
		}
		switch envelope.Kind {
		case "header":
			var header workerRecordingV2Header
			if err := json.Unmarshal(line, &header); err != nil || strings.TrimSpace(header.RecordingID) == "" || strings.TrimSpace(header.WorkerSessionID) == "" {
				tailReason, tailSessionID = "INVALID_HEADER", envelope.WorkerSessionID
				break
			}
			if expectedRecordingID != "" && header.RecordingID != expectedRecordingID {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: artifact recording identity %q does not match %q", recordings.ErrWorkerRecordingReplay, header.RecordingID, expectedRecordingID)
			}
			if snapshot.RecordingID == "" {
				snapshot.RecordingID = header.RecordingID
			} else if snapshot.RecordingID != header.RecordingID {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: v2 artifact contains multiple recording identities", recordings.ErrWorkerRecordingDuplicate)
			}
			if _, exists := states[header.WorkerSessionID]; exists {
				tailReason, tailSessionID = "DUPLICATE_HEADER", header.WorkerSessionID
				break
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
			currentSessionID = header.WorkerSessionID
		case "record":
			var recordLine workerRecordingV2Record
			if err := json.Unmarshal(line, &recordLine); err != nil || recordLine.Record.IsZero() {
				tailReason, tailSessionID = "MALFORMED_TAIL", recordLine.WorkerSessionID
				if tailSessionID == "" {
					tailSessionID = currentSessionID
				}
				break
			}
			if err := recordLine.Record.Validate(); err != nil {
				tailReason, tailSessionID = "INVALID_RECORD", recordLine.WorkerSessionID
				if tailSessionID == "" {
					tailSessionID = currentSessionID
				}
				break
			}
			sessionID := recordLine.WorkerSessionID
			if sessionID == "" {
				sessionID = findSessionByTopic(snapshot.Sessions, recordLine.Record.ID.Topic, currentSessionID)
			}
			state := states[sessionID]
			if state == nil {
				tailReason, tailSessionID = "RECORD_WITHOUT_HEADER", sessionID
				break
			}
			state.session.Records = append(state.session.Records, recordLine.Record.Detached())
			currentSessionID = sessionID
		case "health":
			var health workerRecordingV2Health
			if err := json.Unmarshal(line, &health); err != nil {
				tailReason, tailSessionID = "MALFORMED_TAIL", envelope.WorkerSessionID
				break
			}
			sessionID := health.WorkerSessionID
			if sessionID == "" {
				sessionID = currentSessionID
			}
			state := states[sessionID]
			if state == nil {
				tailReason, tailSessionID = "HEALTH_WITHOUT_HEADER", sessionID
				break
			}
			if !validWorkerRecordingHealthStatus(health.Status) {
				tailReason, tailSessionID = "INVALID_HEALTH", sessionID
				break
			}
			state.healthStatus = health.Status
			state.healthReason = strings.TrimSpace(health.Reason)
			if health.ExecutionTerminal != nil {
				state.session.ExecutionTerminal = cloneWorkerRecordingTerminal(health.ExecutionTerminal)
			}
			currentSessionID = sessionID
		default:
			tailReason, tailSessionID = "UNKNOWN_ENVELOPE", envelope.WorkerSessionID
			if tailSessionID == "" {
				tailSessionID = currentSessionID
			}
		}
		if tailReason != "" {
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
	for _, state := range states {
		if state.session == nil {
			continue
		}
		if state.session.FactorySessionID == "" || state.session.AttemptID == "" || len(state.session.WorkIDs) == 0 {
			mergeSessionMetadata(state.session, metadataFromRecords(state.session.Records))
		}
		if state.session.WorkerSessionID == tailSessionID && tailReason != "" {
			state.tailReason = tailReason
		}
		if state.healthReason != "" {
			switch state.healthStatus {
			case recordings.WorkerRecordingStatusDegraded:
				state.session.Failure = state.healthReason
			case recordings.WorkerRecordingStatusIncomplete:
				state.session.InterruptionReason = state.healthReason
			}
		}
		if state.healthReason == "" && (state.healthStatus == "" || state.healthStatus == recordings.WorkerRecordingStatusActive) {
			base, err := reduceWorkerSession(snapshot.RecordingID, *state.session, state.session.Records)
			if err != nil {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: reduce v2 Worker Session prefix: %v", recordings.ErrWorkerRecordingReplay, err)
			}
			if base.Terminal == nil && state.session.InterruptionReason == "" {
				state.session.InterruptionReason = recordings.WorkerRecordingInterruptionProcessStopped
			}
		}
		if state.tailReason != "" {
			base, err := reduceWorkerSession(snapshot.RecordingID, *state.session, state.session.Records)
			if err != nil {
				return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: reduce v2 valid prefix: %v", recordings.ErrWorkerRecordingCorruptTail, err)
			}
			if base.Terminal != nil {
				state.session.Failure = state.tailReason
				state.session.InterruptionReason = ""
			} else if state.session.Failure == "" {
				state.session.InterruptionReason = state.tailReason
			}
		}
		projection, err := reduceWorkerSession(snapshot.RecordingID, *state.session, state.session.Records)
		if err != nil {
			return recordings.WorkerRecordingSnapshot{}, fmt.Errorf("%w: reduce v2 Worker Session %q: %v", recordings.ErrWorkerRecordingReplay, state.session.WorkerSessionID, err)
		}
		applyProjectionToSession(state.session, projection)
	}
	return snapshot, nil
}

func (writer *FileWriter) ensureV2ArtifactLocked(ctx context.Context, snapshot recordings.WorkerRecordingSnapshot) error {
	_, err := writer.storage.ReadFile(writer.v2Path(snapshot.RecordingID))
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Worker recording v2 artifact: %w", err)
	}
	path := writer.v2Path(snapshot.RecordingID)
	for _, session := range snapshot.Sessions {
		if err := writer.appendV2Line(ctx, path, workerRecordingV2Header{
			SchemaVersion:    workerRecordingV2SchemaVersion,
			Kind:             "header",
			RecordingID:      snapshot.RecordingID,
			WorkerSessionID:  session.WorkerSessionID,
			FactorySessionID: session.FactorySessionID,
			Topic:            session.Topic,
			WorkIDs:          append([]string(nil), session.WorkIDs...),
			AttemptID:        session.AttemptID,
			Status:           recordings.WorkerRecordingStatusActive,
		}); err != nil {
			return err
		}
		for _, record := range session.Records {
			if err := writer.appendV2Line(ctx, path, workerRecordingV2Record{
				SchemaVersion:   workerRecordingV2SchemaVersion,
				Kind:            "record",
				WorkerSessionID: session.WorkerSessionID,
				Record:          record.Detached(),
			}); err != nil {
				return err
			}
		}
		if session.Status == recordings.WorkerRecordingStatusComplete || session.Status == recordings.WorkerRecordingStatusDegraded {
			if err := writer.appendV2Line(ctx, path, workerRecordingV2Health{
				SchemaVersion:     workerRecordingV2SchemaVersion,
				Kind:              "health",
				WorkerSessionID:   session.WorkerSessionID,
				Status:            session.Status,
				Reason:            healthReasonFromSession(session),
				LastPosition:      session.LastPosition,
				ExecutionTerminal: cloneWorkerRecordingTerminal(session.ExecutionTerminal),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (writer *FileWriter) v2SessionHeaderPresentLocked(ctx context.Context, recordingID, workerSessionID string) bool {
	data, err := writer.storage.ReadFile(writer.v2Path(recordingID))
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 16<<10), workerRecordingV2MaxLineBytes)
	for scanner.Scan() {
		var header workerRecordingV2Header
		if json.Unmarshal(scanner.Bytes(), &header) == nil && header.SchemaVersion == workerRecordingV2SchemaVersion && header.Kind == "header" && header.WorkerSessionID == workerSessionID {
			return true
		}
	}
	return false
}

func (writer *FileWriter) appendV2Line(ctx context.Context, path string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	appender, ok := writer.storage.(platformreplay.Appender)
	if !ok || appender == nil {
		return fmt.Errorf("%w: storage does not expose append-and-sync", recordings.ErrWorkerRecordingAppend)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Worker recording v2 line: %w", err)
	}
	data = append(data, '\n')
	if len(data) > workerRecordingV2MaxLineBytes {
		return fmt.Errorf("%w: v2 line exceeds %d bytes", recordings.ErrWorkerRecordingAppend, workerRecordingV2MaxLineBytes)
	}
	if err := appender.AppendFile(path, data); err != nil {
		return fmt.Errorf("%w: append Worker recording v2 line: %v", recordings.ErrWorkerRecordingAppend, err)
	}
	return nil
}

func (writer *FileWriter) path(recordingID string) string {
	digest := sha256.Sum256([]byte(recordingID))
	return filepath.Join(writer.root, hex.EncodeToString(digest[:])+".worker.json")
}

func (writer *FileWriter) v2Path(recordingID string) string {
	digest := sha256.Sum256([]byte(recordingID))
	return filepath.Join(writer.root, hex.EncodeToString(digest[:])+workerRecordingV2Suffix)
}

type workerRecordingSessionMetadata struct {
	FactorySessionID string
	WorkIDs          []string
	AttemptID        string
}

func recordingMetadata(record recordings.WorkerRecordingRecord) workerRecordingSessionMetadata {
	metadata := workerRecordingSessionMetadata{
		FactorySessionID: strings.TrimSpace(record.FactorySessionID),
		WorkIDs:          append([]string(nil), record.WorkIDs...),
		AttemptID:        strings.TrimSpace(record.AttemptID),
	}
	if metadata.hasValues() {
		return metadata
	}
	return metadataFromRecords([]events.Record{record.Record})
}

func metadataFromRecords(records []events.Record) workerRecordingSessionMetadata {
	for _, record := range records {
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindSession {
			continue
		}
		var payload workers.SessionPayload
		if json.Unmarshal(draft.Payload, &payload) != nil {
			continue
		}
		return workerRecordingSessionMetadata{
			FactorySessionID: strings.TrimSpace(payload.FactorySessionID),
			WorkIDs:          append([]string(nil), payload.WorkIDs...),
			AttemptID:        strings.TrimSpace(payload.AttemptID),
		}
	}
	return workerRecordingSessionMetadata{}
}

func (metadata workerRecordingSessionMetadata) hasValues() bool {
	return metadata.FactorySessionID != "" || metadata.AttemptID != "" || len(metadata.WorkIDs) > 0
}

func mergeSessionMetadata(session *recordings.WorkerSessionRecordingSnapshot, metadata workerRecordingSessionMetadata) {
	if session == nil {
		return
	}
	if session.FactorySessionID == "" {
		session.FactorySessionID = strings.TrimSpace(metadata.FactorySessionID)
	}
	if session.AttemptID == "" {
		session.AttemptID = strings.TrimSpace(metadata.AttemptID)
	}
	if len(session.WorkIDs) == 0 && len(metadata.WorkIDs) > 0 {
		session.WorkIDs = append([]string(nil), metadata.WorkIDs...)
	}
}

func reduceWorkerSession(recordingID string, session recordings.WorkerSessionRecordingSnapshot, records []events.Record) (recordings.WorkerRecordingProjection, error) {
	return (recordings.WorkerRecordingCodec{}).ReduceWorkerRecording(recordings.WorkerRecordingHistory{
		RecordingID:        recordingID,
		WorkerSessionID:    session.WorkerSessionID,
		FactorySessionID:   session.FactorySessionID,
		WorkIDs:            append([]string(nil), session.WorkIDs...),
		AttemptID:          session.AttemptID,
		Topic:              session.Topic,
		Failure:            session.Failure,
		InterruptionReason: session.InterruptionReason,
		ExecutionTerminal:  cloneWorkerRecordingTerminal(session.ExecutionTerminal),
		Records:            records,
	})
}

func applyProjectionToSession(session *recordings.WorkerSessionRecordingSnapshot, projection recordings.WorkerRecordingProjection) {
	if session == nil {
		return
	}
	session.FactorySessionID = projection.FactorySessionID
	session.WorkIDs = append([]string(nil), projection.WorkIDs...)
	session.AttemptID = projection.AttemptID
	session.Topic = projection.Topic
	session.Status = projection.Status
	session.LastPosition = projection.LastPosition
	session.Failure = projection.Degradation
	session.InterruptionReason = projection.InterruptionReason
	session.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.ExecutionTerminal)
	session.Records = cloneEventRecords(projection.Records)
}

func healthReason(projection recordings.WorkerRecordingProjection) string {
	if projection.Status == recordings.WorkerRecordingStatusDegraded {
		return strings.TrimSpace(projection.Degradation)
	}
	return strings.TrimSpace(projection.InterruptionReason)
}

func healthReasonFromSession(session recordings.WorkerSessionRecordingSnapshot) string {
	if session.Status == recordings.WorkerRecordingStatusDegraded {
		return strings.TrimSpace(session.Failure)
	}
	return strings.TrimSpace(session.InterruptionReason)
}

func validWorkerRecordingHealthStatus(status recordings.WorkerRecordingStatus) bool {
	switch status {
	case "", recordings.WorkerRecordingStatusComplete, recordings.WorkerRecordingStatusDegraded, recordings.WorkerRecordingStatusIncomplete, recordings.WorkerRecordingStatusActive:
		return true
	default:
		return false
	}
}

func findSessionByTopic(sessions []recordings.WorkerSessionRecordingSnapshot, topic events.Topic, preferred string) string {
	if preferred != "" {
		for _, session := range sessions {
			if session.WorkerSessionID == preferred {
				return preferred
			}
		}
	}
	for _, session := range sessions {
		if session.Topic == topic {
			return session.WorkerSessionID
		}
	}
	return ""
}

type recordingCatalogEntry struct {
	key        string
	projection recordings.WorkerRecordingProjection
}

func recordingCatalogKey(projection recordings.WorkerRecordingProjection) string {
	return projection.WorkerSessionID + "\x00" + projection.RecordingID
}

func catalogProjectionPage(entries []recordingCatalogEntry) []recordings.WorkerRecordingProjection {
	values := make([]recordings.WorkerRecordingProjection, len(entries))
	for index, entry := range entries {
		values[index] = cloneProjection(entry.projection)
	}
	return values
}

func encodeRecordingCatalogCursor(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeRecordingCatalogCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return "", recordings.ErrWorkerRecordingCursor
	}
	return string(decoded), nil
}

func catalogDiagnosticCode(err error) recordings.WorkerRecordingCatalogDiagnosticCode {
	switch {
	case errors.Is(err, recordings.ErrWorkerRecordingCorruptTail):
		return recordings.WorkerRecordingCatalogMalformedTail
	case errors.Is(err, recordings.ErrWorkerRecordingCompatibility):
		return recordings.WorkerRecordingCatalogUnsupported
	case errors.Is(err, recordings.ErrWorkerRecordingRetention):
		return recordings.WorkerRecordingCatalogRetention
	default:
		return recordings.WorkerRecordingCatalogUnreadable
	}
}

func catalogDiagnosticMessage(err error) string {
	if errors.Is(err, recordings.ErrWorkerRecordingCorruptTail) {
		return "valid Worker recording prefix retained; tail is not readable"
	}
	if errors.Is(err, recordings.ErrWorkerRecordingCompatibility) {
		return "Worker recording schema is not supported"
	}
	return "Worker recording artifact could not be read"
}

func catalogProjectionDiagnostic(
	projection recordings.WorkerRecordingProjection,
	path string,
) (recordings.WorkerRecordingCatalogDiagnostic, bool) {
	reason := strings.TrimSpace(projection.Degradation)
	if reason == "" {
		reason = strings.TrimSpace(projection.InterruptionReason)
	}
	code := recordings.WorkerRecordingCatalogDiagnosticCode(reason)
	switch code {
	case recordings.WorkerRecordingCatalogMalformedTail:
		return recordings.WorkerRecordingCatalogDiagnostic{
			RecordingID: projection.RecordingID,
			Path:        path,
			Code:        recordings.WorkerRecordingCatalogMalformedTail,
			Message:     "valid Worker recording prefix retained; tail is not readable",
		}, true
	case recordings.WorkerRecordingCatalogUnsupported:
		return recordings.WorkerRecordingCatalogDiagnostic{
			RecordingID: projection.RecordingID,
			Path:        path,
			Code:        recordings.WorkerRecordingCatalogUnsupported,
			Message:     "Worker recording schema is not supported",
		}, true
	default:
		return recordings.WorkerRecordingCatalogDiagnostic{}, false
	}
}

func cloneSnapshot(snapshot recordings.WorkerRecordingSnapshot) recordings.WorkerRecordingSnapshot {
	clone := snapshot
	clone.Sessions = make([]recordings.WorkerSessionRecordingSnapshot, len(snapshot.Sessions))
	for i, session := range snapshot.Sessions {
		clone.Sessions[i] = session
		clone.Sessions[i].WorkIDs = append([]string(nil), session.WorkIDs...)
		clone.Sessions[i].ExecutionTerminal = cloneWorkerRecordingTerminal(session.ExecutionTerminal)
		clone.Sessions[i].Records = cloneEventRecords(session.Records)
	}
	return clone
}

func cloneProjection(projection recordings.WorkerRecordingProjection) recordings.WorkerRecordingProjection {
	clone := projection
	clone.WorkIDs = append([]string(nil), projection.WorkIDs...)
	clone.Opening = projection.Opening.Detached()
	clone.Terminal = cloneWorkerRecordingTerminal(projection.Terminal)
	clone.ExecutionTerminal = cloneWorkerRecordingTerminal(projection.ExecutionTerminal)
	clone.Records = cloneEventRecords(projection.Records)
	return clone
}

func cloneWorkerRecordingTerminal(terminal *recordings.WorkerRecordingTerminal) *recordings.WorkerRecordingTerminal {
	if terminal == nil {
		return nil
	}
	clone := *terminal
	return &clone
}

func cloneEventRecords(records []events.Record) []events.Record {
	clone := make([]events.Record, len(records))
	for index, record := range records {
		clone[index] = record.Detached()
	}
	return clone
}

func findOrCreateSession(snapshot *recordings.WorkerRecordingSnapshot, workerSessionID string) *recordings.WorkerSessionRecordingSnapshot {
	for i := range snapshot.Sessions {
		if snapshot.Sessions[i].WorkerSessionID == workerSessionID {
			return &snapshot.Sessions[i]
		}
	}
	snapshot.Sessions = append(snapshot.Sessions, recordings.WorkerSessionRecordingSnapshot{
		WorkerSessionID: workerSessionID,
		Records:         []events.Record{},
	})
	return &snapshot.Sessions[len(snapshot.Sessions)-1]
}

func (writer *FileWriter) cacheSnapshot(snapshot recordings.WorkerRecordingSnapshot) {
	if writer.snapshots == nil {
		writer.snapshots = make(map[string]recordings.WorkerRecordingSnapshot)
	}
	writer.snapshots[snapshot.RecordingID] = cloneSnapshot(snapshot)
}

func workerRecordingContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
