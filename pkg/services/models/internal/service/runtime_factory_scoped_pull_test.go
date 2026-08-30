package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRootPullModelForScopeValidatesBeforeRuntimeResolution(t *testing.T) {
	t.Parallel()

	root := &Root{}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := root.PullModelForScope(t.Context(), models.PullModelRequest{Name: "voice"}); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("unavailable scoped runtime error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestIsRemovableCacheAbsenceClassifiesAssetAbsenceErrors(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		models.ErrModelCacheNotFound,
		models.ErrAssetSourceMissing,
		models.ErrAssetSourceUnsupported,
		models.ErrAssetUnavailable,
		models.ErrNotAvailable,
	} {
		if !isRemovableCacheAbsence(err) {
			t.Fatalf("isRemovableCacheAbsence(%v) = false, want true", err)
		}
	}
	if isRemovableCacheAbsence(errors.New("cache is busy")) {
		t.Fatal("isRemovableCacheAbsence(non-absence) = true, want false")
	}
}

func TestRuntimeServicePullModelForScopeValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	runtime := &runtimeService{}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("empty pull request error = %v, want ErrNotFound", err)
	}
	if _, err := runtime.PullModelForScope(context.Background(), models.PullModelRequest{Name: "voice"}); err == nil {
		t.Fatal("delegated pull error = nil, want unavailable runtime failure")
	}
}

func TestRootPullModelForScopeFallsBackToCanonicalBuiltInResolution(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		models.BuiltInModelNameASR,
		models.BuiltInModelNameTTS,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, scope, assets := newPullFallbackRoot(t, name)
			result, err := root.PullModelForScope(context.Background(), models.PullModelRequest{
				Scope: scope,
				Name:  name,
			})
			if err != nil {
				t.Fatalf("PullModelForScope(%q): %v", name, err)
			}
			if result.ModelName != name || result.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" ||
				result.ReadinessState != "READY" {
				t.Fatalf("pull result = %#v, want successful managed-runtime result", result)
			}
			if assets.request.Scope != scope || assets.request.Name != name {
				t.Fatalf("asset preparation request = %#v, want scope %s and model %q", assets.request, scope.String(), name)
			}
		})
	}
}

func TestRootPullModelForScopePreservesUnknownCatalogMiss(t *testing.T) {
	t.Parallel()

	root, _, assets := newPullFallbackRoot(t, "")
	scope := firstPullFallbackScope(t, root)
	_, err := root.PullModelForScope(context.Background(), models.PullModelRequest{
		Scope: scope,
		Name:  "unknown-model",
	})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("unknown PullModelForScope error = %v, want ErrNotFound", err)
	}
	if assets.request.Name != "" {
		t.Fatalf("unknown asset preparation request = %#v, want no preparation", assets.request)
	}
}

func TestRootPullModelForScopeKeepsExistingFactoryPullResult(t *testing.T) {
	t.Parallel()

	root, scope, assets := newPullFallbackRoot(t, "")
	runtime := root.runtimeByScope[scope].(*pullCatalogMissRuntime)
	runtime.result = models.PullResult{
		ModelName:          "factory-model",
		ProviderLocality:   string(models.LocalityLocal),
		Outcome:            "ALREADY_PRESENT",
		ManagedPullOutcome: "ALREADY_READY",
		ReadinessState:     "READY",
		LifecycleState:     "INSTALLED",
	}
	runtime.err = nil

	result, err := root.PullModelForScope(context.Background(), models.PullModelRequest{
		Scope: scope,
		Name:  "factory-model",
	})
	if err != nil {
		t.Fatalf("Factory PullModelForScope: %v", err)
	}
	if result.ModelName != "factory-model" || result.Outcome != "ALREADY_PRESENT" {
		t.Fatalf("Factory pull result = %#v, want existing result", result)
	}
	if assets.request.Name != "" {
		t.Fatalf("Factory fallback asset preparation request = %#v, want none", assets.request)
	}
}

func newPullFallbackRoot(t *testing.T, modelName string) (*Root, models.RuntimeScopeRef, *preparationAssetService) {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "pull-fallback-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	ref, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	})
	if err != nil {
		t.Fatalf("open runtime scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	assets := &preparationAssetService{result: models.PrepareModelAssetsResult{
		Outcome: models.AssetPreparationPrepared,
		Asset: models.AssetSnapshot{
			ModelName: modelName,
			Readiness: models.AssetReadinessAvailable,
		},
	}}
	root := &Root{
		runtimeScopes: scopes,
		assets:        assets,
		runtimeByScope: map[models.RuntimeScopeRef]models.Service{
			scope: &pullCatalogMissRuntime{err: models.ErrNotFound},
		},
	}
	return root, scope, assets
}

func firstPullFallbackScope(t *testing.T, root *Root) models.RuntimeScopeRef {
	t.Helper()
	for scope := range root.runtimeByScope {
		return scope
	}
	t.Fatal("pull fallback root has no runtime scope")
	return models.RuntimeScopeRef{}
}

