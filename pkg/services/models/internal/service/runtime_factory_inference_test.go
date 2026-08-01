package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestRootDelegatesInferenceThroughInjectedOwner(t *testing.T) {
	t.Parallel()

	privateInference := &delegatingInferenceService{}
	root := &Root{inference: privateInference}
	request := models.InvokeModelRequest{ModelName: "scoped-model", Operation: "generate"}

	_, err := root.InvokeModelWithLease(context.Background(), request)
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("InvokeModelWithLease error = %v, want ErrUnsupportedOperation", err)
	}
	if privateInference.invokeRequest != request {
		t.Fatalf("InvokeModelWithLease request = %#v, want %#v", privateInference.invokeRequest, request)
	}

	cancelRequest := models.CancelInvocationRequest{Invocation: mustInferenceInvocationRef(t, "inv-1")}
	_, err = root.CancelInvocation(context.Background(), cancelRequest)
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("CancelInvocation error = %v, want ErrUnsupportedOperation", err)
	}
	if privateInference.cancelRequest != cancelRequest {
		t.Fatalf("CancelInvocation request = %#v, want %#v", privateInference.cancelRequest, cancelRequest)
	}
}

func TestScopedRuntimeResolutionDoesNotReplaceInjectedInferenceOwner(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "inference-root-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{}
		},
	})
	if err != nil {
		t.Fatalf("open runtime scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateRef))
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}

	privateInference := &delegatingInferenceService{}
	root := &Root{
		runtimeScopes:  scopes,
		assets:         inferenceRecordingAssetsService{},
		inference:      privateInference,
		runtimeByScope: make(map[models.RuntimeScopeRef]models.Service),
	}

	_, err = root.PullModelForScope(context.Background(), models.PullModelRequest{
		Scope: scope,
		Name:  "voice",
	})
	if err == nil {
		t.Fatal("PullModelForScope error = nil, want scoped runtime construction failure")
	}
	if root.inference != privateInference {
		t.Fatal("scoped runtime resolution replaced process-scoped inference owner")
	}
}

func TestInferenceWireConstructionIsInert(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "inference-inert-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	assets := inferenceRecordingAssetsService{}
	launcher := &inferenceRecordingProcessLauncher{}
	clock := &inferenceTestClock{}
	runtimeHost, err := runtimehostwire.NewService(
		scopes,
		assets,
		launcher,
		http.DefaultClient,
		clock,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	inference, err := inferencewire.NewService(
		scopes,
		assets,
		catalog,
		runtimeHost,
		inference.InputEchoInvocationRuntime{},
		inference.InertArtifactFileSystem{},
		clock.Now,
	)
	if err != nil {
		t.Fatalf("construct Inference: %v", err)
	}
	if inference == nil {
		t.Fatal("construct Inference returned nil service")
	}
	if launcher.starts != 0 || clock.timerCalls != 0 {
		t.Fatalf(
			"inert construction side effects = (starts %d, timers %d), want 0/0",
			launcher.starts,
			clock.timerCalls,
		)
	}
}

type delegatingInferenceService struct {
	invokeCalls   int
	invokeRequest models.InvokeModelRequest
	cancelRequest models.CancelInvocationRequest
}

func (service *delegatingInferenceService) InvokeModelWithLease(
	_ context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	service.invokeCalls++
	service.invokeRequest = request
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (service *delegatingInferenceService) CancelInvocation(
	_ context.Context,
	request models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	service.cancelRequest = request
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

type inferenceRecordingProcessLauncher struct {
	starts int
}

func (launcher *inferenceRecordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher called during inert inference composition")
}

type inferenceTestClock struct {
	timerCalls int
}

func (clock *inferenceTestClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (clock *inferenceTestClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert inference composition")
}

type inferenceRecordingAssetsService struct{}

var _ scopedassets.Service = inferenceRecordingAssetsService{}

func (inferenceRecordingAssetsService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{}, models.ErrUnsupportedOperation
}

func mustInferenceInvocationRef(t *testing.T, value string) models.ModelInvocationRef {
	t.Helper()
	ref, err := (models.ModelInvocationRef{}).Parse(value)
	if err != nil {
		t.Fatalf("parse invocation ref: %v", err)
	}
	return ref
}
