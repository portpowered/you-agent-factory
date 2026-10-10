package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func workerCaptureReader(service workersessions.Service) recordings.WorkerRecordingReader {
	reader, _ := service.(recordings.WorkerRecordingReader)
	return reader
}

// A historical default selector is not a physical owner. Resolve it once at
// activation from the validated opening of the canonically associated attempt,
// then pin that concrete owner for subsequent selected reads. Never resolve an
// explicit scope this way, or accept an opening for a different dispatch.
func prepareCapturedWorkerAliasScopes(scopes map[string]string, reader recordings.WorkerRecordingReader, sources ...[]recordings.FactoryEvent) {
	summaries, ok := reader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return
	}
	for _, source := range sources {
		for _, event := range source {
			if event.Type != recordings.FactoryEventTypeDispatchWorkerSessionAssoc {
				continue
			}
			var association interfaces.DispatchWorkerSessionAssociationEventPayload
			if event.DecodePayload(&association) != nil || scopes[association.WorkerSessionID] != "~default" {
				continue
			}
			summary, err := summaries.LookupWorkerSessionSummary(context.Background(), association.WorkerSessionID)
			if err != nil {
				continue // Retain unresolved provenance; the selected read fails closed.
			}
			if owner := capturedWorkerAliasOwner(summary.Capture, association.WorkerSessionID, stringPointerValue(event.Context.DispatchID)); owner != "" {
				scopes[association.WorkerSessionID] = owner
			}
		}
	}
}

