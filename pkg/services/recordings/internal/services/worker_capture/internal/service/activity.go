package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type activityCursor struct {
	Profile    string `json:"profile"`
	Session    string `json:"session"`
	Generation string `json:"generation"`
	Head       uint64 `json:"head"`
	After      uint64 `json:"after"`
}

func (writer *FileWriter) catalogEntry(session *recordingSession) recordings.WorkerSessionCatalogEntry {
	var draft workers.Draft
	var opening workers.SessionPayload
	_ = json.Unmarshal(session.records[0].Payload, &draft)
	_ = json.Unmarshal(draft.Payload, &opening)
	origin := "direct"
	if opening.FactorySessionID != "" {
		origin = "factory"
	}
	return recordings.WorkerSessionCatalogEntry{
		Version: 1, WorkerSessionID: session.projection.WorkerSessionID,
		RecordingID: session.projection.RecordingID, RecordingGenerationID: session.generation,
		Origin: origin, FactorySessionID: opening.FactorySessionID, OwnerEpoch: session.ownerEpoch,
		CommittedPosition: uint64(session.projection.LastPosition),
	}
}

func (writer *FileWriter) indexSession(session *recordingSession) {
	entry := writer.catalogEntry(session)
	writer.catalogMu.Lock()
	_, indexed := writer.catalog[entry.WorkerSessionID]
	writer.catalog[entry.WorkerSessionID] = entry
	writer.catalogMu.Unlock()
	if indexed || entry.CommittedPosition != 1 {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	digest := sha256.Sum256([]byte(entry.WorkerSessionID))
	// This file is only a cache. A failed replacement leaves the synced journal
	// authoritative and reconstruction below repairs the missing/stale entry.
	_ = writer.storage.WriteFile(filepath.Join(writer.root, "catalog", hex.EncodeToString(digest[:])+".json"), data)
}

// LookupWorkerSessionCapture resolves identity from the store's cached index.
func (writer *FileWriter) LookupWorkerSessionCapture(ctx context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerSessionCatalogEntry{}, err
	}
	if strings.TrimSpace(id) == "" {
		return recordings.WorkerSessionCatalogEntry{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	writer.catalogMu.Lock()
	cached, indexed := writer.catalog[id]
	writer.catalogMu.Unlock()
	if indexed {
		return cached, nil
	}
	if err := writer.rebuildCatalog(ctx); err != nil {
		return recordings.WorkerSessionCatalogEntry{}, err
	}
	writer.catalogMu.Lock()
	entry, ok := writer.catalog[id]
	writer.catalogMu.Unlock()
	if !ok {
		return recordings.WorkerSessionCatalogEntry{}, os.ErrNotExist
	}
	return entry, nil
}

func (writer *FileWriter) rebuildCatalog(ctx context.Context) error {
	writer.rebuildMu.Lock()
	defer writer.rebuildMu.Unlock()
	if writer.catalogLoaded {
		return nil
	}
	err := writer.directory.ScanDirectory(writer.root, 64, func(files []os.DirEntry) error {
		return writer.indexCatalogFiles(ctx, files)
	})
	if errors.Is(err, os.ErrNotExist) {
		writer.catalogLoaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Worker capture index: %w", err)
	}
	writer.catalogLoaded = true
	return nil
}

func (writer *FileWriter) indexCatalogFiles(ctx context.Context, files []os.DirEntry) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.IsDir() || (!strings.HasSuffix(file.Name(), ".worker.jsonl") && !strings.HasSuffix(file.Name(), ".worker.json")) {
			continue
		}
		data, err := writer.storage.ReadFile(filepath.Join(writer.root, file.Name()))
		if err != nil {
			continue
		}
		id, err := writer.recordingFileIdentity(file.Name(), data)
		if err != nil {
			continue
		}
		writer.rebuildRecordingIndex(ctx, id)
	}
	return nil
}

func (writer *FileWriter) rebuildRecordingIndex(ctx context.Context, id string) {
	// Join this store's append barrier so reconstruction cannot advertise bytes
	// that another goroutine has written but has not yet synchronized.
	entry := writer.entry(id)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if writer.hydrate(ctx, id, entry) != nil {
		return
	}
	for _, session := range entry.sessions {
		if len(session.records) == 0 {
			continue
		}
		indexed := writer.catalogEntry(session)
		writer.catalogMu.Lock()
		if current, ok := writer.catalog[indexed.WorkerSessionID]; !ok || current.CommittedPosition < indexed.CommittedPosition {
			writer.catalog[indexed.WorkerSessionID] = indexed
		}
		writer.catalogMu.Unlock()
	}
}

