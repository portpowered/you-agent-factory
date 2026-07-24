package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// sealedPeerService is a peer-shaped Models root consumer for story 007. It
// implements every published slice on the singular Service using only
// root-package types — no models/internal, HostProcessLauncher, CatalogHost,
// or nested host-supervisor imports.
type sealedPeerService struct {
	bound bool

	list    models.List
	detail  models.Detail
	pull    models.PullResult
	runtime models.Runtime
	lease   models.HostLease
	infer   models.LocalInvocationResult

	listErr    error
	getErr     error
	pullErr    error
	inspectErr error
	acquireErr error
	releaseErr error
	inferErr   error
}

func (s sealedPeerService) ForRuntime(binding models.RuntimeBinding) (models.Service, error) {
	if err := models.ValidateRuntimeBinding(binding); err != nil {
		return nil, err
	}
	next := s
	next.bound = true
	return next, nil
}

func (s sealedPeerService) ListModels(context.Context) (models.List, error) {
	if s.listErr != nil {
		return models.List{}, s.listErr
	}
	return s.list, nil
}

func (s sealedPeerService) GetModel(_ context.Context, name string) (models.Detail, error) {
	if err := models.ValidateGetModelRequest(models.GetModelRequest{Name: name}); err != nil {
		return models.Detail{}, err
	}
	if s.getErr != nil {
		return models.Detail{}, s.getErr
	}
	detail := s.detail
	if detail.Name == "" {
		detail.Summary.Name = name
	}
	return detail, nil
}

func (s sealedPeerService) PullModel(_ context.Context, name string) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(models.PullModelRequest{Name: name}); err != nil {
		return models.PullResult{}, err
	}
	if s.pullErr != nil {
		return models.PullResult{}, s.pullErr
	}
	result := s.pull
	if result.ModelName == "" {
		result.ModelName = name
	}
	return result, nil
}

func (s sealedPeerService) InspectRuntime(_ context.Context, name string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: name}); err != nil {
		return models.Runtime{}, err
	}
	if s.inspectErr != nil {
		return models.Runtime{}, s.inspectErr
	}
	runtime := s.runtime
	if runtime.Identity == "" {
		runtime.Identity = name
	}
	return runtime, nil
}

func (s sealedPeerService) AcquireLease(_ context.Context, request models.AcquireLeaseRequest) (models.HostLease, error) {
	if err := models.ValidateAcquireLeaseRequest(request); err != nil {
		return models.HostLease{}, err
	}
	if s.acquireErr != nil {
		return models.HostLease{}, s.acquireErr
	}
	lease := s.lease
	if lease.ID == "" {
		lease.ID = "lease-" + request.ModelName
	}
	if request.Holder != "" {
		lease.Holder = request.Holder
	}
	return lease, nil
}

func (s sealedPeerService) ReleaseLease(_ context.Context, request models.ReleaseLeaseRequest) error {
	if err := models.ValidateReleaseLeaseRequest(request); err != nil {
		return err
	}
	if s.releaseErr != nil {
		return s.releaseErr
	}
	return nil
}

func (s sealedPeerService) InvokeLocal(_ context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	if s.inferErr != nil {
		return models.LocalInvocationResult{}, s.inferErr
	}
	if !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{Handled: false}, nil
	}
	return s.infer, nil
}

type sealedSuccessExpectations struct {
	list    models.List
	detail  models.Detail
	pull    models.PullResult
	runtime models.Runtime
	lease   models.HostLease
	infer   models.LocalInvocationResult
}

func assertSealedCatalogSuccess(t *testing.T, service models.Service, want sealedSuccessExpectations) {
	t.Helper()

	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("catalog ListModels: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].Name != want.list.Results[0].Name {
		t.Fatalf("catalog ListModels = %#v, want Models-owned list", list)
	}

	detail, err := service.GetModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("catalog GetModel: %v", err)
	}
	if detail.Name != want.detail.Name || detail.Status != want.detail.Status {
		t.Fatalf("catalog GetModel = %#v, want Models-owned detail", detail)
	}
}

