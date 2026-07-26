package models_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"testing"
	"time"
)

// peerModelsService is a fake peer implementer of the singular Models root
// Service. It compiles against only the published root package and does not
// import models/internal.
type peerModelsService struct {
	unsupportedRuntimeScopePeer
}

func (peerModelsService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return peerModelsService{}, nil
}

func (peerModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: nil}, nil
}

func (peerModelsService) GetModel(_ context.Context, name string) (models.Detail, error) {
	return models.Detail{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
}

func (peerModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (peerModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupported
}

func (peerModelsService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, models.ErrHostRuntimeNotReady
}

func (peerModelsService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return models.ErrHostLeaseNotFound
}

func (peerModelsService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestRootServiceAuthority_FakePeerGetModelNotFound(t *testing.T) {
	t.Parallel()

	var service models.Service = peerModelsService{}
	_, err := service.GetModel(context.Background(), "missing-model")
	if err == nil {
		t.Fatal("GetModel error = nil, want ErrNotFound")
	}
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrNotFound", err)
	}
}

func TestRootServiceAuthority_AggregateSurfaceRemainsOnSingularService(t *testing.T) {
	t.Parallel()

	var service models.Service = peerModelsService{}

	bound, err := service.ForRuntime(models.RuntimeBinding{CacheDirectory: "cache"})
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}
	if bound == nil {
		t.Fatal("ForRuntime returned nil Service view")
	}

	list, err := service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if list.Results == nil {
		// Empty catalog is valid; Results must remain a usable Models-owned List.
		list.Results = []models.Summary{}
	}

	if _, err := service.PullModel(context.Background(), "model"); !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("PullModel error = %v, want ErrUnsupportedOperation", err)
	}
	if _, err := service.InspectRuntime(context.Background(), "model"); !errors.Is(err, models.ErrUnsupported) {
		t.Fatalf("InspectRuntime error = %v, want ErrUnsupported", err)
	}

	result, err := service.InvokeLocal(context.Background(), models.LocalInvocationRequest{})
	if err != nil {
		t.Fatalf("InvokeLocal: %v", err)
	}
	if result.Handled {
		t.Fatal("InvokeLocal Handled = true, want false for unsupported peer path")
	}
}

// sealedPeerService is a peer-shaped Models root consumer for story 007. It
// implements every published slice on the singular Service using only
// root-package types — no models/internal, HostProcessLauncher, CatalogHost,
// or nested host-supervisor imports.
type sealedPeerService struct {
	unsupportedRuntimeScopePeer
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
		list:   models.List{Results: []models.Summary{{Name: "local-model", Status: models.StatusReady}}},
		detail: models.Detail{Summary: models.Summary{Name: "local-model", Status: models.StatusReady}},
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

func TestInvocationErrorFromManagedRuntime_ReadyAllowsInvocation(t *testing.T) {
	t.Parallel()

	err := (models.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateInstalled,
	}).InvocationError()
	if err != nil {
		t.Fatalf("error = %v, want nil for READY", err)
	}
}

func TestInvocationErrorFromManagedRuntime_MissingUsesManagedVocabulary(t *testing.T) {
	t.Parallel()

	err := (models.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: models.ReadinessStateMissing,
		LifecycleState: models.LifecycleStateNotInstalled,
	}).InvocationError()
	if err == nil {
		t.Fatal("error = nil, want managed runtime missing")
	}
	if !errors.Is(err, models.ErrMissing) {
		t.Fatalf("error = %v, want ErrMissing", err)
	}
	var readinessErr *models.InvocationError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %T, want *InvocationError", err)
	}
	if readinessErr.ReadinessState != models.ReadinessStateMissing ||
		readinessErr.LifecycleState != models.LifecycleStateNotInstalled {
		t.Fatalf(
			"readiness = (%s, %s), want MISSING NOT_INSTALLED",
			readinessErr.ReadinessState,
			readinessErr.LifecycleState,
		)
	}
}

func TestInvocationErrorFromManagedRuntime_LoadingAndFailed(t *testing.T) {
	t.Parallel()

	loadingErr := (models.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: models.ReadinessStateLoading,
		LifecycleState: models.LifecycleStateLoading,
	}).InvocationError()
	if !errors.Is(loadingErr, models.ErrLoading) {
		t.Fatalf("loading error = %v, want ErrLoading", loadingErr)
	}

	failedErr := (models.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: models.ReadinessStateFailed,
		LifecycleState: models.LifecycleStateNotInstalled,
	}).InvocationError()
	if !errors.Is(failedErr, models.ErrFailed) {
		t.Fatalf("failed error = %v, want ErrFailed", failedErr)
	}
}