func (writer *FileWriter) recordingFileIdentity(name string, data []byte) (string, error) {
	var identity struct {
		RecordingID string `json:"recordingId"`
	}
	suffix := ""
	if strings.HasSuffix(name, ".worker.jsonl") {
		data, _, _ = bytes.Cut(data, []byte{'\n'})
		suffix = "l"
	}
	if json.Unmarshal(data, &identity) != nil || identity.RecordingID == "" || filepath.Base(writer.path(identity.RecordingID)+suffix) != name {
		return "", recordings.ErrWorkerRecordingReplay
	}
	return identity.RecordingID, nil
}

// ReadWorkerCapturedActivity returns a finite page from the committed store,
// never the live Events head or a provider-native file.
func (writer *FileWriter) ReadWorkerCapturedActivity(ctx context.Context, request recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	limit := request.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 1000 {
		return recordings.WorkerCapturedActivityPage{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	catalog, err := writer.LookupWorkerSessionCapture(ctx, request.WorkerSessionID)
	if err != nil {
		return recordings.WorkerCapturedActivityPage{}, err
	}
	entry := writer.entry(catalog.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, catalog.RecordingID, entry); err != nil {
		return recordings.WorkerCapturedActivityPage{}, err
	}
	session := entry.sessions[request.WorkerSessionID]
	if session == nil || len(session.records) == 0 {
		return recordings.WorkerCapturedActivityPage{}, os.ErrNotExist
	}
	catalog = writer.catalogEntry(session)
	cursor, err := writer.pageCursor(request.NextToken, catalog)
	if err != nil {
		return recordings.WorkerCapturedActivityPage{}, err
	}
	page := recordings.WorkerCapturedActivityPage{
		Catalog: catalog, Opening: session.records[0].Detached(),
		Health: session.projection.Status, HealthReason: session.projection.Degradation,
		Terminal: cloneWorkerRecordingTerminal(session.projection.ExecutionTerminal),
		Records:  make([]recordings.WorkerCapturedRecord, 0, limit),
	}
	page.Catalog.CommittedPosition = cursor.Head
	if page.Terminal != nil && uint64(page.Terminal.Position) > cursor.Head {
		page.Terminal = nil
	}
	end := min(cursor.After+uint64(limit), cursor.Head)
	for _, record := range session.records[cursor.After:end] {
		captured := recordings.WorkerCapturedRecord{Record: record.Detached()}
		if stamp, ok := session.capturedAt[strconv.FormatUint(uint64(record.ID.Position), 10)]; ok {
			captured.CapturedAt = timePointer(stamp)
		}
		page.Records = append(page.Records, captured)
	}
	page.TokenUsage = capturedUsage(session, cursor.Head)
	page.NextToken, err = cursor.continuation(end, page.Terminal != nil)
	return page, err
}

func (cursor activityCursor) continuation(end uint64, terminal bool) (string, error) {
	// A drained prefix without a captured terminal remains resumable. Interior
	// page tokens pin their head; an at-head token can observe a later commit.
	if end == cursor.Head && terminal {
		return "", nil
	}
	cursor.After = end
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (writer *FileWriter) pageCursor(token string, catalog recordings.WorkerSessionCatalogEntry) (activityCursor, error) {
	digest := sha256.Sum256([]byte(filepath.Clean(writer.root)))
	expected := activityCursor{Profile: hex.EncodeToString(digest[:]), Session: catalog.WorkerSessionID, Generation: catalog.RecordingGenerationID, Head: catalog.CommittedPosition}
	if token == "" {
		return expected, nil
	}
	if len(token) > 4096 {
		return activityCursor{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return activityCursor{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	var cursor activityCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.Profile != expected.Profile || cursor.Session != expected.Session || cursor.Generation != expected.Generation || cursor.Head > expected.Head || cursor.Head == 0 || cursor.After > cursor.Head {
		return activityCursor{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	if cursor.After == cursor.Head {
		cursor.Head = expected.Head
	}
	return cursor, nil
}

func capturedUsage(session *recordingSession, head uint64) *workers.UsagePayload {
	var result *workers.UsagePayload
	for _, record := range session.records[:head] {
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
			continue
		}
		var usage workers.UsagePayload
		if json.Unmarshal(draft.Payload, &usage) == nil {
			result = &usage
		}
	}
	return result
}

func timePointer(value time.Time) *time.Time { stamp := value.UTC(); return &stamp }
