package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Lifecycle fixtures accept their authored identities through this controlled
// catalog edge; provider selection behavior has dedicated resolver witnesses.
func testSubagentProviderIdentity(_ context.Context, identity string) (string, error) {
	return strings.TrimSpace(identity), nil
}

type subagentTargetFake struct {
	factorysessions.Service
	start                 factorysessions.SessionStartRequest
	invoke                factorysessions.SessionInvokeRequest
	control               factorysessions.SessionControlRequest
	started               bool
	startErr              error
	closed                bool
	controlHasDeadline    bool
	controlTimeToDeadline time.Duration
	invokeErr             error
	closeErr              error
	invokeResult          *factorysessions.InvocationResult
	projection            factorysessions.SessionProjection
	projectionErr         error
	getCalls              int
	getBeforeClose        bool
	getContextActive      bool
	getHasDeadline        bool
}

func (fake *subagentTargetFake) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	fake.start = request
	fake.started = true
	if fake.startErr != nil {
		return factorysessions.SessionStartResult{}, fake.startErr
	}
	return factorysessions.SessionStartResult{SessionID: "session-1", Mode: factorysessions.SessionOperationModeLive, Status: "RUNNING"}, nil
}

func (fake *subagentTargetFake) Invoke(_ context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	fake.invoke = request
	if fake.invokeErr != nil {
		return factorysessions.InvocationResult{}, fake.invokeErr
	}
	if fake.invokeResult != nil {
		return *fake.invokeResult, nil
	}
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "subagent answer"}},
	}, nil
}

func (fake *subagentTargetFake) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	fake.control = request
	fake.closed = request.SessionID == "session-1" && request.Operation == factorysessions.SessionControlClose
	if deadline, ok := ctx.Deadline(); ok {
		fake.controlHasDeadline = true
		fake.controlTimeToDeadline = time.Until(deadline)
	}
	return factorysessions.SessionControlResult{}, fake.closeErr
}

func (fake *subagentTargetFake) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	fake.getCalls++
	fake.getBeforeClose = !fake.closed && sessionID == "session-1"
	fake.getContextActive = ctx.Err() == nil
	_, fake.getHasDeadline = ctx.Deadline()
	return fake.projection, fake.projectionErr
}

func TestSubagentRunsPackagedFactoryWithDefaultsAndReturnsText(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-1" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Summarize this"})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Result.Text != "subagent answer" || response.Result.SessionID != "session-1" {
		t.Fatalf("Subagent result = %#v", response.Result)
	}
	if target.start.Caller != nil || target.invoke.Caller != nil {
		t.Fatal("ordinary subagent acquired caller authority")
	}
	assertSubagentStart(t, target)
	assertSubagentInvocationAndClose(t, target)
	if target.getCalls != 0 {
		t.Fatalf("successful invocation read timeout snapshot %d times", target.getCalls)
	}
}

func assertSubagentStart(t *testing.T, target *subagentTargetFake) {
	t.Helper()
	if target.start.Mode != factorysessions.SessionOperationModeLive || !target.start.ActivationOnly {
		t.Fatalf("start mode = %q activationOnly = %t", target.start.Mode, target.start.ActivationOnly)
	}
	if target.start.Correlation.RequestID != "request-1" {
		t.Fatalf("start correlation = %#v", target.start.Correlation)
	}
	if target.start.Definition.FactoryID != factorydefinitions.PackagedSubagentFactoryName {
		t.Fatalf("start definition = %#v", target.start.Definition)
	}
	if target.start.Source.Kind != factoryruntime.WorkflowSourceKindFactoryID || target.start.Source.FactoryID != factorydefinitions.PackagedSubagentFactoryName {
		t.Fatalf("start source = %#v", target.start.Source)
	}
	if target.start.Args["workingRoot"] != "C:/project" {
		t.Fatalf("workingRoot = %#v", target.start.Args)
	}
	if target.start.FolderPath != "C:/project" {
		t.Fatalf("folderPath = %q", target.start.FolderPath)
	}
	if target.start.RuntimeSelection == nil || target.start.RuntimeSelection.ExecutionBaseDir != "C:/project" || target.start.RuntimeSelection.Mode != factorysessions.SessionRuntimeModeService {
		t.Fatalf("runtime selection = %#v", target.start.RuntimeSelection)
	}
}

