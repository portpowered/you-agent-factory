package root

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/wire"
)

func TestExecuteWithDependencies_SelectsStartupModesAndSidecars(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantMode     Mode
		wantSidecars SidecarPolicy
	}{
		{
			name:     "default invocation",
			args:     []string{"you"},
			wantMode: ModeDefaultRun,
			wantSidecars: SidecarPolicy{
				API: true, Dashboard: true, WorkerScheduler: true, Watchers: true,
			},
		},
		{
			name:     "explicit local batch",
			args:     []string{"you", "run", "--dir", "factory", "--no-record", "--quiet"},
			wantMode: ModeLocalRun,
			wantSidecars: SidecarPolicy{
				API: true, WorkerScheduler: true,
			},
		},
		{
			name:     "api service",
			args:     []string{"you", "run", "--dir", "factory", "--continuously", "--no-record"},
			wantMode: ModeAPIService,
			wantSidecars: SidecarPolicy{
				API: true, Dashboard: true, WorkerScheduler: true, Watchers: true,
			},
		},
		{
			name:         "mcp serve",
			args:         []string{"you", "mcp", "serve", "--fixture-catalog", "unused.json"},
			wantMode:     ModeMCPServe,
			wantSidecars: SidecarPolicy{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := &recordingGraphBuilder{graph: &ApplicationGraph{}}
			initializer := &recordingInitializer{}
			err := ExecuteWithDependencies(Input{
				Args: test.args, Env: homeEnvironment(t.TempDir()), Context: context.Background(),
			}, Dependencies{GraphBuilder: builder, Initializer: initializer})
			if err != nil {
				t.Fatalf("ExecuteWithDependencies() error = %v", err)
			}
			if builder.calls != 1 {
				t.Fatalf("graph build calls = %d, want 1", builder.calls)
			}
			if initializer.calls != 1 {
				t.Fatalf("initializer calls = %d, want 1", initializer.calls)
			}
			if builder.request.Policy.Mode != test.wantMode || initializer.input.Graph.Policy.Mode != test.wantMode {
				t.Fatalf("selected modes = builder %q, initializer %q; want %q", builder.request.Policy.Mode, initializer.input.Graph.Policy.Mode, test.wantMode)
			}
			if builder.request.Policy.Sidecars != test.wantSidecars || initializer.input.Graph.Policy.Sidecars != test.wantSidecars {
				t.Fatalf("selected sidecars = builder %+v, initializer %+v; want %+v", builder.request.Policy.Sidecars, initializer.input.Graph.Policy.Sidecars, test.wantSidecars)
			}
			if initializer.input.Graph != builder.graph {
				t.Fatal("initializer did not receive the graph returned by construction")
			}
		})
	}
}

func TestExecuteWithDependencies_ClientAndHelpCommandsAvoidStartup(t *testing.T) {
	tests := [][]string{
		{"you", "--help"},
		{"you", "docs", "agents"},
		{"you", "init", "--help"},
		{"you", "work", "list", "--help"},
	}
	for _, args := range tests {
		builder := &recordingGraphBuilder{}
		initializer := &recordingInitializer{}
		var stdout bytes.Buffer
		err := ExecuteWithDependencies(Input{
			Args: args, Env: homeEnvironment(t.TempDir()), Stdout: &stdout, Context: context.Background(),
		}, Dependencies{GraphBuilder: builder, Initializer: initializer})
		if err != nil {
			t.Fatalf("ExecuteWithDependencies(%v) error = %v", args, err)
		}
		if builder.calls != 0 || initializer.calls != 0 {
			t.Fatalf("ExecuteWithDependencies(%v) startup calls = build %d, initialize %d; want zero", args, builder.calls, initializer.calls)
		}
	}
}

func TestExecuteWithDependencies_ConstructionFailureShortCircuitsStartup(t *testing.T) {
	buildErr := errors.New("worker provider unavailable")
	builder := &recordingGraphBuilder{err: buildErr}
	initializer := &recordingInitializer{}

	err := ExecuteWithDependencies(Input{
		Args: []string{"you", "run", "--no-record", "--quiet"},
		Env:  homeEnvironment(t.TempDir()), Context: context.Background(),
	}, Dependencies{GraphBuilder: builder, Initializer: initializer})
	if !errors.Is(err, buildErr) {
		t.Fatalf("ExecuteWithDependencies() error = %v, want wrapped construction error", err)
	}
	if initializer.calls != 0 {
		t.Fatalf("initializer calls = %d, want 0 after construction failure", initializer.calls)
	}
}

func TestExecuteStartup_ProductionInvocationConstructionFailurePreventsInitializer(t *testing.T) {
	text := "Plan the sprint"
	runConfig := runcli.RunConfig{
		Dir:                      t.TempDir(),
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		DisableDefaultRecording:  true,
	}
	initializer := &recordingInitializer{}
	dependencies := dependenciesFromWireCore(wire.InjectWireCore(), Dependencies{Initializer: initializer})

	err := executeStartup(context.Background(), startupcli.Request{
		Kind: startupcli.KindRun, Run: startupcli.RunIntent{WorkerSidecarsEnabled: true}, RunConfig: &runConfig,
	}, dependencies)
	if err == nil {
		t.Fatal("executeStartup() error = nil, want invocation bootstrap construction failure")
	}
	if !strings.Contains(err.Error(), "construct factory invocation bootstrap") {
		t.Fatalf("executeStartup() error = %v, want actionable invocation construction context", err)
	}
	if initializer.calls != 0 {
		t.Fatalf("initializer calls = %d, want 0 after invocation construction failure", initializer.calls)
	}
}

func TestExecuteWithDependencies_PreservesInitializerFailureAndContext(t *testing.T) {
	startupErr := errors.New("api listener stopped")
	graph := &ApplicationGraph{}
	builder := &recordingGraphBuilder{graph: graph}
	initializer := &recordingInitializer{err: startupErr}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ExecuteWithDependencies(Input{
		Args: []string{"you", "mcp", "serve"}, Env: homeEnvironment(t.TempDir()), Context: ctx,
	}, Dependencies{GraphBuilder: builder, Initializer: initializer})
	if !errors.Is(err, startupErr) {
		t.Fatalf("ExecuteWithDependencies() error = %v, want initializer error", err)
	}
	if initializer.ctx != ctx {
		t.Fatal("initializer did not receive the supplied parent context")
	}
	if initializer.ctx.Err() != context.Canceled {
		t.Fatalf("initializer context error = %v, want canceled", initializer.ctx.Err())
	}
}

type recordingGraphBuilder struct {
	calls   int
	request GraphRequest
	graph   *ApplicationGraph
	err     error
}

func (builder *recordingGraphBuilder) Build(_ context.Context, request GraphRequest) (*ApplicationGraph, error) {
	builder.calls++
	builder.request = request
	if builder.graph != nil {
		builder.graph.Policy = request.Policy
	}
	return builder.graph, builder.err
}

type recordingInitializer struct {
	calls int
	ctx   context.Context
	input Initialization
	err   error
}

func (initializer *recordingInitializer) Run(ctx context.Context, input Initialization) error {
	initializer.calls++
	initializer.ctx = ctx
	initializer.input = input
	return initializer.err
}
