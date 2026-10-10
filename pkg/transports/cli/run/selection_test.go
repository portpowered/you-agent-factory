package run

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
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

func TestPrepareCanonicalSessionIDPreservesExplicitSessionUUID(t *testing.T) {
	t.Parallel()
	const id = "00000000-0000-4000-8000-000000000001"
	cfg, err := prepareCanonicalSessionIDForRun(RunConfig{FactorySessionID: id, RecordPath: "source.jsonl", CanonicalSessionIDGenerator: func() string {
		t.Fatal("explicit UUID must not allocate a second recording identity")
		return ""
	}})
	if err != nil || cfg.CanonicalSessionID != id {
		t.Fatalf("recording identity = %q err=%v", cfg.CanonicalSessionID, err)
	}
}

func TestMapCurrentFactoryFailurePreservesSessionIdentityDiagnostic(t *testing.T) {
	t.Parallel()
	err := ValidateRunSessionIdentity(RunConfig{FactorySessionID: "validation-factory"})
	if got := MapCurrentFactoryFailure(err); got != err {
		t.Fatalf("mapped identity diagnostic = %v, want original %v", got, err)
	}
	if got := MapInvocationFailure(err); got != err {
		t.Fatalf("mapped invocation diagnostic = %v, want original %v", got, err)
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
	var scope factorysessions.DirectJavaScriptRunScope
	var scopeRegistered bool
	direct.onRun = func() { scope, scopeRegistered = owner.DirectJavaScript(direct.request.ScopeID) }
	factory, err := NewSelectionFactory(
		func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{},
		testResponsePresentation(),
		direct,
		nil, nil, nil, owner, nil,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	err = factory(RunConfig{
		FactoryConfigPath: "workflow.cjs", MockWorkersEnabled: true,
		JSONOutput: true, Output: output,
	}).Run(t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if direct.request.SourcePath != "workflow.cjs" || !direct.request.MockWorkersEnabled || !direct.request.JSONOutput {
		t.Fatalf("direct opening request = %#v", direct.request)
	}
	if !scopeRegistered || scope.Output != output {
		t.Fatalf("direct opening scope = %#v, want output writer", scope)
	}
	if _, ok := owner.DirectJavaScript(direct.request.ScopeID); ok {
		t.Fatal("direct JavaScript presentation scope remained after application run")
	}
}

func TestRunSelectionCarriesInvocationCancellationToDirectJavaScriptHost(t *testing.T) {
	want := &selectionCancellationStub{}
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{}, testResponsePresentation(), direct,
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	err = factory(RunConfig{
		FactoryConfigPath: "workflow.cjs", Port: 7437,
	}).Run(t.Context(), processcontract.RunIntent{
		APIEnabled: true, WorkerSidecarsEnabled: true, Cancellation: want,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if direct.request.Host == nil {
		t.Fatal("direct JavaScript host request = nil")
	}
	if direct.cancellation != want {
		t.Fatalf("direct JavaScript cancellation = %p, want %p", direct.cancellation, want)
	}
}

func TestRunSelectionExecutesLocalRunWithIntent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wantCancellation := &selectionCancellationStub{}
	var gotConfig RunConfig
	var gotCancellation initializer.InvocationCancellation
	var ran bool
	factory, err := NewSelectionFactory(
		func(_ context.Context, _ *factorysessions.SessionStartRequest, cancellation initializer.InvocationCancellation, _ factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			gotCancellation = cancellation
			return runFuncRunner(func(runCtx context.Context) error {
				if runCtx != ctx {
					t.Fatal("local runner received a different context")
				}
				ran = true
				return nil
			}), nil
		},
		testInvocationOperation{}, testResponsePresentation(),
		&selectionDirectJavaScriptStub{},
		nil, nil, func(cfg RunConfig, _ *workers.MockWorkersConfig) *factorysessions.SessionStartRequest {
			gotConfig = cfg
			return &factorysessions.SessionStartRequest{}
		}, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	if err := factory(ensureTestRecordingsCLI(RunConfig{Port: 7437, DisableDefaultRecording: true})).Run(ctx, processcontract.RunIntent{
		WorkerSidecarsEnabled: true, Cancellation: wantCancellation,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran || gotConfig.Port != 0 || gotCancellation != wantCancellation {
		t.Fatalf("local run = ran:%t port:%d cancellation:%p", ran, gotConfig.Port, gotConfig.Cancellation)
	}
}

func TestRunSelectionDirectJavaScriptClosesPresentationAfterRunFailure(t *testing.T) {
	want := errors.New("run failed")
	owner := newTestOpeningPresentationOwner()
	direct := &selectionDirectJavaScriptStub{supported: true, runErr: want}
	factory, err := NewSelectionFactory(
		func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{}, testResponsePresentation(), direct,
		nil, nil, nil, owner, nil,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	err = factory(RunConfig{FactoryConfigPath: "workflow.cjs"}).Run(
		t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true},
	)
	if !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
	if _, ok := owner.DirectJavaScript(direct.request.ScopeID); ok {
		t.Fatal("direct JavaScript presentation scope remained after run failure")
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

func TestRunSelectionDirectJavaScriptCleansPresentationOnRunFailure(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		runErr error
	}{
		{name: "validation failure", runErr: errors.New("direct run failed")},
		{name: "lifecycle failure", runErr: errors.New("application build failed")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner := newTestOpeningPresentationOwner()
			direct := &selectionDirectJavaScriptStub{supported: true, runErr: testCase.runErr}
			factory, err := NewSelectionFactory(
				func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
					return nil, nil
				},
				testInvocationOperation{}, testResponsePresentation(), direct,
				nil, nil, nil, owner, nil,
			)
			if err != nil {
				t.Fatalf("NewSelectionFactory: %v", err)
			}
			selection := factory(RunConfig{FactoryConfigPath: "workflow.cjs", Output: &bytes.Buffer{}})
			err = selection.Run(t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true})
			if err == nil {
				t.Fatal("direct selection Run error = nil")
			}
			if _, ok := owner.DirectJavaScript(direct.request.ScopeID); ok {
				t.Fatal("direct JavaScript presentation scope remained after failed run")
			}
		})
	}
}

func TestRunSelectionSupportsDirectJavaScriptWithoutPresentationOwner(t *testing.T) {
	direct := &selectionDirectJavaScriptStub{supported: true}
	factory, err := NewSelectionFactory(
		func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return nil, nil
		},
		testInvocationOperation{}, testResponsePresentation(), direct,
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewSelectionFactory: %v", err)
	}
	err = factory(RunConfig{FactoryConfigPath: "workflow.cjs"}).Run(
		t.Context(), processcontract.RunIntent{WorkerSidecarsEnabled: true},
	)
	if err != nil {
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
	runErr       error
	onRun        func()
}

type selectionCancellationStub struct{}

func (*selectionCancellationStub) Cancel() {}

func (s *selectionDirectJavaScriptStub) Supports(string) bool { return s.supported }

func (s *selectionDirectJavaScriptStub) Run(
	_ context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
	cancellation initializer.InvocationCancellation,
	discloseStartup func(),
) error {
	s.request = request
	s.cancellation = cancellation
	if s.onRun != nil {
		s.onRun()
	}
	if s.runErr != nil {
		return s.runErr
	}
	if discloseStartup != nil {
		discloseStartup()
	}
	return nil
}

func TestRunSessionIdentity(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"validation-factory", "../escape"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			err := ValidateRunSessionIdentity(RunConfig{FactorySessionID: id})
			var invalid *clidiag.LocalFailure
			if !errors.As(err, &invalid) || invalid.Code != "BAD_REQUEST" || invalid.Message != "--session must be "+factorysessions.SessionIdentityForm {
				t.Fatalf("identity error = %v", err)
			}
		})
	}
	for _, id := range []string{"", "  ", "~default", "12345678-1234-1234-1234-1234567890ab"} {
		if err := ValidateRunSessionIdentity(RunConfig{FactorySessionID: id}); err != nil {
			t.Fatal(err)
		}
	}
}