func assertSealedAssetsSuccess(t *testing.T, service models.Service, want sealedSuccessExpectations) {
	t.Helper()

	pull, err := service.PullModel(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("assets PullModel: %v", err)
	}
	if pull.ModelName != want.pull.ModelName || pull.ManagedPullOutcome != want.pull.ManagedPullOutcome {
		t.Fatalf("assets PullModel = %#v, want Models-owned pull result", pull)
	}
}

func assertSealedHostLeaseSuccess(t *testing.T, service models.Service, want sealedSuccessExpectations) {
	t.Helper()

	runtime, err := service.InspectRuntime(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("host InspectRuntime: %v", err)
	}
	if runtime.Identity != want.runtime.Identity || runtime.ReadinessState != want.runtime.ReadinessState {
		t.Fatalf("host InspectRuntime = %#v, want Models-owned readiness", runtime)
	}

	lease, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{
		ModelName: "local-model",
		Holder:    "peer",
	})
	if err != nil {
		t.Fatalf("host AcquireLease: %v", err)
	}
	if lease.ID != want.lease.ID || lease.Holder != "peer" || lease.Endpoint != want.lease.Endpoint {
		t.Fatalf("host AcquireLease = %#v, want Models-owned lease", lease)
	}
	if err := service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: lease.ID}); err != nil {
		t.Fatalf("host ReleaseLease: %v", err)
	}
}

func assertSealedInferSuccess(t *testing.T, service models.Service, want sealedSuccessExpectations) {
	t.Helper()

	infer, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Name:          "local-worker",
			Type:          models.RuntimeWorkerTypeInference,
			Model:         "local-model",
			ModelLocality: string(models.LocalityLocal),
		},
		ModelOperation: "generate",
	})
	if err != nil {
		t.Fatalf("infer InvokeLocal: %v", err)
	}
	if !infer.Handled || infer.Content != want.infer.Content {
		t.Fatalf("infer InvokeLocal = %#v, want Models-owned handled result", infer)
	}
}

func assertSealedSuccessPaths(t *testing.T, service models.Service, want sealedSuccessExpectations) {
	t.Helper()
	assertSealedCatalogSuccess(t, service, want)
	assertSealedAssetsSuccess(t, service, want)
	assertSealedHostLeaseSuccess(t, service, want)
	assertSealedInferSuccess(t, service, want)
}

func TestRootContractSeal_AllSlicesReachableThroughSingularService(t *testing.T) {
	t.Parallel()

	want := sealedSuccessExpectations{
		list:    models.List{Results: []models.Summary{{Name: "local-model", Status: models.StatusReady}}},
		detail:  models.Detail{Summary: models.Summary{Name: "local-model", Status: models.StatusReady}},
		pull: models.PullResult{
			ModelName:          "local-model",
			ManagedPullOutcome: string(models.PullOutcomeAlreadyPresent),
			DownloadedFiles:    []models.DownloadedFile{{Path: "weights.bin", Bytes: 12}},
		},
		runtime: models.Runtime{
			Identity:       "local-model",
			ReadinessState: models.ReadinessStateReady,
			LifecycleState: models.LifecycleStateLoaded,
			Locality:       models.LocalityLocal,
		},
		lease: models.HostLease{
			ID:       "lease-1",
			Identity: models.HostIdentity{Name: "local-model", Locality: models.LocalityLocal},
			Endpoint: "http://127.0.0.1:8080",
		},
		infer: models.LocalInvocationResult{Handled: true, Content: "sealed-output"},
	}

	var service models.Service = sealedPeerService{
		list:    want.list,
		detail:  want.detail,
		pull:    want.pull,
		runtime: want.runtime,
		lease:   want.lease,
		infer:   want.infer,
	}

	bound, err := service.ForRuntime(models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig:  func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	})
	if err != nil {
		t.Fatalf("runtime-scope ForRuntime: %v", err)
	}
	if bound == nil {
		t.Fatal("runtime-scope ForRuntime must return singular root Service view")
	}
	assertSealedSuccessPaths(t, bound, want)
}

