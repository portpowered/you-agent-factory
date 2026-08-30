package modelhost

import (
	"context"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

type stubPackagedRuntimeHost struct {
	ensureHost apisurface.ModelHostSnapshot
}

func (s stubPackagedRuntimeHost) InspectModelHost(
	_ context.Context,
	request apisurface.InspectModelHostRequest,
) (apisurface.InspectModelHostResult, error) {
	return apisurface.InspectModelHostResult{Host: s.ensureHost.Clone()}, nil
}

func (s stubPackagedRuntimeHost) EnsureModelHost(
	_ context.Context,
	_ apisurface.EnsureModelHostRequest,
) (apisurface.EnsureModelHostResult, error) {
	return apisurface.EnsureModelHostResult{
		Host:    s.ensureHost.Clone(),
		Outcome: apisurface.HostEnsureAlreadyReady,
	}, nil
}

func (s stubPackagedRuntimeHost) StopModelHost(
	context.Context,
	apisurface.StopModelHostRequest,
) (apisurface.StopModelHostResult, error) {
	return apisurface.StopModelHostResult{}, apisurface.ErrUnsupportedOperation
}

func (stubPackagedRuntimeHost) AcquireModelLease(
	context.Context,
	apisurface.AcquireModelLeaseRequest,
) (apisurface.AcquireModelLeaseResult, error) {
	return apisurface.AcquireModelLeaseResult{}, apisurface.ErrUnsupportedOperation
}

func (stubPackagedRuntimeHost) GetModelLease(
	context.Context,
	apisurface.GetModelLeaseRequest,
) (apisurface.GetModelLeaseResult, error) {
	return apisurface.GetModelLeaseResult{}, apisurface.ErrUnsupportedOperation
}

func (stubPackagedRuntimeHost) ReleaseModelLease(
	context.Context,
	apisurface.ReleaseModelLeaseRequest,
) (apisurface.ReleaseModelLeaseResult, error) {
	return apisurface.ReleaseModelLeaseResult{}, apisurface.ErrUnsupportedOperation
}

func (stubPackagedRuntimeHost) CloseRuntimeScope(context.Context, apisurface.RuntimeScopeRef) error {
	return nil
}

func TestScopedCompatHostAcquireLease_AllowsCLIPathWithoutSupervisedEndpoint(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, llamaCppCatalogFactoryConfigWithoutHealthEndpoint())
	scope, err := (apisurface.RuntimeScopeRef{}).Parse("factory-session:test-scope")
	if err != nil {
		t.Fatalf("Parse scope: %v", err)
	}
	host, err := NewScopedCompatHost(
		scope,
		stubPackagedRuntimeHost{
			ensureHost: apisurface.ModelHostSnapshot{
				Scope:          scope,
				ModelName:      "OMNIVOICE_Q4_K_M",
				ReadinessState: apisurface.ReadinessStateReady,
				LifecycleState: apisurface.LifecycleStateInstalled,
			},
		},
		stubAssetGateway{
			byModel: map[string]CacheInspection{
				"OMNIVOICE_Q4_K_M": {
					Supported:          true,
					Installed:          true,
					InstalledFileCount: 2,
				},
			},
		},
		DefaultManagedRuntimeSourceResolverAdapter(),
		Diagnostics{},
	)
	if err != nil {
		t.Fatalf("NewScopedCompatHost: %v", err)
	}

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.Endpoint != "" {
		t.Fatalf("endpoint = %q, want empty CLI path without supervised health endpoint", lease.Endpoint)
	}
}

func llamaCppCatalogFactoryConfigWithoutHealthEndpoint() *testFactoryConfig {
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Backend = "LLAMACPP"
	cfg.Workers[0].Command = "omnivoice-llamacpp"
	return cfg
}
