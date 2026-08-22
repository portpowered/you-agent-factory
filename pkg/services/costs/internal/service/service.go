package service

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service owns valuation policy while retaining no report or request state.
// Provider pricing and canonical metrics remain behind two injected owner
// contracts.
type Service struct {
	pricing providers.PriceTableReader
	metrics factoryvisualization.RuntimeMetricsQuery
	logger  logging.Logger
}

// New constructs the stateless Costs operation.
func New(
	pricing providers.PriceTableReader,
	metrics factoryvisualization.RuntimeMetricsQuery,
	logger logging.Logger,
) (costs.CostsQuery, error) {
	switch {
	case pricing == nil:
		return nil, errors.New("construct Costs query: price-table reader is required")
	case metrics == nil:
		return nil, errors.New("construct Costs query: runtime metrics query is required")
	}
	service := &Service{
		pricing: pricing,
		metrics: metrics,
		logger:  logging.EnsureLogger(logger),
	}
	return service.QueryCosts, nil
}

// QueryCosts loads the provider-owned table, queries canonical usage, and
// calculates one deterministic report from detached local accumulators.
func (service *Service) QueryCosts(
	ctx context.Context,
	request costs.QueryRequest,
) (costs.Report, error) {
	if service == nil || service.pricing == nil || service.metrics == nil {
		return costs.Report{}, &costs.QueryError{
			Kind:    costs.QueryErrorInvalidInput,
			Message: "query runtime costs: dependencies are required",
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := request.Validate(); err != nil {
		return costs.Report{}, &costs.QueryError{
			Kind:    costs.QueryErrorInvalidInput,
			Message: "query runtime costs: " + err.Error(),
			Cause:   err,
		}
	}
	if err := ctx.Err(); err != nil {
		return costs.Report{}, err
	}

	sessionID := strings.TrimSpace(request.FactorySessionID)
	runtimeID := strings.TrimSpace(request.RuntimeInstanceID)
	service.logger.Info(
		"runtime costs query started",
		"scope_kind", scopeForRequest(request).Kind,
		"factory_session_id", sessionID,
		"runtime_instance_id", runtimeID,
	)

	table, err := service.readPriceTable()
	if err != nil {
		service.logger.Error(
			"runtime costs query failed",
			"scope_kind", scopeForRequest(request).Kind,
			"factory_session_id", sessionID,
			"status", "PRICING_ERROR",
			"error_kind", queryErrorKind(err),
		)
		return costs.Report{}, err
	}
	metrics, err := service.metrics.QueryRuntimeMetrics(ctx, factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot:       strings.TrimSpace(request.MetricsRoot),
		SessionID:         sessionID,
		RuntimeInstanceID: runtimeID,
	})
	if err != nil {
		wrapped := &costs.QueryError{
			Kind:    costs.QueryErrorMetricsFailed,
			Message: "query runtime costs: query canonical runtime metrics",
			Cause:   err,
		}
		service.logger.Error(
			"runtime costs query failed",
			"scope_kind", scopeForRequest(request).Kind,
			"factory_session_id", sessionID,
			"status", "METRICS_ERROR",
			"error_kind", queryErrorKind(wrapped),
		)
		return costs.Report{}, wrapped
	}
	if err := ctx.Err(); err != nil {
		return costs.Report{}, err
	}

	report, err := calculateReport(ctx, table, metrics.UsageRows, scopeForRequest(request))
	if err != nil {
		wrapped := &costs.QueryError{
			Kind:    costs.QueryErrorInvalidUsage,
			Message: "query runtime costs: calculate valuation",
			Cause:   err,
		}
		service.logger.Error(
			"runtime costs query failed",
			"scope_kind", scopeForRequest(request).Kind,
			"factory_session_id", sessionID,
			"status", "VALUATION_ERROR",
			"error_kind", queryErrorKind(wrapped),
		)
		return costs.Report{}, wrapped
	}
	service.logger.Info(
		"runtime costs query completed",
		"scope_kind", report.Scope.Kind,
		"factory_session_id", sessionID,
		"runtime_instance_id", runtimeID,
		"status", report.Status,
		"encountered_rows", report.Coverage.EncounteredRows,
		"priced_rows", report.Coverage.PricedRows,
		"unpriced_rows", report.Coverage.UnpricedRows,
		"encountered_provider_models", report.Coverage.EncounteredProviderModels,
		"priced_provider_models", report.Coverage.PricedProviderModels,
		"unpriced_provider_models", report.Coverage.UnpricedProviderModels,
	)
	return report, nil
}

func (service *Service) readPriceTable() (providers.PriceTable, error) {
	table, err := service.pricing.ReadPriceTable()
	if err != nil {
		return providers.PriceTable{}, &costs.QueryError{
			Kind:    costs.QueryErrorSettingsReadFailed,
			Message: "query runtime costs: read provider price table",
			Cause:   err,
		}
	}
	table, err = table.Normalize()
	if err != nil {
		return providers.PriceTable{}, &costs.QueryError{
			Kind:    costs.QueryErrorInvalidPriceTable,
			Message: "query runtime costs: validate provider price table",
			Cause:   err,
		}
	}
	return table, nil
}

func scopeForRequest(request costs.QueryRequest) costs.Scope {
	sessionID := strings.TrimSpace(request.FactorySessionID)
	if sessionID == "" {
		return costs.Scope{Kind: costs.ScopeAllFactorySessions}
	}
	return costs.Scope{Kind: costs.ScopeFactorySession, FactorySessionID: sessionID}
}

func queryErrorKind(err error) string {
	var queryErr *costs.QueryError
	if errors.As(err, &queryErr) && queryErr != nil && queryErr.Kind != "" {
		return string(queryErr.Kind)
	}
	return "UNKNOWN"
}
