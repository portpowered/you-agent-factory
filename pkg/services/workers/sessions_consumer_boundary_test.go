package workers_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

func TestSessionsConsumerCanNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var _ workers.Provider = (workers.Provider)(nil)
	var _ workers.PTYAllocator = (agypty.PTYAllocator)(nil)
	var _ *workers.MockPTYAllocator = (*agypty.MockAllocator)(nil)
	var _ workers.ProviderRegistry = (*providerregistry.Registry)(nil)

	type conductorInvocationFactory = func(
		workers.ProviderRegistry,
		workers.CommandRunner,
		workers.PTYAllocator,
		workers.ProgressPublisher,
	) (workers.InvocationExecutor, error)

	type durableProviderFactory = func(workers.CommandRunner) (workers.Provider, error)

	var _ conductorInvocationFactory
	var _ durableProviderFactory
}
