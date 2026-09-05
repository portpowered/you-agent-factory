package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	// MetricsInvalidGroupByCode identifies a grouping value that the metrics
	// command cannot interpret.
	MetricsInvalidGroupByCode = "METRICS_INVALID_GROUP_BY"
	// MetricsHomeDirectoryFailedCode identifies a failure resolving the
	// operator home directory used to locate runtime metrics.
	MetricsHomeDirectoryFailedCode = "METRICS_HOME_DIRECTORY_FAILED"
	// MetricsQueryFailedCode identifies a failure reading the metrics query.
	MetricsQueryFailedCode = "METRICS_QUERY_FAILED"
	// MetricsInvalidRequestCode identifies an authored HTTP request rejected by
	// the metrics route.
	MetricsInvalidRequestCode = "METRICS_INVALID_REQUEST"
	// MetricsSessionNotFoundCode identifies an unknown public live session ID.
	MetricsSessionNotFoundCode = "METRICS_SESSION_NOT_FOUND"
	// MetricsScopeUnavailableCode identifies a known session with no retained
	// metrics identity.
	MetricsScopeUnavailableCode = "METRICS_SESSION_SCOPE_UNAVAILABLE"
	// MetricsSessionEventsFailedCode identifies a failure reading the bounded
	// canonical Factory Event replay required by the session report.
	MetricsSessionEventsFailedCode = "METRICS_SESSION_EVENTS_FAILED"
	// MetricsUnsupportedSessionOptionCode identifies a session flag owned by a
	// later metrics lens/detail slice and not implemented by this report spine.
	MetricsUnsupportedSessionOptionCode = "METRICS_UNSUPPORTED_SESSION_OPTION"
)

const (
	metricsQueryCauseInvalidInput  = "invalid query input"
	metricsQueryCauseReadArtifacts = "read artifacts"
	metricsQueryCauseReadFailure   = "read failure"
	metricsQueryCauseTimedOut      = "request timed out"
)

// MetricsError is the safe CLI failure contract for the metrics command. Its
// message is suitable for central diagnostics; Cause remains available to
// callers that need errors.Is/errors.As inspection.
type MetricsError struct {
	Code    string
	Message string
	Cause   error
}

func (err *MetricsError) Error() string {
	if err == nil {
		return ""
	}
	code := strings.TrimSpace(err.Code)
	message := strings.TrimSpace(err.Message)
	if code == "" {
		code = MetricsQueryFailedCode
	}
	if message == "" {
		message = "metrics command failed"
	}
	return fmt.Sprintf("%s: %s", code, message)
}

func (err *MetricsError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// CLIErrorCode exposes the stable code to the central CLI diagnostics
// boundary without coupling that boundary to the concrete metrics error.
func (err *MetricsError) CLIErrorCode() string {
	if err == nil || strings.TrimSpace(err.Code) == "" {
		return MetricsQueryFailedCode
	}
	return strings.TrimSpace(err.Code)
}

// CLIErrorMessage exposes only the already-sanitized message to the central
// CLI diagnostics boundary.
func (err *MetricsError) CLIErrorMessage() string {
	if err == nil || strings.TrimSpace(err.Message) == "" {
		return "metrics command failed"
	}
	return strings.TrimSpace(err.Message)
}

// CLIErrorFamily supplies the public response family through the shared
// FamilyCodedError seam. Invalid command input is a bad request; local
// artifact and environment failures remain internal failures.
func (err *MetricsError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil {
		return ""
	}
	switch err.CLIErrorCode() {
	case MetricsInvalidGroupByCode, MetricsInvalidRequestCode:
		return factoryapi.ErrorFamilyBadRequest
	case MetricsSessionNotFoundCode:
		return factoryapi.ErrorFamilyNotFound
	case MetricsUnsupportedSessionOptionCode:
		return factoryapi.ErrorFamilyBadRequest
	}
	return factoryapi.ErrorFamilyInternalServerError
}

func newMetricsError(code, message string, cause error) *MetricsError {
	return &MetricsError{Code: code, Message: message, Cause: cause}
}

func newMetricsQueryError(err error) *MetricsError {
	return newMetricsError(
		MetricsQueryFailedCode,
		"query Factory Runtime metrics: "+metricsQueryCause(err),
		err,
	)
}

func metricsQueryCause(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return metricsQueryCauseTimedOut
	}
	var queryErr *factoryvisualization.RuntimeMetricsQueryError
	if errors.As(err, &queryErr) {
		switch queryErr.Kind {
		case factoryvisualization.RuntimeMetricsQueryInvalidInput:
			return metricsQueryCauseInvalidInput
		case factoryvisualization.RuntimeMetricsQueryReadFailed:
			return metricsQueryCauseReadArtifacts
		}
	}
	return metricsQueryCauseReadFailure
}
