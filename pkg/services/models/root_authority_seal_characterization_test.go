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

// sealedPeerService implements every target operation through the singular
// Models root Service. It imports only root contract types and shares one
// opaque runtime scope across all capability slices.
type sealedPeerService struct {
	*scopedInferencePeer
	catalog    map[string]models.Detail
	assets     map[string]models.AssetSnapshot
	catalogErr error
	assetErr   error
}

var _ models.Service = (*sealedPeerService)(nil)

func newSealedPeerService() *sealedPeerService {
	return &sealedPeerService{
		scopedInferencePeer: newScopedInferencePeer(),
		catalog: map[string]models.Detail{
			"local-model": {
				Summary: models.Summary{
					Name:       "local-model",
					Status:     models.StatusReady,
					Operations: []models.Operation{{Name: "generate"}, {Name: "hold"}},
					ManagedRuntime: models.Runtime{
						Identity:       "local-model",
						ReadinessState: models.ReadinessStateReady,
						LifecycleState: models.LifecycleStateInstalled,
					},
				},
			},
		},
		assets: make(map[string]models.AssetSnapshot),
	}
}

func (s *sealedPeerService) ListCatalog(
	_ context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.ListModelsResult{}, err
	}
	if s.catalogErr != nil {
		return models.ListModelsResult{}, s.catalogErr
	}
	result := models.ListModelsResult{Models: make([]models.Summary, 0, len(s.catalog))}
	for _, detail := range s.catalog {
		result.Models = append(result.Models, detail.Summary.Clone())
	}
	return result, nil
}

func (s *sealedPeerService) GetCatalogModel(
	_ context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.GetModelResult{}, err
	}
	if s.catalogErr != nil {
		return models.GetModelResult{}, s.catalogErr
	}
	detail, ok := s.catalog[request.Name]
	if !ok {
		return models.GetModelResult{}, models.ErrNotFound
	}
	return models.GetModelResult{Model: detail.Clone()}, nil
}

func (s *sealedPeerService) GetModelReadiness(
	ctx context.Context,
	request models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	found, err := s.GetCatalogModel(ctx, models.GetModelRequest{
		Scope: request.Scope, Name: request.Name, Operation: request.Operation,
	})
	if err != nil {
		return models.GetModelReadinessResult{}, err
	}
	return models.GetModelReadinessResult{
		ModelName: found.Model.Name,
		Readiness: found.Model.ManagedRuntime.Clone(),
	}, nil
}

func (s *sealedPeerService) PrepareModelAssets(
	_ context.Context,
	request models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	if s.assetErr != nil {
		return models.PrepareModelAssetsResult{}, s.assetErr
	}
	asset := models.AssetSnapshot{
		ModelName: request.Name,
		Readiness: models.AssetReadinessAvailable,
		Integrity: models.AssetIntegrityVerified,
		Artifacts: []models.AssetArtifact{{Name: "weights.bin", Bytes: 42, SHA256: "abc"}},
	}
	s.assets[request.Name] = asset.Clone()
	return models.PrepareModelAssetsResult{
		Asset: asset.Clone(), Outcome: models.AssetPreparationPrepared,
	}, nil
}

func (s *sealedPeerService) InspectModelAssets(
	_ context.Context,
	request models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.InspectModelAssetsResult{}, err
	}
	if s.assetErr != nil {
		return models.InspectModelAssetsResult{}, s.assetErr
	}
	asset, ok := s.assets[request.Name]
	if !ok {
		return models.InspectModelAssetsResult{}, models.ErrAssetUnavailable
	}
	return models.InspectModelAssetsResult{Asset: asset.Clone()}, nil
}

func (s *sealedPeerService) RemoveModelAssets(
	_ context.Context,
	request models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	if err := request.Validate(); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.RemoveModelAssetsResult{}, err
	}
	if s.assetErr != nil {
		return models.RemoveModelAssetsResult{}, s.assetErr
	}
	delete(s.assets, request.Name)
	return models.RemoveModelAssetsResult{
		ModelName: request.Name,
		Readiness: models.AssetReadinessMissing,
		Outcome:   models.AssetRemovalRemoved,
	}, nil
}

func assertSealedCatalogAndAssets(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
) {
	t.Helper()
	list, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: scope})
	if err != nil || len(list.Models) != 1 {
		t.Fatalf("ListCatalog = %#v, %v", list, err)
	}
	if _, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	if _, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("GetModelReadiness: %v", err)
	}
	if _, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if _, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope, Name: "local-model", VerifyIntegrity: true,
	}); err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
}

func acquireSealedLease(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
	holder string,
) models.ModelLease {
	t.Helper()
	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: holder,
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if _, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope, Lease: acquired.Lease.Lease,
	}); err != nil {
		t.Fatalf("GetModelLease: %v", err)
	}
	return acquired.Lease
}

func assertSealedHostLeaseAndInference(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
) {
	t.Helper()
	if _, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	if _, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	released := acquireSealedLease(t, service, scope, "release-peer")
	if _, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope, Lease: released.Lease,
	}); err != nil {
		t.Fatalf("ReleaseModelLease: %v", err)
	}
	inference := acquireSealedLease(t, service, scope, "infer-peer")
	if _, err := service.InvokeModelWithLease(context.Background(), inferenceRequest(
		scope, inference, "generate",
	)); err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	held := acquireSealedLease(t, service, scope, "infer-peer")
	accepted, err := service.InvokeModelWithLease(context.Background(), inferenceRequest(scope, held, "hold"))
	if err != nil {
		t.Fatalf("InvokeModelWithLease hold: %v", err)
	}
	if _, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: accepted.Invocation,
	}); err != nil {
		t.Fatalf("CancelInvocation: %v", err)
	}
	if _, err := service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("StopModelHost: %v", err)
	}
}

