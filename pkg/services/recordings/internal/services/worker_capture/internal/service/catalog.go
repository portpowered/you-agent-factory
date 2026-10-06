package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"sort"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type catalogCursor struct {
	Generation string `json:"generation"`
	After      string `json:"after"`
}

const (
	defaultCatalogPageLimit = 64
	maxCatalogPageLimit     = 1000
	maxCatalogTokenBytes    = 4096
)

// acceptCatalogEntry requires catalogMu. The existing ID-only lookup cannot
// select between scopes or generations, so a collision must never choose one.
func (writer *FileWriter) acceptCatalogEntry(entry recordings.WorkerSessionCatalogEntry) {
	if _, ambiguous := writer.ambiguous[entry.WorkerSessionID]; ambiguous {
		return
	}
	current, exists := writer.catalog[entry.WorkerSessionID]
	if exists && (current.RecordingID != entry.RecordingID || current.RecordingGenerationID != entry.RecordingGenerationID || current.FactorySessionID != entry.FactorySessionID) {
		if writer.ambiguous == nil {
			writer.ambiguous = make(map[string]struct{})
		}
		writer.ambiguous[entry.WorkerSessionID] = struct{}{}
		delete(writer.catalog, entry.WorkerSessionID)
		return
	}
	if !exists || current.CommittedPosition <= entry.CommittedPosition {
		writer.catalog[entry.WorkerSessionID] = entry
	}
}

// ListWorkerSessionCaptures enumerates the rebuilt committed identity index.
// No journal is scanned on subsequent pages. Like lookup, this read operation
// intentionally avoids per-request logging on the high-volume capture path.
func (writer *FileWriter) ListWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	limit := request.Limit
	if limit == 0 {
		limit = defaultCatalogPageLimit
	}
	if limit < 1 || limit > maxCatalogPageLimit || len(request.NextToken) > maxCatalogTokenBytes {
		return recordings.WorkerCapturedCatalogPage{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	if err := ctx.Err(); err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	if err := writer.rebuildCatalog(ctx); err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	// Association queries need the complete membership set to prove absence or
	// uniqueness. ID-scoped reads still retain healthy histories and prefixes.
	writer.catalogMu.Lock()
	unproven := writer.catalogDamaged || len(writer.ambiguous) != 0
	writer.catalogMu.Unlock()
	if request.RequireCompleteMembership && unproven {
		return recordings.WorkerCapturedCatalogPage{}, recordings.ErrWorkerRecordingReplay
	}
	entries := writer.catalogEntries()
	generation := writer.catalogGeneration(entries)
	after, err := decodeCatalogCursor(request.NextToken, generation)
	if err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	start := 0
	if after != "" {
		start = sort.Search(len(entries), func(i int) bool { return entries[i].WorkerSessionID >= after })
		if start == len(entries) || entries[start].WorkerSessionID != after {
			return recordings.WorkerCapturedCatalogPage{}, recordings.ErrInvalidWorkerRecordingRequest
		}
		start++
	}
	end := min(start+limit, len(entries))
	items, err := writer.capturedCatalogItems(ctx, entries[start:end], generation)
	if err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	page := recordings.WorkerCapturedCatalogPage{
		Items:        items,
		GenerationID: generation,
	}
	if end < len(entries) {
		data, err := json.Marshal(catalogCursor{Generation: generation, After: entries[end-1].WorkerSessionID})
		if err != nil {
			return recordings.WorkerCapturedCatalogPage{}, err
		}
		page.NextToken = base64.RawURLEncoding.EncodeToString(data)
	}
	return page, ctx.Err()
}

func (writer *FileWriter) catalogEntries() []recordings.WorkerSessionCatalogEntry {
	writer.catalogMu.Lock()
	entries := make([]recordings.WorkerSessionCatalogEntry, 0, len(writer.catalog))
	for _, entry := range writer.catalog {
		entries = append(entries, entry)
	}
	writer.catalogMu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].WorkerSessionID < entries[j].WorkerSessionID })
	return entries
}

func (writer *FileWriter) catalogGeneration(entries []recordings.WorkerSessionCatalogEntry) string {
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	_ = encoder.Encode([]string{filepath.Clean(writer.root), writer.ownerEpoch})
	for _, entry := range entries {
		// Commits within an existing capture do not change catalog membership.
		_ = encoder.Encode([]string{entry.WorkerSessionID, entry.RecordingID, entry.RecordingGenerationID, entry.FactorySessionID})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func decodeCatalogCursor(token, generation string) (string, error) {
	if token == "" {
		return "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", recordings.ErrInvalidWorkerRecordingRequest
	}
	var cursor catalogCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.Generation != generation || cursor.After == "" {
		return "", recordings.ErrInvalidWorkerRecordingRequest
	}
	return cursor.After, nil
}
