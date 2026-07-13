package root

import (
	"bytes"
	"context"
	"errors"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
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
			builder := &recordingGraphBuilder{graph: &recordingGraph{lifecycle: startupcli.LifecycleFunc(func(context.Context) error { return nil })}}
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
			if builder.request.Mode != test.wantMode || initializer.input.Mode != test.wantMode {
				t.Fatalf("selected modes = builder %q, initializer %q; want %q", builder.request.Mode, initializer.input.Mode, test.wantMode)
			}
			if builder.request.Sidecars != test.wantSidecars || initializer.input.Sidecars != test.wantSidecars {
				t.Fatalf("selected sidecars = builder %+v, initializer %+v; want %+v", builder.request.Sidecars, initializer.input.Sidecars, test.wantSidecars)
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

func TestExecuteWithDependencies_PreservesInitializerFailureAndContext(t *testing.T) {
	startupErr := errors.New("api listener stopped")
	graph := &recordingGraph{lifecycle: startupcli.LifecycleFunc(func(context.Context) error { return nil })}
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

type recordingGraph struct{ lifecycle startupcli.Lifecycle }

func (graph recordingGraph) Lifecycle() startupcli.Lifecycle { return graph.lifecycle }

type recordingGraphBuilder struct {
	calls   int
	request GraphRequest
	graph   ApplicationGraph
	err     error
}

func (builder *recordingGraphBuilder) Build(_ context.Context, request GraphRequest) (ApplicationGraph, error) {
	builder.calls++
	builder.request = request
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