func assertSubagentInvocationAndClose(t *testing.T, target *subagentTargetFake) {
	t.Helper()
	if target.invoke.SessionID != "session-1" {
		t.Fatalf("invoke session = %q", target.invoke.SessionID)
	}
	if got := target.invoke.Args; len(got) != 2 || got["input"] != "Summarize this" || got["workingRoot"] != "C:/project" {
		t.Fatalf("default invocation args = %#v", got)
	}
	if target.control.Operation != factorysessions.SessionControlClose || target.control.SessionID != "session-1" {
		t.Fatalf("control = %#v", target.control)
	}
	if !target.started || !target.closed {
		t.Fatalf("lifecycle start=%t close=%t", target.started, target.closed)
	}
	if !target.controlHasDeadline {
		t.Fatal("close context has no deadline")
	}
	if target.controlTimeToDeadline < 14*time.Second || target.controlTimeToDeadline > 16*time.Second {
		t.Fatalf("close context deadline = %s, want about 15s", target.controlTimeToDeadline)
	}
}

func TestSubagentUsesRequestedWorkingRoot(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "server-root", func() string { return "request-3" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Inspect this repository", WorkingRoot: "selected-root",
	})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if got := target.start.Args["workingRoot"]; got != "selected-root" {
		t.Fatalf("workingRoot = %#v, want selected-root", got)
	}
	if got := target.invoke.Args["workingRoot"]; got != "selected-root" {
		t.Fatalf("invocation workingRoot = %#v, want selected-root", got)
	}
}

func TestSubagentForwardsModelOverridesAndCleansUpOnInvocationFailure(t *testing.T) {
	target := &subagentTargetFake{invokeErr: errors.New("provider unavailable")}
	response := mcpfactorysession.Subagent(context.Background(), target, "", func() string { return "request-2" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Explain this", Provider: "opencode", Model: "local-model", ReasoningEffort: "high",
	})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	args := target.invoke.Args
	if args["workerProvider"] != "opencode" || args["workerModel"] != "local-model" || args["workerReasoningEffort"] != "high" {
		t.Fatalf("override args = %#v", args)
	}
	if !target.closed {
		t.Fatal("Factory Session was not closed after invocation failure")
	}
	if target.getCalls != 0 {
		t.Fatalf("non-timeout failure read timeout snapshot %d times", target.getCalls)
	}
}

