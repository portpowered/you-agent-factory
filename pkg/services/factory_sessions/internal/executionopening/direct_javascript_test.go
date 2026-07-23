package executionopening

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
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
				service factorysessions.ExecutionService,
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
			operation, err := NewDirectJavaScriptRunOperation(builder, runSync, func() string { return "direct-test-id" })
			if err != nil {
				t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
			}
			source := filepath.Join(t.TempDir(), "workflow.mjs")
			if err := operation.Run(context.Background(), factorysessions.DirectJavaScriptRunRequest{
				SourcePath: source, MockWorkersEnabled: testCase.mockWorkers,
				JSONOutput: true, Output: output,
			}); err != nil {
				t.Fatalf("Run: %v", err)
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
		func(context.Context, factorysessions.ExecutionService, factorysessions.StartRequest, bool, io.Writer) error {
			return runFailure
		},
		func() string { return "direct-test-id" },
	)
	if err != nil {
		t.Fatalf("NewDirectJavaScriptRunOperation: %v", err)
	}
	err = operation.Run(context.Background(), factorysessions.DirectJavaScriptRunRequest{SourcePath: "workflow.js"})
	if !errors.Is(err, runFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("Run error = %v, want joined run and close failures", err)
	}
}

type ownedExecutionStub struct {
	factorysessions.ExecutionService
	closed   bool
	closeErr error
}

func (s *ownedExecutionStub) Close() error {
	s.closed = true
	return s.closeErr
}
