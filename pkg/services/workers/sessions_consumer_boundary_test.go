package workers_test

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type rootPTYAllocator struct{}

func (rootPTYAllocator) Allocate(
	context.Context,
	workers.PTYProcessLaunch,
	workers.PTYSessionConfig,
) (workers.PTYSession, error) {
	return nil, nil
}

type rootProviderRegistry struct{}

func (rootProviderRegistry) UsesNativeRunner(string) bool                      { return false }
func (rootProviderRegistry) CanonicalIdentity(identity string) (string, error) { return identity, nil }
func (rootProviderRegistry) RunnerIdentities() []string                        { return nil }
func (rootProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	return workers.RunnerMetadata{}, nil
}
func (rootProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{}, nil
}
func (rootProviderRegistry) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
}

func TestSessionsConsumerCanNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var _ workers.Provider = (workers.Provider)(nil)
	var _ workers.PTYAllocator = rootPTYAllocator{}
	var _ workers.PTYAllocator = (*workers.MockPTYAllocator)(nil)
	var _ workers.ProviderRegistry = rootProviderRegistry{}

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
