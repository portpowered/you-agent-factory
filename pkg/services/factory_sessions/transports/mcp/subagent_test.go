package factorysession_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type subagentTargetFake struct {
	factorysessions.TargetExecutionService
	start        factorysessions.StartRequest
	invoke       factorysessions.InvocationRequest
	started      bool
	closed       bool
	invokeErr    error
	invokeResult *factorysessions.InvocationResult
	wait         <-chan struct{}
}

func (fake *subagentTargetFake) StartAsync(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	fake.start = request
	fake.started = true
	return factorysessions.AsyncStartResult{SessionID: "session-1", Status: "RUNNING"}, nil
}

func (fake *subagentTargetFake) InvokeFactorySession(ctx context.Context, _ string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	fake.invoke = request
	if fake.wait != nil {
		select {
		case <-fake.wait:
		case <-ctx.Done():
			return factorysessions.InvocationResult{}, ctx.Err()
		}
	}
	if fake.invokeResult != nil {
		return *fake.invokeResult, fake.invokeErr
	}
	if fake.invokeErr != nil {
		return factorysessions.InvocationResult{}, fake.invokeErr
	}
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "subagent answer"}},
	}, nil
}

func TestSubagentWaitsForLongInvocationAndForwardsOneHourTimeout(t *testing.T) {
	release := make(chan struct{})
	target := &subagentTargetFake{wait: release}
	timeoutMillis := int64(3_600_000)
	finished := make(chan mcpfactorysession.ToolResponse[mcpfactorysession.SubagentResult], 1)
	go func() {
		finished <- mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-long" }, mcpfactorysession.SubagentInput{Prompt: "Wait for the result", TimeoutMillis: &timeoutMillis})
	}()
	select {
	case response := <-finished:
		t.Fatalf("subagent returned before provider completion: %#v", response)
	case <-time.After(1500 * time.Millisecond):
	}
	close(release)
	select {
	case response := <-finished:
		if response.Error != nil || response.Result == nil || response.Result.Status != string(factorysessions.InvocationTerminalStatusCompleted) {
			t.Fatalf("subagent response = %#v", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subagent did not return after provider completion")
	}
	if target.invoke.TimeoutMillis == nil || *target.invoke.TimeoutMillis != timeoutMillis || !target.closed {
		t.Fatalf("invocation timeout = %#v, closed = %t", target.invoke.TimeoutMillis, target.closed)
	}
}

func (fake *subagentTargetFake) CloseFactorySession(_ context.Context, sessionID string) error {
	fake.closed = sessionID == "session-1"
	return nil
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
	if target.start.Source.Kind != factoryruntime.WorkflowSourceKindFactoryID || target.start.Source.FactoryID != factorydefinitions.PackagedSubagentFactoryName {
		t.Fatalf("start source = %#v", target.start.Source)
	}
	if target.start.Args["workingRoot"] != "C:/project" {
		t.Fatalf("workingRoot = %#v", target.start.Args)
	}
	if got := *target.invoke.Args; len(got) != 1 || got["input"] != "Summarize this" {
		t.Fatalf("default invocation args = %#v", got)
	}
	if !target.started || !target.closed {
		t.Fatalf("lifecycle start=%t close=%t", target.started, target.closed)
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
	args := *target.invoke.Args
	if args["workerProvider"] != "opencode" || args["workerModel"] != "local-model" || args["workerReasoningEffort"] != "high" {
		t.Fatalf("override args = %#v", args)
	}
	if !target.closed {
		t.Fatal("Factory Session was not closed after invocation failure")
	}
}

func TestSubagentTimeoutReportsPossibleWorkspaceEffects(t *testing.T) {
	timeout := int64(180000)
	target := &subagentTargetFake{invokeResult: &factorysessions.InvocationResult{
		Status:    factorysessions.InvocationTerminalStatusTimedOut,
		ErrorCode: string(factorysessions.InvocationErrorCodeTimedOut),
		Message:   "sensitive provider transcript",
	}}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout" }, mcpfactorysession.SubagentInput{
		Prompt: "Edit a file", TimeoutMillis: &timeout,
	})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.timed_out" || response.Error.Retryable || response.Error.SessionID != "session-1" {
		t.Fatalf("timeout envelope = %#v", response.Error)
	}
	if response.Error.Details["partialEffectsPossible"] != true || response.Error.Details["timeoutMillis"] != timeout || response.Error.Details["invocationCode"] != string(factorysessions.InvocationErrorCodeTimedOut) {
		t.Fatalf("timeout details = %#v", response.Error.Details)
	}
	if strings.Contains(response.Error.Message, "sensitive") || !target.closed {
		t.Fatalf("timeout leaked provider text or skipped close: %#v", response.Error)
	}
}

func TestSubagentTimeoutResultWithInvocationErrorKeepsTimeoutEnvelope(t *testing.T) {
	timeout := int64(5000)
	target := &subagentTargetFake{
		invokeErr: context.DeadlineExceeded,
		invokeResult: &factorysessions.InvocationResult{
			Status:    factorysessions.InvocationTerminalStatusTimedOut,
			ErrorCode: string(factorysessions.InvocationErrorCodeTimedOut),
			Message:   "private provider output",
		},
	}
	response := mcpfactorysession.Subagent(context.Background(), target, "C:/project", func() string { return "request-timeout-error" }, mcpfactorysession.SubagentInput{
		Prompt: "Edit a file", TimeoutMillis: &timeout,
	})
	if response.Result != nil || response.Error == nil {
		t.Fatalf("timeout response = %#v", response)
	}
	if response.Error.Code != "factory_session.subagent.timed_out" || response.Error.SessionID != "session-1" || response.Error.Retryable {
		t.Fatalf("timeout envelope = %#v", response.Error)
	}
	if response.Error.Details["partialEffectsPossible"] != true || response.Error.Details["timeoutMillis"] != timeout {
		t.Fatalf("timeout details = %#v", response.Error.Details)
	}
	if strings.Contains(response.Error.Message, "private") || !target.closed {
		t.Fatalf("timeout leaked provider text or skipped close: %#v", response.Error)
	}
}
