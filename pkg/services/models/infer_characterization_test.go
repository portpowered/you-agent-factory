package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// inferPeerService is a fake peer implementer of Models root Service that
// exercises plain infer/local-invocation contracts using only root-package types.
type inferPeerService struct {
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
