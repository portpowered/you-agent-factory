package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestOpenInvocationRetainsInjectedOperationWithoutOpeningRuntime(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	buildCalls := 0
	lifecycleStarted := false
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		buildCalls++
		if lifecycleStarted {
			t.Fatal("invocation bootstrap constructed after lifecycle start")
		}
		return stubInvocationService{
			run: func(ctx context.Context) error {
				lifecycleStarted = true
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	factory := testRunnerOpeners{invocation: openTestInvocationRunner}
	operation, err := Open(context.Background(), ensureTestRecordingsCLI(RunConfig{
		Dir:                      t.TempDir(),
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   io.Discard,
		DisableDefaultRecording:  true,
	}), factory.BuildRunner, factory.Invocation(), testResponsePresentation(), nil, testMockWorkersConfigLoader, testRuntimeOpeningRequestFactory)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if buildCalls != 0 || lifecycleStarted {
		t.Fatalf("after construction: build calls = %d, lifecycle started = %t; want 0, false", buildCalls, lifecycleStarted)
	}

	err = operation.Run(context.Background())
	if err != nil {
		t.Fatalf("Operation.Run() error = %v", err)
	}
	if buildCalls != 1 || !lifecycleStarted {
		t.Fatalf("after initialization: build calls = %d, lifecycle started = %t; want 1, true", buildCalls, lifecycleStarted)
	}
}

func TestInvocationTargetCarriesOnlyBoundedRuntimeSelection(t *testing.T) {
	t.Parallel()

	target := invocationTarget(RunConfig{
		Dir:                   "/tmp/factory",
		FactoryConfigPath:     "/tmp/factory/factory.yaml",
		CanonicalSessionID:    "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba",
		Worktree:              "feature-login",
		Port:                  7437,
		WorkerReasoningEffort: "xhigh",
	}, nil)
	if target.FactoryDir != "/tmp/factory" {
		t.Fatalf("FactoryDir = %q, want /tmp/factory", target.FactoryDir)
	}
	if target.FactorySourcePath != "/tmp/factory/factory.yaml" {
		t.Fatalf(
			"FactorySourcePath = %q, want /tmp/factory/factory.yaml",
			target.FactorySourcePath,
		)
	}
	if target.Worktree != "feature-login" {
		t.Fatalf("Worktree = %q, want feature-login", target.Worktree)
	}
	if target.WorkerReasoningEffort != "xhigh" {
		t.Fatalf("WorkerReasoningEffort = %q, want xhigh", target.WorkerReasoningEffort)
	}
	if target.CanonicalSessionID != "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba" {
		t.Fatalf("CanonicalSessionID = %q, want preallocated UUID", target.CanonicalSessionID)
	}
}

func TestRun_FactoryInvocationUsesNoServerBootstrapConfig(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var captured *testRuntimeSelections
	var capturedEdges serviceedges.Edges
	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, edges serviceedges.Edges) (sessionInvocationRunner, error) {
		cloned := *cfg
		captured = &cloned
		capturedEdges = edges
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured == nil {
		t.Fatal("expected factory invocation bootstrap config capture")
	}
	if captured.Port != 0 {
		t.Fatalf("captured Port = %d, want 0", captured.Port)
	}
	if capturedEdges.APIServerStarter != nil {
		t.Fatal("captured APIServerStarter = non-nil, want nil")
	}
}

func TestRun_FactoryInvocationReleasesSessionThroughFactoryServiceOwnership(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var closedSessionID string
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				}, nil
			},
			close: func(_ context.Context, sessionID string) error {
				closedSessionID = sessionID
				return nil
			},
		}, nil
	}

	var output bytes.Buffer
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if closedSessionID == "" {
		t.Fatal("expected CloseFactorySession through bootstrap ownership path")
	}
}

