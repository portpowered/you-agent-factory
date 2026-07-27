package run

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

func TestSplitFlagTerminatorPreservesCanonicalRunTokenization(t *testing.T) {
	args := []string{"--named", "alpha", "input", "--", "--named", "positional"}
	flagArgs, positional, terminated := SplitFlagTerminator(args)

	if !terminated ||
		!reflect.DeepEqual(flagArgs, []string{"--named", "alpha", "input"}) ||
		!reflect.DeepEqual(positional, []string{"--named", "positional"}) {
		t.Fatalf(
			"SplitFlagTerminator() = (%#v, %#v, %t)",
			flagArgs,
			positional,
			terminated,
		)
	}
}

func TestRunSelectionOwnsDirectJavaScriptTransportChoice(t *testing.T) {
	output := &bytes.Buffer{}
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, RunConfig, RuntimeRunnerBuilder, InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
			t.Fatal("regular run opener called for direct JavaScript")
			return nil, nil
		},
		func(context.Context, factorysessions.ApplicationOpeningRequest, *zap.Logger, factoryvisualization.Sink) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{},
		testResponsePresentation(),
		direct,
		func(ctx context.Context, open initializer.ApplicationOpeningOperation) (initializer.LocalRuntimeRunner, error) {
			opened, err := open(ctx)
			if err != nil {
				return nil, err
			}
			return runtimeapplication.NewManagedRunner(opened.Plan, opened.Diagnostics)
		},
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

func TestApplyRunIntentDisablesUnrequestedServerWithoutSuppressingTerminalPresentation(t *testing.T) {
	cfg, err := applyRunIntent(
		RunConfig{Port: 7437},
		processcontract.RunIntent{WorkerSidecarsEnabled: true},
	)
	if err != nil {
		t.Fatalf("applyRunIntent: %v", err)
	}
	if cfg.Port != 0 {
		t.Fatalf("port = %d, want server disabled", cfg.Port)
	}
	if cfg.SuppressDashboardRendering {
		t.Fatal("server-disabled run suppressed terminal presentation")
	}
}

type selectionDirectJavaScriptStub struct {
	supported bool
	request   factorysessions.DirectJavaScriptRunRequest
}

func (s *selectionDirectJavaScriptStub) Supports(string) bool { return s.supported }

func (s *selectionDirectJavaScriptStub) Open(
	_ context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
) (factorysessions.DirectJavaScriptApplication, error) {
	s.request = request
	return factorysessions.DirectJavaScriptApplication{
		Plan: lifecycle.Plan{Components: []lifecycle.NamedComponent{{
			Name: "direct JavaScript",
			Component: lifecycle.NewRunner(func(context.Context) error {
				return nil
			}),
			Primary: true,
		}}},
	}, nil
}