func TestSubagentSurfacesSafeProviderThrottleFailure(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive ACP session/prompt output with token secret",
		FailureReason: "throttled",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-throttled" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_throttled" || !response.Error.Retryable {
		t.Fatalf("throttled response = %#v", response)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["failureReason"] != "throttled" {
		t.Fatalf("throttled diagnostic = %#v", response.Error)
	}
	assertSubagentClosedDiagnostic(t, response.Error)
	if strings.Contains(response.Error.Message, "sensitive") || !target.closed {
		t.Fatalf("terminal failure leaked provider text or skipped close: %#v", response.Error)
	}
}

func TestSubagentSurfacesProviderMisconfiguredFailure(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive provider configuration token",
		FailureReason: string(workers.WorkFailureTypeMisconfigured),
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-misconfigured" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_misconfigured" || response.Error.Retryable {
		t.Fatalf("misconfigured response = %#v", response)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["failureReason"] != string(workers.WorkFailureTypeMisconfigured) {
		t.Fatalf("misconfigured diagnostic = %#v", response.Error)
	}
	if response.Error.Details["suggestedAction"] == "" {
		t.Fatal("misconfigured missing suggestedAction")
	}
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(fmt.Sprint(response.Error.Details), "sensitive") || !target.closed {
		t.Fatalf("misconfigured terminal failure leaked provider text or skipped close: %#v", response.Error)
	}
}

func TestSubagentPiMisconfigurationSuggestsVersionCheck(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive local model endpoint",
		FailureReason: string(workers.WorkFailureTypeMisconfigured),
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-pi-misconfigured" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Read README", Provider: "pi"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_misconfigured" {
		t.Fatalf("Pi misconfigured response = %#v", response)
	}
	action := fmt.Sprint(response.Error.Details["suggestedAction"])
	if !strings.Contains(action, "pi --version") || !strings.Contains(action, "0.81.0") || strings.Contains(fmt.Sprint(response.Error), "sensitive") {
		t.Fatalf("Pi suggested action = %q; response = %#v", action, response.Error)
	}
}

func TestSubagentSurfacesProviderExecutableMissingFailure(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive provider executable path",
		FailureReason: string(workers.WorkFailureTypeMissingExecutable),
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-executable-missing" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_executable_missing" || response.Error.Retryable {
		t.Fatalf("executable missing response = %#v", response)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["failureReason"] != string(workers.WorkFailureTypeMissingExecutable) {
		t.Fatalf("executable missing diagnostic = %#v", response.Error)
	}
	if response.Error.Details["suggestedAction"] == "" {
		t.Fatal("executable missing missing suggestedAction")
	}
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(fmt.Sprint(response.Error.Details), "sensitive") || !target.closed {
		t.Fatalf("executable missing terminal failure leaked provider text or skipped close: %#v", response.Error)
	}
}

func TestSubagentTimeoutReportsPossibleWorkspaceEdits(t *testing.T) {
	timeout := int64(60_000)
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:    factorysessions.InvocationTerminalStatusTimedOut,
		ErrorCode: "INVOCATION_TIMED_OUT",
		Message:   "private provider output",
		RequestID: "request-timeout",
		TraceID:   "trace-abc-123",
		WorkID:    "work-42",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Edit one file", TimeoutMillis: &timeout,
	})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["timeoutMillis"] != timeout || response.Error.Details["partialEffectsPossible"] != true {
		t.Fatalf("timeout diagnostic = %#v", response.Error)
	}
	if response.Error.Details["requestId"] != "request-timeout" || response.Error.Details["traceId"] != "trace-abc-123" || response.Error.Details["workId"] != "work-42" {
		t.Fatalf("timeout IDs = %#v", response.Error.Details)
	}
	if _, ok := response.Error.Details["provider"]; ok {
		t.Fatalf("unselected provider in timeout details = %#v", response.Error.Details)
	}
	if _, ok := response.Error.Details["model"]; ok {
		t.Fatalf("unselected model in timeout details = %#v", response.Error.Details)
	}
	assertSubagentTimeoutSuggestedAction(t, response.Error.Details["suggestedAction"])
	assertSubagentClosedDiagnostic(t, response.Error)
	if !strings.Contains(response.Error.Message, "workspace edits may have occurred") || strings.Contains(response.Error.Message, "private") || !target.closed {
		t.Fatalf("timeout message or cleanup = %#v", response.Error)
	}
}

type stalledSubagentTarget struct {
	*subagentTargetFake
	blockInvoke, blockSnapshot, blockClose bool
	release                                chan struct{}
	entered                                chan struct{}
	finished                               chan struct{}
}

type lateStartSubagentTarget struct {
	*subagentTargetFake
	entered chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func (target *lateStartSubagentTarget) Start(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	close(target.entered)
	<-target.release // Simulates a Start that publishes a session after its context expires.
	return target.subagentTargetFake.Start(ctx, request)
}

func (target *lateStartSubagentTarget) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	result, err := target.subagentTargetFake.Control(ctx, request)
	close(target.closed)
	return result, err
}

func TestSubagentClosesSessionCreatedAfterStartTimeout(t *testing.T) {
	timeout := int64(20)
	target := &lateStartSubagentTarget{
		subagentTargetFake: &subagentTargetFake{},
		entered:            make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{}),
	}
	released := false
	defer func() {
		if !released {
			close(target.release)
		}
	}()
	returned := make(chan mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], 1)
	go func() {
		returned <- mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "late-start" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", TimeoutMillis: &timeout})
	}()
	<-target.entered
	select {
	case response := <-returned:
		if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Details["phase"] != "start" {
			t.Fatalf("start timeout response = %#v", response)
		}
		if response.Error.Details["sessionClosed"] == true {
			t.Fatalf("start timeout falsely claimed close: %#v", response.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled Start held the MCP response past its timeout")
	}
	close(target.release)
	released = true
	select {
	case <-target.closed:
		if !target.subagentTargetFake.closed {
			t.Fatal("late Start did not close its Factory Session")
		}
	case <-time.After(time.Second):
		t.Fatal("late Start did not attempt Factory Session cleanup")
	}
}

func (target *stalledSubagentTarget) Invoke(ctx context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	if target.blockInvoke {
		defer close(target.finished)
		close(target.entered)
		<-target.release // Deliberately ignores ctx, as a stalled downstream dependency can.
	}
	return target.subagentTargetFake.Invoke(ctx, request)
}

func (target *stalledSubagentTarget) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if target.blockSnapshot {
		defer close(target.finished)
		close(target.entered)
		<-target.release
	}
	return target.subagentTargetFake.GetFactorySession(ctx, sessionID)
}

