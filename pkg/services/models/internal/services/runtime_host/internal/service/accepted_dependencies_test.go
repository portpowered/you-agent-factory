package service_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestRuntimeHostAcceptedDependenciesPreserveDomainErrors(t *testing.T) {
	t.Parallel()
	ref := mustParseScope(t, "selected")
	for _, test := range []struct {
		name                     string
		scopeErr, assetErr, want error
	}{
		{"foreign", runtimescopes.ErrScopeForeign, nil, models.ErrRuntimeScopeForeign},
		{"closed", runtimescopes.ErrScopeClosed, nil, models.ErrRuntimeScopeClosed},
		{"stale", runtimescopes.ErrScopeUnknown, nil, models.ErrRuntimeScopeStale},
		{"asset failure", nil, models.ErrAssetUnavailable, models.ErrAssetUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scopes := &acceptedScopes{err: test.scopeErr, bindings: map[runtimescopes.Reference]models.RuntimeBinding{"selected": {}}}
			assets := &acceptedAssets{err: test.assetErr}
			host := acceptedHost(scopes, assets, &recordingProcessLauncher{})
			_, inspectErr := host.InspectModelHost(t.Context(), models.InspectModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
			_, ensureErr := host.EnsureModelHost(t.Context(), models.EnsureModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
			_, handoffErr := host.EnsureModelHostWithConfiguration(t.Context(), modelseffects.ResolvedHostConfiguration{Scope: ref, ModelName: "OMNIVOICE_Q4_K_M"})
			_, stopErr := host.StopModelHost(t.Context(), models.StopModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
			for _, err := range []error{inspectErr, ensureErr, handoffErr, stopErr} {
				if !errors.Is(err, test.want) {
					t.Fatalf("error = %v, want %v", err, test.want)
				}
			}
		})
	}
	t.Run("invalid request and cancellation do not call peers", func(t *testing.T) {
		t.Parallel()
		host := acceptedHost(&acceptedScopes{}, &acceptedAssets{}, &recordingProcessLauncher{})
		if _, err := host.EnsureModelHost(t.Context(), models.EnsureModelHostRequest{}); !errors.Is(err, models.ErrRuntimeScopeInvalid) {
			t.Fatalf("invalid request = %v", err)
		}
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("selected cancellation")
		cancel(cause)
		if _, err := host.EnsureModelHost(ctx, models.EnsureModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}); !errors.Is(err, models.ErrHostCancelled) || !errors.Is(err, cause) {
			t.Fatalf("cancelled request = %v", err)
		}
	})
}

func TestRuntimeHostAcceptedDependenciesFailedStartRetriesAndClosingScopePreservesPeer(t *testing.T) {
	t.Parallel()
	selected, peer := mustParseScope(t, "selected"), mustParseScope(t, "peer")
	config := supervisedRuntimeConfig()
	scopes := &acceptedScopes{bindings: map[runtimescopes.Reference]models.RuntimeBinding{
		"selected": {CacheDirectory: "selected-cache", RuntimeConfig: func() *models.RuntimeConfig { return &config }},
		"peer":     {CacheDirectory: "peer-cache", RuntimeConfig: func() *models.RuntimeConfig { return &config }},
	}}
	assets := &acceptedAssets{inspection: scopedassets.RuntimeCacheInspection{Supported: true, Installed: true, Revision: "revision", CachePath: "cache"}}
	launcher := &acceptedLauncher{failNext: true}
	host := acceptedHost(scopes, assets, launcher)
	t.Cleanup(func() {
		if err := host.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		for _, process := range launcher.processes {
			select {
			case <-process.waited:
			case <-time.After(5 * time.Second):
				t.Error("owned process did not join")
			}
		}
	})
	request := models.EnsureModelHostRequest{Scope: selected, Name: "OMNIVOICE_Q4_K_M"}
	if result, err := host.EnsureModelHost(t.Context(), request); err == nil || result.Host.ReadinessState == models.ReadinessStateReady {
		t.Fatalf("failed start = %#v, %v", result, err)
	}
	for _, scope := range []models.RuntimeScopeRef{selected, peer} {
		request.Scope = scope
		if result, err := host.EnsureModelHost(t.Context(), request); err != nil || result.Host.ReadinessState != models.ReadinessStateReady {
			t.Fatalf("ensure %s = %#v, %v", scope.String(), result, err)
		}
	}
	if err := host.CloseRuntimeScope(t.Context(), selected); err != nil {
		t.Fatal(err)
	}
	select {
	case <-launcher.processes[0].waited:
	case <-time.After(5 * time.Second):
		t.Fatal("selected process was not joined")
	}
	select {
	case <-launcher.processes[1].stopped:
		t.Fatal("peer stopped with selected scope")
	default:
	}
	request.Scope = peer
	if result, err := host.EnsureModelHost(t.Context(), request); err != nil || result.Outcome != models.HostEnsureAlreadyReady {
		t.Fatalf("held peer reuse = %#v, %v", result, err)
	}
	if len(launcher.processes) != 2 {
		t.Fatalf("peer was relaunched: %d processes", len(launcher.processes))
	}
}

type acceptedScopes struct {
	runtimescopes.Service
	bindings map[runtimescopes.Reference]models.RuntimeBinding
	err      error
}

func (s *acceptedScopes) Resolve(ref runtimescopes.Reference) (models.RuntimeBinding, error) {
	if s.err != nil {
		return models.RuntimeBinding{}, s.err
	}
	if s.bindings == nil {
		panic("unexpected scope resolution")
	}
	return s.bindings[ref], nil
}

type acceptedAssets struct {
	scopedassets.Service
	inspection scopedassets.RuntimeCacheInspection
	err        error
}

func (a *acceptedAssets) InspectRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	return a.inspection, a.err
}

type acceptedLeases struct{ hostleases.Service }
type acceptedHTTP struct{}

func (acceptedHTTP) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

type acceptedLauncher struct {
	mu        sync.Mutex
	failNext  bool
	processes []*controlledManagedProcess
}

func (l *acceptedLauncher) Start(_ context.Context, spec modelseffects.HostProcessStartSpec) (modelseffects.HostManagedProcess, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failNext {
		l.failNext = false
		return nil, errors.New("selected start failure")
	}
	p := newControlledManagedProcess(spec.HealthEndpoint)
	l.processes = append(l.processes, p)
	return p, nil
}

type acceptedHostContract interface {
	runtimehost.Service
	EnsureModelHostWithConfiguration(context.Context, modelseffects.ResolvedHostConfiguration) (models.EnsureModelHostResult, error)
	CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	Shutdown(context.Context) error
}

func acceptedHost(scopes runtimescopes.Service, assets scopedassets.Service, launcher modelseffects.HostProcessLauncher) acceptedHostContract {
	return internalservice.New(scopes, assets, acceptedLeases{}, internalservice.NewSlotState(), launcher, acceptedHTTP{}, newDeterministicHostClock(), nil, nil, models.AssetHostPlatform{}, nil, nil, func(path string) (string, error) { return path, nil }, nil, 0, 0).(acceptedHostContract)
}
