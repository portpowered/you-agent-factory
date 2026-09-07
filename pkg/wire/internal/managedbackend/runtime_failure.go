package managedbackend

import (
	"context"
	"errors"
	"fmt"
)

const (
	runtimeStageBackendExtract = "BACKEND_EXTRACT"
	runtimeStageBackendStart   = "BACKEND_START"
	runtimeFailureCancelled    = "CANCELLED"
	runtimeFailureTimedOut     = "TIMED_OUT"
	runtimeFailureExtraction   = "EXTRACTION_FAILED"
	runtimeFailureProcessStart = "PROCESS_START_FAILED"
)

type runtimeStageError struct {
	stage string
	class string
	cause error
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

type runtimeFailureClassifier interface {
	ModelRuntimeStage() string
	ModelRuntimeFailureClass() string
}

// WrapBackendExtractFailure marks archive selection, extraction, and launch
// materialization failures without exposing the archive path or nested error.
func WrapBackendExtractFailure(err error) error {
	return wrapRuntimeStageFailure(runtimeStageBackendExtract, runtimeFailureClass(err, runtimeFailureExtraction), err)
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
