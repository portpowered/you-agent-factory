package acp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const acpHelperEnvironment = "YOU_TEST_ACP_AGENT_HELPER"

func TestFactoryRunRoutesExecutorProviderThroughACPAdapter(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP vertical slice"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var processStarts atomic.Int32
	fallback := &legacyProvider{err: errors.New("legacy provider route was unexpectedly invoked")}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want 1", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("legacy provider calls = %d, want 0; ACP must be selected by executorProvider", got)
	}
	assertACPProviderSession(t, events)
}

func TestFactoryRunProjectsOperatorConfiguredACPIntegrationIntoInvocationCatalog(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"configured ACP provider"}`))
	writeACPWorker(t, dir, "custom-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var processStarts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithConfiguredHome(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second, func(home string) {
		configDir := filepath.Join(home, ".you-agent-factory")
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			t.Fatalf("create operator config directory: %v", err)
		}
		config := []byte(`{"workers":{"acp":{"integrations":[{"id":"entry-1","name":"custom-acp","transport":"stdio","command":"custom-agent acp"}]}}}`)
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
			t.Fatalf("write operator config: %v", err)
		}
	})

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 1 {
		t.Fatalf("configured ACP process starts = %d, want 1", got)
	}
	assertProviderSession(t, events, "custom-acp")
}

func assertACPProviderSession(t *testing.T, events []factoryapi.FactoryEvent) {
	assertProviderSession(t, events, "cursor-acp")
}

func assertProviderSession(t *testing.T, events []factoryapi.FactoryEvent, provider string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil || payload.ProviderSession.Id == nil {
			continue
		}
		if *payload.ProviderSession.Provider != provider || *payload.ProviderSession.Id != "acp-session-functional-1" {
			t.Fatalf("Provider Session = %#v, want %s/acp-session-functional-1", payload.ProviderSession, provider)
		}
		return
	}
	t.Fatal("Factory events omitted the ACP Provider Session reference")
}

func TestRootConstructionDoesNotStartACPProcess(t *testing.T) {
	var processStarts atomic.Int32
	_ = support.BuildProcess(t, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
	})
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts during root construction = %d, want 0", got)
	}
}

func TestUnknownExecutorProviderFailsBeforeACPProcessStart(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"unknown ACP provider"}`))
	writeACPWorker(t, dir, "missing-acp")

	var processStarts atomic.Int32
	fallback := &legacyProvider{response: workers.InferenceResponse{Content: "legacy COMPLETE"}}
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts for unknown provider = %d, want 0", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("legacy provider calls for unknown ACP provider = %d, want 0", got)
	}
}