type pullCatalogMissRuntime struct {
	models.Service
	result models.PullResult
	err    error
}

func (runtime *pullCatalogMissRuntime) PullModel(context.Context, string) (models.PullResult, error) {
	return runtime.result, runtime.err
}

func TestRootCloseRuntimeScopePreventsConcurrentLazyRuntimeReinsertion(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:close-race")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	scopes := newCloseRaceRuntimeScopes()
	runtime := &closeRaceRuntime{}
	root := &Root{
		runtimeScopes:  scopes,
		runtimeByScope: make(map[models.RuntimeScopeRef]models.Service),
	}
	invokeResult := make(chan error, 1)
	go func() {
		resolved, invokeErr := root.scopedRuntimeWithBuilder(
			scope,
			func(models.RuntimeBinding) (models.Service, error) { return runtime, nil },
		)
		if invokeErr == nil {
			_, invokeErr = resolved.InvokeLocal(
				context.Background(),
				models.LocalInvocationRequest{Scope: scope},
			)
		}
		invokeResult <- invokeErr
	}()

	awaitCloseRaceSignal(t, scopes.resolveStarted, "initial scope resolution")
	if _, err := root.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: scope},
	); err != nil {
		t.Fatalf("CloseRuntimeScope() error = %v, want nil", err)
	}

	select {
	case err := <-invokeResult:
		if !errors.Is(err, models.ErrRuntimeScopeClosed) {
			t.Fatalf("concurrent InvokeLocal() error = %v, want ErrRuntimeScopeClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent InvokeLocal() did not return")
	}
	if runtime.invokeCalls != 0 {
		t.Fatalf("closed-scope runtime invocation calls = %d, want 0", runtime.invokeCalls)
	}
	root.runtimeMu.RLock()
	retained := root.runtimeByScope[scope]
	root.runtimeMu.RUnlock()
	if retained != nil {
		t.Fatal("runtime capability was reinserted after its scope closed")
	}
}

type closeRaceRuntimeScopes struct {
	mu             sync.Mutex
	resolveCalls   int
	closed         bool
	resolveStarted chan struct{}
	closeCompleted chan struct{}
	closeOnce      sync.Once
}

func newCloseRaceRuntimeScopes() *closeRaceRuntimeScopes {
	return &closeRaceRuntimeScopes{
		resolveStarted: make(chan struct{}),
		closeCompleted: make(chan struct{}),
	}
}

func (scopes *closeRaceRuntimeScopes) Open(models.RuntimeBinding) (runtimescopes.Reference, error) {
	return "", errors.New("unexpected runtime scope open")
}

func (scopes *closeRaceRuntimeScopes) Resolve(
	runtimescopes.Reference,
) (models.RuntimeBinding, error) {
	scopes.mu.Lock()
	scopes.resolveCalls++
	call := scopes.resolveCalls
	closed := scopes.closed
	scopes.mu.Unlock()
	if call == 1 {
		close(scopes.resolveStarted)
		<-scopes.closeCompleted
		return models.RuntimeBinding{}, nil
	}
	if closed {
		return models.RuntimeBinding{}, runtimescopes.ErrScopeClosed
	}
	return models.RuntimeBinding{}, nil
}

func (scopes *closeRaceRuntimeScopes) Close(runtimescopes.Reference) error {
	scopes.mu.Lock()
	scopes.closed = true
	scopes.mu.Unlock()
	scopes.closeOnce.Do(func() {
		close(scopes.closeCompleted)
	})
	return nil
}

type closeRaceRuntime struct {
	models.Service
	invokeCalls int
}

func (runtime *closeRaceRuntime) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	runtime.invokeCalls++
	return models.LocalInvocationResult{}, nil
}

func awaitCloseRaceSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func TestRootRemoveModelAssetsRefusesAnInUseCacheBeforeMutation(t *testing.T) {
	scope, err := (models.RuntimeScopeRef{}).Parse("remove-guard:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}

	assets := &removeGuardAssets{
		inspection: scopedassets.RuntimeCacheInspection{Installed: true},
	}
	root := &Root{
		assets:      assets,
		runtimeHost: &removeGuardHost{stopErr: models.ErrHostCapacityExhausted},
	}

	_, err = root.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: scope,
		Name:  "managed-model",
	})
	if !errors.Is(err, models.ErrModelCacheInUse) {
		t.Fatalf("RemoveModelAssets error = %v, want ErrModelCacheInUse", err)
	}
	if assets.removeCalls != 0 {
		t.Fatalf("asset removal calls = %d, want 0 while cache is in use", assets.removeCalls)
	}
}

