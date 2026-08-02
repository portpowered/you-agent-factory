package executionopening

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestDirectJavaScriptRunOperationSupportsCustomerSourceExtensions(t *testing.T) {
	operation := &directJavaScriptRunOperation{}
	for _, source := range []string{"workflow.js", "WORKFLOW.MJS", " workflow.cjs "} {
		if !operation.Supports(source) {
			t.Fatalf("Supports(%q) = false", source)
		}
	}
	if operation.Supports("factory.json") {
		t.Fatal("Supports(factory.json) = true")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestDirectJavaScriptRunOperationOwnsOpeningRequestPolicyAndCleanup(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		mockWorkers bool
		wantMode    string
	}{
		{name: "live", wantMode: factorysessions.ChildExecutorModeLive},
		{name: "mock", mockWorkers: true, wantMode: factorysessions.ChildExecutorModeFake},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owned := &ownedExecutionStub{}
			var provider, projectRoot, fixturePath, builderMode string
			builder := func(
				_ context.Context, gotProvider, gotRoot, gotFixture, gotMode string,
			) (roles.OwnedExecutionService, error) {
				provider, projectRoot, fixturePath, builderMode = gotProvider, gotRoot, gotFixture, gotMode
				return owned, nil
			}
			var started factorysessions.StartRequest
			var jsonOutput bool
			var outputMatches bool
			output := &bytes.Buffer{}
			runSync := func(
				_ context.Context,
				service durableexecution.Service,
				request factorysessions.StartRequest,
				jsonValue bool,
				writer io.Writer,
			) error {
				if service != owned {
					t.Fatal("sync runner did not receive opened execution service")
				}
				started, jsonOutput, outputMatches = request, jsonValue, writer == output
				return nil
			}
			operation, err := NewDirectJavaScriptRunOperation(
				builder, runSync, func() string { return "direct-test-id" },
				func(roles.OwnedExecutionService, roles.DirectJavaScriptLifecycle, factorysessions.DirectJavaScriptRunRequest) (lifecycle.Component, error) {
					return nil, nil
				},
			)
			if err != nil {
				t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
			}
			source := filepath.Join(t.TempDir(), "workflow.mjs")
			opened, err := operation.Open(context.Background(), factorysessions.DirectJavaScriptRunRequest{
				SourcePath: source, MockWorkersEnabled: testCase.mockWorkers,
				JSONOutput: true, Output: output,
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if err := lifecycle.NewManager().Run(context.Background(), opened.Plan); err != nil {
				t.Fatalf("Run lifecycle: %v", err)
			}
			if provider != string(factorysessions.ExecutionProviderJavaScriptRuntime) || projectRoot != filepath.Dir(source) || fixturePath != "" || builderMode != testCase.wantMode {
				t.Fatalf("builder inputs = (%q, %q, %q, %q)", provider, projectRoot, fixturePath, builderMode)
			}
			if started.Source.Kind != factoryruntime.WorkflowSourceKindWorkflowFile || started.Source.WorkflowFile != source {
				t.Fatalf("start source = %#v", started.Source)
			}
			if !strings.HasPrefix(started.RequestID, "run-") || started.Runtime == nil || started.Runtime.ChildExecutorMode != testCase.wantMode {
				t.Fatalf("start request = %#v", started)
			}
			if !jsonOutput || !outputMatches || !owned.closed {
				t.Fatalf("presentation/cleanup = json:%v output:%v closed:%v", jsonOutput, outputMatches, owned.closed)
			}
		})
	}
}

func TestDirectJavaScriptRunOperationJoinsExecutionAndCloseFailures(t *testing.T) {
	runFailure := errors.New("run failed")
	closeFailure := errors.New("close failed")
	owned := &ownedExecutionStub{closeErr: closeFailure}
	operation, err := NewDirectJavaScriptRunOperation(
		func(context.Context, string, string, string, string) (roles.OwnedExecutionService, error) {
			return owned, nil
		},
		func(context.Context, durableexecution.Service, factorysessions.StartRequest, bool, io.Writer) error {
			return runFailure
		},
		func() string { return "direct-test-id" },
		func(roles.OwnedExecutionService, roles.DirectJavaScriptLifecycle, factorysessions.DirectJavaScriptRunRequest) (lifecycle.Component, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	opened, err := operation.Open(
		context.Background(),
		factorysessions.DirectJavaScriptRunRequest{SourcePath: "workflow.js"},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	err = lifecycle.NewManager().Run(context.Background(), opened.Plan)
	if !errors.Is(err, runFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("Run error = %v, want joined run and close failures", err)
	}
}

func TestDirectJavaScriptRunOperationGatesHostedCompletionOnReadiness(t *testing.T) {
	owned := &ownedExecutionStub{}
	var ready atomic.Bool
	operation, err := NewDirectJavaScriptRunOperation(
		func(context.Context, string, string, string, string) (roles.OwnedExecutionService, error) {
			return owned, nil
		},
		func(context.Context, durableexecution.Service, factorysessions.StartRequest, bool, io.Writer) error {
			if !ready.Load() {
				t.Fatal("direct JavaScript completion started before listener readiness")
			}
			return nil
		},
		func() string { return "direct-test-id" },
		func(_ roles.OwnedExecutionService, _ roles.DirectJavaScriptLifecycle, request factorysessions.DirectJavaScriptRunRequest) (lifecycle.Component, error) {
			return lifecycle.NewRunner(func(ctx context.Context) error {
				ready.Store(true)
				request.RuntimeHostObserver(factorysessions.RuntimeHostBinding{Port: request.Host.Port})
				<-ctx.Done()
				return ctx.Err()
			}), nil
		},
	)
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	opened, err := operation.Open(context.Background(), factorysessions.DirectJavaScriptRunRequest{
		SourcePath: "workflow.js",
		Host:       &factorysessions.RuntimeHostRequest{Port: 7437},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := lifecycle.NewManager().Run(context.Background(), opened.Plan); err != nil {
		t.Fatalf("Run lifecycle: %v", err)
	}
	if !owned.closed {
		t.Fatal("hosted execution was not closed")
	}
}

type ownedExecutionStub struct {
	durableexecution.Service
	closed   bool
	closeErr error
}

func (s *ownedExecutionStub) Close() error {
	s.closed = true
	return s.closeErr
}