func (target *stalledSubagentTarget) Control(ctx context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	if target.blockClose {
		defer close(target.finished)
		close(target.entered)
		<-target.release
	}
	return target.subagentTargetFake.Control(ctx, request)
}

func TestSubagentTimeoutReturnsWhenDependencyIgnoresContext(t *testing.T) {
	for _, step := range []string{"invoke", "snapshot", "close"} {
		t.Run(step, func(t *testing.T) {
			timeout := int64(20)
			target := &stalledSubagentTarget{
				subagentTargetFake: &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
				blockInvoke:        step == "invoke", blockSnapshot: step == "snapshot", blockClose: step == "close",
				release: make(chan struct{}), entered: make(chan struct{}), finished: make(chan struct{}),
			}
			returned := make(chan mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], 1)
			go func() {
				returned <- mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-stalled" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file", TimeoutMillis: &timeout})
			}()
			select {
			case <-target.entered:
			case <-time.After(time.Second):
				close(target.release)
				t.Fatal("stalled dependency was never entered")
			}
			defer func() {
				close(target.release)
				<-target.finished
			}()
			select {
			case response := <-returned:
				if response.Error == nil || response.Result != nil {
					t.Fatalf("%s response = %#v", step, response)
				}
				wantCode := "factory_session.subagent.timed_out"
				if step == "close" {
					wantCode = "factory_session.subagent.cleanup_timed_out"
				}
				if response.Error.Code != wantCode {
					t.Fatalf("%s error code = %q, want %q", step, response.Error.Code, wantCode)
				}
				if step == "close" && response.Error.Details["sessionClosed"] == true {
					t.Fatalf("unconfirmed close claimed success: %#v", response.Error)
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("%s held the synchronous response past the bounded cleanup window", step)
			}
		})
	}
}

func TestSubagentTimeoutSnapshotPrecedesCloseAndExcludesProjectionText(t *testing.T) {
	secret := "sensitive prompt and provider output"
	projection := factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{LifecycleControlStatus: secret},
		Runtime: factorysessions.RuntimeProjection{
			Status:                 secret,
			LifecycleControlStatus: &secret,
			Progress: factorysessions.RuntimeProgress{
				FactoryState: secret, InFlightCount: 2,
				Categories: factorysessions.RuntimeStatusCategories{Initial: 1, Processing: 2, Terminal: 3, Failed: 4},
			},
			JavaScript: &factorysessions.JavaScriptRuntimeProjection{
				Phase: &secret, ScriptStatus: factorydefinitions.FactorySessionJavaScriptScriptStatusRunning,
			},
		},
	}
	projection.Runtime.JavaScript.ChildDispatchCounts.Completed = 5
	projection.Runtime.JavaScript.ChildDispatchCounts.Queued = 6
	projection.Runtime.JavaScript.ChildDispatchCounts.Running = 7
	for _, tc := range []struct {
		name      string
		invokeErr error
		result    *factorysessions.InvocationResult
	}{
		{name: "terminal timed out", result: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut}},
		{name: "deadline exceeded", invokeErr: fmt.Errorf("private: %w", context.DeadlineExceeded)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &subagentTargetFake{invokeErr: tc.invokeErr, invokeResult: tc.result, projection: projection}
			response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-snapshot" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: secret})
			if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" {
				t.Fatalf("timeout response = %#v", response)
			}
			if target.getCalls != 1 || !target.getBeforeClose || !target.getContextActive || !target.getHasDeadline || !target.closed {
				t.Fatalf("snapshot and close sequence: calls=%d beforeClose=%t active=%t deadline=%t closed=%t", target.getCalls, target.getBeforeClose, target.getContextActive, target.getHasDeadline, target.closed)
			}
			assertSubagentTimeoutSnapshot(t, response.Error, secret)
		})
	}
}

