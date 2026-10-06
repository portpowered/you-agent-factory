package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

//nolint:gocyclo // Check identity, health, order and pinned watermarks together before presenting a complete transcript.
func (s *LogReader) capturedTranscript(ctx context.Context, id, scope string) (workersessions.ReadTranscriptResult, error) {
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: id, Limit: 1000}
	var first recordings.WorkerCapturedActivityPage
	var records []recordings.WorkerCapturedRecord
	var previous uint64
	for {
		if err := observationContextError(ctx); err != nil {
			return workersessions.ReadTranscriptResult{}, err
		}
		page, err := s.reader.ReadWorkerCapturedActivity(ctx, request)
		if err != nil {
			return workersessions.ReadTranscriptResult{}, capturedTranscriptError(err)
		}
		if request.NextToken == "" {
			first = page
			if scope != "" && page.Catalog.FactorySessionID != strings.TrimSpace(scope) {
				return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
			}
		} else if page.Catalog != first.Catalog || page.Terminal == nil || *page.Terminal != *first.Terminal {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		if page.Catalog.WorkerSessionID != id || page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil ||
			page.Terminal.Position < 1 || uint64(page.Terminal.Position) > page.Catalog.CommittedPosition {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		for _, record := range page.Records {
			position := uint64(record.Record.ID.Position)
			if position != previous+1 || position > first.Catalog.CommittedPosition || record.Truncated || record.Record.ID.Topic != first.Opening.ID.Topic {
				return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
			}
			previous = position
			records = append(records, record)
		}
		if page.NextToken == "" {
			break
		}
		if len(page.Records) == 0 || page.NextToken == request.NextToken {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		request.NextToken = page.NextToken
	}
	if previous != first.Catalog.CommittedPosition {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	return capturedTranscriptResult(first, records)
}

func capturedTranscriptError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return workersessions.ErrObservationCanceled
	case errors.Is(err, os.ErrNotExist):
		return workersessions.ErrObservationSessionNotFound
	case errors.Is(err, recordings.ErrWorkerRecordingIncomplete):
		return workersessions.ErrObservationTranscriptUnavailable
	default:
		return workersessions.ErrObservationTranscriptProjectionUnavailable
	}
}

func capturedTranscriptResult(page recordings.WorkerCapturedActivityPage, records []recordings.WorkerCapturedRecord) (workersessions.ReadTranscriptResult, error) {
	opening, err := historyOpening(recordings.WorkerCapturedCatalogItem{Catalog: page.Catalog, Opening: page.Opening})
	if err != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	opening, err = capturedTranscriptLatestAttempt(opening, records)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	var terminal workers.SessionPayload
	var draft workers.Draft
	for _, record := range records {
		if record.Record.ID.Position != page.Terminal.Position {
			continue
		}
		if json.Unmarshal(record.Record.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &terminal) != nil ||
			!capturedTerminalDraftMatches(draft, terminal, opening, page.Terminal.Status) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
	}
	if terminal.Continuation == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	ref := providers.SessionRef{Provider: providers.ID(terminal.Continuation.Provider), Kind: terminal.Continuation.Kind, ID: terminal.Continuation.ID}
	if ref.Validate() != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	projected, err := (recordings.WorkerCapturedActivityPage{Records: records}).ProjectTranscript()
	if err != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: opening.WorkerSessionID, ProviderSession: ref, WorkIDs: append([]string(nil), opening.WorkIDs...),
		AttemptID: opening.AttemptID, TurnID: draft.TurnID, State: workersessions.State(page.Terminal.Status),
		Entries: make([]workersessions.TranscriptEntry, len(projected)),
	}
	for index, entry := range projected {
		result.Entries[index] = workersessions.TranscriptEntry{Type: workersessions.TranscriptEntryType(entry.Type), Order: entry.Order,
			Text: entry.Text, Summary: entry.Summary, Name: entry.Name, CallID: entry.CallID, Arguments: entry.Arguments,
			Output: entry.Output, Status: entry.Status, Timestamp: entry.Timestamp, TurnIndex: entry.TurnIndex}
	}
	if result.Validate() != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	return result, nil
}

