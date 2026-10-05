package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const capturedPayloadLimit = 1 << 20

// Bound only the returned representation. The journal and spill retain exact
// source bytes; opening identity and usage projections still consume the journal.
func (writer *FileWriter) boundCapturedPayload(session *recordingSession, captured *recordings.WorkerCapturedRecord) error {
	payload := captured.Record.Payload
	if len(payload) <= capturedPayloadLimit {
		return nil
	}
	if _, err := writer.spillCapturedPayload(session, captured.Record); err != nil {
		return err
	}
	// JSON escaping needs at most six bytes per source byte. Reserve room for
	// the event envelope and trim at a UTF-8 boundary so even escaped previews
	// remain valid and comfortably below the returned-event cap.
	end := (capturedPayloadLimit - 8192) / 6
	for end > 0 && !utf8.RuneStart(payload[end]) {
		end--
	}
	preview, err := json.Marshal(struct {
		Preview string `json:"preview"`
	}{string(payload[:end])})
	if err != nil {
		return err
	}
	captured.Record.Payload = preview
	captured.Truncated = true
	captured.OriginalBytes = int64(len(payload))
	captured.ReturnedBytes = int64(len(preview))
	captured.ArtifactRef = session.projection.WorkerSessionID + "/" + strconv.FormatUint(uint64(captured.Record.ID.Position), 10)
	return nil
}

func (writer *FileWriter) spillCapturedPayload(session *recordingSession, record events.Record) ([]byte, error) {
	// Raw IDs never become filesystem paths. Generation fences replacement
	// captures, and the writer root supplies the selected profile boundary.
	key := session.generation + "/" + session.projection.WorkerSessionID + "/" + strconv.FormatUint(uint64(record.ID.Position), 10)
	digest := sha256.Sum256([]byte(key))
	path := filepath.Join(writer.root, "worker-payloads", hex.EncodeToString(digest[:])+".json")
	data, err := writer.storage.ReadFile(path)
	if err == nil && bytes.Equal(data, record.Payload) {
		return data, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := writer.storage.WriteFile(path, record.Payload); err != nil {
		return nil, err
	}
	return append([]byte(nil), record.Payload...), nil
}

// ReadWorkerCapturedArtifact streams only the selected committed oversized
// record. The ref is an identity, never a caller-selected storage path.
func (writer *FileWriter) ReadWorkerCapturedArtifact(ctx context.Context, id, ref string) (io.ReadCloser, error) {
	prefix := id + "/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, recordings.ErrInvalidWorkerRecordingRequest
	}
	sequence := strings.TrimPrefix(ref, prefix)
	position, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil || position == 0 || strconv.FormatUint(position, 10) != sequence {
		return nil, recordings.ErrInvalidWorkerRecordingRequest
	}
	catalog, err := writer.LookupWorkerSessionCapture(ctx, id)
	if err != nil {
		return nil, err
	}
	entry := writer.entry(catalog.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, catalog.RecordingID, entry); err != nil {
		return nil, err
	}
	session := entry.sessions[id]
	if session == nil || position > uint64(len(session.records)) {
		return nil, os.ErrNotExist
	}
	record := session.records[position-1]
	if len(record.Payload) <= capturedPayloadLimit {
		return nil, os.ErrNotExist
	}
	data, err := writer.spillCapturedPayload(session, record)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
