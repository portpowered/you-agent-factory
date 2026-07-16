package initializer_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

func TestRunProcessStartsExactlyOneConstructedMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		graph func(*recordingProcessApplication) *initializer.ProcessGraph
	}{
		{name: "run", graph: func(application *recordingProcessApplication) *initializer.ProcessGraph {
			return &initializer.ProcessGraph{Policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}}, Run: application}
		}},
		{name: "MCP", graph: func(application *recordingProcessApplication) *initializer.ProcessGraph {
			return &initializer.ProcessGraph{Policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeMCPServe}, MCP: application}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), processContextKey{}, test.name)
			wantErr := errors.New("lifecycle stopped")
			application := &recordingProcessApplication{err: wantErr}
			err := initializer.RunProcess(ctx, test.graph(application))
			if !errors.Is(err, wantErr) {
				t.Fatalf("RunProcess() error = %v, want %v", err, wantErr)
			}
			if application.calls != 1 || application.ctx != ctx {
				t.Fatalf("application calls/context = %d/%v, want 1/supplied context", application.calls, application.ctx)
			}
		})
	}
}

func TestRunProcessRejectsMissingOrAmbiguousGraph(t *testing.T) {
	t.Parallel()
	application := &recordingProcessApplication{}
	for _, graph := range []*initializer.ProcessGraph{
		nil,
		{},
		{Policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}}, Run: application, MCP: application},
	} {
		if err := initializer.RunProcess(context.Background(), graph); err == nil {
			t.Fatalf("RunProcess(%+v) error = nil, want validation error", graph)
		}
	}
	if application.calls != 0 {
		t.Fatalf("ambiguous application calls = %d, want 0", application.calls)
	}
}

type processContextKey struct{}

type recordingProcessApplication struct {
	calls int
	ctx   context.Context
	err   error
}

func (application *recordingProcessApplication) Run(ctx context.Context) error {
	application.calls++
	application.ctx = ctx
	return application.err
}

func TestApplicationRunStartsWaitsAndShutsDownInReverse(t *testing.T) {
	t.Parallel()

	fixture := newApplicationFixture()
	application, err := initializer.NewApplication(initializer.ModeCLI, fixture.graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if len(fixture.starts) != 0 {
		t.Fatalf("construction starts = %v, want none", fixture.starts)
	}
	if application.Graph() != fixture.graph {
		t.Fatal("application did not retain the constructed graph identity")
	}
	runDone := make(chan error, 1)
	go func() { runDone <- application.Run(context.Background()) }()
	close(fixture.cli.done)
	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fixture.starts, []string{"runtime", "workers", "dashboard", "cli"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("start order = %v, want %v", got, want)
	}
	if got, want := fixture.stops, []string{"cli", "dashboard", "workers", "runtime"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stop order = %v, want %v", got, want)
	}
	if fixture.graph.closes != 1 {
		t.Fatalf("graph closes = %d, want one", fixture.graph.closes)
	}
	if err := application.Shutdown(context.Background()); err != nil || fixture.graph.closes != 1 {
		t.Fatalf("repeated Shutdown() = %v, closes %d", err, fixture.graph.closes)
	}
}

func TestNewApplicationSelectsOnlyTheModeLifecyclePlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mode      initializer.Mode
		complete  func(*applicationFixture)
		wantStart []string
		wantStop  []string
	}{
		{
			name: "API", mode: initializer.ModeAPI,
			complete:  func(fixture *applicationFixture) { close(fixture.api.done) },
			wantStart: []string{"runtime", "workers", "dashboard", "api"},
			wantStop:  []string{"api", "dashboard", "workers", "runtime"},
		},
		{
			name: "CLI", mode: initializer.ModeCLI,
			complete:  func(fixture *applicationFixture) { close(fixture.cli.done) },
			wantStart: []string{"runtime", "workers", "dashboard", "cli"},
			wantStop:  []string{"cli", "dashboard", "workers", "runtime"},
		},
		{
			name: "MCP", mode: initializer.ModeMCP,
			complete:  func(fixture *applicationFixture) { close(fixture.mcp.done) },
			wantStart: []string{"mcp"},
			wantStop:  []string{"mcp"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApplicationFixture()
			application, err := initializer.NewApplication(test.mode, fixture.graph)
			if err != nil {
				t.Fatalf("NewApplication() error = %v", err)
			}
			if application.Graph() != fixture.graph || len(fixture.starts) != 0 {
				t.Fatalf("selection changed graph identity or started lifecycle: graph %p starts %v", application.Graph(), fixture.starts)
			}

			done := make(chan error, 1)
			go func() { done <- application.Run(context.Background()) }()
			test.complete(fixture)
			if err := <-done; err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if !reflect.DeepEqual(fixture.starts, test.wantStart) {
				t.Fatalf("starts = %v, want %v", fixture.starts, test.wantStart)
			}
			if !reflect.DeepEqual(fixture.stops, test.wantStop) {
				t.Fatalf("stops = %v, want %v", fixture.stops, test.wantStop)
			}
		})
	}
}

