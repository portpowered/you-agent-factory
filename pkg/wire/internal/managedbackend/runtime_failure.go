package managedbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	runtimeStageBackendExtract = "BACKEND_EXTRACT"
	runtimeStageBackendStart   = "BACKEND_START"
	runtimeFailureCancelled    = "CANCELLED"
	runtimeFailureTimedOut     = "TIMED_OUT"
	runtimeFailureExtraction   = "EXTRACTION_FAILED"
	runtimeFailureProcessStart = "PROCESS_START_FAILED"

	runtimeSubcauseArchiveSelection    = "ARCHIVE_SELECTION"
	runtimeSubcauseArchiveOpen         = "ARCHIVE_OPEN"
	runtimeSubcauseEntryValidation     = "ENTRY_VALIDATION"
	runtimeSubcauseEntryCopy           = "ENTRY_COPY"
	runtimeSubcauseExecutableDiscovery = "EXECUTABLE_DISCOVERY"
	runtimeSubcauseEndpointReservation = "ENDPOINT_RESERVATION"
	runtimeSubcauseCleanup             = "CLEANUP"
)

type runtimeStageError struct {
	stage    string
	class    string
	subcause string
	cause    error
}

func (failure *runtimeStageError) Error() string {
	if failure == nil {
		return ""
	}
	return fmt.Sprintf("model runtime stage failed: %s (%s)", failure.stage, failure.class)
}

func (failure *runtimeStageError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func (failure *runtimeStageError) ModelRuntimeStage() string {
	if failure == nil {
		return ""
	}
	return failure.stage
}

func (failure *runtimeStageError) ModelRuntimeFailureClass() string {
	if failure == nil {
		return ""
	}
	return failure.class
}

func (failure *runtimeStageError) ModelRuntimeFailureSubcause() string {
	if failure == nil {
		return ""
	}
	return failure.subcause
}

type runtimeFailureClassifier interface {
	ModelRuntimeStage() string
	ModelRuntimeFailureClass() string
}

type runtimeFailureSubcauseClassifier interface {
	ModelRuntimeFailureSubcause() string
}

// WrapBackendExtractFailure marks one archive selection/materialization
// operation without exposing the archive path or nested error.
func WrapBackendExtractFailure(subcause string, err error) error {
	if err == nil {
		return nil
	}
	var classifier runtimeFailureClassifier
	if errors.As(err, &classifier) && classifier != nil &&
		classifier.ModelRuntimeStage() != "" &&
		classifier.ModelRuntimeFailureClass() != "" {
		var subcauseClassifier runtimeFailureSubcauseClassifier
		if errors.As(err, &subcauseClassifier) && subcauseClassifier != nil &&
			normalizeRuntimeFailureSubcause(subcauseClassifier.ModelRuntimeFailureSubcause()) != "" {
			return err
		}
	}
	return newBackendExtractFailure(subcause, err)
}

func newBackendExtractFailure(subcause string, err error) error {
	if err == nil {
		return nil
	}
	return &runtimeStageError{
		stage:    runtimeStageBackendExtract,
		class:    runtimeFailureClass(err, runtimeFailureExtraction),
		subcause: normalizeRuntimeFailureSubcause(subcause),
		cause:    err,
	}
}

// WrapBackendStartFailure marks the OS process start boundary while preserving
// the original error for errors.Is/errors.As callers.
func WrapBackendStartFailure(err error) error {
	return wrapRuntimeStageFailure(runtimeStageBackendStart, runtimeFailureClass(err, runtimeFailureProcessStart), err)
}

func wrapRuntimeStageFailure(stage, class string, err error) error {
	if err == nil {
		return nil
	}
	var classifier runtimeFailureClassifier
	if errors.As(err, &classifier) && classifier != nil && classifier.ModelRuntimeStage() != "" && classifier.ModelRuntimeFailureClass() != "" {
		return err
	}
	return &runtimeStageError{stage: stage, class: class, cause: err}
}

func runtimeFailureClass(err error, fallback string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return runtimeFailureCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return runtimeFailureTimedOut
	default:
		return fallback
	}
}

func normalizeRuntimeFailureSubcause(subcause string) string {
	value := strings.ToUpper(strings.TrimSpace(subcause))
	switch value {
	case runtimeSubcauseArchiveSelection,
		runtimeSubcauseArchiveOpen,
		runtimeSubcauseEntryValidation,
		runtimeSubcauseEntryCopy,
		runtimeSubcauseExecutableDiscovery,
		runtimeSubcauseEndpointReservation,
		runtimeSubcauseCleanup:
		return value
	default:
		return ""
	}
}
