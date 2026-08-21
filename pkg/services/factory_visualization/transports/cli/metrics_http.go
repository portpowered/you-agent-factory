package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

// Operation is the injected HTTP-backed operation for one base metrics report.
// The CLI does not read metrics artifacts when this operation is configured.
type Operation func(context.Context, MetricsConfig) error

// Client is the narrow generated response-aware capability used by the base
// metrics command. Wire owns client construction and server selection.
type Client interface {
	GetMetricsWithResponse(
		context.Context,
		*generatedclient.GetMetricsParams,
		...generatedclient.RequestEditorFn,
	) (*generatedclient.GetMetricsClientResponse, error)
}

// ClientFactory constructs one generated client for the selected server.
type ClientFactory func(string) (Client, error)

// NewOperation binds the generated HTTP client factory to the base metrics
// command. It renders only after the complete report and any error response
// have been decoded, so failures cannot leave partial stdout.
func NewOperation(factory ClientFactory) Operation {
	return func(ctx context.Context, config MetricsConfig) error {
		if err := validateMetricsOperationConfig(ctx, config); err != nil {
			return err
		}
		if factory == nil {
			return newMetricsError(MetricsQueryFailedCode, "query runtime metrics: client factory is required", nil)
		}
		client, err := factory(strings.TrimSpace(config.Server))
		if err != nil {
			return newMetricsError(MetricsQueryFailedCode, "build metrics client", err)
		}
		if client == nil {
			return newMetricsError(MetricsQueryFailedCode, "build metrics client: client is required", nil)
		}
		response, err := client.GetMetricsWithResponse(ctx, metricsRequestParams(config.SessionID))
		if err != nil {
			return newMetricsError(MetricsQueryFailedCode, "query runtime metrics", err)
		}
		if response == nil || response.JSON200 == nil {
			return metricsResponseError(response)
		}
		result := metricsReportFromAPI(*response.JSON200)
		output, err := renderMetricsOutput(config.GroupBy, config.SessionID, config.JSON, result)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(config.Output, output); err != nil {
			return fmt.Errorf("write metrics output: %w", err)
		}
		return nil
	}
}

// RunMetricsOperation invokes the generated-client-backed operation after
// validating the command boundary.
func RunMetricsOperation(ctx context.Context, operation Operation, config MetricsConfig) error {
	if err := validateMetricsOperationConfig(ctx, config); err != nil {
		return err
	}
	if operation == nil {
		return newMetricsError(MetricsQueryFailedCode, "query runtime metrics: operation is required", nil)
	}
	return operation(ctx, config)
}

func validateMetricsOperationConfig(ctx context.Context, config MetricsConfig) error {
	if ctx == nil {
		return fmt.Errorf("query runtime metrics: context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("render metrics: output writer is required")
	}
	if strings.TrimSpace(config.Server) == "" {
		return newMetricsError(MetricsQueryFailedCode, "query runtime metrics: server is required", nil)
	}
	if _, err := normalizeMetricsGroupBy(config.GroupBy); err != nil {
		return err
	}
	return nil
}

func metricsRequestParams(sessionID string) *generatedclient.GetMetricsParams {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &generatedclient.GetMetricsParams{SessionId: &sessionID}
}

func metricsResponseError(response *generatedclient.GetMetricsClientResponse) error {
	if response == nil {
		return newMetricsError(MetricsQueryFailedCode, "query runtime metrics: empty server response", nil)
	}
	if response.JSON404 != nil {
		return newMetricsError(
			MetricsSessionNotFoundCode,
			metricsResponseMessage(response.JSON404.Message, "the requested Factory Session was not found"),
			nil,
		)
	}
	if response.JSON503 != nil {
		return newMetricsError(
			MetricsScopeUnavailableCode,
			metricsResponseMessage(response.JSON503.Message, "the selected Factory Session metrics scope is unavailable"),
			nil,
		)
	}
	if response.JSON400 != nil {
		return newMetricsError(
			MetricsInvalidRequestCode,
			metricsResponseMessage(response.JSON400.Message, "the metrics request was invalid"),
			nil,
		)
	}
	if response.StatusCode() != 0 {
		return newMetricsError(
			MetricsQueryFailedCode,
			fmt.Sprintf("query runtime metrics: server returned HTTP %d", response.StatusCode()),
			nil,
		)
	}
	return newMetricsError(MetricsQueryFailedCode, "query runtime metrics: response did not contain a metrics report", nil)
}

func metricsResponseMessage(message, fallback string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fallback
	}
	return message
}