func capturedWorkerAliasOwner(item recordings.WorkerCapturedCatalogItem, workerID, dispatchID string) string {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(item.Opening.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil {
		return ""
	}
	owner := strings.TrimSpace(item.Catalog.FactorySessionID)
	if owner == "" || owner == "~default" || opening.FactorySessionID != owner ||
		item.Catalog.WorkerSessionID != workerID || opening.WorkerSessionID != workerID ||
		opening.DispatchID == "" || opening.DispatchID != dispatchID {
		return ""
	}
	return owner
}

// Scoped lists select only matching capture health from the activated store.
// An empty selection still checks the selected recording's prepared health.
func (s *recordedWorkerSessionObservation) selectedRecordingHealth(ctx context.Context, recorded, live []workersessions.Observation) (map[string]workerRecordingHealth, error) {
	if s.recordingReader == nil || s.recordingID == "" {
		return nil, nil
	}
	reader, ok := s.recordingReader.(recordings.WorkerRecordingHealthReader)
	if !ok {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	ids := make([]string, 0, len(recorded)+len(live))
	seen := make(map[string]bool, cap(ids))
	for _, rows := range [][]workersessions.Observation{recorded, live} {
		for _, row := range rows {
			if !seen[row.WorkerSessionID] {
				seen[row.WorkerSessionID] = true
				ids = append(ids, row.WorkerSessionID)
			}
		}
	}
	snapshot, err := reader.CurrentWorkerRecordingHealth(ctx, s.recordingID, ids)
	if err != nil {
		return nil, recordingHealthLoadError(err)
	}
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	for _, session := range snapshot.Sessions {
		if !seen[session.WorkerSessionID] {
			return nil, workersessions.ErrObservationRecordingCorrupt
		}
	}
	return workerRecordingHealthMap(snapshot, s.recordingID)
}

// Canonical associations establish physical membership, including restored
// attempts rebound to the current Factory Session. Capture is selected by that
// identity and recording, and contributes only committed usage/kill facts.
// Health and confirmation retain their request-owned samples in ListObservations.
func (s *recordedWorkerSessionObservation) withSelectedCapturedIdentity(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	observation.TokenUsage = nil
	if s.factorySessionID != "" {
		observation.FactorySessionID = s.factorySessionID
	}
	if observation.State == workersessions.StateCanceled && s.Service != nil {
		archived, found, err := s.selectedCapturedCancellation(ctx, observation)
		if err != nil || found {
			return archived, err
		}
	}
	if s.recordingReader == nil || s.recordingID == "" {
		return observation, nil
	}
	reader, ok := s.recordingReader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, observation.WorkerSessionID)
	if failure := observationContextError(ctx); failure != nil {
		return workersessions.Observation{}, failure
	}
	if err != nil {
		if failure := recordingHealthLoadError(err); failure != nil {
			return workersessions.Observation{}, failure
		}
		return observation, nil
	}
	item := summary.Capture
	restoredScope := s.restoredWorkerScopes[observation.WorkerSessionID]
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(item.Opening.Payload, &draft) == nil && json.Unmarshal(draft.Payload, &opening) == nil &&
		opening.DispatchID != "" && opening.DispatchID != observation.AttemptID {
		// Legacy IDs can collide across owners. This committed opening cannot
		// enrich the canonical peer's different physical attempt.
		return observation, nil
	}
	if s.captureHasAnotherPhysicalScope(observation.WorkerSessionID, item.Catalog) {
		return observation, nil
	}
	if !s.selectedCaptureMatches(observation.WorkerSessionID, item.Catalog) {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}
	if restoredScope != "" {
		observation, err = s.withRestoredCaptureHealth(observation, item)
		if err != nil {
			return workersessions.Observation{}, err
		}
	}
	observation.TokenUsage = capturedWorkerUsageRecords(item.MetadataRecords, item.Catalog.CommittedPosition)
	if (observation.State == workersessions.StateCompleted || observation.State == workersessions.StateFailed) && s.Service != nil {
		observation, err = s.withSelectedCapturedTerminal(ctx, observation, item)
	}
	return observation, err
}

// Canonical Work state stays independent of execution capture. Only an exact,
// matching terminal enriches its summary; no recording activity is replayed.
func (s *recordedWorkerSessionObservation) withSelectedCapturedTerminal(ctx context.Context, observation workersessions.Observation, item recordings.WorkerCapturedCatalogItem) (workersessions.Observation, error) {
	observation.EndedAt, observation.Duration = nil, nil
	observation.DurationBasis = workersessions.DurationBasisUnavailable
	if item.Terminal == nil || item.Terminal.Position < 1 || uint64(item.Terminal.Position) > item.Catalog.CommittedPosition {
		return observation, nil
	}
	captured, found, err := archivedFactoryWorker(ctx, s.Service, item.Catalog.FactorySessionID, observation.WorkerSessionID)
	if err != nil || !found {
		return observation, err
	}
	if captured.AttemptID != observation.AttemptID || captured.State != observation.State {
		return observation, nil
	}
	observation.StartedAt, observation.EndedAt, observation.Duration = captured.StartedAt, captured.EndedAt, captured.Duration
	observation.DurationBasis = captured.DurationBasis
	observation.ProviderSession, observation.ProviderSessionAvailable = captured.ProviderSession, captured.ProviderSessionAvailable
	observation.Transcript = captured.Transcript
	observation.Failure, observation.TerminalCause = captured.Failure, captured.TerminalCause
	return observation, nil
}

func (s *recordedWorkerSessionObservation) selectedCaptureMatches(workerID string, catalog recordings.WorkerSessionCatalogEntry) bool {
	if catalog.WorkerSessionID != workerID {
		return false
	}
	if restoredScope := s.restoredWorkerScopes[workerID]; restoredScope != "" {
		return catalog.FactorySessionID == restoredScope
	}
	return catalog.RecordingID == s.recordingID
}

func (s *recordedWorkerSessionObservation) captureHasAnotherPhysicalScope(workerID string, catalog recordings.WorkerSessionCatalogEntry) bool {
	scope := s.executionFactorySessionID
	if restored := s.restoredWorkerScopes[workerID]; restored != "" {
		return false // Restored membership requires this exact capture scope.
	}
	return scope != "" && catalog.FactorySessionID != "" && catalog.FactorySessionID != scope
}

// A restored canonical association authorizes this exact physical capture in
// its original scope. The summary has already validated its committed opening.
func (s *recordedWorkerSessionObservation) withRestoredCaptureHealth(observation workersessions.Observation, item recordings.WorkerCapturedCatalogItem) (workersessions.Observation, error) {
	health, err := workerRecordingHealthMap(recordings.WorkerRecordingSnapshot{
		RecordingID: item.Catalog.RecordingID,
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: observation.WorkerSessionID, Status: item.Health,
			Failure: item.HealthReason, InterruptionReason: item.HealthReason,
			Records: []events.Record{item.Opening},
		}},
	}, item.Catalog.RecordingID)
	if err != nil {
		return workersessions.Observation{}, err
	}
	rows := []workersessions.Observation{observation}
	s.decorateRecordingHealth(rows, health)
	return rows[0], nil
}