func TestRootContractSeal_AggregateFakeExercisesEveryTargetOperation(t *testing.T) {
	t.Parallel()

	fake := newSealedPeerService()
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	assertSealedCatalogAndAssets(t, service, opened.Scope)
	assertSealedHostLeaseAndInference(t, service, opened.Scope)
	if _, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	closed, err := service.CloseRuntimeScope(
		context.Background(),
		models.CloseRuntimeScopeRequest{Scope: opened.Scope},
	)
	if err != nil || !closed.Closed {
		t.Fatalf("CloseRuntimeScope = %#v, %v", closed, err)
	}
}

func assertRootSealErrorIs(t *testing.T, label string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s = %v, want %v", label, err, want)
	}
}

func TestRootContractSeal_AggregateFakePreservesNormalizedFailures(t *testing.T) {
	t.Parallel()

	fake := newSealedPeerService()
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	_, err = service.ListCatalog(context.Background(), models.ListModelsRequest{})
	assertRootSealErrorIs(t, "scope", err, models.ErrRuntimeScopeInvalid)
	fake.catalogErr = models.ErrUnavailable
	_, err = service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	assertRootSealErrorIs(t, "catalog", err, models.ErrUnavailable)
	fake.assetErr = models.ErrAssetIntegrityFailed
	_, err = service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	assertRootSealErrorIs(t, "asset", err, models.ErrAssetIntegrityFailed)
	_, err = service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	assertRootSealErrorIs(t, "host", err, models.ErrHostRuntimeNotReady)
	unknownLease, _ := (models.ModelLeaseRef{}).Parse("host-peer:lease:missing")
	_, err = service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: opened.Scope, Lease: unknownLease,
	})
	assertRootSealErrorIs(t, "lease", err, models.ErrHostLeaseNotFound)
	unknownInvocation, _ := (models.ModelInvocationRef{}).Parse("infer-peer:invocation:missing")
	_, err = service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: opened.Scope, Invocation: unknownInvocation,
	})
	assertRootSealErrorIs(t, "cancellation", err, models.ErrInvocationNotFound)
	if _, err = service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: opened.Scope, Name: "local-model",
	}); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	lease := acquireSealedLease(t, service, opened.Scope, "infer-peer")
	fake.failures["failure"] = models.ErrInferenceFailed
	_, err = service.InvokeModelWithLease(
		context.Background(),
		inferenceRequest(opened.Scope, lease, "failure"),
	)
	assertRootSealErrorIs(t, "inference", err, models.ErrInferenceFailed)
}

func TestRootContractSeal_OpenScopeNeedsNoConstructionEffects(t *testing.T) {
	t.Parallel()

	fake := newSealedPeerService()
	request := models.OpenRuntimeScopeRequest{Config: models.RuntimeScopeConfig{
		CacheDirectory: "detached-cache",
		Runtime: models.RuntimeConfig{Workers: []models.RuntimeWorker{{
			Name: "peer", Args: []string{"--detached"},
		}}},
	}}
	opened, err := fake.OpenRuntimeScope(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	request.Config.Runtime.Workers[0].Args[0] = "--mutated"
	stored := fake.open[opened.Scope]
	if stored.Runtime.Workers[0].Args[0] != "--detached" {
		t.Fatalf("stored config = %#v, want detached configuration", stored)
	}
}

func TestRootContractSeal_LegacyRequestValidationRemainsObservable(t *testing.T) {
	t.Parallel()

	if err := models.ValidateRuntimeBinding(models.RuntimeBinding{}); !errors.Is(
		err, models.ErrInvalidRuntimeBinding,
	) {
		t.Fatalf("ValidateRuntimeBinding = %v, want ErrInvalidRuntimeBinding", err)
	}
	if err := models.ValidateRuntimeBinding(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	}); err != nil {
		t.Fatalf("ValidateRuntimeBinding valid: %v", err)
	}
	validations := []struct {
		name    string
		invalid error
		want    error
		valid   error
	}{
		{
			name:    "get model",
			invalid: models.ValidateGetModelRequest(models.GetModelRequest{}),
			want:    models.ErrNotFound,
			valid: models.ValidateGetModelRequest(
				models.GetModelRequest{Name: "local-model"},
			),
		},
		{
			name:    "pull model",
			invalid: models.ValidatePullModelRequest(models.PullModelRequest{}),
			want:    models.ErrNotFound,
			valid: models.ValidatePullModelRequest(
				models.PullModelRequest{Name: "local-model"},
			),
		},
		{
			name:    "inspect runtime",
			invalid: models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{}),
			want:    models.ErrNotFound,
			valid: models.ValidateInspectRuntimeRequest(
				models.InspectRuntimeRequest{Name: "local-model"},
			),
		},
		{
			name:    "acquire lease",
			invalid: models.ValidateAcquireLeaseRequest(models.AcquireLeaseRequest{}),
			want:    models.ErrNotFound,
			valid: models.ValidateAcquireLeaseRequest(
				models.AcquireLeaseRequest{ModelName: "local-model"},
			),
		},
		{
			name:    "release lease",
			invalid: models.ValidateReleaseLeaseRequest(models.ReleaseLeaseRequest{}),
			want:    models.ErrHostLeaseNotFound,
			valid: models.ValidateReleaseLeaseRequest(
				models.ReleaseLeaseRequest{LeaseID: "lease-1"},
			),
		},
	}
	for _, validation := range validations {
		if !errors.Is(validation.invalid, validation.want) {
			t.Fatalf("%s invalid = %v, want %v", validation.name, validation.invalid, validation.want)
		}
		if validation.valid != nil {
			t.Fatalf("%s valid = %v", validation.name, validation.valid)
		}
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
