package http

import (
	"context"
	"errors"
	"strings"

	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps the Costs operation into the authored HTTP representation. It
// receives the canonical runtime/configuration paths from the opened runtime;
// it does not read either source or perform valuation itself.
type Adapter struct {
	query   costs.CostsQuery
	request costs.QueryRequest
}

// NewAdapter constructs the Costs HTTP adapter for one opened runtime.
func NewAdapter(query costs.CostsQuery, metricsRoot, operatorSettingsPath string) *Adapter {
	if query == nil {
		return nil
	}
	return &Adapter{
		query: query,
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
	request.FactorySessionID = strings.TrimSpace(sessionID)
	report, err := a.query.Query(ctx, request)
	if err != nil {
		return factoryapi.CostsReport{}, err
	}
	return reportToAPI(report), nil
}