func TestRootContractSeal_TypedFailuresStayDistinctPerSlice(t *testing.T) {
	t.Parallel()

	var service models.Service = sealedPeerService{
		listErr:    models.ErrUnavailable,
		getErr:     fmt.Errorf("%w: missing", models.ErrNotFound),
		pullErr:    models.ErrNotAvailable,
		inspectErr: models.ErrHostMissingAssets,
		acquireErr: models.ErrHostCapacityExhausted,
		releaseErr: models.ErrHostLeaseNotFound,
		inferErr:   models.ErrUnsupportedResponseMode,
	}

	if _, err := service.ForRuntime(models.RuntimeBinding{}); !errors.Is(err, models.ErrInvalidRuntimeBinding) {
		t.Fatalf("runtime-scope missing cache = %v, want ErrInvalidRuntimeBinding", err)
	}
	if _, err := service.ListModels(context.Background()); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("catalog unavailable = %v, want ErrUnavailable", err)
	}
	if _, err := service.GetModel(context.Background(), "missing"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("catalog missing = %v, want ErrNotFound", err)
	}
	if _, err := service.PullModel(context.Background(), "missing"); !errors.Is(err, models.ErrNotAvailable) {
		t.Fatalf("assets not-available = %v, want ErrNotAvailable", err)
	}
	if _, err := service.InspectRuntime(context.Background(), "missing"); !errors.Is(err, models.ErrHostMissingAssets) {
		t.Fatalf("host missing-assets = %v, want ErrHostMissingAssets", err)
	}
	if _, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "busy"}); !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("host capacity = %v, want ErrHostCapacityExhausted", err)
	}
	if err := service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "gone"}); !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("host lease-not-found = %v, want ErrHostLeaseNotFound", err)
	}
	if _, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Name:          "local-worker",
			Type:          models.RuntimeWorkerTypeInference,
			Model:         "local-model",
			ModelLocality: string(models.LocalityLocal),
		},
		ModelOperation: "generate",
	}); !errors.Is(err, models.ErrUnsupportedResponseMode) {
		t.Fatalf("infer unsupported-response-mode = %v, want ErrUnsupportedResponseMode", err)
	}
}

func TestRootContractSeal_PeerDoesNotNeedConstructionPortsForPublishedSlices(t *testing.T) {
	t.Parallel()

	// Compiling and exercising this Service fake with only RuntimeBinding /
	// plain request vocabulary proves HostProcessLauncher, HostManagedProcess,
	// ProcessDependencies, and LocalRuntimeHooks are Wire/construction ports,
	// not the peer-facing source of truth for runtime-scope, catalog, assets,
	// host/lease, or infer.
	var service models.Service = sealedPeerService{
		list: models.List{Results: []models.Summary{}},
		pull: models.PullResult{ManagedPullOutcome: string(models.PullOutcomeAlreadyPresent)},
		runtime: models.Runtime{
			Identity:       "local-model",
			ReadinessState: models.ReadinessStateReady,
		},
		lease: models.HostLease{ID: "lease-1", Endpoint: "http://127.0.0.1:1"},
		infer: models.LocalInvocationResult{Handled: true},
	}

	bound, err := service.ForRuntime(models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig:  func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	})
	if err != nil {
		t.Fatalf("ForRuntime without construction ports: %v", err)
	}
	if bound == nil {
		t.Fatal("ForRuntime returned nil Service")
	}
	if _, err := bound.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if _, err := bound.PullModel(context.Background(), "local-model"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if _, err := bound.InspectRuntime(context.Background(), "local-model"); err != nil {
		t.Fatalf("InspectRuntime: %v", err)
	}
	if _, err := bound.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"}); err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if err := bound.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if _, err := bound.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Name:          "local-worker",
			Type:          models.RuntimeWorkerTypeInference,
			Model:         "local-model",
			ModelLocality: string(models.LocalityLocal),
		},
	}); err != nil {
		t.Fatalf("InvokeLocal: %v", err)
	}
}