func TestNewApplicationRejectsMissingRequiredLifecycleBeforeActivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    initializer.Mode
		missing string
		remove  func(*initializer.ApplicationLifecycles)
	}{
		{name: "API runtime", mode: initializer.ModeAPI, missing: "runtime sidecar", remove: func(edges *initializer.ApplicationLifecycles) { edges.Runtime = nil }},
		{name: "API workers", mode: initializer.ModeAPI, missing: "workers sidecar", remove: func(edges *initializer.ApplicationLifecycles) { edges.Workers = nil }},
		{name: "API transport", mode: initializer.ModeAPI, missing: "API transport", remove: func(edges *initializer.ApplicationLifecycles) { edges.API = nil }},
		{name: "CLI runtime", mode: initializer.ModeCLI, missing: "runtime sidecar", remove: func(edges *initializer.ApplicationLifecycles) { edges.Runtime = nil }},
		{name: "CLI workers", mode: initializer.ModeCLI, missing: "workers sidecar", remove: func(edges *initializer.ApplicationLifecycles) { edges.Workers = nil }},
		{name: "CLI transport", mode: initializer.ModeCLI, missing: "CLI transport", remove: func(edges *initializer.ApplicationLifecycles) { edges.CLI = nil }},
		{name: "MCP transport", mode: initializer.ModeMCP, missing: "MCP transport", remove: func(edges *initializer.ApplicationLifecycles) { edges.MCP = nil }},
		{name: "typed nil transport", mode: initializer.ModeMCP, missing: "MCP transport", remove: func(edges *initializer.ApplicationLifecycles) {
			var lifecycle *applicationLifecycle
			edges.MCP = lifecycle
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApplicationFixture()
			test.remove(&fixture.graph.lifecycles)
			application, err := initializer.NewApplication(test.mode, fixture.graph)
			if application != nil || err == nil {
				t.Fatalf("NewApplication() = (%v, %v), want validation failure", application, err)
			}
			if !strings.Contains(err.Error(), string(test.mode)) || !strings.Contains(err.Error(), test.missing) {
				t.Fatalf("NewApplication() error = %q, want mode %q and role %q", err, test.mode, test.missing)
			}
			if len(fixture.starts) != 0 || len(fixture.stops) != 0 || fixture.graph.closes != 0 {
				t.Fatalf("rejected plan changed lifecycle state: starts %v stops %v closes %d", fixture.starts, fixture.stops, fixture.graph.closes)
			}
		})
	}
}

func TestNewApplicationAllowsMissingOptionalDashboard(t *testing.T) {
	t.Parallel()

	fixture := newApplicationFixture()
	fixture.graph.lifecycles.Dashboard = nil
	application, err := initializer.NewApplication(initializer.ModeCLI, fixture.graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- application.Run(context.Background()) }()
	close(fixture.cli.done)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fixture.starts, []string{"runtime", "workers", "cli"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("starts = %v, want %v", got, want)
	}
}