func (s *LogReader) transcriptByProvider(ctx context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	request := recordings.WorkerCapturedCatalogRequest{Limit: 1000}
	selectedID := ""
	for {
		if err := observationContextError(ctx); err != nil {
			return workersessions.ReadTranscriptResult{}, err
		}
		page, err := s.reader.ListWorkerSessionCaptures(ctx, request)
		if err != nil {
			return workersessions.ReadTranscriptResult{}, capturedTranscriptError(err)
		}
		for _, item := range page.Items {
			if req.FactorySessionID != "" && item.Catalog.FactorySessionID != strings.TrimSpace(req.FactorySessionID) {
				continue
			}
			if !capturedTranscriptTupleMatches(item, req.ProviderSession) {
				continue
			}
			// Preserve the legacy deterministic lookup when continuation attempts
			// share a provider tuple. Only one exact Worker journal is ever reduced.
			if selectedID == "" || item.Catalog.WorkerSessionID < selectedID {
				selectedID = item.Catalog.WorkerSessionID
			}
		}
		if page.NextToken == "" {
			break
		}
		if page.NextToken == request.NextToken {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		request.NextToken = page.NextToken
	}
	if selectedID == "" {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
	}
	result, err := s.capturedTranscript(ctx, selectedID, req.FactorySessionID)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if result.ProviderSession != req.ProviderSession {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	return result, nil
}

func capturedTranscriptTupleMatches(item recordings.WorkerCapturedCatalogItem, ref providers.SessionRef) bool {
	if item.Terminal == nil {
		return false
	}
	for _, record := range item.MetadataRecords {
		if record.ID.Position != item.Terminal.Position {
			continue
		}
		var draft workers.Draft
		var payload workers.SessionPayload
		if json.Unmarshal(record.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &payload) != nil || payload.Continuation == nil {
			return false
		}
		continuation := payload.Continuation
		return providers.SessionRef{Provider: providers.ID(continuation.Provider), Kind: continuation.Kind, ID: continuation.ID} == ref
	}
	return false
}

func capturedTerminalDraftMatches(draft workers.Draft, terminal, opening workers.SessionPayload, status string) bool {
	phase, err := terminalPhase(workersessions.State(status))
	return err == nil && draft.Kind == workers.KindSession && draft.Phase == phase &&
		draft.DispatchID == opening.AttemptID && terminal.Status == status &&
		(terminal.WorkerSessionID == "" || terminal.WorkerSessionID == opening.WorkerSessionID) &&
		(terminal.AttemptID == "" || terminal.AttemptID == opening.AttemptID) &&
		(terminal.FactorySessionID == "" || terminal.FactorySessionID == opening.FactorySessionID)
}

// A retry is a committed change of attempt inside one Worker journal. Validate
// its predecessor before using the final attempt as transcript correlation.
func capturedTranscriptLatestAttempt(opening workers.SessionPayload, records []recordings.WorkerCapturedRecord) (workers.SessionPayload, error) {
	for _, record := range records {
		var draft workers.Draft
		if json.Unmarshal(record.Record.Payload, &draft) != nil {
			return opening, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseUpdated {
			continue
		}
		var payload workers.SessionPayload
		if json.Unmarshal(draft.Payload, &payload) != nil {
			return opening, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		if payload.AttemptReason != workers.AttemptReasonRetry {
			continue
		}
		if payload.ValidateLineage() != nil || payload.Lineage == nil || payload.Lineage.PreviousAttemptID != opening.AttemptID ||
			payload.Lineage.PreviousDispatchID != opening.AttemptID || payload.WorkerSessionID != opening.WorkerSessionID ||
			payload.FactorySessionID != opening.FactorySessionID || payload.AttemptID != draft.DispatchID {
			return opening, workersessions.ErrObservationTranscriptProjectionUnavailable
		}
		opening.AttemptID = payload.AttemptID
	}
	return opening, nil
}
