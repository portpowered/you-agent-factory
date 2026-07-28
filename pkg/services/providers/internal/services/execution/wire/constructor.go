package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	internal "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal"
	acpadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/acp"
)

func New(adapters map[providers.ExecutionKind]execution.Adapter) execution.Service {
	return internal.New(adapters)
}

func NewACP(newCommand platformprocess.CommandFactory) (execution.Service, error) {
	adapter, err := acpadapter.New(newCommand)
	if err != nil {
		return nil, err
	}
	return New(map[providers.ExecutionKind]execution.Adapter{
		providers.ExecutionKindACP: {Execute: adapter.Execute, ExecuteStream: adapter.ExecuteStream},
	}), nil
}
