package run

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type directJavaScriptSessionsStub struct {
	factorysessions.Service
	request factorysessions.SessionStartRequest
	ready   *atomic.Bool
}

func (stub *directJavaScriptSessionsStub) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	if stub.ready != nil && !stub.ready.Load() {
		return factorysessions.SessionStartResult{}, context.DeadlineExceeded
	}
	stub.request = request
	return factorysessions.SessionStartResult{Sync: &factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "direct-session", Status: string(factorysessions.LifecycleStatusSucceeded)},
		SyncOutcome:      "COMPLETED",
	}}, nil
}

type directJavaScriptPresentationStub struct {
	factorysessions.OpeningPresentationOwner
	scope factorysessions.DirectJavaScriptRunScope
}

func (stub directJavaScriptPresentationStub) DirectJavaScript(id factorysessions.OpeningScopeID) (factorysessions.DirectJavaScriptRunScope, bool) {
	return stub.scope, id == "test-scope"
}

func TestCanonicalDirectJavaScriptRunUsesProcessSessions(t *testing.T) {
	var output bytes.Buffer
	var disclosed bool
	root := &directJavaScriptSessionsStub{}
	op, err := NewDirectJavaScriptRunOperation(root, func() string { return "direct-id" },
		func(factorysessions.Service, factorysessions.RuntimeHostRequest, initializer.InvocationCancellation, factorysessions.RuntimeHostObserver) (lifecycle.Component, error) {
			t.Fatal("HTTP host opened without API request")
			return nil, nil
		}, directJavaScriptPresentationStub{scope: factorysessions.DirectJavaScriptRunScope{Output: &output}},
		func(ctx context.Context, plan lifecycle.Plan, diagnostics runtimeartifact.Diagnostics, ready <-chan initializer.RuntimeHostBinding) (initializer.LocalRuntimeRunner, error) {
			if disclosed {
				t.Fatal("startup was disclosed before lifecycle construction")
			}
			return directJavaScriptTestBuilder(ctx, plan, diagnostics, ready)
		})
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	source := filepath.Join(t.TempDir(), "workflow.mjs")
	if err := op.Run(context.Background(), factorysessions.DirectJavaScriptRunRequest{SourcePath: source, MockWorkersEnabled: true, ScopeID: "test-scope"}, nil, func() { disclosed = true }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !disclosed {
		t.Fatal("startup disclosure was not committed")
	}
	if root.request.Mode != factorysessions.SessionOperationModeDurable || !root.request.Synchronous || root.request.FolderPath != filepath.Dir(source) {
		t.Fatalf("canonical start = %#v, want durable sync source directory", root.request)
	}
	if root.request.Source.Kind != factoryruntime.WorkflowSourceKindWorkflowFile || root.request.Source.WorkflowFile != source || root.request.RuntimeOptions.ChildExecutorMode != factorysessions.ChildExecutorModeFake || root.request.Correlation.RequestID != "run-direct-id" {
		t.Fatalf("canonical selections = %#v, want JavaScript source and mock workers", root.request)
	}
	if !strings.Contains(output.String(), "Factory session direct-session completed (SUCCEEDED).") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestCanonicalDirectJavaScriptHostedRunWaitsForHTTPReadiness(t *testing.T) {
	var ready atomic.Bool
	root := &directJavaScriptSessionsStub{ready: &ready}
	op, err := NewDirectJavaScriptRunOperation(root, func() string { return "host-id" },
		func(sessions factorysessions.Service, host factorysessions.RuntimeHostRequest, _ initializer.InvocationCancellation, observer factorysessions.RuntimeHostObserver) (lifecycle.Component, error) {
			if sessions != root || host.Port != 7437 {
				t.Fatalf("host binding = (%T, %#v), want process root and selected port", sessions, host)
			}
			return lifecycle.NewRunner(func(ctx context.Context) error {
				ready.Store(true)
				observer(factorysessions.RuntimeHostBinding{Port: host.Port})
				<-ctx.Done()
				return ctx.Err()
			}), nil
		}, directJavaScriptPresentationStub{scope: factorysessions.DirectJavaScriptRunScope{Output: &bytes.Buffer{}}}, directJavaScriptTestBuilder)
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	err = op.Run(context.Background(), factorysessions.DirectJavaScriptRunRequest{
		SourcePath: "workflow.js", Host: &factorysessions.RuntimeHostRequest{Port: 7437}, ScopeID: "test-scope",
	}, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ready.Load() || root.request.Correlation.RequestID != "run-host-id" {
		t.Fatalf("host readiness or canonical start missing: ready=%v request=%#v", ready.Load(), root.request)
	}
}

func directJavaScriptTestBuilder(_ context.Context, plan lifecycle.Plan, diagnostics runtimeartifact.Diagnostics, _ <-chan initializer.RuntimeHostBinding) (initializer.LocalRuntimeRunner, error) {
	return runtimeapplication.NewManagedRunner(plan, diagnostics)
}

func TestDirectJavaScriptRunCleansHostWhenLifecycleBuildFails(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		buildErr  error
		nilRunner bool
		cancel    bool
	}{
		{name: "builder error", buildErr: errors.New("builder failed")},
		{name: "nil runner", nilRunner: true},
		{name: "canceled context", cancel: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var stopped bool
			var disclosed bool
			root := &directJavaScriptSessionsStub{}
			build := func(ctx context.Context, plan lifecycle.Plan, diagnostics runtimeartifact.Diagnostics, ready <-chan initializer.RuntimeHostBinding) (initializer.LocalRuntimeRunner, error) {
				if testCase.cancel {
					if err := ctx.Err(); err == nil {
						t.Fatal("builder did not receive cancellation")
					}
					return nil, ctx.Err()
				}
				if testCase.nilRunner {
					return nil, nil
				}
				return nil, testCase.buildErr
			}
			op, err := NewDirectJavaScriptRunOperation(root, func() string { return "failed-id" },
				func(factorysessions.Service, factorysessions.RuntimeHostRequest, initializer.InvocationCancellation, factorysessions.RuntimeHostObserver) (lifecycle.Component, error) {
					return lifecycle.Functions{StopFunc: func(ctx context.Context) error {
						if ctx.Err() != nil {
							t.Fatal("host cleanup received a canceled context")
						}
						stopped = true
						return nil
					}}, nil
				}, directJavaScriptPresentationStub{scope: factorysessions.DirectJavaScriptRunScope{Output: &bytes.Buffer{}}}, build)
			if err != nil {
				t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if testCase.cancel {
				cancel()
			}
			err = op.Run(ctx, factorysessions.DirectJavaScriptRunRequest{
				SourcePath: "workflow.js", Host: &factorysessions.RuntimeHostRequest{}, ScopeID: "test-scope",
			}, nil, func() { disclosed = true })
			if err == nil || !stopped || disclosed || root.request.Correlation.RequestID != "" {
				t.Fatalf("Run = %v, stopped=%t disclosed=%t start=%#v", err, stopped, disclosed, root.request)
			}
		})
	}
}
