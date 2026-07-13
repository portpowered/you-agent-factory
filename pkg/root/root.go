// Package root selects the process mode and transfers a constructed graph to
// pkg/initializer. Dependency construction belongs to pkg/wire and lifecycle
// execution belongs to pkg/initializer.
package root

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// Inputs contains normalized process selection and explicit graph inputs.
type Inputs struct {
	Mode  initializer.Mode
	Graph wire.Inputs
}

// Start constructs one graph before delegating lifecycle activation to
// initializer. A construction failure cannot expose a partially started app.
func Start(ctx context.Context, inputs Inputs) (*initializer.Application, error) {
	graph, err := wire.Build(ctx, inputs.Graph)
	if err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	application, err := initializer.Start(ctx, inputs.Mode, graph)
	if err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	return application, nil
}
