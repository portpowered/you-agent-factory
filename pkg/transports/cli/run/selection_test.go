package run

import (
	"bytes"
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

func TestRunSelectionOwnsDirectJavaScriptTransportChoice(t *testing.T) {
	output := &bytes.Buffer{}
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, RunConfig, RuntimeRunnerBuilder, factorysessions.InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
			t.Fatal("regular run opener called for direct JavaScript")
			return nil, nil
		},
		func(context.Context, factorysessions.ApplicationOpeningRequest, *zap.Logger, factoryvisualization.Sink) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{},
		testResponsePresentation(),
		direct,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	application, err := factory(RunConfig{
		FactoryConfigPath: "workflow.cjs", MockWorkersEnabled: true,
		JSONOutput: true, Output: output,
	}).Open(t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := application.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if direct.request.SourcePath != "workflow.cjs" || !direct.request.MockWorkersEnabled ||
		!direct.request.JSONOutput || direct.request.Output != output {
		t.Fatalf("direct request = %#v", direct.request)
	}
}

type selectionDirectJavaScriptStub struct {
	supported bool
	request   factorysessions.DirectJavaScriptRunRequest
}

func (s *selectionDirectJavaScriptStub) Supports(string) bool { return s.supported }

func (s *selectionDirectJavaScriptStub) Run(_ context.Context, request factorysessions.DirectJavaScriptRunRequest) error {
	s.request = request
	return nil
}