func assertSubagentTimeoutSnapshot(t *testing.T, envelope *mcpfactorysession.ToolErrorEnvelope, secret string) {
	t.Helper()
	progress, ok := envelope.Details["progress"].(map[string]any)
	if !ok || progress["available"] != true || progress["inFlightDispatches"] != 2 || progress["scriptStatus"] != "RUNNING" {
		t.Fatalf("progress = %#v", envelope.Details["progress"])
	}
	if got := progress["workCounts"]; !reflect.DeepEqual(got, map[string]any{"initial": 1, "processing": 2, "terminal": 3, "failed": 4}) {
		t.Fatalf("work counts = %#v", got)
	}
	if got := progress["childDispatchCounts"]; !reflect.DeepEqual(got, map[string]any{"completed": 5, "queued": 6, "running": 7}) {
		t.Fatalf("child dispatch counts = %#v", got)
	}
	for _, key := range []string{"runtimeState", "factoryState", "lifecycleControlStatus", "phase"} {
		if _, ok := progress[key]; ok {
			t.Fatalf("unsafe progress key %q in %#v", key, progress)
		}
	}
	if strings.Contains(fmt.Sprint(envelope), secret) {
		t.Fatalf("timeout leaked projection text: %#v", envelope)
	}
	assertSubagentClosedDiagnostic(t, envelope)
}

func TestSubagentTimeoutSnapshotFailureStillCloses(t *testing.T) {
	target := &subagentTargetFake{
		invokeResult:  &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut},
		projectionErr: errors.New("sensitive projection failure"),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-snapshot-error" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || !target.closed || target.getCalls != 1 {
		t.Fatalf("snapshot failure response = %#v, target = %#v", response, target)
	}
	if got := response.Error.Details["progress"]; !reflect.DeepEqual(got, map[string]any{"available": false}) {
		t.Fatalf("progress = %#v", got)
	}
	if strings.Contains(fmt.Sprint(response.Error), "sensitive projection failure") {
		t.Fatalf("snapshot failure leaked read error: %#v", response.Error)
	}
}

func TestSubagentTimeoutSnapshotRejectsUnknownScriptStatus(t *testing.T) {
	secret := "sensitive provider output"
	target := &subagentTargetFake{
		invokeResult: &factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusTimedOut},
		projection: factorysessions.SessionProjection{Runtime: factorysessions.RuntimeProjection{
			JavaScript: &factorysessions.JavaScriptRuntimeProjection{ScriptStatus: factorydefinitions.FactorySessionJavaScriptScriptStatus(secret)},
		}},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-unknown-status" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file"})
	progress, ok := response.Error.Details["progress"].(map[string]any)
	if !ok || progress["available"] != true {
		t.Fatalf("progress = %#v", response.Error.Details["progress"])
	}
	if _, ok := progress["scriptStatus"]; ok || strings.Contains(fmt.Sprint(response.Error), secret) {
		t.Fatalf("unknown script status leaked: %#v", response.Error)
	}
}

func TestSubagentTimeoutCloseFailureDoesNotClaimClosedSession(t *testing.T) {
	target := &subagentTargetFake{
		invokeErr: fmt.Errorf("private: %w", context.DeadlineExceeded),
		closeErr:  errors.New("private close failure"),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-close-failure" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.cleanup_failed" || target.getCalls != 1 || !target.getBeforeClose {
		t.Fatalf("close failure response = %#v, target = %#v", response, target)
	}
	if _, ok := response.Error.Details["sessionClosed"]; ok {
		t.Fatalf("failed close claimed session closed: %#v", response.Error)
	}
	if strings.Contains(fmt.Sprint(response.Error), "private") {
		t.Fatalf("close failure leaked error: %#v", response.Error)
	}
}

func TestSubagentTimeoutReportsExplicitProviderAndModel(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusTimedOut,
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Edit one file", Provider: "opencode", Model: "local-model",
	})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Details["provider"] != "opencode" || response.Error.Details["model"] != "local-model" {
		t.Fatalf("selected provider and model = %#v", response.Error.Details)
	}
	assertSubagentTimeoutSuggestedAction(t, response.Error.Details["suggestedAction"])
	assertSubagentClosedDiagnostic(t, response.Error)
}

