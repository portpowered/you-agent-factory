package wire_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferenceinternalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/service"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
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

	for _, test := range inferenceDependencyCases(scopes, assets, catalog, runtimeHost) {
		t.Run(test.name, func(t *testing.T) {
			assertInferenceConstruction(t, test)
		})
	}
}

type inferenceDependencyCase struct {
	name              string
	scopes            runtimescopes.Service
	assets            scopedassets.Service
	catalog           modelcatalog.Service
	runtimeHost       runtimehost.Service
	invocationRuntime inferenceinternalservice.InvocationRuntime
	fileSystem        modelseffects.InvocationArtifactFileSystem
	includeClock      bool
	wantContains      string
	wantInvalidDeps   bool
}

func inferenceDependencyCases(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	catalog modelcatalog.Service,
	runtimeHost runtimehost.Service,
) []inferenceDependencyCase {
	return []inferenceDependencyCase{
		{
			name: "valid", scopes: scopes, assets: assets, catalog: catalog,
			runtimeHost: runtimeHost, invocationRuntime: testInvocationRuntime{},
			fileSystem: inference.InertArtifactFileSystem{}, includeClock: true,
		},
		{
			name: "scopes", assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: testInvocationRuntime{},
			fileSystem:        inference.InertArtifactFileSystem{}, includeClock: true,
			wantContains: "Runtime Scopes", wantInvalidDeps: true,
		},
		{
			name: "assets", scopes: scopes, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: testInvocationRuntime{},
			fileSystem:        inference.InertArtifactFileSystem{}, includeClock: true,
			wantContains: "Assets", wantInvalidDeps: true,
		},
		{
			name: "catalog", scopes: scopes, assets: assets, runtimeHost: runtimeHost,
			invocationRuntime: testInvocationRuntime{},
			fileSystem:        inference.InertArtifactFileSystem{}, includeClock: true,
			wantContains: "Catalog", wantInvalidDeps: true,
		},
		{
			name: "runtime host", scopes: scopes, assets: assets, catalog: catalog,
			invocationRuntime: testInvocationRuntime{},
			fileSystem:        inference.InertArtifactFileSystem{}, includeClock: true,
			wantContains: "Runtime Host", wantInvalidDeps: true,
		},
		{
			name: "invocation runtime", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			fileSystem: inference.InertArtifactFileSystem{}, includeClock: true,
			wantContains: "invocation runtime", wantInvalidDeps: true,
		},
		{
			name: "filesystem", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: testInvocationRuntime{}, includeClock: true,
			wantContains: "filesystem", wantInvalidDeps: true,
		},
		{
			name: "clock", scopes: scopes, assets: assets, catalog: catalog, runtimeHost: runtimeHost,
			invocationRuntime: testInvocationRuntime{},
			fileSystem:        inference.InertArtifactFileSystem{},
			wantContains:      "clock", wantInvalidDeps: true,
		},
	}
}

func assertInferenceConstruction(t *testing.T, test inferenceDependencyCase) {
	t.Helper()

	clock := &testInferenceClock{}
	var clockFn func() time.Time
	if test.includeClock {
		clockFn = clock.Now
	}

	service, err := inferencewire.NewService(
		test.scopes,
		test.assets,
		test.catalog,
		test.runtimeHost,
		test.invocationRuntime,
		test.fileSystem,
		clockFn,
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
		if clock.calls != 0 {
			t.Fatalf("clock calls during validation = %d, want 0", clock.calls)
		}
		return
	}
	if service == nil || err != nil {
		t.Fatalf("NewService = (%#v, %v), want service", service, err)
	}
	if clock.calls != 0 {
		t.Fatalf("clock calls during construction = %d, want 0", clock.calls)
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

type testInvocationRuntime struct{}

func (testInvocationRuntime) Invoke(
	context.Context,
	inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	return inference.InvocationRuntimeResult{}, nil
}

type testInferenceClock struct {
	calls int
}

func (clock *testInferenceClock) Now() time.Time {
	clock.calls++
	return time.Unix(0, 0)
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

func (recordingRuntimeHostService) CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error {
	return nil
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

func (recordingAssetsService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
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
