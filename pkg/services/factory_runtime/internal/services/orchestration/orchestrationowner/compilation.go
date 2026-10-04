package orchestrationowner

import (
	"context"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

type compilationAdapter struct {
	service orchestration.Service
}

// NewCompilation constructs the Runtime orchestration compile port.
func NewCompilation(service orchestration.Service) factoryruntime.OrchestrationCompilation {
	return &compilationAdapter{service: service}
}

func (a *compilationAdapter) Compile(
	ctx context.Context,
	req factoryruntime.OrchestrationCompileRequest,
) (factoryruntime.OrchestrationCompileResult, error) {
	if a == nil || a.service == nil {
		return factoryruntime.OrchestrationCompileResult{}, fmt.Errorf("orchestration compilation is required")
	}
	result, err := a.service.Compile(ctx, orchestration.CompileRequest{
		Config:       req.Config,
		FactoryDir:   req.FactoryDir,
		SourceReader: req.SourceReader,
	})
	if err != nil {
		return factoryruntime.OrchestrationCompileResult{}, err
	}
	return factoryruntime.OrchestrationCompileResult{
		Kind: factoryruntime.OrchestrationKind(result.Kind),
	}, nil
}

func (a *compilationAdapter) CompilePetriNet(
	ctx context.Context,
	req factoryruntime.OrchestrationCompileRequest,
) (*state.Net, error) {
	if a == nil || a.service == nil {
		return nil, fmt.Errorf("orchestration compilation is required")
	}
	result, err := a.service.Compile(ctx, orchestration.CompileRequest{
		Config:       req.Config,
		FactoryDir:   req.FactoryDir,
		SourceReader: req.SourceReader,
	})
	if err != nil {
		return nil, err
	}
	if result.Kind != orchestration.KindPetri {
		return nil, fmt.Errorf("orchestration kind %q is not PETRI", result.Kind)
	}
	net := orchestration.PetriNet(result.Binding)
	if net == nil {
		return nil, fmt.Errorf("orchestration PETRI binding missing compiled net")
	}
	return net, nil
}
