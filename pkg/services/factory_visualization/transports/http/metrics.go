package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const (
	MetricsSessionNotFoundCode                        = "METRICS_SESSION_NOT_FOUND"
	MetricsScopeUnavailableCode                       = "METRICS_SESSION_SCOPE_UNAVAILABLE"
	MetricsQueryFailedCode                            = "METRICS_QUERY_FAILED"
	MetricsInvalidRequestCode                         = "METRICS_INVALID_REQUEST"
	metricsScopeNotFound        metricsScopeErrorKind = "NOT_FOUND"
	metricsScopeUnavailable     metricsScopeErrorKind = "UNAVAILABLE"
)

type metricsScopeErrorKind string

// MetricsScopeError is the typed boundary failure for a selected metrics
// scope. Its message is safe for public HTTP and CLI diagnostics; Cause is
// retained for local errors.Is/errors.As inspection.
type MetricsScopeError struct {
	Kind      metricsScopeErrorKind
	SessionID string
	Message   string
	Cause     error
}

func (err *MetricsScopeError) Error() string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	return "metrics session scope is unavailable"
}

func (err *MetricsScopeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func NewMetricsSessionNotFoundError(sessionID string, cause error) error {
	sessionID = strings.TrimSpace(sessionID)
	return &MetricsScopeError{
		Kind:      metricsScopeNotFound,
		SessionID: sessionID,
		Message: fmt.Sprintf(
			"Factory Session %q was not found; use `you session list --scope live` to choose a live ID",
			sessionID,
		),
		Cause: cause,
	}
}

func NewMetricsScopeUnavailableError(sessionID string, cause error) error {
	sessionID = strings.TrimSpace(sessionID)
	return &MetricsScopeError{
		Kind:      metricsScopeUnavailable,
		SessionID: sessionID,
		Message: fmt.Sprintf(
			"Factory Session %q has no retained metrics scope; use `you session list --scope live` to choose a live ID",
			sessionID,
		),
		Cause: cause,
	}
}

// MetricsAdapter maps the stateless Visualization query into the authored
// metrics representation. It is request-local and receives the opened
// runtime's metrics root during composition.
type MetricsAdapter struct {
	query    factoryvisualization.RuntimeMetricsQuery
	resolver factorysessions.RuntimeMetricsScopeResolver
	request  factoryvisualization.RuntimeMetricsQueryRequest
}

func NewMetricsAdapter(
	query factoryvisualization.RuntimeMetricsQuery,
	resolver factorysessions.RuntimeMetricsScopeResolver,
	metricsRoot string,
) *MetricsAdapter {
	if query == nil {
		return nil
	}
	return &MetricsAdapter{
		query:    query,
		resolver: resolver,
		request:  factoryvisualization.RuntimeMetricsQueryRequest{MetricsRoot: strings.TrimSpace(metricsRoot)},
	}
}

func (adapter *MetricsAdapter) GetMetrics(
	ctx context.Context,
	sessionID string,
) (factoryapi.MetricsReport, error) {
	if adapter == nil || adapter.query == nil {
		return factoryapi.MetricsReport{}, errors.New("Factory Visualization metrics query is required")
	}
	requestedID := strings.TrimSpace(sessionID)
	request := adapter.request
	request.SessionID = requestedID
	request.SessionIDs = nil
	if requestedID != "" {
		if adapter.resolver == nil {
			return factoryapi.MetricsReport{}, NewMetricsScopeUnavailableError(requestedID, errors.New("metrics session scope resolver is required"))
		}
		scope, err := adapter.resolver.ResolveRuntimeMetricsScope(ctx, requestedID)
		if err != nil {
			return factoryapi.MetricsReport{}, metricsScopeErrorFromResolver(requestedID, err)
		}
		retainedIDs := normalizedRetainedMetricsIDs(scope.RetainedFactorySessionIDs)
		if len(retainedIDs) == 0 {
			return factoryapi.MetricsReport{}, NewMetricsScopeUnavailableError(requestedID, nil)
		}
		// SessionID is the effective canonical filter for compatibility with
		// existing query consumers; SessionIDs retains the complete resolved set
		// for source generations that contribute to one selected session.
		request.SessionID = retainedIDs[0]
		request.SessionIDs = retainedIDs
	}
	result, err := adapter.query.QueryRuntimeMetrics(ctx, request)
	if err != nil {
		return factoryapi.MetricsReport{}, err
	}
	return metricsReportToAPI(result, requestedID), nil
}

func metricsScopeErrorFromResolver(sessionID string, err error) error {
	if err == nil {
		return nil
	}
	var scopeErr *MetricsScopeError
	if errors.As(err, &scopeErr) && scopeErr != nil {
		return err
	}
	if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound) {
		return NewMetricsSessionNotFoundError(sessionID, err)
	}
	return NewMetricsScopeUnavailableError(sessionID, err)
}

func normalizedRetainedMetricsIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsRetainedMetricsID(result, value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func containsRetainedMetricsID(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// MetricsHandler owns HTTP error mapping and encoding for the Visualization
// metrics operation. Route registration remains in the top-level transport.
type MetricsHandler struct {
	adapter *MetricsAdapter
	logger  *zap.Logger
}

func NewMetricsHandler(adapter *MetricsAdapter, logger *zap.Logger) *MetricsHandler {
	if adapter == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MetricsHandler{adapter: adapter, logger: logger}
}

func (handler *MetricsHandler) GetMetrics(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.GetMetricsParams,
) {
	if handler == nil || handler.adapter == nil {
		handler.writeError(w, http.StatusInternalServerError, MetricsQueryFailedCode, "Factory Visualization metrics handler is unavailable")
		return
	}
	sessionID := ""
	if params.SessionId != nil {
		sessionID = *params.SessionId
	}
	report, err := handler.adapter.GetMetrics(r.Context(), sessionID)
	if err != nil {
		handler.writeQueryError(w, err)
		return
	}
	handler.writeJSON(w, http.StatusOK, report)
}

func (handler *MetricsHandler) writeQueryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := MetricsQueryFailedCode
	message := "failed to query runtime metrics"
	var scopeErr *MetricsScopeError
	if errors.As(err, &scopeErr) && scopeErr != nil {
		message = scopeErr.Error()
		if scopeErr.Kind == metricsScopeNotFound {
			status = http.StatusNotFound
			code = MetricsSessionNotFoundCode
		} else {
			status = http.StatusServiceUnavailable
			code = MetricsScopeUnavailableCode
		}
	} else {
		var queryErr *factoryvisualization.RuntimeMetricsQueryError
		if errors.As(err, &queryErr) && queryErr != nil {
			message = queryErr.Error()
			if queryErr.Kind == factoryvisualization.RuntimeMetricsQueryInvalidInput {
				status = http.StatusBadRequest
				code = MetricsInvalidRequestCode
			}
		}
	}
	handler.writeError(w, status, code, message)
}

func (handler *MetricsHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	handler.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  metricsErrorFamily(status),
		Code:    factoryapi.ErrorResponseCode(code),
	})
}

func (handler *MetricsHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && handler != nil && handler.logger != nil {
		handler.logger.Error("encode metrics response failed", zap.Error(err))
	}
}

func metricsErrorFamily(status int) factoryapi.ErrorFamily {
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		return factoryapi.ErrorFamilyBadRequest
	}
	return factoryapi.ErrorFamilyInternalServerError
}

func metricsReportToAPI(
	result factoryvisualization.RuntimeMetricsQueryResult,
	sessionID string,
) factoryapi.MetricsReport {
	return factoryapi.MetricsReport{
		Scope:        metricsScopeToAPI(sessionID),
		Cost:         factoryapi.MetricsCost{Availability: string(result.Cost.Availability)},
		Totals:       metricsAggregateToAPI(result.Totals),
		Workstations: metricsBreakdownsToAPI(result.Workstations),
		WorkerTypes:  metricsBreakdownsToAPI(result.WorkerTypes),
		Providers:    metricsBreakdownsToAPI(result.Providers),
		UsageRows:    metricsUsageRowsToAPI(result.UsageRows),
	}
}

func metricsScopeToAPI(sessionID string) factoryapi.MetricsScope {
	sessionID = strings.TrimSpace(sessionID)
	scope := factoryapi.MetricsScope{Kind: "ALL_FACTORY_SESSIONS"}
	if sessionID != "" {
		scope.Kind = "FACTORY_SESSION"
		scope.FactorySessionId = &sessionID
	}
	return scope
}

func metricsBreakdownsToAPI(values []factoryvisualization.RuntimeMetricsBreakdown) []factoryapi.MetricsBreakdown {
	result := make([]factoryapi.MetricsBreakdown, 0, len(values))
	for _, value := range values {
		result = append(result, factoryapi.MetricsBreakdown{
			Key:       value.Key,
			Aggregate: metricsAggregateToAPI(value.Aggregate),
		})
	}
	return result
}

func metricsAggregateToAPI(value factoryvisualization.RuntimeMetricsAggregate) factoryapi.MetricsAggregate {
	return factoryapi.MetricsAggregate{
		InputTokens:         value.InputTokens,
		OutputTokens:        value.OutputTokens,
		CompletedDispatches: value.CompletedDispatches,
		FailuresByReason:    cloneMetricFailures(value.FailuresByReason),
		DispatchLatency:     metricsDurationToAPI(value.DispatchDuration),
		ProviderLatency:     metricsDurationToAPI(value.ProviderDuration),
	}
}

func metricsDurationToAPI(value *factoryvisualization.RuntimeMetricsDuration) factoryapi.MetricsDuration {
	if value == nil {
		return factoryapi.MetricsDuration{Unit: "milliseconds"}
	}
	unit := strings.TrimSpace(value.Unit)
	if unit == "" {
		unit = "milliseconds"
	}
	return factoryapi.MetricsDuration{
		Unit:    unit,
		Samples: value.Samples,
		P50:     value.P50,
		P95:     value.P95,
	}
}

func cloneMetricFailures(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{}
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func metricsUsageRowsToAPI(values []factoryvisualization.RuntimeMetricsUsageRow) []factoryapi.MetricsUsageRow {
	result := make([]factoryapi.MetricsUsageRow, 0, len(values))
	for _, value := range values {
		result = append(result, factoryapi.MetricsUsageRow{
			FactorySessionId:      optionalMetricString(value.FactorySessionID),
			WorkId:                optionalMetricString(value.WorkID),
			DispatchId:            optionalMetricString(value.DispatchID),
			WorkerSessionId:       optionalMetricString(value.WorkerSessionID),
			Provider:              optionalMetricString(value.Provider),
			Model:                 optionalMetricString(value.Model),
			InputTokens:           value.InputTokens,
			OutputTokens:          value.OutputTokens,
			CachedInputTokens:     value.CachedInputTokens,
			ReasoningOutputTokens: value.ReasoningOutputTokens,
		})
	}
	return result
}

func optionalMetricString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