func assertSubagentTimeoutSuggestedAction(t *testing.T, value any) {
	t.Helper()
	if action, ok := value.(string); !ok || !strings.Contains(action, "Inspect the workspace for partial edits") || !strings.Contains(action, "provider logs") || !strings.Contains(action, "requestId") || !strings.Contains(action, "If you retry, use another configured model or a longer timeout.") || strings.Contains(action, "before retrying. Then retry") {
		t.Fatalf("timeout suggested action = %#v", value)
	}
}

func assertSubagentClosedDiagnostic(t *testing.T, envelope *mcpfactorysession.ToolErrorEnvelope) {
	t.Helper()
	if envelope.Details["sessionClosed"] != true || !strings.Contains(envelope.Message, "you.subagent cleanup closed the live Factory Session") {
		t.Fatalf("closed-session diagnostic = %#v", envelope)
	}
	purpose, ok := envelope.Details["sessionIdPurpose"].(string)
	if !ok || !strings.Contains(purpose, "log correlation") || !strings.Contains(purpose, "you.factory_session.get may return session.not_found") {
		t.Fatalf("sessionId purpose = %#v", envelope.Details["sessionIdPurpose"])
	}
	action, ok := envelope.Details["suggestedAction"].(string)
	if !ok || !strings.Contains(action, "workspace") || !strings.Contains(action, "provider logs") || strings.Contains(action, "Inspect the Factory Session") {
		t.Fatalf("closed-session suggested action = %#v", envelope.Details["suggestedAction"])
	}
}

func TestSubagentReportsEmptyResultWhenCompletedWithoutText(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:  factorysessions.InvocationTerminalStatusCompleted,
		Message: "sensitive provider output",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-empty" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Summarize this"})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.empty_result" || response.Error.Retryable {
		t.Fatalf("empty result error = %#v", response.Error)
	}
	if response.Error.SessionID != "session-1" {
		t.Fatalf("empty result session = %q", response.Error.SessionID)
	}
	if !strings.Contains(response.Error.Message, "without returning") {
		t.Fatalf("empty result message = %q", response.Error.Message)
	}
	assertSubagentClosedDiagnostic(t, response.Error)
	if strings.Contains(fmt.Sprint(response.Error), "sensitive provider output") {
		t.Fatalf("empty result leaked provider text: %#v", response.Error)
	}
	if !target.closed {
		t.Fatal("Factory Session was not closed after empty result")
	}
}

func TestSubagentWaitTimeoutAndCancellation(t *testing.T) {
	tenMinutes := int64(600_000)
	twentyMinutes := int64(1_200_000)
	for _, test := range []struct {
		name          string
		timeoutMillis *int64
		wantMillis    int64
	}{
		{name: "omitted", wantMillis: twentyMinutes},
		{name: "explicit ten minutes", timeoutMillis: &tenMinutes, wantMillis: tenMinutes},
		{name: "explicit twenty minutes", timeoutMillis: &twentyMinutes, wantMillis: twentyMinutes},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := &subagentTargetFake{}
			response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-cancel" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
				Prompt: "Run this", TimeoutMillis: test.timeoutMillis,
			})
			if response.Error != nil || response.Result == nil {
				t.Fatalf("Subagent response = %#v", response)
			}
			if !target.invoke.Wait.CancelOnTimeout {
				t.Fatalf("CancelOnTimeout not forwarded: Wait = %#v", target.invoke.Wait)
			}
			if target.invoke.Wait.TimeoutMillis != test.wantMillis {
				t.Fatalf("TimeoutMillis = %d, want %d", target.invoke.Wait.TimeoutMillis, test.wantMillis)
			}
		})
	}
}

func TestSubagentInvokeErrorIsPrivateAndIncludesSessionID(t *testing.T) {
	target := &subagentTargetFake{invokeErr: errors.New("sensitive provider token abc123")}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-invoke-err" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.invocation_failed" {
		t.Fatalf("invoke error code = %q", response.Error.Code)
	}
	if response.Error.SessionID != "session-1" {
		t.Fatalf("invoke error session = %q", response.Error.SessionID)
	}
	if response.Error.Details["requestId"] != "request-invoke-err" || response.Error.Details["partialEffectsPossible"] != true {
		t.Fatalf("invoke error details = %#v", response.Error.Details)
	}
	assertSubagentClosedDiagnostic(t, response.Error)
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(response.Error.Message, "abc123") {
		t.Fatalf("invoke error leaked raw text: %q", response.Error.Message)
	}
	if !target.closed {
		t.Fatal("Factory Session was not closed after invoke error")
	}
}

