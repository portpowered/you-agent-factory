package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type LogReader struct {
	reader recordings.WorkerCapturedActivityReader
	logger logging.Logger
}

func NewLogReader(reader recordings.WorkerCapturedActivityReader, logger logging.Logger) *LogReader {
	return &LogReader{reader: reader, logger: logger}
}

func (s *LogReader) ReadLogs(ctx context.Context, req workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	if err := req.Validate(); err != nil {
		return workersessions.LogPage{}, err
	}
	if ctx == nil {
		return workersessions.LogPage{}, workersessions.ErrLogsUnavailable
	}
	page, err := s.reader.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{
		WorkerSessionID: req.WorkerSessionID, Limit: req.Limit, NextToken: req.NextToken,
		BoundPayload: true,
	})
	if err != nil {
		s.logger.Info("worker session logs read", "workerSessionID", req.WorkerSessionID, "outcome", "unavailable")
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return workersessions.LogPage{}, err
		case errors.Is(err, os.ErrNotExist):
			return workersessions.LogPage{}, workersessions.ErrObservationSessionNotFound
		case errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest):
			return workersessions.LogPage{}, workersessions.ErrInvalidLogsRequest
		default:
			return workersessions.LogPage{}, workersessions.ErrLogsUnavailable
		}
	}
	result := workersessions.LogPage{
		WorkerSessionID: page.Catalog.WorkerSessionID, RecordingGenerationID: page.Catalog.RecordingGenerationID,
		CommittedPosition: page.Catalog.CommittedPosition, Health: string(page.Health),
		Events: make([]workersessions.ObservationEvent, 0, len(page.Records)), NextToken: page.NextToken,
	}
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil {
		return workersessions.LogPage{}, workersessions.ErrLogsUnavailable
	}
	result.FactorySessionID = opening.FactorySessionID
	result.WorkIDs = append([]string{}, opening.WorkIDs...)
	if page.Terminal != nil {
		result.TerminalPosition = uint64(page.Terminal.Position)
	}
	for _, captured := range page.Records {
		event := projectObservationEvent(captured.Record, req.WorkerSessionID)
		event.CapturedAt = captured.CapturedAt
		event.Truncated = captured.Truncated
		event.OriginalBytes = captured.OriginalBytes
		event.ReturnedBytes = captured.ReturnedBytes
		event.ArtifactRef = captured.ArtifactRef
		result.Events = append(result.Events, event.Clone())
	}
	// Pages can be requested repeatedly while active; log only safe metadata.
	s.logger.Debug("worker session logs read", "workerSessionID", req.WorkerSessionID, "outcome", "success", "event_count", len(result.Events), "committed_position", result.CommittedPosition)
	return result, nil
}

func (s *LogReader) ReadLogsArtifact(ctx context.Context, id, ref string) (io.ReadCloser, error) {
	if err := (workersessions.ReadLogsRequest{WorkerSessionID: id}).Validate(); err != nil {
		return nil, err
	}
	reader, ok := s.reader.(recordings.WorkerCapturedArtifactReader)
	if !ok || ctx == nil {
		return nil, workersessions.ErrLogsUnavailable
	}
	stream, err := reader.ReadWorkerCapturedArtifact(ctx, id, ref)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, workersessions.ErrObservationSessionNotFound
	case errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest):
		return nil, workersessions.ErrInvalidLogsRequest
	case err != nil:
		return nil, workersessions.ErrLogsUnavailable
	default:
		return stream, nil
	}
}

func (r *registry) ReadLogs(ctx context.Context, req workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	if r.logs == nil {
		return workersessions.LogPage{}, workersessions.ErrLogsUnavailable
	}
	return r.logs.ReadLogs(ctx, req)
}

func (r *registry) ReadLogsArtifact(ctx context.Context, id, ref string) (io.ReadCloser, error) {
	if r.logs == nil {
		return nil, workersessions.ErrLogsUnavailable
	}
	return r.logs.ReadLogsArtifact(ctx, id, ref)
}

