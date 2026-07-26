package models_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"strings"
	"sync"
	"testing"
)

// runtimeScopePeerService is a fake peer implementer of Models root Service
// that registers detached configuration without importing implementation or
// construction packages.
type runtimeScopePeerService struct {
	mu     sync.Mutex
	owner  string
	next   int
	open   map[models.RuntimeScopeRef]models.RuntimeScopeConfig
	closed map[models.RuntimeScopeRef]struct{}
}

func newRuntimeScopePeerService(owner string) *runtimeScopePeerService {
	return &runtimeScopePeerService{
		owner:  owner,
		open:   make(map[models.RuntimeScopeRef]models.RuntimeScopeConfig),
		closed: make(map[models.RuntimeScopeRef]struct{}),
	}
}

func (s *runtimeScopePeerService) OpenRuntimeScope(
	_ context.Context,
	request models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	scope, err := (models.RuntimeScopeRef{}).Parse(fmt.Sprintf("%s:%d", s.owner, s.next))
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	s.open[scope] = request.Config.Clone()
	return models.OpenRuntimeScopeResult{Scope: scope}, nil
}

func (s *runtimeScopePeerService) CloseRuntimeScope(
	_ context.Context,
	request models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if request.Scope.IsZero() {
		return models.CloseRuntimeScopeResult{}, models.ErrRuntimeScopeInvalid
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.CloseRuntimeScopeResult{}, err
	}
	delete(s.open, request.Scope)
	s.closed[request.Scope] = struct{}{}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

func (s *runtimeScopePeerService) scopeUseError(scope models.RuntimeScopeRef) error {
	if scope.IsZero() {
		return models.ErrRuntimeScopeInvalid
	}
	if !strings.HasPrefix(scope.String(), s.owner+":") {
		return models.ErrRuntimeScopeForeign
	}
	if _, ok := s.closed[scope]; ok {
		return models.ErrRuntimeScopeClosed
	}
	if _, ok := s.open[scope]; !ok {
		return models.ErrRuntimeScopeStale
	}
	return nil
}

func (*runtimeScopePeerService) ListCatalog(
	context.Context,
	models.ListModelsRequest,
) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) GetCatalogModel(
	context.Context,
	models.GetModelRequest,
) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

func (s *runtimeScopePeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return s, nil
}

func (*runtimeScopePeerService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: []models.Summary{}}, nil
}

func (*runtimeScopePeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (*runtimeScopePeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (*runtimeScopePeerService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, models.ErrHostRuntimeNotReady
}

func (*runtimeScopePeerService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrHostLeaseNotFound
}

func (*runtimeScopePeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestRuntimeScope_OpenRegistersDetachedConfigAndReturnsOpaqueRef(t *testing.T) {
	t.Parallel()

	fake := newRuntimeScopePeerService("factory-session-a")
	var service models.Service = fake
	request := models.OpenRuntimeScopeRequest{Config: models.RuntimeScopeConfig{
		CacheDirectory: "cache",
		Runtime: models.RuntimeConfig{
			FactoryDirectory: "factory",
			Workers: []models.RuntimeWorker{{
				Name:          "writer",
				Type:          models.RuntimeWorkerTypeInference,
				Model:         "local-model",
				ModelLocality: models.RuntimeModelLocalityLocal,
				Args:          []string{"--detached"},
			}},
		},
	}}

	result, err := service.OpenRuntimeScope(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	if result.Scope.IsZero() {
		t.Fatal("OpenRuntimeScope Scope is zero")
	}

	request.Config.Runtime.Workers[0].Model = "mutated"
	request.Config.Runtime.Workers[0].Args[0] = "--mutated"
	stored := fake.open[result.Scope]
	if stored.Runtime.Workers[0].Model != "local-model" ||
		stored.Runtime.Workers[0].Args[0] != "--detached" {
		t.Fatalf("registered config = %#v, want detached original values", stored)
	}
}

func TestRuntimeScope_CloseClassifiesInvalidStaleClosedAndForeignRefs(t *testing.T) {
	t.Parallel()

	fake := newRuntimeScopePeerService("factory-session-a")
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	stale, _ := (models.RuntimeScopeRef{}).Parse("factory-session-a:missing")
	foreign, _ := (models.RuntimeScopeRef{}).Parse("factory-session-b:1")

	assertCloseErrorIs(t, service, models.RuntimeScopeRef{}, models.ErrRuntimeScopeInvalid)
	assertCloseErrorIs(t, service, stale, models.ErrRuntimeScopeStale)
	assertCloseErrorIs(t, service, foreign, models.ErrRuntimeScopeForeign)

	closed, err := service.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: opened.Scope},
	)
	if err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if !closed.Closed || closed.Scope != opened.Scope {
		t.Fatalf("CloseRuntimeScope result = %#v, want closed issued scope", closed)
	}
	assertCloseErrorIs(t, service, opened.Scope, models.ErrRuntimeScopeClosed)
}

func assertCloseErrorIs(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
	want error,
) {
	t.Helper()
	_, err := service.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: scope},
	)
	if !errors.Is(err, want) {
		t.Fatalf("CloseRuntimeScope(%q) error = %v, want %v", scope.String(), err, want)
	}
}

