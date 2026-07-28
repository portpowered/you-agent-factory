package cli_test

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

type recordingProvidersRoot struct {
	listProvidersCalls int
	getProviderCalls   int
	executeCalls       int
}

func (fake *recordingProvidersRoot) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	fake.listProvidersCalls++
	return providers.ListProvidersResult{}, nil
}

func (fake *recordingProvidersRoot) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	fake.getProviderCalls++
	return providers.GetProviderResult{}, nil
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
