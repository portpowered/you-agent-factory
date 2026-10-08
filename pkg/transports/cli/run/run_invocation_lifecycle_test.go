package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"io"
	"io/fs"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	err := RunSelected(context.Background(), ensureTestRecordingsCLI(RunConfig{
		Logger:                   zap.NewNop(),
		Dir:                      t.TempDir(),
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   io.Discard,
		DisableDefaultRecording:  true,
	}), factory.BuildRunner, factory.Invocation(), testResponsePresentation(), nil, nil, testSessionStartRequestFactory, nil, nil)
	if err != nil {
		t.Fatalf("RunSelected() error = %v", err)
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

func TestMapServerFailureRetainsSafePrimaryAndCleanupFileCauses(t *testing.T) {
	t.Parallel()
	primary := &fs.PathError{Op: "read recording", Path: "current-board.json", Err: fs.ErrPermission}
	cleanup := &fs.PathError{Op: "close recording", Path: "successor.json", Err: fs.ErrClosed}
	startup := &initializer.RuntimeHostStartupError{Cause: errors.Join(
		fmt.Errorf("restore PRIVATE payload: %w", primary), cleanup, errors.New("PRIVATE token"))}
	mapped := MapServerFailure(startup)
	var stderr bytes.Buffer
	if !WriteInvocationError(&stderr, mapped, false) {
		t.Fatal("startup failure did not render an ErrorResponse")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != ServerStartFailedCode || response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("unexpected startup code/family: %#v", response)
	}
	core, logs := observer.New(zap.ErrorLevel)
	logRunServiceOutcome(context.Background(), RunConfig{Logger: zap.New(core), WithServer: true}, startup)
	if logs.Len() != 1 {
		t.Fatalf("failure log count = %d", logs.Len())
	}
	loggedCause, _ := logs.All()[0].ContextMap()["cause"].(string)
	for _, diagnostic := range []string{response.Message, loggedCause} {
		for _, want := range []string{`read recording "current-board.json": permission denied`,
			`close recording "successor.json": file already closed`} {
			if !strings.Contains(diagnostic, want) {
				t.Fatalf("diagnostic %q omits %q", diagnostic, want)
			}
		}
		if strings.Contains(diagnostic, "PRIVATE") {
			t.Fatalf("diagnostic leaks payload: %q", diagnostic)
		}
	}
	if !errors.Is(mapped, primary) || !errors.Is(mapped, cleanup) {
		t.Fatal("startup mapping lost original cause identities")
	}
}

func TestMapServerFailure_CharacterizesUncodedRestoreCauseRedaction(t *testing.T) {
	t.Parallel()
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			cause := errors.New(`restore Work board: active Work "synthetic-cron-input" has no current place occupancy; payload=PRIVATE`)
			startup := &initializer.RuntimeHostStartupError{Cause: fmt.Errorf("activate resumed runtime: %w", cause)}
			mapped := MapServerFailureForInvocation(startup, true)
			var stderr bytes.Buffer
			if !WriteInvocationError(&stderr, mapped, debug) {
				t.Fatal("startup failure did not render a standard error response")
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			want := "requested server did not start: runtime startup failed (failure_class=runtime_startup_failed)"
			if string(response.Code) != ServerStartFailedCode || response.Family != factoryapi.ErrorFamilyInternalServerError || response.Message != want {
				t.Fatalf("response = %#v, want generic startup failure", response)
			}
			if strings.Contains(stderr.String(), "PRIVATE") || strings.Contains(stderr.String(), "synthetic-cron-input") {
				t.Fatal("uncoded restore cause leaked into stderr")
			}
			if !errors.Is(mapped, cause) {
				t.Fatal("startup mapping lost the underlying cause identity")
			}
		})
	}
}

func TestMapServerFailure_RestoreContextIsSafeThroughStartupWrappers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		reason factoryruntime.WorkRestoreReason
		places []string
		want   []string
	}{
		{"missing", factoryruntime.WorkRestoreMissingPlacement, nil, []string{"work-corrupt", "has no current place occupancy"}},
		{"conflict", factoryruntime.WorkRestoreConflictingPlacement, []string{"task:ready", "task:done"}, []string{"work-corrupt", "task:ready", "task:done", "conflicting current places"}},
		{"topology", factoryruntime.WorkRestoreUnknownPlace, []string{"task:missing"}, []string{"work-corrupt", "task:missing", "not present in the current Factory topology"}},
		{"history", factoryruntime.WorkRestoreInvalidHistory, []string{"task:ready"}, []string{"work-corrupt", "task:ready", "inconsistent recorded placement or history"}},
	} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/debug=%t", tc.name, debug), func(t *testing.T) {
				t.Parallel()
				secret := errors.New("PRIVATE-PROMPT token=PRIVATE-TOKEN")
				restore := &factoryruntime.WorkRestoreError{Reason: tc.reason, WorkID: "work-corrupt", PlaceIDs: tc.places, Cause: secret}
				activation := &factoryruntime.RuntimeActivationError{Kind: factoryruntime.RuntimeActivationErrorFailed, Cause: fmt.Errorf("restore: %w", restore)}
				startup := &initializer.RuntimeHostStartupError{Cause: activation}
				mapped := MapServerFailureForInvocation(startup, true)
				var stderr bytes.Buffer
				if !WriteInvocationError(&stderr, mapped, debug) {
					t.Fatal("restore error was not rendered")
				}
				var response factoryapi.ErrorResponse
				if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if string(response.Code) != ServerStartFailedCode || response.Family != factoryapi.ErrorFamilyInternalServerError {
					t.Fatalf("response = %#v, want stable startup code/family", response)
				}
				for _, want := range append(tc.want, "requested server did not start:", "failure_class=runtime_startup_failed") {
					if !strings.Contains(response.Message, want) {
						t.Fatalf("message %q missing %q", response.Message, want)
					}
				}
				if strings.Contains(stderr.String(), "PRIVATE") {
					t.Fatal("raw cause leaked")
				}
				var gotRestore *factoryruntime.WorkRestoreError
				var gotActivation *factoryruntime.RuntimeActivationError
				if !errors.Is(mapped, secret) || !errors.As(mapped, &gotRestore) || gotRestore != restore || !errors.As(mapped, &gotActivation) || gotActivation != activation {
					t.Fatal("mapping lost error-chain identity")
				}
			})
		}
	}
}

func TestMapServerFailure_RestoreUnknownReasonAndOrdinaryReplayKeepFallback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		reason factoryruntime.WorkRestoreReason
		resume bool
	}{
		{"PRIVATE-UNKNOWN", true}, {factoryruntime.WorkRestoreMissingPlacement, false},
	} {
		restore := &factoryruntime.WorkRestoreError{Reason: tc.reason, WorkID: "PRIVATE-ID", Cause: errors.New("PRIVATE-PROMPT")}
		mapped := MapServerFailureForInvocation(&initializer.RuntimeHostStartupError{Cause: restore}, tc.resume)
		var stderr bytes.Buffer
		WriteInvocationError(&stderr, mapped, true)
		if strings.Contains(stderr.String(), "PRIVATE") || !strings.Contains(stderr.String(), "runtime startup failed") {
			t.Fatalf("fallback changed or leaked context: %s", stderr.String())
		}
	}
}
