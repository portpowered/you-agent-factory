package wire_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestNewServiceRequiresInferenceDependencies(t *testing.T) {
	t.Parallel()

	scopes := testRuntimeScopes(t)
	catalog := testCatalogService(t, scopes)
	assets := recordingAssetsService{}
	runtimeHost := testRuntimeHostService()
	clock := testInferenceClock{}

	tests := []struct {
		name            string
		scopes          runtimescopes.Service
		assets          scopedassets.Service
		catalog         modelcatalog.Service
		runtimeHost     runtimehost.Service
		invocationRuntime inference.InvocationRuntime
		fileSystem      models.InvocationArtifactFileSystem
		clock           func() time.Time
		wantContains    string
		wantInvalidDeps bool
	}{
		{
			name: "valid", scopes: scopes, assets: assets, catalog: catalog,
			runtimeHost: runtimeHost, invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
		},
		{
			name: "scopes", assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
			wantContains: "Runtime Scopes", wantInvalidDeps: true,
		},
		{
			name: "assets", scopes: scopes, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
			wantContains: "Assets", wantInvalidDeps: true,
		},
		{
			name: "catalog", scopes: scopes, assets: assets, runtimeHost: runtimeHost,
			invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
			wantContains: "Catalog", wantInvalidDeps: true,
		},
		{
			name: "runtime host", scopes: scopes, assets: assets, catalog: catalog,
			invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
			wantContains: "Runtime Host", wantInvalidDeps: true,
		},
		{
			name: "invocation runtime", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			fileSystem: inference.InertArtifactFileSystem{}, clock: clock.Now,
			wantContains: "invocation runtime", wantInvalidDeps: true,
		},
		{
			name: "filesystem", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: inference.InputEchoInvocationRuntime{}, clock: clock.Now,
			wantContains: "filesystem", wantInvalidDeps: true,
		},
		{
			name: "clock", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: inference.InputEchoInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{},
			wantContains: "clock", wantInvalidDeps: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := inferencewire.NewService(
				test.scopes,
				test.assets,
				test.catalog,
				test.runtimeHost,
				test.invocationRuntime,
				test.fileSystem,
				test.clock,
			)
			if test.wantInvalidDeps {
				if service != nil || err == nil {
					t.Fatalf("NewService = (%#v, %v), want dependency error", service, err)
				}
				if !errors.Is(err, models.ErrInvalidInferenceDependencies) {
					t.Fatalf("error = %v, want ErrInvalidInferenceDependencies", err)
				}
				if test.wantContains != "" && !strings.Contains(err.Error(), test.wantContains) {
					t.Fatalf("error = %q, want substring %q", err.Error(), test.wantContains)
				}
				if clock.timerCalls != 0 {
					t.Fatalf("timer calls during validation = %d, want 0", clock.timerCalls)
				}
				return
			}
			if service == nil || err != nil {
				t.Fatalf("NewService = (%#v, %v), want service", service, err)
			}
			if clock.timerCalls != 0 {
				t.Fatalf("timer calls during construction = %d, want 0", clock.timerCalls)
			}
		})
	}
}

func testRuntimeScopes(t *testing.T) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "inference-wire-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	return scopes
}

func testCatalogService(t *testing.T, scopes runtimescopes.Service) modelcatalog.Service {
	t.Helper()
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct catalog: %v", err)
	}
	return service
}

func testRuntimeHostService() runtimehost.Service {
	return recordingRuntimeHostService{}
}

type testInferenceClock struct {
	timerCalls int
}

func (clock *testInferenceClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (clock *testInferenceClock) NewTimer(time.Duration) models.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert inference construction")
}

type recordingRuntimeHostService struct{}

var _ runtimehost.Service = recordingRuntimeHostService{}

func (recordingRuntimeHostService) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (recordingRuntimeHostService) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (recordingRuntimeHostService) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (recordingRuntimeHostService) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (recordingRuntimeHostService) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (recordingRuntimeHostService) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

type recordingAssetsService struct{}

var _ scopedassets.Service = recordingAssetsService{}

func (recordingAssetsService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (recordingAssetsService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (recordingAssetsService) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (recordingAssetsService) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{}, models.ErrUnsupportedOperation
}
