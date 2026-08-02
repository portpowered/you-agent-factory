package cli_test

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

type recordingProvidersRoot struct {
	providers.Service
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
	fake.listProvidersCalls++
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
	fake.getProviderCalls++
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
	fake.executeCalls++
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
	if root.listProvidersCalls != 0 {
		t.Fatalf("ListProviders calls = %d, want 0 during construction", root.listProvidersCalls)
	}
	if root.getProviderCalls != 0 {
		t.Fatalf("GetProvider calls = %d, want 0 during construction", root.getProviderCalls)
	}
	if root.executeCalls != 0 {
		t.Fatalf("Execute calls = %d, want 0 during construction", root.executeCalls)
	}
}
