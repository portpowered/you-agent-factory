package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// assetsPeerService is a fake peer implementer of Models root Service that
// exercises plain asset-pull contracts using only root-package types.
type assetsPeerService struct {
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
