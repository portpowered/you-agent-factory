package factorysession_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type subagentTargetFake struct {
	factorysessions.Service
	start                 factorysessions.SessionStartRequest
	invoke                factorysessions.SessionInvokeRequest
	control               factorysessions.SessionControlRequest
	started               bool
	closed                bool
	controlHasDeadline    bool
	controlTimeToDeadline time.Duration
	invokeErr             error
	closeErr              error
	invokeResult          *factorysessions.InvocationResult
}

func (fake *subagentTargetFake) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	fake.start = request
	fake.started = true
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

func TestSubagentRunsPackagedFactoryWithDefaultsAndReturnsText(t *testing.T) {
	target := &subagentTargetFake{}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-1" }, mcpfactorysession.SubagentInput{Prompt: "Summarize this"})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("Subagent response = %#v", response)
	}
	if response.Result.Text != "subagent answer" || response.Result.SessionID != "session-1" {
		t.Fatalf("Subagent result = %#v", response.Result)
	}
	assertSubagentStart(t, target)
	assertSubagentInvocationAndClose(t, target)
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
	response := mcpfactorysession.Subagent(context.Background(), target, "server-root", func() string { return "request-3" }, mcpfactorysession.SubagentInput{
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
	response := mcpfactorysession.Subagent(context.Background(), target, "", func() string { return "request-2" }, mcpfactorysession.SubagentInput{
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
}

func TestSubagentSurfacesSafeProviderThrottleFailure(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusFailed,
		ErrorCode:     "INVOCATION_RUNTIME_FAILURE",
		Message:       "sensitive ACP session/prompt output with token secret",
		FailureReason: "throttled",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-throttled" }, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.provider_throttled" || !response.Error.Retryable {
		t.Fatalf("throttled response = %#v", response)
	}
	if response.Error.SessionID != "session-1" || response.Error.Details["failureReason"] != "throttled" {
		t.Fatalf("throttled diagnostic = %#v", response.Error)
	}
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-misconfigured" }, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-pi-misconfigured" }, mcpfactorysession.SubagentInput{Prompt: "Read README", Provider: "pi"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-executable-missing" }, mcpfactorysession.SubagentInput{Prompt: "Edit a file", Provider: "opencode"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, mcpfactorysession.SubagentInput{
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
	if !strings.Contains(response.Error.Message, "workspace edits may have occurred") || strings.Contains(response.Error.Message, "private") || !target.closed {
		t.Fatalf("timeout message or cleanup = %#v", response.Error)
	}
}

func TestSubagentTimeoutReportsExplicitProviderAndModel(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusTimedOut,
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, mcpfactorysession.SubagentInput{
		Prompt: "Edit one file", Provider: "opencode", Model: "local-model",
	})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Details["provider"] != "opencode" || response.Error.Details["model"] != "local-model" {
		t.Fatalf("selected provider and model = %#v", response.Error.Details)
	}
	assertSubagentTimeoutSuggestedAction(t, response.Error.Details["suggestedAction"])
}

func assertSubagentTimeoutSuggestedAction(t *testing.T, value any) {
	t.Helper()
	if action, ok := value.(string); !ok || !strings.Contains(action, "Inspect the workspace for partial edits") || !strings.Contains(action, "another configured model or a longer timeout") {
		t.Fatalf("timeout suggested action = %#v", value)
	}
}

func TestSubagentReportsEmptyResultWhenCompletedWithoutText(t *testing.T) {
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-empty" }, mcpfactorysession.SubagentInput{Prompt: "Summarize this"})
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
	if !target.closed {
		t.Fatal("Factory Session was not closed after empty result")
	}
}

func TestSubagentForwardsCancelOnTimeout(t *testing.T) {
	target := &subagentTargetFake{}
	timeout := int64(30_000)
	mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-cancel" }, mcpfactorysession.SubagentInput{
		Prompt: "Run this", TimeoutMillis: &timeout,
	})
	if !target.invoke.Wait.CancelOnTimeout {
		t.Fatalf("CancelOnTimeout not forwarded: Wait = %#v", target.invoke.Wait)
	}
	if target.invoke.Wait.TimeoutMillis != timeout {
		t.Fatalf("TimeoutMillis = %d, want %d", target.invoke.Wait.TimeoutMillis, timeout)
	}
}

func TestSubagentInvokeErrorIsPrivateAndIncludesSessionID(t *testing.T) {
	target := &subagentTargetFake{invokeErr: errors.New("sensitive provider token abc123")}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-invoke-err" }, mcpfactorysession.SubagentInput{Prompt: "Do work"})
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
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-deadline" }, mcpfactorysession.SubagentInput{
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
	if strings.Contains(response.Error.Message, "sensitive") || strings.Contains(response.Error.Message, "abc123") || !target.closed {
		t.Fatalf("deadline error leaked raw text or skipped close: %#v", response.Error)
	}
}

func TestSubagentCloseErrorIsPrivateAndIncludesSessionID(t *testing.T) {
	target := &subagentTargetFake{
		invokeErr: errors.New("sensitive provider token abc123"),
		closeErr:  errors.New("sensitive close failure xyz789"),
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-close-err" }, mcpfactorysession.SubagentInput{Prompt: "Do work"})
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
}