func TestRuntimeScope_ReferenceCarriesAcrossScopeBoundRequests(t *testing.T) {
	t.Parallel()

	var service models.Service = newRuntimeScopePeerService("factory-session-a")
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	scope := opened.Scope
	requestScopes := []models.RuntimeScopeRef{
		(models.ListModelsRequest{Scope: scope}).Scope,
		(models.GetModelRequest{Scope: scope}).Scope,
		(models.PullModelRequest{Scope: scope}).Scope,
		(models.InspectRuntimeRequest{Scope: scope}).Scope,
		(models.AcquireLeaseRequest{Scope: scope}).Scope,
		(models.ReleaseLeaseRequest{Scope: scope}).Scope,
		(models.LocalInvocationRequest{Scope: scope}).Scope,
	}
	for i, got := range requestScopes {
		if got != scope {
			t.Fatalf("request scope %d = %q, want %q", i, got.String(), scope.String())
		}
	}
}

type unsupportedRuntimeScopePeer struct{}

func (unsupportedRuntimeScopePeer) OpenRuntimeScope(
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (s catalogPeerService) GetModel(_ context.Context, name string) (models.Detail, error) {
	if err := models.ValidateGetModelRequest(models.GetModelRequest{Name: name}); err != nil {
		return models.Detail{}, err
	}
	if s.unavailable {
		return models.Detail{}, models.ErrUnavailable
	}
	detail, ok := s.entries[name]
	if !ok {
		return models.Detail{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	return detail, nil
}

func (catalogPeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (catalogPeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (catalogPeerService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, models.ErrHostRuntimeNotReady
}

func (catalogPeerService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrHostLeaseNotFound
}

func (catalogPeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestCatalog_ListAndGetReturnDetachedModelsOwnedShapes(t *testing.T) {
	t.Parallel()

	detail := models.Detail{
		Summary: models.Summary{
			Name:             "local-model",
			ProviderLocality: models.LocalityLocal,
			Status:           models.StatusReady,
			LoadState:        models.LoadStateUnloaded,
			ManagedRuntime: models.Runtime{
				Identity:       "local-model",
				ReadinessState: models.ReadinessStateReady,
				LifecycleState: models.LifecycleStateInstalled,
				Locality:       models.LocalityLocal,
			},
		},
		Capabilities: []models.Capability{{
			Worker:           "writer",
			ProviderLocality: models.LocalityLocal,
		}},
	}
	var service models.Service = catalogPeerService{
		entries: map[string]models.Detail{"local-model": detail},
	}

	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(list.Results) != 1 {
		t.Fatalf("ListModels Results len = %d, want 1", len(list.Results))
	}
	if list.Results[0].Name != "local-model" {
		t.Fatalf("ListModels Results[0].Name = %q, want local-model", list.Results[0].Name)
	}
	if list.Results[0].Status != models.StatusReady {
		t.Fatalf("ListModels Status = %s, want READY", list.Results[0].Status)
	}
	if list.Results[0].ManagedRuntime.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("ListModels readiness = %s, want READY", list.Results[0].ManagedRuntime.ReadinessState)
	}

	got, err := service.GetModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if got.Name != detail.Name || len(got.Capabilities) != 1 {
		t.Fatalf("GetModel detail = %#v, want detached Models-owned Detail", got)
	}
}

func TestCatalog_MissingUnsupportedAndUnavailableAreDistinctTypedOutcomes(t *testing.T) {
	t.Parallel()

	var service models.Service = catalogPeerService{entries: map[string]models.Detail{}}

	_, err := service.GetModel(context.Background(), "missing-model")
	if err == nil {
		t.Fatal("GetModel error = nil, want ErrNotFound")
	}
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrNotFound", err)
	}
	if errors.Is(err, models.ErrUnavailable) || errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModel error = %v, must stay distinct from unavailable/unsupported", err)
	}

	_, err = service.PullModel(context.Background(), "local-model")
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("PullModel error = %v, want ErrUnsupportedOperation", err)
	}
	if errors.Is(err, models.ErrNotFound) || errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("PullModel error = %v, must stay distinct from not-found/unavailable", err)
	}

	unavailable := catalogPeerService{unavailable: true}
	var unavailableService models.Service = unavailable
	_, err = unavailableService.ListModels(context.Background())
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListModels error = %v, want ErrUnavailable", err)
	}
	_, err = unavailableService.GetModel(context.Background(), "local-model")
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("GetModel unavailable error = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, models.ErrNotFound) || errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModel unavailable error = %v, must stay distinct from not-found/unsupported", err)
	}
}

