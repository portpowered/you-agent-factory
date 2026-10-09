package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/events"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (s *LogReader) applyArchivedTerminalCause(ctx context.Context, page recordings.WorkerCapturedActivityPage, observation *workersessions.Observation) {
	if page.Terminal == nil {
		return
	}
	observation.TerminalCause = s.archivedTerminalCause(ctx, page, *observation)
	if page.Health == recordings.WorkerRecordingStatusIncomplete && page.HealthReason == "OWNER_LOST" &&
		page.Terminal.Phase == workers.PhaseFailed && page.Terminal.Position == 0 && observation.State == workersessions.StateFailed {
		cause := "OWNER_LOST"
		observation.TerminalCause = &cause
		observation.Failure = &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone, Detail: "the recorded worker supervisor is no longer alive"}
	}
}

func (s *LogReader) archivedTerminalCause(ctx context.Context, page recordings.WorkerCapturedActivityPage, observation workersessions.Observation) *string {
	if cause := naturalTerminalCause(observation.State); cause != nil {
		return cause
	}
	// Legacy/capture-only readers may lack control rows. They still expose
	// identity and history, but cannot establish an operator cause.
	reader, readable := s.reader.(recordings.WorkerRecordingReader)
	operations, controllable := s.reader.(recordings.WorkerControlOperationStore)
	if !readable || !controllable {
		return nil
	}
	snapshot, err := reader.LoadWorkerRecording(ctx, page.Catalog.RecordingID)
	if err != nil || snapshot.RecordingID != page.Catalog.RecordingID {
		return nil
	}
	attemptID := capturedTerminalAttempt(snapshot, observation.WorkerSessionID, uint64(page.Terminal.Position), observation.State)
	if attemptID == "" {
		return nil
	}
	target := recordings.WorkerControlTarget{
		RecordingID: page.Catalog.RecordingID, WorkerSessionID: observation.WorkerSessionID,
		FactorySessionID: observation.FactorySessionID, RecordingGenerationID: page.Catalog.RecordingGenerationID,
		OwnerEpoch: page.Catalog.OwnerEpoch, ExpectedAttemptID: attemptID,
	}
	records, err := operations.ListWorkerControlOperations(ctx, target)
	if err != nil {
		return nil
	}
	return committedStopCause(records, target, observation.State)
}

func capturedTerminalAttempt(snapshot recordings.WorkerRecordingSnapshot, id string, position uint64, state workersessions.State) string {
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != id {
			continue
		}
		return capturedTerminalAttemptRecords(session.Records, position, state)
	}
	return ""
}

func capturedTerminalAttemptRecords(records []events.Record, position uint64, state workersessions.State) string {
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
	attempt := capturedTerminalAttemptRecords(item.MetadataRecords, uint64(item.Terminal.Position), observation.State)
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
