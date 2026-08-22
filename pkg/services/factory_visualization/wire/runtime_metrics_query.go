package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
)

// NewRuntimeMetricsQuery constructs the stateless Factory Visualization
// metrics query from the narrow Platform Metrics reader and safe operation
// logger supplied by composition.
func NewRuntimeMetricsQuery(
	reader factoryvisualization.RuntimeMetricsReader,
	logger logging.Logger,
) (factoryvisualization.RuntimeMetricsQuery, error) {
	return internalservice.NewRuntimeMetricsQuery(reader, logger)
}
