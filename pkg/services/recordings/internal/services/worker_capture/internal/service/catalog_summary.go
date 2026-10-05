package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
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
	if writer.catalogGeneration(writer.catalogEntries()) != generation {
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
	return item, ctx.Err()
}

func (session *recordingSession) capturedSummary() recordings.WorkerCapturedCatalogItem {
	item := recordings.WorkerCapturedCatalogItem{
		Opening:  session.records[0].Detached(),
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
		item.MetadataRecords = append(item.MetadataRecords, session.records[position-1].Detached())
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