func (s *recordedWorkerSessionObservation) selectedCapturedCancellation(ctx context.Context, observation workersessions.Observation) (workersessions.Observation, bool, error) {
	scope := observation.FactorySessionID
	if restored := s.restoredWorkerScopes[observation.WorkerSessionID]; restored != "" {
		scope = restored
	}
	archived, found, err := archivedFactoryWorker(ctx, s.Service, scope, observation.WorkerSessionID)
	if failure := observationContextError(ctx); failure != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return workersessions.Observation{}, false, workersessions.ErrObservationCanceled
	}
	if err != nil || !found {
		return workersessions.Observation{}, false, err
	}
	observation.TokenUsage = cloneRecordedTokenUsage(archived.TokenUsage)
	if archived.State == workersessions.StateTerminated && archived.TerminalCause != nil && *archived.TerminalCause == "OPERATOR_KILL" {
		archived.FactorySessionID = observation.FactorySessionID
		return archived, true, nil
	}
	return observation, true, nil
}

// Optional activity reads use exact physical identity and the same list budget.
// Missing, failed or slow transcript capture leaves committed facts intact.
func (s *recordedWorkerSessionObservation) withSelectedCapturedTranscript(ctx, optionalCtx context.Context, observation workersessions.Observation) (workersessions.Observation, error) {
	if _, prepared := s.recordingReader.(recordings.WorkerCapturedSummaryReader); prepared && s.recordingID != "" && (observation.State == workersessions.StateCompleted || observation.State == workersessions.StateFailed) {
		return observation, observationContextError(ctx)
	}
	if s.Service == nil || !observation.State.Terminal() || !observation.ProviderSessionAvailable || optionalCtx.Err() != nil {
		return observation, observationContextError(ctx)
	}
	scope := s.executionFactorySessionID
	if restored := s.restoredWorkerScopes[observation.WorkerSessionID]; restored != "" {
		scope = restored
	}
	if scope == "" {
		scope = observation.FactorySessionID
	}
	transcript, err := s.ReadTranscriptByWorkerSessionID(optionalCtx, workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: scope})
	if failure := observationContextError(ctx); failure != nil {
		return workersessions.Observation{}, failure
	}
	if optionalCtx.Err() == nil && (errors.Is(err, workersessions.ErrObservationCanceled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return workersessions.Observation{}, workersessions.ErrObservationCanceled
	}
	if err == nil && optionalCtx.Err() == nil && selectedCapturedTranscriptMatches(transcript, observation) {
		observation.Transcript = workersessions.TranscriptAvailabilityAvailable
	}
	return observation, nil
}

func selectedCapturedTranscriptMatches(transcript workersessions.ReadTranscriptResult, observation workersessions.Observation) bool {
	return transcript.WorkerSessionID == observation.WorkerSessionID && transcript.ProviderSession == observation.ProviderSession && transcript.AttemptID == observation.AttemptID && transcript.State == observation.State
}

// The committed opening supplies a Work selector, not authority. Membership
// still comes from the selected canonical association in the owning ledger.
// Missing capture correlation uses the ledger's prepared selector; a selected
// storage failure never falls back to canonical replay.
func (s *recordedWorkerSessionObservation) readSelectedCapturedWorker(ctx context.Context, workerID string) (workersessions.Observation, bool, bool, error) {
	if s.recordingID == "" {
		return workersessions.Observation{}, false, false, nil
	}
	reader, ok := s.recordingReader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return workersessions.Observation{}, false, false, nil
	}
	if _, ok := s.ledger.(recordings.WorkerSessionWorkProjectionReader); !ok {
		return workersessions.Observation{}, false, false, nil
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, workerID)
	if failure := observationContextError(ctx); failure != nil {
		return workersessions.Observation{}, false, true, failure
	}
	if err != nil {
		failure := s.selectedCapturedLookupError(ctx, workerID, err)
		return workersessions.Observation{}, false, failure != nil, failure
	}
	var draft workers.Draft
	var opening workers.SessionPayload
	if len(summary.Capture.Opening.Payload) == 0 {
		return workersessions.Observation{}, false, false, nil
	}
	if s.captureHasAnotherPhysicalScope(workerID, summary.Capture.Catalog) {
		return workersessions.Observation{}, false, false, nil
	}
	if json.Unmarshal(summary.Capture.Opening.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil {
		return workersessions.Observation{}, false, true, workersessions.ErrObservationRecordingCorrupt
	}
	if len(opening.WorkIDs) == 0 {
		return workersessions.Observation{}, false, false, nil
	}
	if opening.WorkerSessionID != workerID || summary.Capture.Catalog.WorkerSessionID != workerID {
		return workersessions.Observation{}, false, true, workersessions.ErrObservationRecordingCorrupt
	}
	observation, found, err := s.readSelectedWorkerSummary(ctx, workerID, opening.WorkIDs[0], opening.DispatchID)
	// A colliding capture can belong to a different physical owner. Absence
	// from this Work selection is not absence from the selected canonical
	// ledger; its prepared exact selector still resolves the legacy peer.
	return observation, found, found || err != nil, err
}

