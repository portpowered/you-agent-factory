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
		writer.catalogOrderID = ""
		return
	}
	if !exists {
		writer.catalogOrderID = ""
	}
	if !exists || current.CommittedPosition <= entry.CommittedPosition {
		writer.catalog[entry.WorkerSessionID] = entry
	}
}

// ListWorkerSessionCaptures enumerates the rebuilt committed identity index.
// No journal is scanned on subsequent pages. Like lookup, this read operation
// intentionally avoids per-request logging on the high-volume capture path.
func (writer *FileWriter) ListWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return writer.listWorkerSessionCaptures(ctx, request)
}

// ListPreparedWorkerSessionCaptures shares catalog membership and paging with
// explicit recording reads, but cannot rebuild or hydrate missing summaries.
func (writer *FileWriter) ListPreparedWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	request.PreparedSummariesOnly = true
	return writer.listWorkerSessionCaptures(ctx, request)
}

func (writer *FileWriter) listWorkerSessionCaptures(ctx context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
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
	if err := writer.prepareCatalogRead(ctx, request.PreparedSummariesOnly); err != nil {
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
	entries, generation := writer.catalogMembership()
	after, err := decodeCatalogCursor(request.NextToken, generation)
	if err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	start, err := catalogPageStart(entries, after)
	if err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	end := min(start+limit, len(entries))
	items, err := writer.capturedCatalogItems(ctx, entries[start:end], generation, request.PreparedSummariesOnly)
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

func catalogPageStart(entries []recordings.WorkerSessionCatalogEntry, after string) (int, error) {
	if after == "" {
		return 0, nil
	}
	start := sort.Search(len(entries), func(i int) bool { return entries[i].WorkerSessionID >= after })
	if start == len(entries) || entries[start].WorkerSessionID != after {
		return 0, recordings.ErrInvalidWorkerRecordingRequest
	}
	return start + 1, nil
}

func (writer *FileWriter) prepareCatalogRead(ctx context.Context, preparedOnly bool) error {
	if !preparedOnly {
		return writer.rebuildCatalog(ctx)
	}
	if err := writer.lockCatalogRebuild(ctx); err != nil {
		return err
	}
	prepared := writer.catalogLoaded
	writer.unlockCatalogRebuild()
	if !prepared {
		return recordings.ErrWorkerRecordingReplay
	}
	return ctx.Err()
}

// catalogMembership returns an immutable identity snapshot. Existing-session
// commits do not change membership: capturedCatalogItem reads their current
// synced facts separately. Reuse sorting and hashing across pages and requests
// until an addition, collision or owner epoch changes the identity set.
// Only CPU work occurs under catalogMu; journal reads use the append barrier.
func (writer *FileWriter) catalogMembership() ([]recordings.WorkerSessionCatalogEntry, string) {
	writer.catalogMu.Lock()
	defer writer.catalogMu.Unlock()
	if writer.catalogOrderID != "" && writer.catalogOrderOwner == writer.ownerEpoch {
		return writer.catalogOrder, writer.catalogOrderID
	}
	entries := make([]recordings.WorkerSessionCatalogEntry, 0, len(writer.catalog))
	for _, entry := range writer.catalog {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].WorkerSessionID < entries[j].WorkerSessionID })
	writer.catalogOrder = entries
	writer.catalogOrderID = writer.catalogGeneration(entries)
	writer.catalogOrderOwner = writer.ownerEpoch
	return entries, writer.catalogOrderID
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
