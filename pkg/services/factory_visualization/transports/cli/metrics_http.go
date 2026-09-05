package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Operation is the injected HTTP-backed operation for one base metrics report.
// The CLI does not read metrics artifacts when this operation is configured.
type Operation func(context.Context, MetricsConfig) error

// MetricsCostReportRequest is the server-scoped input for one Costs read.
// The metrics command deliberately passes only the selected server and
// Factory Session; Costs remains responsible for HTTP mapping and valuation.
type MetricsCostReportRequest struct {
	Server    string
	SessionID string
}

// CostReportOperation reads the existing Costs API report without changing its
// schema or rendering policy.
type CostReportOperation func(context.Context, MetricsCostReportRequest) (generatedclient.CostsReport, error)

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

// SessionEventRequest is the narrow server/event input needed by the metrics
// report. The owning CLI root adapts the existing run replay operation to this
// contract without making the visualization package depend on run.
type SessionEventRequest struct {
	Server      string
	SessionID   string
	Diagnostics io.Writer
	Verbose     bool
}

// SessionEventStream is a finite retained Factory Event replay. Implementations
// must honor context cancellation and return io.EOF after the bounded replay.
type SessionEventStream interface {
	Next(context.Context) (factoryapi.FactoryEvent, error)
	Close() error
}

// SessionEventOperation opens the server-owned canonical event lane for one
// selected Factory Session.
type SessionEventOperation interface {
	OpenFactorySessionEvents(context.Context, SessionEventRequest) (SessionEventStream, error)
}

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
		if config.SessionReport {
			return runMetricsSessionOperation(ctx, config, *response.JSON200)
		}
		groupBy, err := normalizeMetricsGroupBy(config.GroupBy)
		if err != nil {
			return err
		}
		output, err := renderMetricsOutput(groupBy, config.SessionID, config.JSON, result)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(config.Output, output); err != nil {
			return fmt.Errorf("write metrics output: %w", err)
		}
		return nil
	}
}

const maxMetricsSessionEvents = 100_000

func runMetricsSessionOperation(
	ctx context.Context,
	config MetricsConfig,
	report generatedclient.MetricsReport,
) error {
	if err := validateMetricsSessionConfig(config); err != nil {
		return err
	}
	if err := validateMetricsSessionScope(report, config.SessionID); err != nil {
		return err
	}
	events, err := readMetricsSessionEvents(ctx, config)
	if err != nil {
		return err
	}
	document, err := reduceMetricsSession(config.SessionID, events)
	if err != nil {
		return newMetricsError(
			MetricsSessionEventsFailedCode,
			"read Factory Session events: retained event data was invalid",
			err,
		)
	}
	var costReport *generatedclient.CostsReport
	if strings.EqualFold(strings.TrimSpace(config.SessionLens), "cost") {
		if config.CostReport == nil {
			return newMetricsError(
				MetricsQueryFailedCode,
				"read Factory Session costs: Costs report operation is required",
				nil,
			)
		}
		report, costErr := config.CostReport(ctx, MetricsCostReportRequest{
			Server:    strings.TrimSpace(config.Server),
			SessionID: strings.TrimSpace(config.SessionID),
		})
		if costErr != nil {
			return costErr
		}
		if err := validateMetricsCostScope(report, config.SessionID); err != nil {
			return err
		}
		costReport = &report
	}
	if config.SessionByWorker || config.SessionByDispatch {
		addMetricsSessionDetails(&document, report.UsageRows, costReport, config.SessionByWorker, config.SessionByDispatch)
	}
	document.Cost = costReport
	output, err := renderMetricsSessionOutput(document, config.JSON)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(config.Output, output); err != nil {
		return fmt.Errorf("write metrics session output: %w", err)
	}
	return nil
}

func validateMetricsCostScope(report generatedclient.CostsReport, sessionID string) error {
	want := strings.TrimSpace(sessionID)
	got := metricStringFromAPI(report.Scope.FactorySessionId)
	if strings.ToUpper(strings.TrimSpace(string(report.Scope.Kind))) != "FACTORY_SESSION" || got != want {
		return newMetricsError(
			MetricsScopeUnavailableCode,
			"the selected Factory Session cost scope was not returned by the server",
			nil,
		)
	}
	return nil
}

func validateMetricsSessionConfig(config MetricsConfig) error {
	if strings.TrimSpace(config.SessionID) == "" {
		return newMetricsError(
			MetricsInvalidRequestCode,
			"metrics session requires a non-empty Factory Session ID",
			nil,
		)
	}
	if lens := strings.TrimSpace(config.SessionLens); lens != "" {
		if !strings.EqualFold(lens, "cost") {
			return newMetricsError(
				MetricsUnsupportedSessionOptionCode,
				fmt.Sprintf("unsupported metrics session lens %q: choose cost", lens),
				nil,
			)
		}
	}
	if config.SessionEvents == nil {
		return newMetricsError(
			MetricsSessionEventsFailedCode,
			"read Factory Session events: canonical event operation is required",
			nil,
		)
	}
	return nil
}

