package workersessions

import (
	"context"
	"errors"
	"io"
)

// LogsService reads finite, committed activity without consulting provider files.
type LogsService interface {
	ReadLogs(context.Context, ReadLogsRequest) (LogPage, error)
}

type LogsArtifactService interface {
	ReadLogsArtifact(context.Context, string, string) (io.ReadCloser, error)
}

// CapturedObservationReader inspects durable terminal identity without granting
// live execution authority. Histories without a terminal remain available as logs.
type CapturedObservationReader interface {
	GetObservationByWorkerSessionID(context.Context, GetObservationByWorkerSessionIDRequest) (Observation, error)
}

type ReadLogsRequest struct {
	WorkerSessionID string
	Limit           int
	NextToken       string
}

func (r ReadLogsRequest) Validate() error {
	if !validSessionID(r.WorkerSessionID) {
		return ErrInvalidSessionID
	}
	if r.Limit < 0 || r.Limit > 1000 {
		return ErrInvalidLogsRequest
	}
	return nil
}

// LogPage contains a head-pinned captured prefix. Health describes capture,
// independently of execution success or whether the worker is still active.
type LogPage struct {
	TerminalPosition      uint64
	FactorySessionID      string
	WorkIDs               []string
	WorkerSessionID       string
	RecordingGenerationID string
	CommittedPosition     uint64
	Health                string
	Events                []ObservationEvent
	NextToken             string
}

var (
	ErrInvalidLogsRequest = errors.New("worker session logs: invalid limit or cursor")
	ErrLogsUnavailable    = errors.New("worker session logs: recording unavailable")
)
