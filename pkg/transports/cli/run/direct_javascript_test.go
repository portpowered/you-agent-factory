package run

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
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
	root := &directJavaScriptSessionsStub{}
	op, err := NewDirectJavaScriptRunOperation(root, func() string { return "direct-id" },
		func(factorysessions.Service, factorysessions.RuntimeHostRequest, initializer.InvocationCancellation, factorysessions.RuntimeHostObserver) (lifecycle.Component, error) {
			t.Fatal("HTTP host opened without API request")
			return nil, nil
		}, directJavaScriptPresentationStub{scope: factorysessions.DirectJavaScriptRunScope{Output: &output}})
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	source := filepath.Join(t.TempDir(), "workflow.mjs")
	app, err := op.Open(context.Background(), factorysessions.DirectJavaScriptRunRequest{SourcePath: source, MockWorkersEnabled: true, ScopeID: "test-scope"}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := lifecycle.NewManager().Run(context.Background(), app.Plan); err != nil {
		t.Fatalf("Run: %v", err)
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
		}, directJavaScriptPresentationStub{scope: factorysessions.DirectJavaScriptRunScope{Output: &bytes.Buffer{}}})
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	app, err := op.Open(context.Background(), factorysessions.DirectJavaScriptRunRequest{
		SourcePath: "workflow.js", Host: &factorysessions.RuntimeHostRequest{Port: 7437}, ScopeID: "test-scope",
	}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := lifecycle.NewManager().Run(context.Background(), app.Plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ready.Load() || root.request.Correlation.RequestID != "run-host-id" {
		t.Fatalf("host readiness or canonical start missing: ready=%v request=%#v", ready.Load(), root.request)
	}
}