func validateMetricsSessionScope(report generatedclient.MetricsReport, sessionID string) error {
	want := strings.TrimSpace(sessionID)
	kind := strings.ToUpper(strings.TrimSpace(report.Scope.Kind))
	got := metricStringFromAPI(report.Scope.FactorySessionId)
	if kind != "FACTORY_SESSION" || got != want {
		return newMetricsError(
			MetricsScopeUnavailableCode,
			"the selected Factory Session metrics scope was not returned by the server",
			nil,
		)
	}
	return nil
}

func readMetricsSessionEvents(ctx context.Context, config MetricsConfig) ([]factoryapi.FactoryEvent, error) {
	stream, err := config.SessionEvents.OpenFactorySessionEvents(ctx, SessionEventRequest{
		Server:      strings.TrimSpace(config.Server),
		SessionID:   strings.TrimSpace(config.SessionID),
		Diagnostics: config.Diagnostics,
		Verbose:     config.Verbose,
	})
	if err != nil {
		return nil, newMetricsSessionEventsError(err)
	}
	if stream == nil {
		return nil, newMetricsError(
			MetricsSessionEventsFailedCode,
			"read Factory Session events: operation returned an empty stream",
			nil,
		)
	}
	closed := false
	defer func() {
		if !closed {
			_ = stream.Close()
		}
	}()
	events := make([]factoryapi.FactoryEvent, 0)
	for len(events) < maxMetricsSessionEvents {
		event, nextErr := stream.Next(ctx)
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, newMetricsSessionEventsError(nextErr)
		}
		if event.Context.SessionId != nil && strings.TrimSpace(*event.Context.SessionId) != strings.TrimSpace(config.SessionID) {
			return nil, newMetricsError(
				MetricsSessionEventsFailedCode,
				"read Factory Session events: the server returned an event from another session",
				nil,
			)
		}
		events = append(events, event)
	}
	if len(events) == maxMetricsSessionEvents {
		return nil, newMetricsError(
			MetricsSessionEventsFailedCode,
			"read Factory Session events: retained replay exceeded the safety bound",
			nil,
		)
	}
	if err := stream.Close(); err != nil {
		return nil, newMetricsSessionEventsError(err)
	}
	closed = true
	return events, nil
}

func newMetricsSessionEventsError(err error) error {
	message := "read Factory Session events: canonical replay failed"
	if errors.Is(err, context.Canceled) {
		message = "read Factory Session events: request canceled"
	} else if errors.Is(err, context.DeadlineExceeded) {
		message = "read Factory Session events: request timed out"
	}
	return newMetricsError(MetricsSessionEventsFailedCode, message, err)
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
	if config.SessionReport {
		if err := validateMetricsSessionConfig(config); err != nil {
			return err
		}
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

func sortedMetricsSessionDetailKeys(values map[string]*metricsSessionDetailAccumulator) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMetricsSessionSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func metricsSessionIdentityLabel(available bool) string {
	if available {
		return "canonical"
	}
	return "unavailable"
}

func metricsSessionDisplayIdentity(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "unavailable"
	}
	return *value
}

func metricsSessionCostItemsForDispatch(index *metricsSessionCostIndex, key string) []generatedclient.CostsLineItem {
	if index == nil {
		return nil
	}
	if key == "unavailable" {
		return index.unknownDispatch
	}
	return index.byDispatch[key]
}

func metricsSessionItemsIdentity(items []generatedclient.CostsLineItem, provider bool) *string {
	value, complete, conflict := metricsSessionItemsIdentityState(items, provider)
	if !complete || conflict {
		return nil
	}
	return value
}

func metricsSessionIdentityWithItems(
	candidate *string,
	items []generatedclient.CostsLineItem,
	provider bool,
) *string {
	if len(items) == 0 {
		return candidate
	}
	value, complete, conflict := metricsSessionItemsIdentityState(items, provider)
	if !complete || conflict {
		return nil
	}
	if candidate == nil {
		return value
	}
	if value == nil || *candidate != *value {
		return nil
	}
	return candidate
}

func metricsSessionItemsIdentityState(
	items []generatedclient.CostsLineItem,
	provider bool,
) (*string, bool, bool) {
	var value string
	conflict := false
	complete := true
	for _, item := range items {
		candidate := ""
		if provider {
			candidate = metricStringFromAPI(item.Provider)
		} else {
			candidate = metricStringFromAPI(item.Model)
		}
		if candidate == "" {
			complete = false
		}
		mergeMetricsSessionIdentity(&value, &conflict, candidate)
	}
	return optionalMetricsSessionString(value), complete, conflict
}

func metricsSessionItemsWorkIDsSet(items []generatedclient.CostsLineItem) map[string]struct{} {
	values := make(map[string]struct{})
	for _, item := range items {
		if workID := metricStringFromAPI(item.WorkId); workID != "" {
			values[workID] = struct{}{}
		}
	}
	return values
}

func metricsSessionItemsWorkIDs(items []generatedclient.CostsLineItem) []string {
	return sortedMetricsSessionSet(metricsSessionItemsWorkIDsSet(items))
}
