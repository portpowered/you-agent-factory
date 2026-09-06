package http

import (
	"context"
	"errors"
	"strings"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps the Costs operation into the authored HTTP representation. It
// receives the canonical runtime/configuration paths from the opened runtime;
// it does not read either source or perform valuation itself.
type Adapter struct {
	query    costs.CostsQuery
	resolver factorysessions.RuntimeMetricsScopeResolver
	request  costs.QueryRequest
}

// NewAdapter constructs the Costs HTTP adapter for one opened runtime.
// The optional resolver keeps direct package callers compatible while the
// opened-runtime Wire path supplies the Factory Sessions-owned operation.
func NewAdapter(
	query costs.CostsQuery,
	metricsRoot, operatorSettingsPath string,
	resolvers ...factorysessions.RuntimeMetricsScopeResolver,
) *Adapter {
	if query == nil {
		return nil
	}
	var resolver factorysessions.RuntimeMetricsScopeResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	return &Adapter{
		query:    query,
		resolver: resolver,
		request: costs.QueryRequest{
			MetricsRoot:          strings.TrimSpace(metricsRoot),
			OperatorSettingsPath: strings.TrimSpace(operatorSettingsPath),
		},
	}
}

// GetMetricsCosts invokes the stateless Costs operation for the requested
// optional Factory Session scope and maps its exact result to the API model.
func (a *Adapter) GetMetricsCosts(ctx context.Context, sessionID string) (factoryapi.CostsReport, error) {
	if a == nil || a.query == nil {
		return factoryapi.CostsReport{}, errors.New("Costs query is required")
	}
	request := a.request
	requestedID := strings.TrimSpace(sessionID)
	request.FactorySessionID = requestedID
	request.RetainedFactorySessionIDs = nil
	if requestedID != "" && a.resolver != nil {
		scope, err := a.resolver.ResolveRuntimeMetricsScope(ctx, requestedID)
		if err != nil {
			if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound) {
				return factoryapi.CostsReport{}, newCostsSessionNotFoundError(requestedID, err)
			}
			return factoryapi.CostsReport{}, &costs.QueryError{
				Kind:    costs.QueryErrorMetricsFailed,
				Message: "query runtime costs: resolve retained Factory Session scope",
				Cause:   err,
			}
		} else {
			retainedIDs := normalizedRetainedFactorySessionIDs(scope.RetainedFactorySessionIDs)
			if len(retainedIDs) == 0 {
				return factoryapi.CostsReport{}, &costs.QueryError{
					Kind:    costs.QueryErrorMetricsFailed,
					Message: "query runtime costs: retained Factory Session scope is empty",
				}
			}
			if !containsFactorySessionID(retainedIDs, requestedID) {
				retainedIDs = append(retainedIDs, requestedID)
			}
			request.RetainedFactorySessionIDs = retainedIDs
		}
	}
	report, err := a.query.Query(ctx, request)
	if err != nil {
		return factoryapi.CostsReport{}, err
	}
	return reportToAPI(report), nil
}

func normalizedRetainedFactorySessionIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsFactorySessionID(result, value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func containsFactorySessionID(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