// scopedInferencePeer characterizes lease-backed inference and cancellation
// using only the Models root contract.
type scopedInferencePeer struct {
	*scopedHostLeasePeer
	nextInvocation int
	invocations    map[models.ModelInvocationRef]models.InvokeModelResult
	failures       map[string]error
}

func newScopedInferencePeer() *scopedInferencePeer {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	return &scopedInferencePeer{
		scopedHostLeasePeer: newScopedHostLeasePeer(func() time.Time { return now }),
		invocations:         make(map[models.ModelInvocationRef]models.InvokeModelResult),
		failures:            make(map[string]error),
	}
}

func (s *scopedInferencePeer) InvokeModelWithLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if err := s.validateInvocationRequest(request); err != nil {
		return models.InvokeModelResult{}, err
	}
	if request.ResponseMode != "" &&
		request.ResponseMode != models.ResponseModeAudioStream {
		return models.InvokeModelResult{}, models.ErrUnsupportedResponseMode
	}
	if failure, ok := s.failures[request.Operation]; ok &&
		!releasesInferenceCapacity(failure) {
		return models.InvokeModelResult{}, failure
	}

	s.nextInvocation++
	invocation, err := (models.ModelInvocationRef{}).Parse(
		fmt.Sprintf("infer-peer:invocation:%d", s.nextInvocation),
	)
	if err != nil {
		return models.InvokeModelResult{}, err
	}
	result := models.InvokeModelResult{
		Invocation:       invocation,
		Scope:            request.Scope,
		Lease:            request.Lease,
		ModelName:        request.ModelName,
		Operation:        request.Operation,
		Status:           models.ModelInvocationStatusAccepted,
		LeaseDisposition: models.InvocationLeaseRetained,
	}
	if ctx.Err() != nil {
		result.Status = models.ModelInvocationStatusCancelled
		result.LeaseDisposition = models.InvocationLeaseReleased
		result.CancellationOutcome = models.InvocationCancellationRequested
		s.releaseInvocationLease(request.Lease)
		s.invocations[invocation] = result
		return result.Clone(), fmt.Errorf("%w: %v", models.ErrInferenceCancelled, ctx.Err())
	}
	if failure, ok := s.failures[request.Operation]; ok {
		result.Status = models.ModelInvocationStatusFailed
		result.LeaseDisposition = models.InvocationLeaseReleased
		s.releaseInvocationLease(request.Lease)
		s.invocations[invocation] = result
		return result.Clone(), failure
	}
	if request.Operation == "hold" {
		s.invocations[invocation] = result
		return result.Clone(), nil
	}

	artifact, err := (models.InferenceArtifactRef{}).Parse("infer-peer:artifact:1")
	if err != nil {
		return models.InvokeModelResult{}, err
	}
	result.Status = models.ModelInvocationStatusCompleted
	result.LeaseDisposition = models.InvocationLeaseReleased
	result.Content = []models.InferenceContent{{
		ContentType: "text/plain",
		Content:     "models-owned-output",
	}}
	result.Artifacts = []models.InferenceArtifact{{
		Artifact:  artifact,
		Name:      "result.txt",
		MediaType: "text/plain",
		SizeBytes: 19,
		Properties: map[string]string{
			"digest": "sha256:detached",
		},
	}}
	s.releaseInvocationLease(request.Lease)
	s.invocations[invocation] = result.Clone()
	return result.Clone(), nil
}

func (s *scopedInferencePeer) validateInvocationRequest(request models.InvokeModelRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return err
	}
	lease, ok := s.leases[request.Lease]
	if !ok || lease.Scope != request.Scope || lease.ModelName != request.ModelName {
		return models.ErrHostLeaseNotFound
	}
	if lease.Holder != request.Holder {
		return models.ErrHostInvalidHolder
	}
	if lease.Status == models.ModelLeaseStatusExpired || !s.now().Before(lease.ExpiresAt) {
		return models.ErrHostLeaseExpired
	}
	if lease.Status != models.ModelLeaseStatusActive {
		return models.ErrHostLeaseNotFound
	}
	return nil
}

func releasesInferenceCapacity(err error) bool {
	return errors.Is(err, models.ErrInferenceTimeout) ||
		errors.Is(err, models.ErrInferenceFailed)
}