func TestCatalog_PeerCompilesWithoutInternalCatalogImports(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// consume list/get without models/internal/catalog or local assemblers.
	req := models.GetModelRequest{Name: "local-model"}
	if err := models.ValidateGetModelRequest(req); err != nil {
		t.Fatalf("ValidateGetModelRequest: %v", err)
	}
	if err := models.ValidateGetModelRequest(models.GetModelRequest{}); err == nil {
		t.Fatal("ValidateGetModelRequest empty name = nil, want error")
	}
	if !errors.Is(models.ValidateGetModelRequest(models.GetModelRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateGetModelRequest empty name must wrap ErrNotFound")
	}

	_ = models.ListModelsRequest{}
	var service models.Service = catalogPeerService{}
	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if list.Results == nil {
		t.Fatal("ListModels Results = nil, want empty Models-owned slice")
	}
}

// assetsPeerService is a fake peer implementer of Models root Service that
// exercises plain asset-pull contracts using only root-package types.
type assetsPeerService struct {
	unsupportedRuntimeScopePeer
	results map[string]models.PullResult
	fails   map[string]error
}

func (assetsPeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return assetsPeerService{}, nil
}

func (assetsPeerService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: []models.Summary{}}, nil
}

func (assetsPeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (s assetsPeerService) PullModel(_ context.Context, name string) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(models.PullModelRequest{Name: name}); err != nil {
		return models.PullResult{}, err
	}
	if fail, ok := s.fails[name]; ok {
		return models.PullResult{}, fail
	}
	result, ok := s.results[name]
	if !ok {
		return models.PullResult{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	return result, nil
}

func (assetsPeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (assetsPeerService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, models.ErrHostRuntimeNotReady
}

func (assetsPeerService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrHostLeaseNotFound
}

func (assetsPeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestAssets_ValidPullReturnsModelsOwnedPullResult(t *testing.T) {
	t.Parallel()

	want := models.PullResult{
		ModelName:          "local-model",
		ProviderLocality:   string(models.LocalityLocal),
		Outcome:            "PULLED",
		CachePath:          "/tmp/models/local-model",
		Revision:           "rev1",
		DownloadedFiles:    []models.DownloadedFile{{Path: "weights.bin", Bytes: 42, SHA256: "abc"}},
		ManagedPullOutcome: string(models.PullOutcomeInstalledSuccessfully),
		ReadinessState:     string(models.ReadinessStateReady),
		LifecycleState:     string(models.LifecycleStateInstalled),
	}
	var service models.Service = assetsPeerService{
		results: map[string]models.PullResult{"local-model": want},
	}

	got, err := service.PullModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if got.ModelName != want.ModelName || got.CachePath != want.CachePath || got.Revision != want.Revision {
		t.Fatalf("PullModel result = %#v, want Models-owned PullResult", got)
	}
	if got.ManagedPullOutcome != string(models.PullOutcomeInstalledSuccessfully) {
		t.Fatalf("ManagedPullOutcome = %q, want %s", got.ManagedPullOutcome, models.PullOutcomeInstalledSuccessfully)
	}
	if len(got.DownloadedFiles) != 1 || got.DownloadedFiles[0].Path != "weights.bin" || got.DownloadedFiles[0].Bytes != 42 {
		t.Fatalf("DownloadedFiles = %#v, want downloaded-file vocabulary", got.DownloadedFiles)
	}
}

func TestAssets_NotAvailablePullUnsupportedAndSourceFetchFailedAreDistinct(t *testing.T) {
	t.Parallel()

	var service models.Service = assetsPeerService{
		fails: map[string]error{
			"missing-assets": models.ErrNotAvailable,
			"cloud-only":     models.ErrPullUnsupported,
			"source-broken": &models.PullError{
				Result: models.PullResult{
					ModelName:          "source-broken",
					ManagedPullOutcome: string(models.PullOutcomeSourceFetchFailed),
					ReadinessState:     string(models.ReadinessStateFailed),
				},
				Cause: models.ErrSourceFetchFailed,
			},
		},
	}

	_, err := service.PullModel(context.Background(), "missing-assets")
	if !errors.Is(err, models.ErrNotAvailable) {
		t.Fatalf("PullModel missing-assets = %v, want ErrNotAvailable", err)
	}
	if errors.Is(err, models.ErrPullUnsupported) || errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("ErrNotAvailable must stay distinct: %v", err)
	}

	_, err = service.PullModel(context.Background(), "cloud-only")
	if !errors.Is(err, models.ErrPullUnsupported) {
		t.Fatalf("PullModel cloud-only = %v, want ErrPullUnsupported", err)
	}
	if errors.Is(err, models.ErrNotAvailable) || errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("ErrPullUnsupported must stay distinct: %v", err)
	}

	_, err = service.PullModel(context.Background(), "source-broken")
	if !errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("PullModel source-broken = %v, want ErrSourceFetchFailed", err)
	}
	var classified *models.PullError
	if !errors.As(err, &classified) || classified.Result.ManagedPullOutcome != string(models.PullOutcomeSourceFetchFailed) {
		t.Fatalf("source-broken error = %#v, want classified PullError with SOURCE_FETCH_FAILED", err)
	}
	if errors.Is(err, models.ErrNotAvailable) || errors.Is(err, models.ErrPullUnsupported) {
		t.Fatalf("ErrSourceFetchFailed must stay distinct: %v", err)
	}
}

func TestAssets_PeerCompilesWithoutNestedAssetGateway(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// pull assets without models/internal/assets or a nested asset-gateway import.
	req := models.PullModelRequest{Name: "local-model"}
	if err := models.ValidatePullModelRequest(req); err != nil {
		t.Fatalf("ValidatePullModelRequest: %v", err)
	}
	if err := models.ValidatePullModelRequest(models.PullModelRequest{}); err == nil {
		t.Fatal("ValidatePullModelRequest empty name = nil, want error")
	}
	if !errors.Is(models.ValidatePullModelRequest(models.PullModelRequest{}), models.ErrNotFound) {
		t.Fatal("ValidatePullModelRequest empty name must wrap ErrNotFound")
	}

	var service models.Service = assetsPeerService{
		results: map[string]models.PullResult{
			"local-model": {
				ModelName:          "local-model",
				ManagedPullOutcome: string(models.PullOutcomeAlreadyPresent),
				DownloadedFiles:    []models.DownloadedFile{},
			},
		},
	}
	result, err := service.PullModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.ManagedPullOutcome != string(models.PullOutcomeAlreadyPresent) {
		t.Fatalf("ManagedPullOutcome = %q, want ALREADY_PRESENT", result.ManagedPullOutcome)
	}
}

// hostLeasePeerService is a fake peer implementer of Models root Service that
// exercises plain host/lease contracts using only root-package types.
type hostLeasePeerService struct {
	unsupportedRuntimeScopePeer
	runtimes map[string]models.Runtime
	leases   map[string]models.HostLease
	acquire  map[string]error
	release  map[string]error
	inspect  map[string]error
}

func (hostLeasePeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return hostLeasePeerService{}, nil
}

func (hostLeasePeerService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: []models.Summary{}}, nil
}

