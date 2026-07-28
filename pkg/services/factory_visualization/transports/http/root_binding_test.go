package http_test

import (
	"context"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationhttp "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_ActivateInvokesVisualizationRoot(t *testing.T) {
	t.Parallel()

	root := &httpVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	result, err := handler.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !root.activateInvoked {
		t.Fatal("Activate was not invoked through the injected Visualization root")
	}
	if result.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("result.State = %q, want STARTED", result.State)
	}
}

func TestHandlerFromRoot_ActivateRequiresInjectedRoot(t *testing.T) {
	t.Parallel()

	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{},
		zap.NewNop(),
	)

	_, err := handler.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err == nil {
		t.Fatal("Activate without injected root = nil, want error")
	}
}

type httpVisualizationRootFake struct {
	activateInvoked bool
}

var _ factoryvisualization.Root = (*httpVisualizationRootFake)(nil)

func (fake *httpVisualizationRootFake) Activate(
	_ context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	fake.activateInvoked = true
	if req.Mode == "" {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	return factoryvisualization.ActivateResult{
		State: factoryvisualization.LifecycleStateStarted,
	}, nil
}

func (fake *httpVisualizationRootFake) Join(
	context.Context,
	factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	panic("unexpected Join call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	panic("unexpected StopDrain call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	panic("unexpected Observe call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	panic("unexpected OpenPresentation call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	panic("unexpected PresentProgress call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	panic("unexpected FinalizePresentation call in HTTP adapter root seam test")
}

func (fake *httpVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	panic("unexpected ClosePresentation call in HTTP adapter root seam test")
}