func TestBuildBatchReportSortsFailuresAndUsesCanonicalReasons(t *testing.T) {
	report := buildBatchReport(factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{
			{WorkID: "work-2", Name: "second Work", WorkTypeID: "task", State: "failed", StateCategory: string(factoryruntime.StateCategoryFailed), TraceID: "trace-2"},
			{WorkID: "work-1", Name: "first Work", WorkTypeID: "task", State: "failed", StateCategory: string(factoryruntime.StateCategoryFailed), TraceID: "trace-1"},
			{WorkID: "done", Name: "successful Work", WorkTypeID: "task", State: "done", StateCategory: string(factoryruntime.StateCategoryTerminal)},
		},
		DispatchHistory: []factoryruntime.CleanInvocationDispatch{
			{Outcome: "FAILED", Reason: "second reason", Outputs: []factoryruntime.CleanInvocationWork{{WorkID: "work-2"}}},
			{Outcome: "FAILED", Reason: "first reason", Outputs: []factoryruntime.CleanInvocationWork{{WorkID: "work-1"}}},
		},
	})

	if report.Status != "FAILED" || len(report.Failures) != 2 {
		t.Fatalf("report = %#v, want two failed Work items", report)
	}
	if got := report.Failures[0]; got.WorkID != "work-1" || got.WorkName != "first Work" || got.WorkState != "task:failed" || got.Reason != "first reason" {
		t.Fatalf("first failure = %#v, want deterministic canonical details", got)
	}
	if got := report.Failures[1]; got.WorkID != "work-2" || got.Reason != "second reason" {
		t.Fatalf("second failure = %#v, want deterministic ordering", got)
	}
}

func TestBuildBatchReportPrefersFinalTerminalReasonOverEarlierDispatch(t *testing.T) {
	report := buildBatchReport(factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{{
			WorkID: "work-breaker", Name: "breaker Work", WorkTypeID: "task", State: "failed",
			StateCategory: string(factoryruntime.StateCategoryFailed),
			FailureReason: "consecutive failures 1 for transition process exceeds max 1",
		}},
		DispatchHistory: []factoryruntime.CleanInvocationDispatch{{
			Outcome: "FAILED", Reason: "worker command failed before the breaker tripped",
			Outputs: []factoryruntime.CleanInvocationWork{{WorkID: "work-breaker"}},
		}},
	})

	if len(report.Failures) != 1 {
		t.Fatalf("report = %#v, want one failure", report)
	}
	if got := report.Failures[0].Reason; got != "consecutive failures 1 for transition process exceeds max 1" {
		t.Fatalf("failure reason = %q, want final circuit-breaker reason", got)
	}
}

func TestReportBatchResultJSONIsParseableAndReturnsFailure(t *testing.T) {
	var output bytes.Buffer
	err := reportBatchResult(RunConfig{JSON: true, Output: &output}, factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{{
			WorkID: "work-1", Name: "failing Work", WorkTypeID: "task", State: "failed",
			StateCategory: string(factoryruntime.StateCategoryFailed),
		}},
	})
	if err == nil {
		t.Fatal("reportBatchResult() error = nil, want batch failure")
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || invocationErr.Code != batchFailureCode {
		t.Fatalf("error = %v, want %s InvocationError", err, batchFailureCode)
	}
	var decoded batchReport
	if decodeErr := json.Unmarshal(output.Bytes(), &decoded); decodeErr != nil {
		t.Fatalf("batch JSON = %q is not parseable: %v", output.String(), decodeErr)
	}
	if decoded.Status != "FAILED" || len(decoded.Failures) != 1 {
		t.Fatalf("decoded report = %#v, want one failure", decoded)
	}
	failure := decoded.Failures[0]
	if failure.WorkName != "failing Work" || failure.WorkState != "task:failed" || strings.TrimSpace(failure.Reason) == "" {
		t.Fatalf("decoded failure = %#v, want name, state, and actionable reason", failure)
	}
}

func TestReportBatchResultSuccessHasNoFailuresAndDoesNotReturnError(t *testing.T) {
	var output bytes.Buffer
	err := reportBatchResult(RunConfig{JSONOutput: true, Output: &output}, factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{{
			WorkID: "work-1", Name: "successful Work", WorkTypeID: "task", State: "done",
			StateCategory: string(factoryruntime.StateCategoryTerminal),
		}},
	})
	if err != nil {
		t.Fatalf("reportBatchResult() error = %v, want nil", err)
	}
	var decoded batchReport
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("success batch JSON = %q is not parseable: %v", output.String(), err)
	}
	if decoded.Status != "COMPLETED" || decoded.Failures == nil || len(decoded.Failures) != 0 {
		t.Fatalf("decoded success report = %#v, want COMPLETED with empty failures", decoded)
	}
}

func TestRunFactoryServiceAndEmitResultLeavesEngineErrorsUnclassified(t *testing.T) {
	wantErr := errors.New("engine failed before terminal report")
	err := runFactoryServiceAndEmitResult(
		context.Background(),
		RunConfig{WorkFile: "work.json"},
		stubFactoryService{run: func(context.Context) error { return wantErr }},
		resolvedRunRecordPath{},
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want original engine error", err)
	}
}
