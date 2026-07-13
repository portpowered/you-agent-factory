package wire

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"go.uber.org/zap"
)

func TestValidateProductionInputsReportsEachMissingEdge(t *testing.T) {
	t.Parallel()

	valid := func() Inputs {
		return Inputs{
			Config:    &runtimehost.Config{Logger: zap.NewNop(), Clock: productionInternalClock{}},
			MCPInput:  strings.NewReader(""),
			MCPOutput: &bytes.Buffer{},
		}
	}
	tests := []struct {
		name string
		ctx  func() context.Context
		edit func(*Inputs)
		want string
	}{
		{name: "nil context", ctx: func() context.Context { return nil }, edit: func(*Inputs) {}, want: "context is required"},
		{name: "canceled context", ctx: canceledProductionContext, edit: func(*Inputs) {}, want: context.Canceled.Error()},
		{name: "config", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config = nil }, want: "config is required"},
		{name: "logger", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config.Logger = nil }, want: "config.logger is required"},
		{name: "clock", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config.Clock = (*productionInternalClock)(nil) }, want: "config.clock is required"},
		{name: "MCP input", ctx: context.Background, edit: func(inputs *Inputs) { inputs.MCPInput = nil }, want: "mcpInput is required"},
		{name: "MCP output", ctx: context.Background, edit: func(inputs *Inputs) { inputs.MCPOutput = nil }, want: "mcpOutput is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := valid()
			test.edit(&inputs)
			if err := validateProductionInputs(test.ctx(), inputs); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateProductionInputs() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAssembleProductionGraphRejectsMissingStartupBundle(t *testing.T) {
	t.Parallel()

	for _, core := range []*runtimehost.Core{nil, {}} {
		graph, err := assembleProductionGraph(core, &runtimehost.Config{}, Inputs{}, &resourceSet{})
		if graph != nil || err == nil || !strings.Contains(err.Error(), "startup runtime bundle is required") {
			t.Fatalf("assembleProductionGraph() = (%v, %v), want missing bundle error", graph, err)
		}
	}
}

func TestFailProductionBuildRetainsConstructionAndCleanupErrors(t *testing.T) {
	t.Parallel()

	constructionErr := errors.New("transport construction failed")
	cleanupErr := errors.New("runtime sink close failed")
	resources := &resourceSet{}
	resources.add("runtime core", &recordingCloser{err: cleanupErr})
	err := failProductionBuild(resources, constructionErr)
	if !errors.Is(err, constructionErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("failProductionBuild() error = %v, want construction and cleanup causes", err)
	}

	if err := failProductionBuild(&resourceSet{}, constructionErr); !errors.Is(err, constructionErr) {
		t.Fatalf("failProductionBuild() without cleanup error = %v, want construction cause", err)
	}
}

func TestRunnerLifecycleWaitAndStopBehavior(t *testing.T) {
	t.Parallel()

	var nilLifecycle *runnerLifecycle
	if err := nilLifecycle.Start(context.Background()); err == nil {
		t.Fatal("nil runner lifecycle Start() succeeded")
	}
	if err := nilLifecycle.Wait(context.Background()); err != nil {
		t.Fatalf("nil runner lifecycle Wait() error = %v", err)
	}
	if err := nilLifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("nil runner lifecycle Stop() error = %v", err)
	}

	lifecycle := newRunnerLifecycle(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err := lifecycle.Wait(context.Background()); err == nil {
		t.Fatal("Wait() before Start() succeeded")
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := lifecycle.Start(context.Background()); err == nil {
		t.Fatal("second Start() succeeded")
	}
	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if err := lifecycle.Wait(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait(canceled context) error = %v, want context.Canceled", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := lifecycle.Wait(nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait(nil) after stop error = %v, want runner cancellation", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestRunnerLifecycleReturnsRunnerFailureFromWaitAndStop(t *testing.T) {
	t.Parallel()

	cause := errors.New("listener failed")
	lifecycle := newRunnerLifecycle(func(context.Context) error { return cause })
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := lifecycle.Wait(context.Background()); !errors.Is(err, cause) {
		t.Fatalf("Wait() error = %v, want runner cause", err)
	}
	if err := lifecycle.Stop(context.Background()); !errors.Is(err, cause) {
		t.Fatalf("Stop() error = %v, want runner cause", err)
	}
}

func canceledProductionContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type productionInternalClock struct{}

func (productionInternalClock) Now() time.Time { return time.Unix(0, 0).UTC() }
