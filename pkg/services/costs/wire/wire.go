// Package wire is the Costs construction boundary.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	costsservice "github.com/portpowered/infinite-you/pkg/services/costs/internal/service"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// NewCostsQuery constructs the stateless Costs operation from its two narrow
// owner capabilities and the process logger.
func NewCostsQuery(
	pricing providers.PriceTableReader,
	metrics factoryvisualization.RuntimeMetricsQuery,
	logger logging.Logger,
) (costs.CostsQuery, error) {
	return costsservice.New(pricing, metrics, logger)
}