func TestApplicationStartFailureUnwindsActivatedEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		failRole   string
		rollbackAt string
		wantEvents []string
	}{
		{
			name:     "first start",
			failRole: "runtime",
			wantEvents: []string{
				"start runtime", "close graph",
			},
		},
		{
			name:     "middle start",
			failRole: "workers",
			wantEvents: []string{
				"start runtime", "start workers",
				"stop runtime", "cancel runtime", "join runtime", "close graph",
			},
		},
		{
			name:       "final start with rollback failure",
			failRole:   "cli",
			rollbackAt: "workers",
			wantEvents: []string{
				"start runtime", "start workers", "start dashboard", "start cli",
				"stop dashboard", "cancel dashboard", "join dashboard",
				"stop workers", "cancel workers", "join workers",
				"stop runtime", "cancel runtime", "join runtime", "close graph",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &lifecycleRecorder{}
			startErr := errors.New("startup unavailable")
			rollbackErr := errors.New("rollback unavailable")
			component := func(name string) *transactionLifecycle {
				lifecycle := &transactionLifecycle{name: name, recorder: recorder}
				if name == test.failRole {
					lifecycle.startErr = startErr
				}
				if name == test.rollbackAt {
					lifecycle.stopErr = rollbackErr
				}
				return lifecycle
			}
			graph := &transactionGraph{
				recorder: recorder,
				lifecycles: initializer.ApplicationLifecycles{
					Runtime: component("runtime"), Workers: component("workers"),
					Dashboard: component("dashboard"), CLI: component("cli"),
				},
			}

			application, err := initializer.Start(context.Background(), initializer.ModeCLI, graph)
			if application != nil || !errors.Is(err, startErr) {
				t.Fatalf("Start() = (%v, %v), want nil application wrapping startup failure", application, err)
			}
			if !strings.Contains(err.Error(), `process mode "cli"`) || !strings.Contains(err.Error(), test.failRole) {
				t.Fatalf("Start() error = %q, want mode and failed component", err)
			}
			if test.rollbackAt != "" && (!errors.Is(err, rollbackErr) || !strings.Contains(err.Error(), "unwind application startup")) {
				t.Fatalf("Start() error = %q, want primary and rollback failure context", err)
			}
			got := recorder.snapshot()
			if test.name == "first start" && !reflect.DeepEqual(got, test.wantEvents) {
				t.Fatalf("lifecycle events = %v, want %v", got, test.wantEvents)
			}
			if test.name == "middle start" {
				assertMiddleStartUnwind(t, got)
			}
			if test.name == "final start with rollback failure" {
				assertTransactionalUnwind(t, got)
			}
		})
	}
}

func assertMiddleStartUnwind(t *testing.T, events []string) {
	t.Helper()
	withoutJoin := make([]string, 0, len(events)-1)
	joinCount := 0
	for _, event := range events {
		if event == "join runtime" {
			joinCount++
			continue
		}
		withoutJoin = append(withoutJoin, event)
	}
	wantWithoutJoin := []string{
		"start runtime", "start workers", "stop runtime", "cancel runtime", "close graph",
	}
	if joinCount != 1 || !reflect.DeepEqual(withoutJoin, wantWithoutJoin) {
		t.Fatalf("lifecycle events = %v, want one runtime join and ordered synchronous events %v", events, wantWithoutJoin)
	}
}

func assertTransactionalUnwind(t *testing.T, events []string) {
	t.Helper()
	filter := func(prefix string) []string {
		var filtered []string
		for _, event := range events {
			if strings.HasPrefix(event, prefix) {
				filtered = append(filtered, event)
			}
		}
		return filtered
	}
	if got, want := filter("start "), []string{"start runtime", "start workers", "start dashboard", "start cli"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("start events = %v, want %v", got, want)
	}
	if got, want := filter("stop "), []string{"stop dashboard", "stop workers", "stop runtime"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stop events = %v, want %v", got, want)
	}
	for _, name := range []string{"runtime", "workers", "dashboard"} {
		got := 0
		for _, event := range events {
			if event == "join "+name {
				got++
			}
		}
		if got != 1 {
			t.Fatalf("join %s count = %d, want one in %v", name, got, events)
		}
	}
	if events[len(events)-1] != "close graph" {
		t.Fatalf("last lifecycle event = %q, want graph close", events[len(events)-1])
	}
}

type lifecycleRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *lifecycleRecorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *lifecycleRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type transactionGraph struct {
	recorder   *lifecycleRecorder
	lifecycles initializer.ApplicationLifecycles
}

func (g *transactionGraph) Close() error {
	g.recorder.record("close graph")
	return nil
}

func (g *transactionGraph) Lifecycles() initializer.ApplicationLifecycles { return g.lifecycles }

func (g *transactionGraph) RuntimeLogMetadata() runtimehost.RuntimeLogDiagnostics {
	return runtimehost.RuntimeLogDiagnostics{}
}

type transactionLifecycle struct {
	name      string
	recorder  *lifecycleRecorder
	startErr  error
	stopErr   error
	cancel    context.CancelFunc
	done      chan struct{}
	stopCalls int
}

func (l *transactionLifecycle) Start(ctx context.Context) error {
	l.recorder.record("start " + l.name)
	if l.startErr != nil {
		return l.startErr
	}
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.done = make(chan struct{})
	go func() {
		<-runCtx.Done()
		l.recorder.record("join " + l.name)
		close(l.done)
	}()
	return nil
}

