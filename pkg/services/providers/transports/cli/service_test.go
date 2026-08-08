package cli_test

import (
	"context"
	"sync"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

// recordingProvidersRoot is shared by parallel parity subtests, so its call
// counters are mutated from more than one goroutine and must be guarded.
type recordingProvidersRoot struct {
	providers.Service
	mu                 sync.Mutex
	listProvidersCalls int
	getProviderCalls   int
	executeCalls       int
	listResult         providers.ListProvidersResult
	listErr            error
	listFn             func(context.Context) (providers.ListProvidersResult, error)
	getResult          providers.GetProviderResult
	getErr             error
	getFn              func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error)
}

func (fake *recordingProvidersRoot) ListProviders(
	ctx context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	fake.mu.Lock()
	fake.listProvidersCalls++
	fake.mu.Unlock()
	if fake.listFn != nil {
		return fake.listFn(ctx)
	}
	if fake.listErr != nil {
		return providers.ListProvidersResult{}, fake.listErr
	}
	return fake.listResult, nil
}

func (fake *recordingProvidersRoot) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	fake.mu.Lock()
	fake.getProviderCalls++
	fake.mu.Unlock()
	if fake.getFn != nil {
		return fake.getFn(ctx, request)
	}
	if fake.getErr != nil {
		return providers.GetProviderResult{}, fake.getErr
	}
	return fake.getResult, nil
}

func (fake *recordingProvidersRoot) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.mu.Lock()
	fake.executeCalls++
	fake.mu.Unlock()
	return providers.ExecuteResult{}, nil
}

func TestNewRequiresProvidersRoot(t *testing.T) {
	t.Parallel()

	if service := providerscli.New(nil); service != nil {
		t.Fatalf("New(nil) = %T, want nil", service)
	}
}

func TestConstructedService_IsInertAgainstProvidersRoot(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{}
	service := providerscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Providers CLI service")
	}
	listCalls, getCalls, executeCalls := root.callCounts()
	if listCalls != 0 {
		t.Fatalf("ListProviders calls = %d, want 0 during construction", listCalls)
	}
	if getCalls != 0 {
		t.Fatalf("GetProvider calls = %d, want 0 during construction", getCalls)
	}
	if executeCalls != 0 {
		t.Fatalf("Execute calls = %d, want 0 during construction", executeCalls)
	}
}

// callCounts returns a consistent snapshot of the recorded call counters.
func (fake *recordingProvidersRoot) callCounts() (list, get, execute int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.listProvidersCalls, fake.getProviderCalls, fake.executeCalls
}
