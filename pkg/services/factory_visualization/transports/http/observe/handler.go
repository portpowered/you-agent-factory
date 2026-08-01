package observe

import (
	"go.uber.org/zap"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/binding"
)

// Adapter owns the observation-specific HTTP representation and invocation
// boundary for Factory Visualization.
type Adapter struct {
	visualization *binding.Handler
	logger        *zap.Logger
}

// NewHandler constructs an observation HTTP adapter over one injected root
// binding. The binding is shared with the parent transport facade.
func NewHandler(root *binding.Handler, logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{visualization: root, logger: logger}
}

func (a *Adapter) root() (factoryvisualization.Root, error) {
	if a == nil {
		return binding.New(nil).Require()
	}
	return a.visualization.Require()
}