func (hostLeasePeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (hostLeasePeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (s hostLeasePeerService) InspectRuntime(_ context.Context, name string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: name}); err != nil {
		return models.Runtime{}, err
	}
	if fail, ok := s.inspect[name]; ok {
		return models.Runtime{}, fail
	}
	runtime, ok := s.runtimes[name]
	if !ok {
		return models.Runtime{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	return runtime, nil
}

func (s hostLeasePeerService) AcquireLease(_ context.Context, request models.AcquireLeaseRequest) (models.HostLease, error) {
	if err := models.ValidateAcquireLeaseRequest(request); err != nil {
		return models.HostLease{}, err
	}
	if fail, ok := s.acquire[request.ModelName]; ok {
		return models.HostLease{}, fail
	}
	lease, ok := s.leases[request.ModelName]
	if !ok {
		return models.HostLease{}, models.ErrHostRuntimeNotReady
	}
	if request.Holder != "" {
		lease.Holder = request.Holder
	}
	return lease, nil
}

func (s hostLeasePeerService) ReleaseLease(_ context.Context, request models.ReleaseLeaseRequest) error {
	if err := models.ValidateReleaseLeaseRequest(request); err != nil {
		return err
	}
	if fail, ok := s.release[request.LeaseID]; ok {
		return fail
	}
	for _, lease := range s.leases {
		if lease.ID == request.LeaseID {
			return nil
		}
	}
	return models.ErrHostLeaseNotFound
}

func (hostLeasePeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestHostLease_ReadinessInspectAndAcquireReleaseReturnModelsOwnedShapes(t *testing.T) {
	t.Parallel()

	wantRuntime := models.Runtime{
		Identity:       "local-model",
		ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateLoaded,
		Locality:       models.LocalityLocal,
	}
	wantLease := models.HostLease{
		ID:       "lease-1",
		Identity: models.HostIdentity{Name: "local-model", Locality: models.LocalityLocal},
		Endpoint: "http://127.0.0.1:8080",
		Holder:   "worker-a",
	}
	var service models.Service = hostLeasePeerService{
		runtimes: map[string]models.Runtime{"local-model": wantRuntime},
		leases:   map[string]models.HostLease{"local-model": wantLease},
	}

	gotRuntime, err := service.InspectRuntime(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("InspectRuntime: %v", err)
	}
	if gotRuntime.Identity != wantRuntime.Identity || gotRuntime.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("InspectRuntime = %#v, want Models-owned Runtime readiness", gotRuntime)
	}

	gotLease, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{
		ModelName: "local-model",
		Holder:    "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if gotLease.ID != wantLease.ID || gotLease.Endpoint != wantLease.Endpoint || gotLease.Holder != "worker-a" {
		t.Fatalf("AcquireLease = %#v, want Models-owned HostLease", gotLease)
	}
	if gotLease.Identity.Name != "local-model" {
		t.Fatalf("AcquireLease identity = %#v, want local-model HostIdentity", gotLease.Identity)
	}

	if err := service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func assertHostLeaseErrorIsOnly(t *testing.T, label string, err error, want error, notWants ...error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s = %v, want %v", label, err, want)
	}
	for _, notWant := range notWants {
		if errors.Is(err, notWant) {
			t.Fatalf("%s must stay distinct from %v: %v", want, notWant, err)
		}
	}
}

func TestHostLease_MissingAssetsLoadingTimeoutCapacityLeaseNotFoundRuntimeNotReadyAreDistinct(t *testing.T) {
	t.Parallel()

	var service models.Service = hostLeasePeerService{
		inspect: map[string]error{
			"missing-assets": models.ErrHostMissingAssets,
			"loading-slow":   models.ErrHostLoadingTimeout,
		},
		acquire: map[string]error{
			"full-capacity":     models.ErrHostCapacityExhausted,
			"runtime-not-ready": models.ErrHostRuntimeNotReady,
		},
		release: map[string]error{
			"unknown-lease": models.ErrHostLeaseNotFound,
		},
		leases: map[string]models.HostLease{},
	}

	_, err := service.InspectRuntime(context.Background(), "missing-assets")
	assertHostLeaseErrorIsOnly(t, "InspectRuntime missing-assets", err, models.ErrHostMissingAssets,
		models.ErrHostLoadingTimeout, models.ErrHostCapacityExhausted)

	_, err = service.InspectRuntime(context.Background(), "loading-slow")
	assertHostLeaseErrorIsOnly(t, "InspectRuntime loading-slow", err, models.ErrHostLoadingTimeout,
		models.ErrHostMissingAssets, models.ErrHostRuntimeNotReady)

	_, err = service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "full-capacity"})
	assertHostLeaseErrorIsOnly(t, "AcquireLease full-capacity", err, models.ErrHostCapacityExhausted,
		models.ErrHostLeaseNotFound, models.ErrHostRuntimeNotReady)

	_, err = service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "runtime-not-ready"})
	assertHostLeaseErrorIsOnly(t, "AcquireLease runtime-not-ready", err, models.ErrHostRuntimeNotReady,
		models.ErrHostCapacityExhausted, models.ErrHostMissingAssets)

	err = service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "unknown-lease"})
	assertHostLeaseErrorIsOnly(t, "ReleaseLease unknown-lease", err, models.ErrHostLeaseNotFound,
		models.ErrHostCapacityExhausted, models.ErrHostRuntimeNotReady)
}

