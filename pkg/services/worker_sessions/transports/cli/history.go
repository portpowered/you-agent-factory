package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// HistoricalWorkerAssociationsReader is the detached Recordings capability
// consumed by the offline history command.
type HistoricalWorkerAssociationsReader interface {
	QueryHistoricalWorkerAssociations(recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error)
}

// HistoryConfig contains one offline Work-scoped historical association read.
type HistoryConfig struct {
	Context      context.Context
	Recording    string
	SessionID    string
	WorkID       string
	OutputFormat string
	JSON         bool
	Output       io.Writer
}

// HistoryOperation reads historical Worker Session associations without
// entering a live Factory Session.
type HistoryOperation func(HistoryConfig) error

type historicalWorkerAssociationsOutput struct {
	FactorySessionID      string                                       `json:"factorySessionId"`
	WorkID                string                                       `json:"workId"`
	Status                recordings.HistoricalWorkerAssociationsState `json:"status"`
	Count                 *int                                         `json:"count,omitempty"`
	WorkerSessionIDs      *[]string                                    `json:"workerSessionIds,omitempty"`
	IncompleteDispatchIDs *[]string                                    `json:"incompleteDispatchIds,omitempty"`
	ErrorCode             *string                                      `json:"errorCode"`
}

// NewHistory binds the public command to one Recordings read capability.
func NewHistory(reader HistoricalWorkerAssociationsReader) HistoryOperation {
	return func(config HistoryConfig) error {
		config.Recording = strings.TrimSpace(config.Recording)
		config.SessionID = strings.TrimSpace(config.SessionID)
		config.WorkID = strings.TrimSpace(config.WorkID)
		jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
		if reader == nil {
			return emitHistoryCLIError(config, jsonOutput, newCLIError(
				"RECORDED_WORKER_HISTORY_UNAVAILABLE",
				"recorded Worker Session history reader is unavailable",
				nil,
			))
		}
		if err := validateHistoryConfig(config, jsonOutput); err != nil {
			return emitHistoryCLIError(config, jsonOutput, err)
		}
		result, err := reader.QueryHistoricalWorkerAssociations(recordings.HistoricalWorkerAssociationsRequest{
			Recording: recordings.HistoricalRecordingIdentity{
				RecordingID: recordings.RecordingID(config.SessionID),
				Artifact:    recordings.RecordingArtifactReference(config.Recording),
			},
			WorkID: config.WorkID,
		})
		if err != nil {
			return emitHistoryCLIError(config, true, newCLIError(
				"RECORDED_WORKER_HISTORY_UNAVAILABLE",
				"failed to query recorded Worker Session history",
				err,
			))
		}
		output := historicalWorkerAssociationsOutput{
			FactorySessionID: result.FactorySessionID,
			WorkID:           result.WorkID,
			Status:           result.State,
		}
		if result.State == recordings.HistoricalWorkerAssociationsAvailable {
			count := result.Count
			output.Count = &count
			workerSessionIDs := result.WorkerSessionIDs
			if workerSessionIDs == nil {
				workerSessionIDs = []string{}
			}
			incompleteDispatchIDs := result.IncompleteDispatchIDs
			if incompleteDispatchIDs == nil {
				incompleteDispatchIDs = []string{}
			}
			output.WorkerSessionIDs = &workerSessionIDs
			output.IncompleteDispatchIDs = &incompleteDispatchIDs
		} else if result.ErrorCode != "" {
			code := result.ErrorCode
			output.ErrorCode = &code
		}
		payload, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("encode recorded Worker Session history: %w", err)
		}
		if config.Output == nil {
			return fmt.Errorf("write recorded Worker Session history: output writer is required")
		}
		if _, err := fmt.Fprintln(config.Output, string(payload)); err != nil {
			return fmt.Errorf("write recorded Worker Session history: %w", err)
		}
		return historyResultFailure(result)
	}
}

func historyResultFailure(result recordings.HistoricalWorkerAssociationsResult) error {
	switch result.State {
	case recordings.HistoricalWorkerAssociationsAvailable:
		return nil
	case recordings.HistoricalWorkerAssociationsGap:
		return newCLIError(
			"RECORDED_WORKER_HISTORY_GAP",
			"recorded Worker Session history contains a completeness gap",
			nil,
		)
	case recordings.HistoricalWorkerAssociationsWorkNotFound:
		return newCLIError("WORK_NOT_FOUND", "Work was not found in the recording", nil)
	default:
		return newCLIError(
			"RECORDED_WORKER_HISTORY_UNAVAILABLE",
			"recorded Worker Session history is unavailable",
			nil,
		)
	}
}

func validateHistoryConfig(config HistoryConfig, jsonOutput bool) error {
	if config.Recording == "" || !filepath.IsAbs(config.Recording) {
		return newCLIError(
			"WORKER_SESSION_HISTORY_INVALID",
			"--recording must be an absolute local file path",
			nil,
		)
	}
	if config.SessionID == "" || config.WorkID == "" {
		return newCLIError(
			"WORKER_SESSION_HISTORY_INVALID",
			"--session and --work-id are required",
			nil,
		)
	}
	if !jsonOutput {
		return newCLIError(
			"WORKER_SESSION_HISTORY_OUTPUT_INVALID",
			"recorded Worker Session history supports only --output json",
			nil,
		)
	}
	return nil
}

func emitHistoryCLIError(config HistoryConfig, jsonOutput bool, err error) error {
	return emitCLIError(ListConfig{Context: config.Context, Output: config.Output}, jsonOutput, err)
}