func TestSubagentInvokeDeadlineExceededReportsPossibleWorkspaceEdits(t *testing.T) {
	timeout := int64(45_000)
	target := &subagentTargetFake{invokeErr: fmt.Errorf("sensitive provider token abc123: %w", context.DeadlineExceeded)}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-deadline" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{
		Prompt: "Edit a file", Provider: "opencode", Model: "local-model", TimeoutMillis: &timeout,
	})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable {
		t.Fatalf("deadline error = %#v", response.Error)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["requestId"] != "request-deadline" || response.Error.Details["partialEffectsPossible"] != true {
		t.Fatalf("deadline diagnostic = %#v", response.Error)
	}
	if response.Error.Details["provider"] != "opencode" || response.Error.Details["model"] != "local-model" {
		t.Fatalf("deadline provider/model = %#v", response.Error.Details)
	}
	if response.Error.Details["timeoutMillis"] != timeout {
		t.Fatalf("deadline timeoutMillis = %#v, want %d", response.Error.Details["timeoutMillis"], timeout)
	}
	assertSubagentTimeoutSuggestedAction(t, response.Error.Details["suggestedAction"])
	assertSubagentClosedDiagnostic(t, response.Error)
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(response.Error.Message, "abc123") || !target.closed {
		t.Fatalf("deadline error leaked raw text or skipped close: %#v", response.Error)
	}
}