func TestHostLease_PeerCompilesWithoutNestedHostImport(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// use host/lease without models/internal/host or local managed-runtime imports.
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: "local-model"}); err != nil {
		t.Fatalf("ValidateInspectRuntimeRequest: %v", err)
	}
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{}); err == nil {
		t.Fatal("ValidateInspectRuntimeRequest empty name = nil, want error")
	}
	if !errors.Is(models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateInspectRuntimeRequest empty name must wrap ErrNotFound")
	}

	if err := models.ValidateAcquireLeaseRequest(models.AcquireLeaseRequest{ModelName: "local-model"}); err != nil {
		t.Fatalf("ValidateAcquireLeaseRequest: %v", err)
	}
	if !errors.Is(models.ValidateAcquireLeaseRequest(models.AcquireLeaseRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateAcquireLeaseRequest empty model must wrap ErrNotFound")
	}

	if err := models.ValidateReleaseLeaseRequest(models.ReleaseLeaseRequest{LeaseID: "lease-1"}); err != nil {
		t.Fatalf("ValidateReleaseLeaseRequest: %v", err)
	}
	if !errors.Is(models.ValidateReleaseLeaseRequest(models.ReleaseLeaseRequest{}), models.ErrHostLeaseNotFound) {
		t.Fatal("ValidateReleaseLeaseRequest empty lease id must wrap ErrHostLeaseNotFound")
	}

	var service models.Service = hostLeasePeerService{
		runtimes: map[string]models.Runtime{
			"local-model": {
				Identity:       "local-model",
				ReadinessState: models.ReadinessStateReady,
				LifecycleState: models.LifecycleStateInstalled,
			},
		},
		leases: map[string]models.HostLease{
			"local-model": {ID: "lease-1", Endpoint: "http://local", Identity: models.HostIdentity{Name: "local-model"}},
		},
	}
	if _, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"}); err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
}

