package run

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func TestPrepareCanonicalSessionIDForRunRequiresInjectedGenerator(t *testing.T) {
	t.Parallel()

	_, err := prepareCanonicalSessionIDForRun(RunConfig{})
	if err == nil {
		t.Fatal("prepareCanonicalSessionIDForRun() error = nil, want missing-generator diagnostic")
	}
	const want = "canonical Factory Session ID generator is required"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

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
	owner := newTestOpeningPresentationOwner()
	factory, err := NewSelectionFactory(
		func(context.Context, RunConfig, RuntimeRunnerBuilder, InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
			t.Fatal("regular run opener called for direct JavaScript")
			return nil, nil
		},
		func(context.Context, *factorysessions.RuntimeOpeningRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
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
		owner,
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
	scope, ok := owner.DirectJavaScript(direct.request.ScopeID)
	if direct.request.SourcePath != "workflow.cjs" || !direct.request.MockWorkersEnabled || !direct.request.JSONOutput {
		t.Fatalf("direct opening request = %#v", direct.request)
	}
	if !ok || scope.Output != output {
		t.Fatalf("direct opening scope = %#v, want output writer", scope)
	}
	if err := application.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := owner.DirectJavaScript(direct.request.ScopeID); ok {
		t.Fatal("direct JavaScript presentation scope remained after application run")
	}
}

func TestRunSelectionCarriesInvocationCancellationToDirectJavaScriptHost(t *testing.T) {
	want := &selectionCancellationStub{}
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, RunConfig, RuntimeRunnerBuilder, InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
			t.Fatal("regular run opener called for direct JavaScript")
			return nil, nil
		},
		func(context.Context, *factorysessions.RuntimeOpeningRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{}, testResponsePresentation(), direct,
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
		FactoryConfigPath: "workflow.cjs", Port: 7437,
	}).Open(t.Context(), processcontract.RunIntent{
		APIEnabled: true, WorkerSidecarsEnabled: true, Cancellation: want,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if direct.request.Host == nil {
		t.Fatal("direct JavaScript host request = nil")
	}
	if direct.cancellation != want {
		t.Fatalf("direct JavaScript cancellation = %p, want %p", direct.cancellation, want)
	}
	if err := application.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
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

func TestApplyRunIntentCarriesInvocationCancellation(t *testing.T) {
	want := &selectionCancellationStub{}
	cfg, err := applyRunIntent(
		RunConfig{Port: 7437},
		processcontract.RunIntent{WorkerSidecarsEnabled: true, Cancellation: want},
	)
	if err != nil {
		t.Fatalf("applyRunIntent: %v", err)
	}
	if cfg.Cancellation != want {
		t.Fatalf("run config cancellation = %p, want %p", cfg.Cancellation, want)
	}
}

func TestRunSelectionDirectJavaScriptCleansPresentationOnOpenFailures(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		openErr    error
		builderErr error
		nilRunner  bool
	}{
		{name: "direct opener", openErr: errors.New("direct open failed")},
		{name: "application builder", builderErr: errors.New("application build failed")},
		{name: "nil application runner", nilRunner: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner := newTestOpeningPresentationOwner()
			direct := &selectionDirectJavaScriptStub{supported: true, openErr: testCase.openErr}
			factory, err := NewSelectionFactory(
				func(context.Context, RunConfig, RuntimeRunnerBuilder, InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
					t.Fatal("regular run opener called for direct JavaScript")
					return nil, nil
				},
				func(context.Context, *factorysessions.RuntimeOpeningRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
					return nil, nil
				},
				testInvocationOperation{}, testResponsePresentation(), direct,
				func(ctx context.Context, open initializer.ApplicationOpeningOperation) (initializer.LocalRuntimeRunner, error) {
					_, err := open(ctx)
					if err != nil {
						return nil, err
					}
					if testCase.builderErr != nil {
						return nil, testCase.builderErr
					}
					if testCase.nilRunner {
						return nil, nil
					}
					return runFuncRunner(func(context.Context) error { return nil }), nil
				}, owner,
			)
			if err != nil {
				t.Fatalf("NewSelectionFactory: %v", err)
			}
			selection := factory(RunConfig{FactoryConfigPath: "workflow.cjs", Output: &bytes.Buffer{}})
			_, err = selection.Open(t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true})
			if err == nil {
				t.Fatal("direct selection Open error = nil")
			}
			if _, ok := owner.DirectJavaScript(direct.request.ScopeID); ok {
				t.Fatal("direct JavaScript presentation scope remained after failed open")
			}
		})
	}
}

func TestRunSelectionSupportsDirectJavaScriptWithoutPresentationOwner(t *testing.T) {
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, RunConfig, RuntimeRunnerBuilder, InvocationOperation, factoryvisualization.ResponsePresentation) (*Operation, error) {
			t.Fatal("regular run opener called for direct JavaScript")
			return nil, nil
		},
		func(context.Context, *factorysessions.RuntimeOpeningRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{}, testResponsePresentation(), direct,
		func(ctx context.Context, open initializer.ApplicationOpeningOperation) (initializer.LocalRuntimeRunner, error) {
			if _, err := open(ctx); err != nil {
				return nil, err
			}
			return runFuncRunner(func(context.Context) error { return nil }), nil
		},
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	application, err := factory(RunConfig{FactoryConfigPath: "workflow.cjs"}).Open(
		t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := application.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestApplyRunIntentRejectsConflictingPolicies(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		intent processcontract.RunIntent
	}{
		{name: "dashboard without API", intent: processcontract.RunIntent{DashboardEnabled: true, WorkerSidecarsEnabled: true}},
		{name: "default invocation without continuous", intent: processcontract.RunIntent{DefaultInvocation: true, WorkerSidecarsEnabled: true}},
		{name: "local run without worker sidecars", intent: processcontract.RunIntent{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := applyRunIntent(RunConfig{}, testCase.intent); err == nil {
				t.Fatal("applyRunIntent error = nil")
			}
		})
	}
}

type selectionDirectJavaScriptStub struct {
	supported    bool
	request      factorysessions.DirectJavaScriptRunRequest
	cancellation initializer.InvocationCancellation
	openErr      error
}

type selectionCancellationStub struct{}

func (*selectionCancellationStub) Cancel() {}

func (s *selectionDirectJavaScriptStub) Supports(string) bool { return s.supported }

func (s *selectionDirectJavaScriptStub) Open(
	_ context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
	cancellation initializer.InvocationCancellation,
) (factorysessions.DirectJavaScriptApplication, error) {
	s.request = request
	s.cancellation = cancellation
	if s.openErr != nil {
		return factorysessions.DirectJavaScriptApplication{}, s.openErr
	}
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