func TestScriptWrapExecutorProviderRetainsLegacyProviderRoute(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"script wrap compatibility"}`))
	writeACPWorker(t, dir, "SCRIPT_WRAP")

	var processStarts atomic.Int32
	fallback := &legacyProvider{response: workers.InferenceResponse{Content: "legacy route COMPLETE"}}
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&processStarts),
		ProviderOverride:              fallback,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := fallback.calls.Load(); got != 1 {
		t.Fatalf("legacy provider calls = %d, want 1", got)
	}
	if got := processStarts.Load(); got != 0 {
		t.Fatalf("ACP process starts for SCRIPT_WRAP = %d, want 0", got)
	}
}

func writeACPWorker(t *testing.T, factoryDir, providerID string) {
	t.Helper()
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: " + providerID + "\n" +
		"model: test-model\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nTest ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ACP worker: %v", err)
	}
}

func acpHelperCommandFactory(starts *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		if (name == "cursor-agent" || name == "custom-agent") && len(args) == 1 && args[0] == "acp" {
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPAgentHelperProcess$")
		}
		return exec.Command(name, args...)
	}
}

type legacyProvider struct {
	calls    atomic.Int32
	response workers.InferenceResponse
	err      error
}

type availableExecutableLocator struct{}

func (availableExecutableLocator) LookPath(file string) (string, error) { return file, nil }

func (p *legacyProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	p.calls.Add(1)
	return p.response, p.err
}

func TestACPAgentHelperProcess(t *testing.T) {
	mode := os.Getenv(acpHelperEnvironment)
	if mode != "1" && mode != "fail" && mode != "auth" && mode != "model" && mode != "resource" && mode != "version" && mode != "init-fail" && mode != "malformed" && mode != "eof" {
		return
	}
	if mode == "malformed" {
		_, _ = fmt.Fprintln(os.Stdout, "{not-json")
		os.Exit(0)
		return
	}
	if mode == "eof" {
		os.Exit(0)
		return
	}
	agent := &functionalAgent{
		failPrompt: mode == "fail", requireAuth: mode == "auth", requireModel: mode == "model",
		requireResource: mode == "resource", incompatibleVersion: mode == "version", failInitialize: mode == "init-fail",
	}
	connection := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.connection = connection
	<-connection.Done()
}

type functionalAgent struct {
	connection          *acpsdk.AgentSideConnection
	failPrompt          bool
	requireAuth         bool
	requireModel        bool
	requireResource     bool
	model               string
	incompatibleVersion bool
	failInitialize      bool
}

func (a *functionalAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	if a.failInitialize {
		return acpsdk.InitializeResponse{}, errors.New("functional ACP initialization failure")
	}
	methods := []acpsdk.AuthMethod{}
	if a.requireAuth {
		methods = append(methods, acpsdk.AuthMethod{Agent: &acpsdk.AuthMethodAgent{Id: "login", Name: "Agent login"}})
	}
	var version acpsdk.ProtocolVersion = acpsdk.ProtocolVersionNumber
	if a.incompatibleVersion {
		version = acpsdk.ProtocolVersion(999)
	}
	return acpsdk.InitializeResponse{
		ProtocolVersion:   version,
		AgentCapabilities: acpsdk.AgentCapabilities{},
		AuthMethods:       methods,
	}, nil
}

func (a *functionalAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	if a.requireAuth {
		return acpsdk.NewSessionResponse{}, acpsdk.NewAuthRequired(nil)
	}
	response := acpsdk.NewSessionResponse{SessionId: "acp-session-functional-1"}
	if a.requireModel {
		category := acpsdk.SessionConfigOptionCategoryModel
		options := acpsdk.SessionConfigSelectOptionsUngrouped{{Name: "Test model", Value: "test-model"}}
		response.ConfigOptions = []acpsdk.SessionConfigOption{{Select: &acpsdk.SessionConfigOptionSelect{
			Type: "select", Id: "model", Name: "Model", Category: &category, CurrentValue: "default",
			Options: acpsdk.SessionConfigSelectOptions{Ungrouped: &options},
		}}}
	}
	return response, nil
}

func (a *functionalAgent) Prompt(ctx context.Context, request acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	if a.requireResource {
		found := false
		for _, block := range request.Prompt {
			if block.ResourceLink != nil && block.ResourceLink.Uri == "https://example.test/fixture.png" && block.ResourceLink.MimeType != nil && *block.ResourceLink.MimeType == "image/png" {
				found = true
			}
		}
		if !found {
			return acpsdk.PromptResponse{}, errors.New("ACP prompt omitted canonical resource link")
		}
	}
	if a.requireModel && a.model != "test-model" {
		return acpsdk.PromptResponse{}, errors.New("advertised model was not applied")
	}
	if a.failPrompt {
		if err := a.connection.SessionUpdate(ctx, acpsdk.SessionNotification{SessionId: request.SessionId, Update: acpsdk.UpdateAgentMessageText("partial ACP answer")}); err != nil {
			return acpsdk.PromptResponse{}, err
		}
		return acpsdk.PromptResponse{}, errors.New("functional ACP prompt failure")
	}
	updates := []acpsdk.SessionUpdate{
		acpsdk.UpdateAgentMessageText("ACP root "),
		acpsdk.UpdateAgentThoughtText("checking the Factory state"),
		{ToolCall: &acpsdk.SessionUpdateToolCall{
			SessionUpdate: "tool_call", ToolCallId: "tool-1", Title: "Inspect Factory",
			Status: acpsdk.ToolCallStatusInProgress, RawInput: map[string]any{"scope": "factory"},
		}},
		{Plan: &acpsdk.SessionUpdatePlan{SessionUpdate: "plan", Entries: []acpsdk.PlanEntry{
			{Content: "Complete the ACP turn", Priority: acpsdk.PlanEntryPriorityHigh, Status: acpsdk.PlanEntryStatusInProgress},
		}}},
		{UsageUpdate: &acpsdk.SessionUsageUpdate{SessionUpdate: "usage_update", Used: 12, Size: 4096}},
		acpsdk.UpdateAgentMessageText("execution COMPLETE"),
		{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			SessionUpdate: "tool_call_update", ToolCallId: "tool-1",
			Title: stringPointer("Inspect Factory"), Status: toolStatusPointer(acpsdk.ToolCallStatusCompleted),
			RawOutput: map[string]any{"ok": true}, Content: []acpsdk.ToolCallContent{{Diff: &acpsdk.ToolCallContentDiff{
				Type: "diff", Path: "factory/result.txt", NewText: "complete\n",
			}}},
		}},
	}
	for _, update := range updates {
		if err := a.connection.SessionUpdate(ctx, acpsdk.SessionNotification{SessionId: request.SessionId, Update: update}); err != nil {
			return acpsdk.PromptResponse{}, err
		}
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func stringPointer(value string) *string { return &value }

func toolStatusPointer(value acpsdk.ToolCallStatus) *acpsdk.ToolCallStatus { return &value }

func (*functionalAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}
func (*functionalAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}
func (*functionalAgent) Cancel(context.Context, acpsdk.CancelNotification) error { return nil }
func (*functionalAgent) CloseSession(context.Context, acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	return acpsdk.CloseSessionResponse{}, nil
}
func (*functionalAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}
func (*functionalAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}
func (a *functionalAgent) SetSessionConfigOption(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	if request.ValueId != nil {
		a.model = string(request.ValueId.Value)
	}
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}
func (*functionalAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

var _ acpsdk.Agent = (*functionalAgent)(nil)