// inferPeerService is a fake peer implementer of Models root Service that
// exercises plain infer/local-invocation contracts using only root-package types.
type inferPeerService struct {
	unsupportedRuntimeScopePeer
	results map[string]models.LocalInvocationResult
	fails   map[string]error
}

func (inferPeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return inferPeerService{}, nil
}

func (inferPeerService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: []models.Summary{}}, nil
}

func (inferPeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (inferPeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (inferPeerService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (inferPeerService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, models.ErrHostRuntimeNotReady
}

func (inferPeerService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrHostLeaseNotFound
}

func (s inferPeerService) InvokeLocal(_ context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	if !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{Handled: false}, nil
	}
	key := request.Worker.Model
	if fail, ok := s.fails[key]; ok {
		return models.LocalInvocationResult{}, fail
	}
	if result, ok := s.results[key]; ok {
		return result, nil
	}
	return models.LocalInvocationResult{Handled: false}, nil
}

func managedInferWorker(model string) models.LocalWorker {
	return models.LocalWorker{
		Name:          "local-worker",
		Type:          models.RuntimeWorkerTypeInference,
		Model:         model,
		ModelLocality: string(models.LocalityLocal),
	}
}

func TestInfer_ValidInvokeReturnsModelsOwnedHandledResult(t *testing.T) {
	t.Parallel()

	want := models.LocalInvocationResult{Handled: true, Content: "models-owned-output"}
	var service models.Service = inferPeerService{
		results: map[string]models.LocalInvocationResult{"local-model": want},
	}

	got, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Worker:         managedInferWorker("local-model"),
		ModelOperation: "generate",
	})
	if err != nil {
		t.Fatalf("InvokeLocal: %v", err)
	}
	if !got.Handled || got.Content != want.Content {
		t.Fatalf("InvokeLocal result = %#v, want Models-owned handled success", got)
	}
}

