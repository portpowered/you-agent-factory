package factorysession

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type subagentTargetFake struct {
	factorysessions.TargetExecutionService
	start     factorysessions.StartRequest
	invoke    factorysessions.InvocationRequest
	started   bool
	closed    bool
	invokeErr error
}

func (fake *subagentTargetFake) StartAsync(_ context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	fake.start = request
	fake.started = true
	return factorysessions.AsyncStartResult{SessionID: "session-1", Status: "RUNNING"}, nil
}

func (fake *subagentTargetFake) InvokeFactorySession(_ context.Context, _ string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	fake.invoke = request
	if fake.invokeErr != nil {
		return factorysessions.InvocationResult{}, fake.invokeErr
	}
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "subagent answer"}},
	}, nil
}

func (fake *subagentTargetFake) CloseFactorySession(_ context.Context, sessionID string) error {
	fake.closed = sessionID == "session-1"
	return nil
}

func TestSubagentRunsPackagedFactoryWithDefaultsAndReturnsText(t *testing.T) {
	target := &subagentTargetFake{}
	response := Subagent(context.Background(), target, "C:/project", SubagentInput{Prompt: "Summarize this"})
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
	response := Subagent(context.Background(), target, "", SubagentInput{
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
