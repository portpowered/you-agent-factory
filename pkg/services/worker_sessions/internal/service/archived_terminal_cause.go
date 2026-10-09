package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Catalog summaries already contain the committed terminal record. Listing
// must not clone every session's transcript to select that single fact.
func (s *LogReader) applyArchivedCatalogTerminalCause(ctx context.Context, item recordings.WorkerCapturedCatalogItem, observation *workersessions.Observation) {
	if item.Terminal == nil {
		return
	}
	observation.TerminalCause = naturalTerminalCause(observation.State)
	if observation.TerminalCause == nil && item.Terminal.Position > 0 && uint64(item.Terminal.Position) <= item.Catalog.CommittedPosition {
		attempt := capturedTerminalRecordAttempt(item.MetadataRecords, uint64(item.Terminal.Position), observation.State)
		if attempt != "" {
			observation.TerminalCause = s.archivedControlCause(ctx, item.Catalog, *observation, attempt)
		}
	}
	applyArchivedOwnerLoss(item.Terminal, item.Health, item.HealthReason, observation)
}

func applyArchivedOwnerLoss(terminal *recordings.WorkerRecordingTerminal, health recordings.WorkerRecordingStatus, reason string, observation *workersessions.Observation) {
	if health == recordings.WorkerRecordingStatusIncomplete && reason == "OWNER_LOST" &&
		terminal.Phase == workers.PhaseFailed && terminal.Position == 0 && observation.State == workersessions.StateFailed {
		cause := "OWNER_LOST"
		observation.TerminalCause = &cause
		observation.Failure = &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone, Detail: "the recorded worker supervisor is no longer alive"}
	}
}

func (s *LogReader) archivedControlCause(ctx context.Context, catalog recordings.WorkerSessionCatalogEntry, observation workersessions.Observation, attemptID string) *string {
	operations, ok := s.reader.(recordings.WorkerControlOperationStore)
	if !ok {
		return nil
	}
	target := recordings.WorkerControlTarget{
		RecordingID: catalog.RecordingID, WorkerSessionID: observation.WorkerSessionID,
		FactorySessionID: observation.FactorySessionID, RecordingGenerationID: catalog.RecordingGenerationID,
		OwnerEpoch: catalog.OwnerEpoch, ExpectedAttemptID: attemptID,
	}
	records, err := operations.ListWorkerControlOperations(ctx, target)
	if err != nil {
		return nil
	}
	return committedStopCause(records, target, observation.State)
}

func capturedTerminalRecordAttempt(records []events.Record, position uint64, state workersessions.State) string {
	for _, record := range records {
		if uint64(record.ID.Position) != position || !isTerminalLifecycleRecord(record) {
			continue
		}
		var draft workers.Draft
		var terminal terminalSessionPayload
		phase, err := terminalPhase(state)
		if err == nil && readPendingInterruptOpeningJSON(record.Payload, &draft) == nil &&
			draft.Kind == workers.KindSession && draft.Phase == phase &&
			readPendingInterruptOpeningJSON(draft.Payload, &terminal) == nil && terminal.Status == string(state) {
			return draft.DispatchID
		}
	}
	return ""
}

func applySummaryTerminalCause(summary recordings.WorkerCapturedSummary, observation *workersessions.Observation) {
	item := summary.Capture
	if item.Terminal == nil {
		return
	}
	observation.TerminalCause = naturalTerminalCause(observation.State)
	if item.Health == recordings.WorkerRecordingStatusIncomplete && item.HealthReason == "OWNER_LOST" &&
		item.Terminal.Phase == workers.PhaseFailed && item.Terminal.Position == 0 && observation.State == workersessions.StateFailed {
		cause := "OWNER_LOST"
		observation.TerminalCause = &cause
		observation.Failure = &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone, Detail: "the recorded worker supervisor is no longer alive"}
		return
	}
	if observation.TerminalCause != nil {
		return
	}
	attempt := capturedTerminalRecordAttempt(item.MetadataRecords, uint64(item.Terminal.Position), observation.State)
	if attempt == "" {
		return
	}
	target := recordings.WorkerControlTarget{
		RecordingID: item.Catalog.RecordingID, WorkerSessionID: observation.WorkerSessionID,
		FactorySessionID: observation.FactorySessionID, RecordingGenerationID: item.Catalog.RecordingGenerationID,
		OwnerEpoch: item.Catalog.OwnerEpoch, ExpectedAttemptID: attempt,
	}
	observation.TerminalCause = committedStopCause(summary.ControlOperations, target, observation.State)
}