func (l *transactionLifecycle) Stop(context.Context) error {
	l.stopCalls++
	l.recorder.record("stop " + l.name)
	l.recorder.record("cancel " + l.name)
	l.cancel()
	<-l.done
	return l.stopErr
}

func (l *transactionLifecycle) Wait(ctx context.Context) error {
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestApplicationDiagnosticsAndValidation(t *testing.T) {
	t.Parallel()

	var nilApplication *initializer.Application
	if nilApplication.Graph() != nil {
		t.Fatal("nil application returned a graph")
	}
	if got := nilApplication.RuntimeLogDiagnostics(); got.Path != "" {
		t.Fatalf("nil diagnostics = %+v, want zero value", got)
	}
	if _, err := initializer.NewApplication(initializer.ModeCLI, nil); err == nil {
		t.Fatal("NewApplication(nil graph) succeeded")
	}
	fixture := newApplicationFixture()
	fixture.graph.diagnostics.Path = "runtime.log"
	application, err := initializer.NewApplication(initializer.Mode("invalid"), fixture.graph)
	if application != nil || err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("NewApplication(invalid mode) = (%v, %v)", application, err)
	}
	application, err = initializer.NewApplication(initializer.ModeCLI, fixture.graph)
	if err != nil {
		t.Fatalf("NewApplication(CLI) error = %v", err)
	}
	if got := application.RuntimeLogDiagnostics(); got.Path != "runtime.log" {
		t.Fatalf("RuntimeLogDiagnostics() = %+v, want graph metadata", got)
	}
	fixture.graph.lifecycles.MCP = &applicationLifecycle{name: "mcp", starts: &fixture.starts, stops: &fixture.stops}
	application, err = initializer.NewApplication(initializer.ModeMCP, fixture.graph)
	if application != nil || err == nil || !strings.Contains(err.Error(), "mcp") || !strings.Contains(err.Error(), "join") {
		t.Fatalf("NewApplication(non-waitable MCP) = (%v, %v), want pre-start join validation", application, err)
	}
	if err := nilApplication.Shutdown(nil); err != nil {
		t.Fatalf("Shutdown(nil context) error = %v", err)
	}
}

type applicationFixture struct {
	graph  *applicationGraph
	starts []string
	stops  []string
	api    *waitableApplicationLifecycle
	cli    *waitableApplicationLifecycle
	mcp    *waitableApplicationLifecycle
}

func newApplicationFixture() *applicationFixture {
	fixture := &applicationFixture{}
	lifecycle := func(name string) *applicationLifecycle {
		return &applicationLifecycle{name: name, starts: &fixture.starts, stops: &fixture.stops}
	}
	fixture.api = &waitableApplicationLifecycle{applicationLifecycle: lifecycle("api"), done: make(chan struct{})}
	fixture.cli = &waitableApplicationLifecycle{applicationLifecycle: lifecycle("cli"), done: make(chan struct{})}
	fixture.mcp = &waitableApplicationLifecycle{applicationLifecycle: lifecycle("mcp"), done: make(chan struct{})}
	fixture.graph = &applicationGraph{lifecycles: initializer.ApplicationLifecycles{
		API: fixture.api, CLI: fixture.cli, MCP: fixture.mcp,
		Runtime: lifecycle("runtime"), Workers: lifecycle("workers"), Dashboard: lifecycle("dashboard"),
	}}
	return fixture
}

type applicationGraph struct {
	lifecycles  initializer.ApplicationLifecycles
	diagnostics runtimehost.RuntimeLogDiagnostics
	closes      int
}

func (g *applicationGraph) Close() error {
	g.closes++
	return nil
}

func (g *applicationGraph) Lifecycles() initializer.ApplicationLifecycles { return g.lifecycles }

func (g *applicationGraph) RuntimeLogMetadata() runtimehost.RuntimeLogDiagnostics {
	return g.diagnostics
}

type applicationLifecycle struct {
	name     string
	starts   *[]string
	stops    *[]string
	startErr error
}

type waitableApplicationLifecycle struct {
	*applicationLifecycle
	done chan struct{}
}

func (l *applicationLifecycle) Start(context.Context) error {
	*l.starts = append(*l.starts, l.name)
	return l.startErr
}

func (l *applicationLifecycle) Stop(context.Context) error {
	*l.stops = append(*l.stops, l.name)
	return nil
}

func (l *waitableApplicationLifecycle) Wait(ctx context.Context) error {
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
