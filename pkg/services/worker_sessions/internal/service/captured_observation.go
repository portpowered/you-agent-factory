package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The terminal stamp measures host capture completion, not provider completion.
// Missing legacy stamps and terminals outside the committed prefix stay unknown.
func applyCapturedTiming(observation *workersessions.Observation, terminal *recordings.WorkerRecordingTerminal, head uint64, stamps map[string]time.Time) {
	if terminal == nil || terminal.Position < 1 || uint64(terminal.Position) > head || observation.StartedAt == nil {
		return
	}
	ended, ok := stamps[strconv.FormatInt(int64(terminal.Position), 10)]
	if !ok || ended.IsZero() || ended.Before(*observation.StartedAt) {
		return
	}
	duration := ended.Sub(*observation.StartedAt)
	observation.EndedAt = &ended
	observation.Duration = &duration
	observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
}

func applyCapturedUsageFacts(observation *workersessions.Observation, draft workers.Draft) {
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated ||
		(draft.DispatchID != "" && draft.DispatchID != observation.AttemptID) {
		return
	}
	if usage, _, ok := usageProjectionFromDraft(draft); ok {
		observation.TokenUsage = usage
	}
	var payload workers.UsagePayload
	if json.Unmarshal(draft.Payload, &payload) == nil && payload.Origin != "SYNTHETIC" && payload.Model != "" {
		observation.Model = &payload.Model
	}
}

// Capture may lag live publication. Only a durable snapshot grants usage facts;
// a missing or unreadable snapshot leaves usage unknown without hiding identity.
func (r *registry) capturedObservationUsage(ctx context.Context, id string) *workersessions.TokenUsage {
	publication := r.publicationFor(id)
	if publication == nil {
		return nil
	}
	publication.mu.Lock()
	recordingID := publication.recordingID
	publication.mu.Unlock()
	if recordingID == "" {
		return nil
	}
	reader, ok := r.recording.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return nil
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, publicWorkerID(id))
	if err != nil || summary.Capture.Catalog.RecordingID != recordingID {
		return nil
	}
	return capturedSessionUsage(recordings.WorkerSessionRecordingSnapshot{Records: summary.Capture.MetadataRecords}, summary.Capture.Catalog.CommittedPosition)
}

func capturedSessionUsage(session recordings.WorkerSessionRecordingSnapshot, head uint64) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
	for _, record := range session.Records {
		if uint64(record.ID.Position) > head {
			continue
		}
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil {
			continue
		}
		if captured, _, ok := usageProjectionFromDraft(draft); ok {
			usage = captured
		}
	}
	return usage
}

// Scoped lists select committed metadata slots for matching physical workers.
// Missing optional capture clears live usage; caller cancellation and damaged
// committed capture fail the whole read. No request loads sibling histories.
func (r *registry) capturedListUsage(ctx context.Context, ids []observationOrder) (map[string]*workersessions.TokenUsage, error) {
	usage := make(map[string]*workersessions.TokenUsage, len(ids))
	reader, supported := r.recording.(recordings.WorkerCapturedSummaryReader)
	for _, item := range ids {
		if err := observationContextError(ctx); err != nil {
			return nil, err
		}
		pub := r.publicationFor(item.id)
		if pub == nil {
			continue
		}
		usage[item.id] = nil
		pub.mu.Lock()
		recordingID := pub.recordingID
		pub.mu.Unlock()
		if recordingID == "" || r.recording == nil {
			continue
		}
		if !supported {
			return nil, workersessions.ErrObservationProjectionUnavailable
		}
		summary, err := reader.LookupWorkerSessionSummary(ctx, publicWorkerID(item.id))
		if err != nil {
			if failure := capturedListReadError(ctx, err); failure != nil {
				return nil, failure
			}
			continue
		}
		capture := summary.Capture
		_, metadata, exists := r.loadObservationState(item.id)
		if !exists || capture.Catalog.FactorySessionID != metadata.factorySessionID || capture.Catalog.RecordingID != recordingID || capture.Catalog.WorkerSessionID != publicWorkerID(item.id) {
			continue
		}
		session := recordings.WorkerSessionRecordingSnapshot{Records: capture.MetadataRecords}
		usage[item.id] = capturedSessionUsage(session, capture.Catalog.CommittedPosition)
	}
	return usage, observationContextError(ctx)
}

func capturedListReadError(ctx context.Context, err error) error {
	if canceled := observationContextError(ctx); canceled != nil {
		return canceled
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return workersessions.ErrObservationCanceled
	}
	if errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		return workersessions.ErrObservationRecordingCorrupt
	}
	return nil
}