func metricsReportFromAPI(report generatedclient.MetricsReport) factoryvisualization.RuntimeMetricsQueryResult {
	return factoryvisualization.RuntimeMetricsQueryResult{
		Cost:         factoryvisualization.RuntimeMetricsCost{Availability: factoryvisualization.RuntimeMetricsCostAvailability(report.Cost.Availability)},
		Totals:       metricsAggregateFromAPI(report.Totals),
		Workstations: metricsBreakdownsFromAPI(report.Workstations),
		WorkerTypes:  metricsBreakdownsFromAPI(report.WorkerTypes),
		Providers:    metricsBreakdownsFromAPI(report.Providers),
		UsageRows:    metricsUsageRowsFromAPI(report.UsageRows),
	}
}

func metricsAggregateFromAPI(value generatedclient.MetricsAggregate) factoryvisualization.RuntimeMetricsAggregate {
	return factoryvisualization.RuntimeMetricsAggregate{
		InputTokens:         value.InputTokens,
		OutputTokens:        value.OutputTokens,
		CompletedDispatches: value.CompletedDispatches,
		FailuresByReason:    cloneMetricFailures(value.FailuresByReason),
		DispatchDuration:    metricsDurationFromAPI(value.DispatchLatency),
		ProviderDuration:    metricsDurationFromAPI(value.ProviderLatency),
	}
}

func metricsDurationFromAPI(value generatedclient.MetricsDuration) *factoryvisualization.RuntimeMetricsDuration {
	unit := strings.TrimSpace(value.Unit)
	if unit == "" {
		unit = "milliseconds"
	}
	return &factoryvisualization.RuntimeMetricsDuration{
		Unit:    unit,
		Samples: value.Samples,
		P50:     value.P50,
		P95:     value.P95,
	}
}

func metricsBreakdownsFromAPI(values []generatedclient.MetricsBreakdown) []factoryvisualization.RuntimeMetricsBreakdown {
	result := make([]factoryvisualization.RuntimeMetricsBreakdown, 0, len(values))
	for _, value := range values {
		result = append(result, factoryvisualization.RuntimeMetricsBreakdown{
			Key:       value.Key,
			Aggregate: metricsAggregateFromAPI(value.Aggregate),
		})
	}
	return result
}

func metricsUsageRowsFromAPI(values []generatedclient.MetricsUsageRow) []factoryvisualization.RuntimeMetricsUsageRow {
	result := make([]factoryvisualization.RuntimeMetricsUsageRow, 0, len(values))
	for _, value := range values {
		result = append(result, factoryvisualization.RuntimeMetricsUsageRow{
			FactorySessionID:      metricStringFromAPI(value.FactorySessionId),
			WorkID:                metricStringFromAPI(value.WorkId),
			DispatchID:            metricStringFromAPI(value.DispatchId),
			WorkerSessionID:       metricStringFromAPI(value.WorkerSessionId),
			Provider:              metricStringFromAPI(value.Provider),
			Model:                 metricStringFromAPI(value.Model),
			InputTokens:           value.InputTokens,
			OutputTokens:          value.OutputTokens,
			CachedInputTokens:     value.CachedInputTokens,
			ReasoningOutputTokens: value.ReasoningOutputTokens,
		})
	}
	return result
}

func metricStringFromAPI(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
