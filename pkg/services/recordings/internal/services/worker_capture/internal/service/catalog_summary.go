package service

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Each slot references an already committed record. Separate slots preserve
// partial SESSION updates (a later title or status cannot erase provider facts).
const (
	summaryUsage = iota
	summaryRunner
	summarySelectionSource
	summaryExecutorProvider
	summaryModelProvider
	summaryContinuation
	summaryPredecessor
	summarySuccessor
	summaryPreviousAttempt
	summaryModel
	summaryReasoning
	summaryStatus
	summaryFactCount
)

func (session *recordingSession) rememberSummary(record events.Record) {
	var draft workers.Draft
	if json.Unmarshal(record.Payload, &draft) != nil || draft.Phase != workers.PhaseUpdated {
		return
	}
	position := uint64(record.ID.Position)
	if draft.Kind == workers.KindUsage {
		var usage workers.UsagePayload
		if json.Unmarshal(draft.Payload, &usage) == nil {
			// Keep only positions in the already retained records, not another
			// payload copy. Pages can select usage at a frozen committed head.
			session.usagePositions = append(session.usagePositions, position)
			if capturedUsageCountersPresent(draft.Payload) {
				session.summaryPositions[summaryUsage] = position
			}
			if usage.Model != "" {
				session.summaryPositions[summaryModel] = position
			}
		}
		return
	}
	if draft.Kind != workers.KindSession {
		return
	}
	var payload workers.SessionPayload
	if json.Unmarshal(draft.Payload, &payload) != nil {
		return
	}
	present := [summaryFactCount]bool{
		summaryContinuation: payload.Continuation != nil,
		summaryModel:        payload.Model != "",
		summaryReasoning:    payload.ReasoningEffort != "",
		summaryStatus:       payload.Status != "",
	}
	if selection := payload.ProviderSelection; selection != nil {
		present[summaryRunner] = selection.RunnerID != ""
		present[summarySelectionSource] = selection.Source != ""
		present[summaryExecutorProvider] = selection.ExecutorProvider != ""
		present[summaryModelProvider] = selection.ModelProvider != ""
	}
	if payload.Lineage != nil {
		present[summaryPredecessor] = payload.Lineage.PredecessorWorkerSessionID != ""
		present[summarySuccessor] = payload.Lineage.SuccessorWorkerSessionID != ""
		present[summaryPreviousAttempt] = payload.Lineage.PreviousDispatchID != ""
	}
	for slot, available := range present {
		if available {
			session.summaryPositions[slot] = position
		}
	}
}

func capturedUsageCountersPresent(payload json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return false
	}
	for _, field := range []string{"inputTokens", "cachedInputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens"} {
		if value := fields[field]; len(value) > 0 && string(value) != "null" {
			return true
		}
	}
	return false
}

func (writer *FileWriter) capturedCatalogItems(ctx context.Context, entries []recordings.WorkerSessionCatalogEntry, generation string) ([]recordings.WorkerCapturedCatalogItem, error) {
	items := make([]recordings.WorkerCapturedCatalogItem, 0, len(entries))
	for _, catalog := range entries {
		item, err := writer.capturedCatalogItem(ctx, catalog)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if _, current := writer.catalogMembership(); current != generation {
		return nil, recordings.ErrInvalidWorkerRecordingRequest
	}
	return items, nil
}

func (writer *FileWriter) capturedCatalogItem(ctx context.Context, catalog recordings.WorkerSessionCatalogEntry) (recordings.WorkerCapturedCatalogItem, error) {
	entry := writer.entry(catalog.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := writer.hydrate(ctx, catalog.RecordingID, entry); err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return recordings.WorkerCapturedCatalogItem{}, canceled
		}
		// Decoder and filesystem errors may contain captured content or paths.
		return recordings.WorkerCapturedCatalogItem{}, recordings.ErrWorkerRecordingReplay
	}
	session := entry.sessions[catalog.WorkerSessionID]
	if session == nil || session.generation != catalog.RecordingGenerationID || len(session.records) == 0 {
		return recordings.WorkerCapturedCatalogItem{}, recordings.ErrWorkerRecordingReplay
	}
	item := session.capturedSummary()
	item.Catalog = writer.catalogEntry(session)
	item.OwnerLost = item.Terminal == nil && session.ownerEpoch != "" && session.ownerEpoch != "historical" && session.ownerEpoch != writer.ownerEpoch
	var err error
	item.SuccessorWorkerSessionID, err = writer.capturedSuccessor(session, item.Catalog)
	if err != nil {
		return recordings.WorkerCapturedCatalogItem{}, err
	}
	return item, ctx.Err()
}

