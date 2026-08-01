package wire_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestNewServiceRequiresRuntimeHostDependencies(t *testing.T) {
	t.Parallel()

	scopes := testRuntimeScopes(t)
	assets := &recordingAssetsService{}
	launcher := &recordingProcessLauncher{}
	clock := testHostClock{}

	tests := []struct {
		name            string
		scopes          runtimescopes.Service
		assets          *recordingAssetsService
		launcher        *recordingProcessLauncher
		hostHTTP        modelseffects.HostHTTPDoer
		clock           modelseffects.HostClock
		wantContains    string
		wantInvalidDeps bool
	}{
		{
			name: "valid", scopes: scopes, assets: assets, launcher: launcher,
			hostHTTP: http.DefaultClient, clock: clock,
		},
		{
			name: "scopes", assets: assets, launcher: launcher,
			hostHTTP: http.DefaultClient, clock: clock, wantContains: "Runtime Scopes",
			wantInvalidDeps: true,
		},
		{
			name: "assets", scopes: scopes, launcher: launcher,
			hostHTTP: http.DefaultClient, clock: clock, wantContains: "Assets",
			wantInvalidDeps: true,
		},
		{
			name: "process launcher", scopes: scopes, assets: assets,
			hostHTTP: http.DefaultClient, clock: clock, wantContains: "process launcher",
			wantInvalidDeps: true,
		},
		{
			name: "host http", scopes: scopes, assets: assets, launcher: launcher,
			clock: clock, wantContains: "HTTP client", wantInvalidDeps: true,
		},
		{
			name: "host clock", scopes: scopes, assets: assets, launcher: launcher,
			hostHTTP: http.DefaultClient, wantContains: "clock", wantInvalidDeps: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := runtimehostwire.NewService(
				test.scopes,
				test.assets,
				test.launcher,
				test.hostHTTP,
				test.clock,
				nil,
				nil,
			)
			if test.wantInvalidDeps {
				if service != nil || err == nil {
					t.Fatalf("NewService = (%#v, %v), want dependency error", service, err)
				}
				if !errors.Is(err, models.ErrInvalidHostDependencies) {
					t.Fatalf("error = %v, want ErrInvalidHostDependencies", err)
				}
				if test.wantContains != "" && !strings.Contains(err.Error(), test.wantContains) {
					t.Fatalf("error = %q, want substring %q", err.Error(), test.wantContains)
				}
				if launcher.starts != 0 {
					t.Fatalf("process starts during validation = %d, want 0", launcher.starts)
				}
				return
			}
			if service == nil || err != nil {
				t.Fatalf("NewService = (%#v, %v), want service", service, err)
			}
			if launcher.starts != 0 {
				t.Fatalf("process starts during construction = %d, want 0", launcher.starts)
			}
		})
	}
}

func testRuntimeScopes(t *testing.T) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "runtime-host-wire-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	return scopes
}

type recordingProcessLauncher struct {
	starts int
}

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher called during inert construction")
}

type testHostClock struct{}

func (testHostClock) Now() time.Time { return time.Unix(0, 0) }
func (testHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	panic("host timer created during inert construction")
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