func (r *registry) GetCapturedObservation(ctx context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	if r.logs == nil {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	projected, err := r.logs.GetObservationByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return r.withContinuationCapability(ctx, projected), observationContextError(ctx)
}

// NewWithCapturedActivity constructs supervision and durable reads as one service.
func NewWithCapturedActivity(
	execution workers.Service, eventsAppender EventsAppender, logger logging.Logger,
	clock platformclock.Source, scheduler platformclock.TimerSource, recording recordings.WorkerSessionRecordingService,
	logs *LogReader,
	operations recordings.WorkerControlOperationStore,
	restart recordings.WorkerRestartInputStore,
	snapshots *HistorySnapshotBudget,
	continuationSupport providers.Service,
	inspection providersessions.Service,
	tokenEntropy io.Reader,
) (workersessions.Service, error) {
	if snapshots == nil {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	service, err := New(execution, eventsAppender, logger, clock, scheduler, recording, operations, restart, inspection, tokenEntropy)
	if err != nil {
		return nil, err
	}
	service.(*registry).continuationSupport = continuationSupport
	service.(*registry).historySnapshots.HistorySnapshotBudget = snapshots
	service.(*registry).logs = logs
	return service, nil
}

// redactExecutionValue protects the supervised publication boundary even if a
// controlled executor bypasses Providers' own sanitizer. Retired secrets remain
// classified for late callbacks, independently of their revoked authority.
func (r *registry) redactExecutionValue(id string, source, target any) (bool, error) {
	r.mu.RLock()
	secrets := append([]string(nil), r.executionSecrets[id]...)
	r.mu.RUnlock()
	if len(secrets) == 0 {
		return false, nil
	}
	payload, err := json.Marshal(source)
	if err != nil {
		return false, errors.New("worker sessions: execution output cannot be sanitized")
	}
	safe := string(payload)
	for _, secret := range secrets {
		safe = strings.ReplaceAll(safe, secret, "[REDACTED]")
	}
	if safe == string(payload) {
		return false, nil
	}
	if err := json.Unmarshal([]byte(safe), target); err != nil {
		return false, errors.New("worker sessions: execution output cannot be sanitized")
	}
	return true, nil
}

func (r *registry) redactExecutionFragment(id string, fragment workers.ProgressFragment) (workers.ProgressFragment, error) {
	var safe workers.ProgressFragment
	changed, err := r.redactExecutionValue(id, fragment, &safe)
	if err != nil {
		return workers.ProgressFragment{}, err
	}
	if !changed {
		return fragment, nil
	}
	// Preserve the typed canonical draft rather than decode it to an opaque map.
	switch draft := fragment.CanonicalDraft.(type) {
	case workers.Draft:
		clone := workers.CloneDraft(draft)
		if _, err := r.redactExecutionValue(id, draft, &clone); err != nil {
			return workers.ProgressFragment{}, err
		}
		safe.CanonicalDraft = clone
	case *workers.Draft:
		if draft != nil {
			clone := workers.CloneDraft(*draft)
			if _, err := r.redactExecutionValue(id, draft, &clone); err != nil {
				return workers.ProgressFragment{}, err
			}
			safe.CanonicalDraft = &clone
		}
	}
	return safe, nil
}

func (r *registry) redactExecutionResult(id string, result workers.WorkstationDispatchResult, executionErr error) (workers.WorkstationDispatchResult, error) {
	// Include transient content explicitly; its durable encoding omits it.
	source := struct {
		Result         workers.WorkstationDispatchResult
		OutputContent  []work.WorkContentPart
		ProposedOutput *workers.ProposedOutput
	}{result, result.Result.OutputContent, result.ProposedOutput}
	var sanitized struct {
		Result         workers.WorkstationDispatchResult
		OutputContent  []work.WorkContentPart
		ProposedOutput *workers.ProposedOutput
	}
	changed, err := r.redactExecutionValue(id, source, &sanitized)
	if err != nil {
		return workers.WorkstationDispatchResult{DispatchID: result.DispatchID, TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed}, err
	}
	if changed {
		result = sanitized.Result
		result.Result.OutputContent = sanitized.OutputContent
		result.ProposedOutput = sanitized.ProposedOutput
	}
	if executionErr == nil {
		return result, nil
	}
	var text string
	changed, err = r.redactExecutionValue(id, executionErr.Error(), &text)
	if err != nil {
		return result, err
	}
	if changed {
		executionErr = &executionDiagnosticError{cause: executionErr, text: text}
	}
	return result, executionErr
}

type executionDiagnosticError struct {
	cause error
	text  string
}

func (e *executionDiagnosticError) Error() string { return e.text }
func (e *executionDiagnosticError) Unwrap() error { return e.cause }