func (session *recordingSession) capturedSummary() recordings.WorkerCapturedCatalogItem {
	if session.catalogSummary == nil {
		item := session.buildCapturedSummary()
		session.catalogSummary = &item
	}
	// The cached selection references immutable committed records. Only the
	// returned page owns mutable payloads/maps; callers cannot poison later
	// lists. Owner and reverse-lineage facts are sampled separately by the
	// writer under their existing barriers, never cached with this selection.
	item := *session.catalogSummary
	item.Opening = item.Opening.Detached()
	item.Terminal = cloneWorkerRecordingTerminal(item.Terminal)
	item.CapturedAt = maps.Clone(item.CapturedAt)
	item.MetadataRecords = make([]events.Record, len(session.catalogSummary.MetadataRecords))
	for index, record := range session.catalogSummary.MetadataRecords {
		item.MetadataRecords[index] = record.Detached()
	}
	return item
}

func (session *recordingSession) buildCapturedSummary() recordings.WorkerCapturedCatalogItem {
	item := recordings.WorkerCapturedCatalogItem{
		Opening:  session.records[0],
		Terminal: cloneWorkerRecordingTerminal(session.projection.ExecutionTerminal),
		Health:   session.projection.Status, HealthReason: session.projection.Degradation,
		CapturedAt: make(map[string]time.Time), MetadataRecords: make([]events.Record, 0, summaryFactCount+1),
	}
	positions := make(map[uint64]struct{}, summaryFactCount+1)
	for _, position := range session.summaryPositions {
		if position > 1 {
			positions[position] = struct{}{}
		}
	}
	if terminal := session.projection.Terminal; terminal != nil && terminal.Position > 1 {
		positions[uint64(terminal.Position)] = struct{}{}
	}
	ordered := make([]uint64, 0, len(positions))
	for position := range positions {
		ordered = append(ordered, position)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	session.copySummaryStamp(item.CapturedAt, 1)
	for _, position := range ordered {
		// The reducer admits contiguous positions starting at one.
		item.MetadataRecords = append(item.MetadataRecords, session.records[position-1])
		session.copySummaryStamp(item.CapturedAt, position)
	}
	return item
}

func (session *recordingSession) copySummaryStamp(stamps map[string]time.Time, position uint64) {
	key := strconv.FormatUint(position, 10)
	if stamp, ok := session.capturedAt[key]; ok {
		stamps[key] = stamp
	}
}

var _ recordings.WorkerCapturedSummaryReader = (*FileWriter)(nil)

// LookupWorkerSessionSummary joins the append barrier, selecting only metadata
// slots already admitted by durable sync. It never retries journal hydration.
func (writer *FileWriter) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	if err := ctx.Err(); err != nil {
		return recordings.WorkerCapturedSummary{}, err
	}
	if strings.TrimSpace(id) == "" {
		return recordings.WorkerCapturedSummary{}, recordings.ErrInvalidWorkerRecordingRequest
	}
	catalog, err := writer.preparedSummaryIdentity(ctx, id)
	if err != nil {
		return recordings.WorkerCapturedSummary{}, err
	}
	entry := writer.entry(catalog.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	session := entry.sessions[id]
	if !entry.loaded || entry.damaged || session == nil || len(session.records) == 0 || session.generation != catalog.RecordingGenerationID {
		return recordings.WorkerCapturedSummary{}, recordings.ErrWorkerRecordingReplay
	}
	// Another journal may have admitted a colliding ID while we joined this
	// append barrier. Never return the earlier, now-ambiguous selection.
	if !writer.summaryCatalogMatches(catalog) {
		return recordings.WorkerCapturedSummary{}, recordings.ErrWorkerRecordingReplay
	}
	item := session.capturedSummary()
	item.Catalog = writer.catalogEntry(session)
	item.OwnerLost = item.Terminal == nil && session.ownerEpoch != "" && session.ownerEpoch != "historical" && session.ownerEpoch != writer.ownerEpoch
	item.SuccessorWorkerSessionID, err = writer.capturedSuccessor(session, item.Catalog)
	if err != nil {
		return recordings.WorkerCapturedSummary{}, err
	}
	return recordings.WorkerCapturedSummary{Capture: item, ControlOperations: entry.summaryControls(item)}, ctx.Err()
}

func (writer *FileWriter) summaryCatalogMatches(catalog recordings.WorkerSessionCatalogEntry) bool {
	writer.catalogMu.Lock()
	defer writer.catalogMu.Unlock()
	current, indexed := writer.catalog[catalog.WorkerSessionID]
	return indexed && current.RecordingID == catalog.RecordingID && current.RecordingGenerationID == catalog.RecordingGenerationID && current.FactorySessionID == catalog.FactorySessionID
}

func (writer *FileWriter) preparedSummaryIdentity(ctx context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	writer.catalogMu.Lock()
	catalog, exists := writer.catalog[id]
	_, ambiguous := writer.ambiguous[id]
	_, unavailable := writer.unavailable[id]
	damaged := writer.catalogDamaged
	writer.catalogMu.Unlock()
	if ambiguous || unavailable {
		return recordings.WorkerSessionCatalogEntry{}, recordings.ErrWorkerRecordingReplay
	}
	if !exists {
		if err := writer.lockCatalogRebuild(ctx); err != nil {
			return recordings.WorkerSessionCatalogEntry{}, err
		}
		prepared := writer.catalogLoaded
		writer.unlockCatalogRebuild()
		if !prepared || damaged {
			return recordings.WorkerSessionCatalogEntry{}, recordings.ErrWorkerRecordingReplay
		}
		return recordings.WorkerSessionCatalogEntry{}, os.ErrNotExist
	}
	return catalog, nil
}

func (entry *recordingEntry) summaryControls(item recordings.WorkerCapturedCatalogItem) []recordings.WorkerControlOperationRecord {
	if item.Terminal == nil || item.Terminal.Position == 0 {
		return nil
	}
	var terminal workers.Draft
	for _, record := range item.MetadataRecords {
		if record.ID.Position == item.Terminal.Position {
			if json.Unmarshal(record.Payload, &terminal) != nil {
				return nil
			}
			break
		}
	}
	if terminal.DispatchID == "" {
		return nil
	}
	catalog := item.Catalog
	target := recordings.WorkerControlTarget{
		WorkerSessionID: catalog.WorkerSessionID, RecordingID: catalog.RecordingID,
		FactorySessionID: catalog.FactorySessionID, RecordingGenerationID: catalog.RecordingGenerationID,
		OwnerEpoch: catalog.OwnerEpoch, ExpectedAttemptID: terminal.DispatchID,
	}
	// Index admission shares the durable append barrier and startup reducer.
	// Only this physical target's facts are copied; sibling histories are never
	// searched. Worker Sessions retains strict result/cause interpretation.
	history := entry.summaryOperations[target]
	result := make([]recordings.WorkerControlOperationRecord, len(history))
	for i, record := range history {
		result[i] = record.Detached()
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Operation.RequestID == result[j].Operation.RequestID {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Operation.RequestID < result[j].Operation.RequestID
	})
	return result
}