func (s *recordedWorkerSessionObservation) selectedCapturedLookupError(ctx context.Context, workerID string, err error) error {
	if errors.Is(err, recordings.ErrWorkerRecordingReplay) && s.Service != nil {
		// Preserve the capture owner's public failure classification rather
		// than reclassifying a damaged unrelated archive as runtime history.
		_, captureErr := s.GetCapturedObservation(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerID})
		if captureErr != nil {
			return captureErr
		}
	}
	return recordingHealthLoadError(err)
}

func (s *recordedWorkerSessionObservation) readSelectedWorkerSummary(ctx context.Context, workerID, workID, dispatchID string) (workersessions.Observation, bool, error) {
	facts, err := s.readSelectedWorkFacts(ctx, workID)
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	return s.readSelectedWorkerFactsSummary(ctx, workerID, dispatchID, facts)
}

func (s *recordedWorkerSessionObservation) readSelectedWorkerFactsSummary(ctx context.Context, workerID, dispatchID string, selected recordings.WorkerSessionWorkFacts) (workersessions.Observation, bool, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, false, err
	}
	// Exact summary reads do not need optional transcript enrichment.
	optionalCtx, cancel := context.WithCancel(ctx)
	cancel()
	rows, _, facts, err := s.projectWorkerSnapshotFacts(ctx, optionalCtx, selected, nil, workerID)
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	if len(rows) == 0 {
		return workersessions.Observation{}, false, nil
	}
	if len(rows) != 1 || (dispatchID != "" && rows[0].AttemptID != dispatchID) {
		return workersessions.Observation{}, false, workersessions.ErrObservationRecordingCorrupt
	}
	health, err := s.selectedRecordingHealth(ctx, rows, nil)
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	s.decorateRecordingHealth(rows, health)
	if err := applySelectedWorkConfirmation(ctx, rows, *facts, s.sampleCompletedFlushWatermark()); err != nil {
		return workersessions.Observation{}, false, err
	}
	return rows[0], true, nil
}

func capturedWorkerUsageRecords(records []events.Record, head uint64) *workersessions.TokenUsage {
	var usage *workersessions.TokenUsage
	for _, record := range records {
		if uint64(record.ID.Position) > head || (head != ^uint64(0) && record.ID.Position < 1) {
			continue
		}
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
			continue
		}
		var captured workersessions.TokenUsage
		if json.Unmarshal(draft.Payload, &captured) != nil ||
			(captured.InputTokens == nil && captured.CachedInputTokens == nil && captured.OutputTokens == nil && captured.ReasoningOutputTokens == nil && captured.TotalTokens == nil) {
			continue
		}
		usage = &captured
	}
	return usage
}

// Replay never reacquires the recorded owner. Recover only the exact committed
// kill disposition from Worker Sessions' archived observation boundary.
func (cfg *runtimeConfig) recoverReplayForce(ctx context.Context, dispatch work.WorkDispatch) error {
	resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver)
	if !ok || cfg.workerSessions == nil {
		return nil
	}
	id, found := resolver.WorkerSessionIDForDispatch(dispatch)
	if !found {
		return nil
	}
	observation, found, err := archivedFactoryWorker(ctx, cfg.workerSessions, canonicalSessionIDFromFactoryConfig(cfg), id)
	if err != nil {
		return fmt.Errorf("recover recorded force disposition for dispatch %s: %w", dispatch.DispatchID, err)
	}
	if found && observation.State == workersessions.StateTerminated && observation.TerminalCause != nil && *observation.TerminalCause == "OPERATOR_KILL" {
		cfg.attempts.recordConfirmedForce(dispatch.DispatchID)
	}
	return nil
}

func archivedFactoryWorker(ctx context.Context, service workersessions.Service, sessionID, workerID string) (workersessions.Observation, bool, error) {
	observation, err := service.GetCapturedObservation(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerID, FactorySessionID: sessionID})
	if errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		return workersessions.Observation{}, false, nil
	}
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	if observation.WorkerSessionID != workerID || observation.FactorySessionID != sessionID {
		return workersessions.Observation{}, false, workersessions.ErrObservationProjectionUnavailable
	}
	return observation.Clone(), true, nil
}