func (s *scopedInferencePeer) CancelInvocation(
	_ context.Context,
	request models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	if err := request.Validate(); err != nil {
		return models.CancelInvocationResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.CancelInvocationResult{}, err
	}
	invocation, ok := s.invocations[request.Invocation]
	if !ok || invocation.Scope != request.Scope {
		return models.CancelInvocationResult{}, models.ErrInvocationNotFound
	}
	result := models.CancelInvocationResult{
		Invocation:       request.Invocation,
		Status:           invocation.Status,
		LeaseDisposition: invocation.LeaseDisposition,
	}
	switch invocation.Status {
	case models.ModelInvocationStatusAccepted:
		invocation.Status = models.ModelInvocationStatusCancelled
		invocation.LeaseDisposition = models.InvocationLeaseReleased
		invocation.CancellationOutcome = models.InvocationCancellationRequested
		s.releaseInvocationLease(invocation.Lease)
		s.invocations[request.Invocation] = invocation
		result.Status = invocation.Status
		result.LeaseDisposition = invocation.LeaseDisposition
		result.Outcome = models.InvocationCancellationRequested
	case models.ModelInvocationStatusCancelled:
		result.Outcome = models.InvocationCancellationAlreadyCancelled
	default:
		result.Outcome = models.InvocationCancellationAlreadyCompleted
	}
	return result, nil
}

func (s *scopedInferencePeer) releaseInvocationLease(ref models.ModelLeaseRef) {
	lease := s.leases[ref]
	lease.Status = models.ModelLeaseStatusReleased
	s.leases[ref] = lease
}

func openReadyInferenceLease(
	t *testing.T,
	service *scopedInferencePeer,
	modelName string,
) (models.RuntimeScopeRef, models.ModelLease) {
	t.Helper()
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	_, err = service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: opened.Scope,
		Name:  modelName,
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  opened.Scope,
		Name:   modelName,
		Holder: "worker-1",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	return opened.Scope, acquired.Lease
}

func inferenceRequest(
	scope models.RuntimeScopeRef,
	lease models.ModelLease,
	operation string,
) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope:     scope,
		Lease:     lease.Lease,
		Holder:    lease.Holder,
		ModelName: lease.ModelName,
		Operation: operation,
		Input: models.InferenceInput{
			ContentType: "text/plain",
			Content:     "hello",
		},
	}
}

func TestScopedInference_CompletionReturnsDetachedOutputAndReleasesLease(t *testing.T) {
	t.Parallel()

	fake := newScopedInferencePeer()
	scope, lease := openReadyInferenceLease(t, fake, "local-model")
	var service models.Service = fake
	result, err := service.InvokeModelWithLease(
		context.Background(),
		inferenceRequest(scope, lease, "generate"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if result.Invocation.IsZero() || result.Lease != lease.Lease ||
		result.Status != models.ModelInvocationStatusCompleted ||
		result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("InvokeModelWithLease = %#v, want completed lease-backed result", result)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "models-owned-output" ||
		len(result.Artifacts) != 1 || result.Artifacts[0].Artifact.IsZero() {
		t.Fatalf("InvokeModelWithLease output = %#v, want normalized content and opaque artifact", result)
	}
	result.Artifacts[0].Properties["digest"] = "peer-mutated"
	retained := fake.invocations[result.Invocation]
	if retained.Artifacts[0].Properties["digest"] != "sha256:detached" {
		t.Fatalf("InvokeModelWithLease retained peer artifact mutation: %#v", retained.Artifacts)
	}
	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: result.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation completed: %v", err)
	}
	if cancelled.Outcome != models.InvocationCancellationAlreadyCompleted {
		t.Fatalf("late cancellation = %#v, want ALREADY_COMPLETED", cancelled)
	}
}

func TestScopedInference_ExplicitAndContextCancellationConverge(t *testing.T) {
	t.Parallel()

	fake := newScopedInferencePeer()
	scope, lease := openReadyInferenceLease(t, fake, "held-model")
	var service models.Service = fake
	accepted, err := service.InvokeModelWithLease(
		context.Background(),
		inferenceRequest(scope, lease, "hold"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease hold: %v", err)
	}
	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: accepted.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation: %v", err)
	}
	if cancelled.Outcome != models.InvocationCancellationRequested ||
		cancelled.Status != models.ModelInvocationStatusCancelled ||
		cancelled.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("CancelInvocation = %#v, want cancelled released outcome", cancelled)
	}
	repeated, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: accepted.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation repeated: %v", err)
	}
	if repeated.Outcome != models.InvocationCancellationAlreadyCancelled {
		t.Fatalf("repeated cancellation = %#v, want ALREADY_CANCELLED", repeated)
	}

	contextScope, contextLease := openReadyInferenceLease(t, fake, "context-model")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	contextResult, err := service.InvokeModelWithLease(
		ctx,
		inferenceRequest(contextScope, contextLease, "generate"),
	)
	if !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("context cancellation = %v, want ErrInferenceCancelled", err)
	}
	if contextResult.Status != cancelled.Status ||
		contextResult.LeaseDisposition != cancelled.LeaseDisposition ||
		contextResult.CancellationOutcome != cancelled.Outcome {
		t.Fatalf("context cancellation = %#v, want explicit cancellation state %#v", contextResult, cancelled)
	}
}

