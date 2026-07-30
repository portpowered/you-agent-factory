package factoryvisualization_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestBind_FakeRootInvokedThroughActivateTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{
		activate: func(_ context.Context, request factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error) {
			invoked = true
			if request.Mode != factoryvisualization.ActivateModeRetainedThenLive {
				t.Fatalf("mode = %q, want %q", request.Mode, factoryvisualization.ActivateModeRetainedThenLive)
			}
			return factoryvisualization.ActivateResult{State: factoryvisualization.LifecycleStateStarted}, nil
		},
	}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	raw, err := operation(
		context.Background(),
		mcpfactoryvisualization.ToolActivate,
		json.RawMessage(`{"mode":"RETAINED_THEN_LIVE"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(activate) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake visualization root was not invoked")
	}
	if !strings.Contains(string(raw), `"State":"STARTED"`) {
		t.Fatalf("CallTool(activate) = %s, want started lifecycle state", raw)
	}
}

func TestBind_UnsupportedToolReturnsStableErrorWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{
		Root: fakeVisualizationRoot{invoked: &invoked},
	})
	_, err := operation(context.Background(), "you.factory_visualization.unknown", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("CallTool(unknown) error = nil, want unsupported tool error")
	}
	if !strings.Contains(err.Error(), "unsupported tool") {
		t.Fatalf("CallTool(unknown) error = %v, want unsupported tool error", err)
	}
	if invoked {
		t.Fatal("fake visualization root was invoked for unknown tool")
	}
}

func TestToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{
		Root: fakeVisualizationRoot{},
	})
	_, err := operation(nil, mcpfactoryvisualization.ToolActivate, json.RawMessage(`{"mode":"RETAINED_THEN_LIVE"}`))
	if err == nil {
		t.Fatal("ToolOperation(nil context) error = nil, want required-context error")
	}
	if !strings.Contains(err.Error(), "MCP request context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
}

type fakeVisualizationRoot struct {
	invoked              *bool
	activate             func(context.Context, factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error)
	join                 func(context.Context, factoryvisualization.JoinRequest) (factoryvisualization.JoinResult, error)
	stopDrain            func(context.Context, factoryvisualization.StopDrainRequest) (factoryvisualization.StopDrainResult, error)
	observe              func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error)
	openPresentation     func(context.Context, factoryvisualization.OpenPresentationRequest) (factoryvisualization.OpenPresentationResult, error)
	presentProgress      func(context.Context, factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error)
	finalizePresentation func(context.Context, factoryvisualization.FinalizePresentationRequest) (factoryvisualization.FinalizePresentationResult, error)
	closePresentation    func(context.Context, factoryvisualization.ClosePresentationRequest) (factoryvisualization.ClosePresentationResult, error)
}

func (f fakeVisualizationRoot) markInvoked() {
	if f.invoked != nil {
		*f.invoked = true
	}
}

func (f fakeVisualizationRoot) Activate(ctx context.Context, req factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error) {
	if f.activate == nil {
		panic("unexpected Activate on fake visualization root")
	}
	f.markInvoked()
	return f.activate(ctx, req)
}

func (f fakeVisualizationRoot) Join(ctx context.Context, req factoryvisualization.JoinRequest) (factoryvisualization.JoinResult, error) {
	if f.join == nil {
		panic("unexpected Join on fake visualization root")
	}
	f.markInvoked()
	return f.join(ctx, req)
}

func (f fakeVisualizationRoot) StopDrain(ctx context.Context, req factoryvisualization.StopDrainRequest) (factoryvisualization.StopDrainResult, error) {
	if f.stopDrain == nil {
		panic("unexpected StopDrain on fake visualization root")
	}
	f.markInvoked()
	return f.stopDrain(ctx, req)
}

func (f fakeVisualizationRoot) Observe(ctx context.Context, req factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
	if f.observe == nil {
		panic("unexpected Observe on fake visualization root")
	}
	f.markInvoked()
	return f.observe(ctx, req)
}

func (f fakeVisualizationRoot) OpenPresentation(ctx context.Context, req factoryvisualization.OpenPresentationRequest) (factoryvisualization.OpenPresentationResult, error) {
	if f.openPresentation == nil {
		panic("unexpected OpenPresentation on fake visualization root")
	}
	f.markInvoked()
	return f.openPresentation(ctx, req)
}

func (f fakeVisualizationRoot) PresentProgress(ctx context.Context, req factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
	if f.presentProgress == nil {
		panic("unexpected PresentProgress on fake visualization root")
	}
	f.markInvoked()
	return f.presentProgress(ctx, req)
}

func (f fakeVisualizationRoot) FinalizePresentation(ctx context.Context, req factoryvisualization.FinalizePresentationRequest) (factoryvisualization.FinalizePresentationResult, error) {
	if f.finalizePresentation == nil {
		panic("unexpected FinalizePresentation on fake visualization root")
	}
	f.markInvoked()
	return f.finalizePresentation(ctx, req)
}

func (f fakeVisualizationRoot) ClosePresentation(ctx context.Context, req factoryvisualization.ClosePresentationRequest) (factoryvisualization.ClosePresentationResult, error) {
	if f.closePresentation == nil {
		panic("unexpected ClosePresentation on fake visualization root")
	}
	f.markInvoked()
	return f.closePresentation(ctx, req)
}
