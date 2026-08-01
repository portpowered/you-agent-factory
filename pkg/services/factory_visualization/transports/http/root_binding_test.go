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

func TestHandlerFromRoot_ForwardsAllVisualizationOperations(t *testing.T) {
	t.Parallel()

	root := &httpVisualizationRootFake{}
	handler := factoryvisualizationhttp.NewHandlerFromRoot(
		factoryvisualizationhttp.RootBinding{Visualization: root},
		zap.NewNop(),
	)

	if _, err := handler.Join(context.Background(), factoryvisualization.JoinRequest{}); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if _, err := handler.StopDrain(context.Background(), factoryvisualization.StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: %v", err)
	}
	if _, err := handler.Observe(context.Background(), factoryvisualization.ObserveRequest{}); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if _, err := handler.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{}); err != nil {
		t.Fatalf("OpenPresentation: %v", err)
	}
	if _, err := handler.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{}); err != nil {
		t.Fatalf("PresentProgress: %v", err)
	}
	if _, err := handler.FinalizePresentation(context.Background(), factoryvisualization.FinalizePresentationRequest{}); err != nil {
		t.Fatalf("FinalizePresentation: %v", err)
	}
	if _, err := handler.ClosePresentation(context.Background(), factoryvisualization.ClosePresentationRequest{}); err != nil {
		t.Fatalf("ClosePresentation: %v", err)
	}

	if !root.joinInvoked || !root.stopDrainInvoked || !root.observeInvoked ||
		!root.openPresentationInvoked || !root.presentProgressInvoked ||
		!root.finalizePresentationInvoked || !root.closePresentationInvoked {
		t.Fatalf("root invocation flags = %#v, want every operation invoked", root)
	}
}

type httpVisualizationRootFake struct {
	activateInvoked             bool
	joinInvoked                 bool
	stopDrainInvoked            bool
	observeInvoked              bool
	openPresentationInvoked     bool
	presentProgressInvoked      bool
	finalizePresentationInvoked bool
	closePresentationInvoked    bool
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
	fake.joinInvoked = true
	return factoryvisualization.JoinResult{}, nil
}

func (fake *httpVisualizationRootFake) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	fake.stopDrainInvoked = true
	return factoryvisualization.StopDrainResult{}, nil
}

func (fake *httpVisualizationRootFake) Observe(
	context.Context,
	factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	fake.observeInvoked = true
	return factoryvisualization.ObserveResult{}, nil
}

func (fake *httpVisualizationRootFake) OpenPresentation(
	context.Context,
	factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	fake.openPresentationInvoked = true
	return factoryvisualization.OpenPresentationResult{}, nil
}

func (fake *httpVisualizationRootFake) PresentProgress(
	context.Context,
	factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	fake.presentProgressInvoked = true
	return factoryvisualization.PresentProgressResult{}, nil
}

func (fake *httpVisualizationRootFake) FinalizePresentation(
	context.Context,
	factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	fake.finalizePresentationInvoked = true
	return factoryvisualization.FinalizePresentationResult{}, nil
}

func (fake *httpVisualizationRootFake) ClosePresentation(
	context.Context,
	factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	fake.closePresentationInvoked = true
	return factoryvisualization.ClosePresentationResult{}, nil
}