func TestRootRemoveModelAssetsSerializesCacheRemovalAgainstLeaseAcquisition(t *testing.T) {
	scope, err := (models.RuntimeScopeRef{}).Parse("remove-serialization:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}

	stopStarted := make(chan struct{})
	allowStop := make(chan struct{})
	removalComplete := make(chan struct{})
	assets := &removeGuardAssets{
		inspection: scopedassets.RuntimeCacheInspection{Installed: true},
		removeResult: models.RemoveModelAssetsResult{
			ModelName: "MANAGED-MODEL", Outcome: models.AssetRemovalRemoved,
		},
		removalComplete: removalComplete,
	}
	host := &removeGuardHost{
		stopStarted:     stopStarted,
		allowStop:       allowStop,
		removalComplete: removalComplete,
		acquireResult:   models.AcquireModelLeaseResult{},
	}
	root := &Root{assets: assets, runtimeHost: host}
	removeDone := make(chan error, 1)
	go func() {
		_, removeErr := root.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
			Scope: scope,
			Name:  "managed-model",
		})
		removeDone <- removeErr
	}()
	awaitCloseRaceSignal(t, stopStarted, "removal host stop")

	acquireDone := make(chan error, 1)
	go func() {
		_, acquireErr := root.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
			Scope: scope,
			Name:  "managed-model",
		})
		acquireDone <- acquireErr
	}()
	close(allowStop)

	select {
	case err := <-removeDone:
		if err != nil {
			t.Fatalf("RemoveModelAssets error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RemoveModelAssets did not finish")
	}
	select {
	case err := <-acquireDone:
		if err != nil {
			t.Fatalf("AcquireModelLease error = %v, want nil after removal", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AcquireModelLease did not finish after removal")
	}
}

func TestRootRemoveModelAssetsLogsStartAndTerminalOutcome(t *testing.T) {
	scope, err := (models.RuntimeScopeRef{}).Parse("remove-logging:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	core, observed := observer.New(zap.InfoLevel)
	root := &Root{
		assets: &removeGuardAssets{
			inspection: scopedassets.RuntimeCacheInspection{Installed: true},
			removeResult: models.RemoveModelAssetsResult{
				ModelName: "MANAGED-MODEL", Outcome: models.AssetRemovalRemoved,
			},
		},
		runtimeHost: &removeGuardHost{},
		process: modelseffects.ProcessDependencies{
			Logger: zap.New(core),
			Clock:  func() time.Time { return time.Unix(123, 0) },
		},
	}
	if _, err := root.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: scope,
		Name:  "managed-model",
	}); err != nil {
		t.Fatalf("RemoveModelAssets error = %v, want nil", err)
	}
	if observed.FilterMessage("models cache removal started").Len() != 1 {
		t.Fatalf("removal start logs = %d, want 1", observed.FilterMessage("models cache removal started").Len())
	}
	completed := observed.FilterMessage("models cache removal completed").All()
	if len(completed) != 1 {
		t.Fatalf("removal terminal logs = %d, want 1", len(completed))
	}
	fields := completed[0].ContextMap()
	if fields["model_name"] != "managed-model" || fields["scope"] != scope.String() || fields["outcome"] != "REMOVED" {
		t.Fatalf("removal terminal fields = %#v, want model/scope/REMOVED", fields)
	}
}

type removeGuardAssets struct {
	inspection      scopedassets.RuntimeCacheInspection
	removeResult    models.RemoveModelAssetsResult
	removalComplete chan struct{}
	removeOnce      sync.Once
	removeCalls     int
}

func (assets *removeGuardAssets) PreflightModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) PrepareModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) InspectModelAssets(context.Context, models.InspectModelAssetsRequest) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) RemoveModelAssets(context.Context, models.RemoveModelAssetsRequest) (models.RemoveModelAssetsResult, error) {
	assets.removeCalls++
	if assets.removalComplete != nil {
		assets.removeOnce.Do(func() { close(assets.removalComplete) })
	}
	if assets.removeResult.Outcome != "" {
		return assets.removeResult, nil
	}
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) ResolveRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (assets *removeGuardAssets) InspectRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	return assets.inspection, nil
}

type removeGuardHost struct {
	stopErr         error
	stopStarted     chan struct{}
	stopOnce        sync.Once
	allowStop       <-chan struct{}
	removalComplete <-chan struct{}
	acquireResult   models.AcquireModelLeaseResult
}

func (host *removeGuardHost) InspectModelHost(context.Context, models.InspectModelHostRequest) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) EnsureModelHost(context.Context, models.EnsureModelHostRequest) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) StopModelHost(context.Context, models.StopModelHostRequest) (models.StopModelHostResult, error) {
	if host.stopStarted != nil {
		host.stopOnce.Do(func() { close(host.stopStarted) })
	}
	if host.allowStop != nil {
		<-host.allowStop
	}
	return models.StopModelHostResult{}, host.stopErr
}

func (host *removeGuardHost) AcquireModelLease(context.Context, models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
	if host.removalComplete != nil {
		select {
		case <-host.removalComplete:
		default:
			return models.AcquireModelLeaseResult{}, errors.New("lease acquisition overlapped cache removal")
		}
	}
	return host.acquireResult, nil
}

func (host *removeGuardHost) GetModelLease(context.Context, models.GetModelLeaseRequest) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) ReleaseModelLease(context.Context, models.ReleaseModelLeaseRequest) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (host *removeGuardHost) CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error {
	return nil
}
