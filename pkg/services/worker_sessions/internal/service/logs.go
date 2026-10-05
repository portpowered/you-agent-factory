package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type LogReader struct {
	reader recordings.WorkerCapturedActivityReader
	logger logging.Logger
}

func NewLogReader(reader recordings.WorkerCapturedActivityReader, logger logging.Logger) (workersessions.LogsService, error) {
	if reader == nil {
		return nil, recordings.ErrMissingWorkerRecordingReader
	}
	return &LogReader{reader: reader, logger: logging.EnsureLogger(logger)}, nil
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