func TestInfer_NotHandledDeclinesWithoutTypedFailure(t *testing.T) {
	t.Parallel()

	var service models.Service = inferPeerService{}
	got, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Worker: models.LocalWorker{Name: "cloud-worker", Type: "AGENT_WORKER"},
	})
	if err != nil {
		t.Fatalf("InvokeLocal not-handled: %v", err)
	}
	if got.Handled {
		t.Fatal("InvokeLocal Handled = true, want false for declined peer path")
	}
}

func TestInfer_ReadinessAndUnsupportedResponseModeAreDistinct(t *testing.T) {
	t.Parallel()

	var service models.Service = inferPeerService{
		fails: map[string]error{
			"missing-model": (models.Runtime{
				Identity:       "missing-model",
				ReadinessState: models.ReadinessStateMissing,
				LifecycleState: models.LifecycleStateNotInstalled,
			}).InvocationError(),
			"loading-model": (models.Runtime{
				Identity:       "loading-model",
				ReadinessState: models.ReadinessStateLoading,
				LifecycleState: models.LifecycleStateLoading,
			}).InvocationError(),
			"failed-model": (models.Runtime{
				Identity:       "failed-model",
				ReadinessState: models.ReadinessStateFailed,
				LifecycleState: models.LifecycleStateNotInstalled,
			}).InvocationError(),
			"unsupported-model": (models.Runtime{
				Identity:       "unsupported-model",
				ReadinessState: models.ReadinessStateUnsupported,
				LifecycleState: models.LifecycleStateNotApplicable,
			}).InvocationError(),
			"bad-response-mode": models.ErrUnsupportedResponseMode,
		},
	}

	cases := []struct {
		model string
		want  error
	}{
		{model: "missing-model", want: models.ErrMissing},
		{model: "loading-model", want: models.ErrLoading},
		{model: "failed-model", want: models.ErrFailed},
		{model: "unsupported-model", want: models.ErrUnsupported},
		{model: "bad-response-mode", want: models.ErrUnsupportedResponseMode},
	}
	for _, tc := range cases {
		_, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{
			Worker: managedInferWorker(tc.model),
		})
		if !errors.Is(err, tc.want) {
			t.Fatalf("InvokeLocal %s = %v, want %v", tc.model, err, tc.want)
		}
		for _, other := range []error{
			models.ErrMissing,
			models.ErrLoading,
			models.ErrFailed,
			models.ErrUnsupported,
			models.ErrUnsupportedResponseMode,
		} {
			if other == tc.want {
				continue
			}
			if errors.Is(err, other) {
				t.Fatalf("InvokeLocal %s must keep %v distinct from %v: %v", tc.model, tc.want, other, err)
			}
		}
	}
}

func TestInfer_PeerCompilesWithoutNestedInvoker(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// invoke infer without models/internal/inference, models/internal/local, or
	// a nested invoker interface import.
	req := models.LocalInvocationRequest{Worker: managedInferWorker("local-model")}
	if err := models.ValidateLocalInvocationRequest(req); err != nil {
		t.Fatalf("ValidateLocalInvocationRequest: %v", err)
	}
	if err := models.ValidateLocalInvocationRequest(models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Type:          models.RuntimeWorkerTypeInference,
			ModelLocality: string(models.LocalityLocal),
		},
	}); err == nil {
		t.Fatal("ValidateLocalInvocationRequest empty managed model = nil, want error")
	}
	if !errors.Is(models.ValidateLocalInvocationRequest(models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Type:          models.RuntimeWorkerTypeInference,
			ModelLocality: string(models.LocalityLocal),
		},
	}), models.ErrNotFound) {
		t.Fatal("ValidateLocalInvocationRequest empty managed model must wrap ErrNotFound")
	}

	var service models.Service = inferPeerService{
		results: map[string]models.LocalInvocationResult{
			"local-model": {Handled: true, Content: "ok"},
		},
	}
	result, err := service.InvokeLocal(context.Background(), req)
	if err != nil {
		t.Fatalf("InvokeLocal: %v", err)
	}
	if !result.Handled || result.Content != "ok" {
		t.Fatalf("InvokeLocal result = %#v, want handled Models-owned shape", result)
	}
}