func TestSubagentCloseErrorIsPrivateAndIncludesSessionID(t *testing.T) {
	target := &subagentTargetFake{
		invokeErr: errors.New("sensitive provider token abc123"),
		closeErr:  errors.New("sensitive close failure xyz789"),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-close-err" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.cleanup_failed" {
		t.Fatalf("close error code = %q", response.Error.Code)
	}
	if response.Error.SessionID != "session-1" {
		t.Fatalf("close error session = %q", response.Error.SessionID)
	}
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(response.Error.Message, "xyz789") || strings.Contains(response.Error.Message, "abc123") {
		t.Fatalf("close error leaked raw text: %q", response.Error.Message)
	}
	if response.Error.Details["requestId"] != "request-close-err" || response.Error.Details["partialEffectsPossible"] != true {
		t.Fatalf("close error details = %#v", response.Error.Details)
	}
	if _, ok := response.Error.Details["sessionClosed"]; ok {
		t.Fatalf("failed cleanup claimed session closed: %#v", response.Error.Details)
	}
}

func TestSubagentCloseDeadlineExceededReportsCleanupTimeout(t *testing.T) {
	target := &subagentTargetFake{
		invokeErr: errors.New("sensitive provider token abc123"),
		closeErr:  fmt.Errorf("sensitive close failure xyz789: %w", context.DeadlineExceeded),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-close-timeout" }, testSubagentProviderIdentity, mcpfactorysession.SubagentInput{Prompt: "Edit a file"})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.cleanup_timed_out" {
		t.Fatalf("close timeout error code = %q", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatalf("close timeout error is retryable: %#v", response.Error)
	}
	if response.Error.SessionID != "session-1" {
		t.Fatalf("close timeout error session = %q", response.Error.SessionID)
	}
	if response.Error.Details["requestId"] != "request-close-timeout" || response.Error.Details["partialEffectsPossible"] != true {
		t.Fatalf("close timeout error details = %#v", response.Error.Details)
	}
	if _, ok := response.Error.Details["sessionClosed"]; ok {
		t.Fatalf("timed-out cleanup claimed session closed: %#v", response.Error.Details)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok || strings.TrimSpace(action) == "" {
		t.Fatalf("close timeout suggested action = %#v", response.Error.Details["suggestedAction"])
	}
	leaked := response.Error.Message + fmt.Sprint(response.Error.Details)
	for _, secret := range []string{"sensitive", "xyz789", "abc123"} {
		if strings.Contains(leaked, secret) {
			t.Fatalf("close timeout error leaked %q: %#v", secret, response.Error)
		}
	}
}

// This adapter witness controls only Factory admission. Live owner authority and
// selected-host RUN are separate public functional obligations.
type callerSubagentTarget struct {
	*subagentTargetFake
	admitted workersessions.CallerIdentity
}

func (target *callerSubagentTarget) Start(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	target.admitted = *request.Caller.Clone()
	result, err := target.subagentTargetFake.Start(ctx, request)
	request.Caller.WorkerSessionID = "changed-by-start"
	request.Caller.Token = "changed-by-start"
	return result, err
}

func TestSubagentForwardsDetachedCallerToBothAdmissions(t *testing.T) {
	t.Parallel()
	caller := &workersessions.CallerIdentity{WorkerSessionID: "exact-caller", Token: "planted-caller-token"}
	want := *caller
	target := &callerSubagentTarget{subagentTargetFake: &subagentTargetFake{}}
	input := mcpfactorysession.SubagentInput{Prompt: "Explain the result", Provider: "controlled", Caller: caller}
	resolve := func(_ context.Context, identity string) (string, error) {
		caller.WorkerSessionID = "changed-by-resolver"
		caller.Token = "changed-by-resolver"
		return identity, nil
	}
	response := mcpfactorysession.Subagent(t.Context(), target, "workspace", func() string { return "caller-request" }, resolve, input)
	if response.Error != nil || response.Result == nil || !target.closed {
		t.Fatalf("response = %#v; closed = %v", response, target.closed)
	}
	if target.admitted != want || target.invoke.Caller == nil || *target.invoke.Caller != want {
		t.Fatal("start or invoke did not retain the exact entry caller")
	}
	if target.invoke.Caller == caller || target.invoke.Caller == target.start.Caller {
		t.Fatal("caller credential pointers are shared between admissions")
	}
	assertSubagentCallerPrivate(t, want, input, target.start, target.invoke, response)
}

func assertSubagentCallerPrivate(t *testing.T, caller workersessions.CallerIdentity, values ...any) {
	t.Helper()
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), caller.Token) || strings.Contains(string(raw), caller.WorkerSessionID) {
			t.Fatal("execution-only caller entered serialized MCP or Factory values")
		}
	}
}

func TestSubagentCallerRefusalIsTypedPrivateAndClosesAdmittedSession(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"start", "invoke", "start-secret-error"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			caller := &workersessions.CallerIdentity{WorkerSessionID: "refused-caller", Token: "refused-caller-token"}
			target := &subagentTargetFake{}
			refusal := fmt.Errorf("%s: %w", caller.Token, workersessions.ErrCallerInvalid)
			wantCode := "WORKER_SESSION_CALLER_INVALID"
			switch phase {
			case "start":
				target.startErr = refusal
			case "invoke":
				target.invokeErr = refusal
			case "start-secret-error":
				target.startErr = fmt.Errorf("external failure: %s", caller.Token)
				wantCode = "factory_session.execution.internal"
			}
			response := mcpfactorysession.Subagent(t.Context(), target, "workspace", func() string { return "refusal-request" }, testSubagentProviderIdentity,
				mcpfactorysession.SubagentInput{Prompt: "Explain the result", Caller: caller})
			if response.Error == nil || response.Result != nil || response.Error.Code != wantCode || response.Error.Retryable {
				t.Fatalf("response = %#v", response)
			}
			if target.closed != (phase == "invoke") || (phase != "invoke" && target.invoke.SessionID != "") {
				t.Fatal("refusal launched invocation or failed to close the admitted session")
			}
			assertSubagentCallerPrivate(t, *caller, response, target.start, target.invoke)
		})
	}
}

func TestSubagentArgumentsCannotSupplyCallerAuthority(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"caller", "Caller", "workerSessionId", "token"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			raw, _ := json.Marshal(map[string]any{"prompt": "Explain", key: "untrusted"})
			response, err := mcpfactorysession.ValidateSubagentArguments(raw)
			if err != nil || response == nil || !strings.Contains(string(response), `"code":"BAD_REQUEST"`) {
				t.Fatalf("authority argument response = %s, %v", response, err)
			}
		})
	}
}