func assertNormalizedInferenceFailures(t *testing.T) {
	t.Helper()
	failures := []error{
		models.ErrAssetUnavailable,
		models.ErrHostRuntimeNotReady,
		models.ErrHostCapacityExhausted,
		models.ErrHostLeaseExpired,
		models.ErrUnsupportedModelOperation,
		models.ErrInferenceTimeout,
		models.ErrInferenceFailed,
	}
	for _, want := range failures {
		fake := newScopedInferencePeer()
		scope, lease := openReadyInferenceLease(t, fake, "failure-model")
		operation := want.Error()
		fake.failures[operation] = want
		result, err := fake.InvokeModelWithLease(
			context.Background(),
			inferenceRequest(scope, lease, operation),
		)
		if !errors.Is(err, want) {
			t.Fatalf("InvokeModelWithLease %v = %v", want, err)
		}
		for _, other := range failures {
			if other != want && errors.Is(err, other) {
				t.Fatalf("InvokeModelWithLease must keep %v distinct from %v: %v", want, other, err)
			}
		}
		if releasesInferenceCapacity(want) {
			if result.Status != models.ModelInvocationStatusFailed ||
				result.LeaseDisposition != models.InvocationLeaseReleased {
				t.Fatalf("terminal failure %v = %#v, want failed released result", want, result)
			}
		} else if !result.Invocation.IsZero() {
			t.Fatalf("pre-acceptance failure %v returned invocation %#v", want, result)
		}
	}
}

func assertInferenceCapabilityFailures(t *testing.T) {
	t.Helper()
	fake := newScopedInferencePeer()
	scope, lease := openReadyInferenceLease(t, fake, "response-model")
	badResponse := inferenceRequest(scope, lease, "generate")
	badResponse.ResponseMode = models.ResponseMode("JSON")
	_, err := fake.InvokeModelWithLease(context.Background(), badResponse)
	if !errors.Is(err, models.ErrUnsupportedResponseMode) {
		t.Fatalf("unsupported response mode = %v, want ErrUnsupportedResponseMode", err)
	}
	invalidLease := inferenceRequest(scope, lease, "generate")
	invalidLease.Lease = models.ModelLeaseRef{}
	_, err = fake.InvokeModelWithLease(context.Background(), invalidLease)
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("invalid lease = %v, want ErrHostLeaseNotFound", err)
	}
}

func assertInferenceScopeFailures(t *testing.T) {
	t.Helper()
	fake := newScopedInferencePeer()
	scope, lease := openReadyInferenceLease(t, fake, "scope-model")
	foreign, err := (models.RuntimeScopeRef{}).Parse("other:1")
	if err != nil {
		t.Fatalf("parse foreign scope: %v", err)
	}
	stale, err := (models.RuntimeScopeRef{}).Parse("host-peer:999")
	if err != nil {
		t.Fatalf("parse stale scope: %v", err)
	}
	for _, tc := range []struct {
		scope models.RuntimeScopeRef
		want  error
	}{
		{scope: models.RuntimeScopeRef{}, want: models.ErrRuntimeScopeInvalid},
		{scope: foreign, want: models.ErrRuntimeScopeForeign},
		{scope: stale, want: models.ErrRuntimeScopeStale},
	} {
		request := inferenceRequest(tc.scope, lease, "generate")
		_, invokeErr := fake.InvokeModelWithLease(context.Background(), request)
		if !errors.Is(invokeErr, tc.want) {
			t.Fatalf("InvokeModelWithLease scope %q = %v, want %v", tc.scope.String(), invokeErr, tc.want)
		}
	}
	unknown, err := (models.ModelInvocationRef{}).Parse("infer-peer:unknown")
	if err != nil {
		t.Fatalf("parse unknown invocation: %v", err)
	}
	_, err = fake.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: unknown,
	})
	if !errors.Is(err, models.ErrInvocationNotFound) {
		t.Fatalf("CancelInvocation unknown = %v, want ErrInvocationNotFound", err)
	}
}

func TestScopedInference_NormalizedFailuresStayDistinct(t *testing.T) {
	t.Parallel()
	assertNormalizedInferenceFailures(t)
	assertInferenceCapabilityFailures(t)
	assertInferenceScopeFailures(t)
}
