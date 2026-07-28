package acp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

const cancellationHelperEnvironment = "YOU_TEST_ACP_CANCELLATION_HELPER"

func TestPromptBlocksMapsInstructionsAndTextWithoutCustomProtocolEncoding(t *testing.T) {
	t.Parallel()

	blocks, err := promptBlocks(providers.ExecuteRequest{
		Instructions: "follow the factory contract",
		Prompt: []providers.ContentPart{
			{Kind: providers.ContentKindText, Text: "first"},
			{Kind: providers.ContentKindText, Text: "second"},
		},
	})
	if err != nil {
		t.Fatalf("promptBlocks() error = %v", err)
	}
	want := []string{"System instructions:\nfollow the factory contract", "first", "second"}
	if len(blocks) != len(want) {
		t.Fatalf("prompt block count = %d, want %d", len(blocks), len(want))
	}
	for index, block := range blocks {
		if block.Text == nil || block.Text.Text != want[index] {
			t.Fatalf("prompt block %d = %#v, want SDK text block %q", index, block, want[index])
		}
	}
}

func TestPromptBlocksMapsDetachedResourceLinksThroughSDKTypes(t *testing.T) {
	t.Parallel()
	blocks, err := promptBlocks(providers.ExecuteRequest{Prompt: []providers.ContentPart{
		{Kind: providers.ContentKindText, Text: "inspect"},
		{Kind: providers.ContentKindResourceLink, Name: "design", URI: "https://example.test/design.png", MimeType: "image/png"},
	}})
	if err != nil {
		t.Fatalf("promptBlocks() error = %v", err)
	}
	if len(blocks) != 2 || blocks[1].ResourceLink == nil || blocks[1].ResourceLink.Uri != "https://example.test/design.png" || blocks[1].ResourceLink.Name != "design" || blocks[1].ResourceLink.MimeType == nil || *blocks[1].ResourceLink.MimeType != "image/png" {
		t.Fatalf("resource-link block = %#v", blocks)
	}
}

func TestPromptBlocksRejectsUnsupportedDetachedContentBeforeProcessStart(t *testing.T) {
	t.Parallel()

	_, err := promptBlocks(providers.ExecuteRequest{
		Prompt: []providers.ContentPart{{Kind: "image", Text: "not-yet-supported"}},
	})
	if !errors.Is(err, providers.ErrInvalidRequest) {
		t.Fatalf("promptBlocks() error = %v, want ErrInvalidRequest", err)
	}
}

func TestSafeACPStderrBoundsAndRedactsSensitiveEnvironmentValues(t *testing.T) {
	t.Parallel()
	secret := "super-secret-provider-token"
	got := safeACPStderr("token="+secret+" "+strings.Repeat("x", 1100), []providers.EnvironmentEntry{
		{Name: "PROVIDER_AUTH_TOKEN", Value: secret},
	})
	if len(got) != 1024 {
		t.Fatalf("safe stderr length = %d, want 1024", len(got))
	}
	if strings.Contains(got, secret) {
		t.Fatalf("safe stderr retained secret: %q", got)
	}
}

func TestProtocolErrorCarriesStableProvidersClassification(t *testing.T) {
	t.Parallel()
	err := protocolError("initialize", "cursor-acp", errors.New("bad frame"), "")
	if !errors.Is(err, providers.ErrProtocol) || !strings.Contains(err.Error(), "initialize") {
		t.Fatalf("protocolError() = %v", err)
	}
}

func TestClientAccumulatesOnlySDKAgentMessageUpdates(t *testing.T) {
	t.Parallel()

	client := &client{}
	for _, update := range []acpsdk.SessionUpdate{
		acpsdk.UpdateAgentMessageText("hello "),
		acpsdk.UpdateAgentThoughtText("private thought"),
		acpsdk.UpdateAgentMessageText("world"),
	} {
		if err := client.SessionUpdate(context.Background(), acpsdk.SessionNotification{
			SessionId: "session-1",
			Update:    update,
		}); err != nil {
			t.Fatalf("SessionUpdate() error = %v", err)
		}
	}
	if got := client.content(); got != "hello world" {
		t.Fatalf("accumulated final content = %q, want %q", got, "hello world")
	}
}

func TestClientEmitsProviderNeutralUpdatesSynchronouslyInSDKOrder(t *testing.T) {
	t.Parallel()

	var got []providers.ExecutionUpdate
	client := &client{emit: func(update providers.ExecutionUpdate) error {
		got = append(got, update)
		return nil
	}}
	client.setSessionID("session-stream")
	for _, update := range []acpsdk.SessionUpdate{
		acpsdk.UpdateAgentMessageText("answer"),
		acpsdk.UpdateAgentThoughtText("reason"),
		{Plan: &acpsdk.SessionUpdatePlan{SessionUpdate: "plan", Entries: []acpsdk.PlanEntry{{Content: "step", Status: acpsdk.PlanEntryStatusPending}}}},
	} {
		if err := client.SessionUpdate(context.Background(), acpsdk.SessionNotification{SessionId: "session-stream", Update: update}); err != nil {
			t.Fatalf("SessionUpdate() error = %v", err)
		}
	}
	want := []providers.ExecutionUpdateKind{providers.ExecutionUpdateMessage, providers.ExecutionUpdateReasoning, providers.ExecutionUpdatePlan}
	if len(got) != len(want) {
		t.Fatalf("update count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].Kind != want[index] || got[index].Sequence != int64(index+1) || got[index].ProviderSessionID != "session-stream" {
			t.Fatalf("update[%d] = %#v, want kind=%s sequence=%d session-stream", index, got[index], want[index], index+1)
		}
	}
}

func TestClientEmitsToolBeforeContainedFileDiffs(t *testing.T) {
	t.Parallel()

	oldText := "before\n"
	var got []providers.ExecutionUpdate
	client := &client{emit: func(update providers.ExecutionUpdate) error {
		got = append(got, update)
		return nil
	}}
	client.setSessionID("session-diff")
	err := client.SessionUpdate(context.Background(), acpsdk.SessionNotification{Update: acpsdk.SessionUpdate{
		ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			SessionUpdate: "tool_call_update", ToolCallId: "tool-1",
			Content: []acpsdk.ToolCallContent{{Diff: &acpsdk.ToolCallContentDiff{Type: "diff", Path: "a.txt", OldText: &oldText, NewText: "after\n"}}},
		},
	}})
	if err != nil {
		t.Fatalf("SessionUpdate() error = %v", err)
	}
	if len(got) != 2 || got[0].Kind != providers.ExecutionUpdateTool || got[1].Kind != providers.ExecutionUpdateFileChange {
		t.Fatalf("updates = %#v, want TOOL then FILE_CHANGE", got)
	}
	if got[1].FileChange == nil || got[1].FileChange.Path != "a.txt" || got[1].FileChange.Operation != "update" || got[1].Sequence != 2 {
		t.Fatalf("file update = %#v", got[1])
	}
}

func TestClientIgnoresKnownUnsupportedSDKUpdateSafely(t *testing.T) {
	t.Parallel()

	emitted := false
	client := &client{emit: func(providers.ExecutionUpdate) error { emitted = true; return nil }}
	err := client.SessionUpdate(context.Background(), acpsdk.SessionNotification{Update: acpsdk.SessionUpdate{
		CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{SessionUpdate: "current_mode_update", CurrentModeId: "code"},
	}})
	if err != nil || emitted {
		t.Fatalf("unsupported extension: error=%v emitted=%v", err, emitted)
	}
}

func TestRequestPermissionUsesOnlySkipPermissionsAndAdvertisedOptions(t *testing.T) {
	t.Parallel()

	options := []acpsdk.PermissionOption{
		{OptionId: "reject", Kind: acpsdk.PermissionOptionKindRejectOnce},
		{OptionId: "allow", Kind: acpsdk.PermissionOptionKindAllowOnce},
	}
	for _, test := range []struct {
		name            string
		skipPermissions bool
		want            acpsdk.PermissionOptionId
	}{
		{name: "normal policy rejects", want: "reject"},
		{name: "skip permissions allows", skipPermissions: true, want: "allow"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response, err := (&client{skipPermissions: test.skipPermissions}).RequestPermission(
				context.Background(),
				acpsdk.RequestPermissionRequest{Options: options},
			)
			if err != nil {
				t.Fatalf("RequestPermission() error = %v", err)
			}
			if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != test.want {
				t.Fatalf("RequestPermission() outcome = %#v, want selected %q", response.Outcome, test.want)
			}
		})
	}
}

func TestRequestPermissionCancelsInsteadOfInventingAnOption(t *testing.T) {
	t.Parallel()

	for _, request := range []struct {
		name    string
		ctx     context.Context
		options []acpsdk.PermissionOption
	}{
		{name: "no compatible option", ctx: context.Background(), options: []acpsdk.PermissionOption{{OptionId: "reject", Kind: acpsdk.PermissionOptionKindRejectAlways}}},
		{name: "cancelled turn", ctx: cancelledContext(), options: []acpsdk.PermissionOption{{OptionId: "allow", Kind: acpsdk.PermissionOptionKindAllowAlways}}},
	} {
		request := request
		t.Run(request.name, func(t *testing.T) {
			t.Parallel()
			response, err := (&client{skipPermissions: true}).RequestPermission(
				request.ctx,
				acpsdk.RequestPermissionRequest{Options: request.options},
			)
			if err != nil {
				t.Fatalf("RequestPermission() error = %v", err)
			}
			if response.Outcome.Cancelled == nil || response.Outcome.Selected != nil {
				t.Fatalf("RequestPermission() outcome = %#v, want cancelled", response.Outcome)
			}
		})
	}
}

func TestExecutionStreamCloseCancelsPromptAndCleansUpProcess(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	adapter, err := New(func(string, ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=^TestACPCancellationAgentHelper$")
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	stream, err := adapter.ExecuteStream(context.Background(), catalog.Descriptor{Provider: providers.Provider{
		ID: "cancel-acp", ExecutionKind: providers.ExecutionKindACP, Command: "cancel-agent",
	}}, providers.ExecuteRequest{
		ProviderID: "cancel-acp", WorkingDirectory: t.TempDir(),
		Prompt:      []providers.ContentPart{{Kind: providers.ContentKindText, Text: "wait"}},
		Environment: cancellationTestEnvironment(ready),
	})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	waitForTestFile(t, ready)
	stream.Close()
	for range stream.Updates {
	}
	outcome := <-stream.Outcome
	if !errors.Is(outcome.Err, context.Canceled) {
		t.Fatalf("outcome error = %v, want context cancellation", outcome.Err)
	}
}

func TestExecutionUsesExistingContextDeadlineAndCleansUpProcess(t *testing.T) {
	adapter, err := New(func(string, ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=^TestACPCancellationAgentHelper$")
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = adapter.Execute(ctx, catalog.Descriptor{Provider: providers.Provider{
		ID: "deadline-acp", ExecutionKind: providers.ExecutionKindACP, Command: "deadline-agent",
	}}, providers.ExecuteRequest{
		ProviderID: "deadline-acp", WorkingDirectory: t.TempDir(),
		Prompt:      []providers.ContentPart{{Kind: providers.ContentKindText, Text: "wait"}},
		Environment: cancellationTestEnvironment(filepath.Join(t.TempDir(), "ready")),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want context deadline", err)
	}
}

func TestClientUnsupportedCapabilitiesFailSafely(t *testing.T) {
	t.Parallel()
	client := &client{}
	checks := []struct {
		name string
		err  error
	}{
		{name: "read file", err: func() error {
			_, err := client.ReadTextFile(context.Background(), acpsdk.ReadTextFileRequest{})
			return err
		}()},
		{name: "write file", err: func() error {
			_, err := client.WriteTextFile(context.Background(), acpsdk.WriteTextFileRequest{})
			return err
		}()},
		{name: "create terminal", err: func() error {
			_, err := client.CreateTerminal(context.Background(), acpsdk.CreateTerminalRequest{})
			return err
		}()},
		{name: "kill terminal", err: func() error {
			_, err := client.KillTerminal(context.Background(), acpsdk.KillTerminalRequest{})
			return err
		}()},
		{name: "terminal output", err: func() error {
			_, err := client.TerminalOutput(context.Background(), acpsdk.TerminalOutputRequest{})
			return err
		}()},
		{name: "release terminal", err: func() error {
			_, err := client.ReleaseTerminal(context.Background(), acpsdk.ReleaseTerminalRequest{})
			return err
		}()},
		{name: "wait terminal", err: func() error {
			_, err := client.WaitForTerminalExit(context.Background(), acpsdk.WaitForTerminalExitRequest{})
			return err
		}()},
	}
	for _, check := range checks {
		if check.err == nil {
			t.Fatalf("%s unexpectedly succeeded", check.name)
		}
	}
}

func TestIndependentACPProvidersExecuteConcurrentlyWithIsolatedSessions(t *testing.T) {
	var starts atomic.Int32
	adapter, err := New(func(string, ...string) *exec.Cmd {
		starts.Add(1)
		return exec.Command(os.Args[0], "-test.run=^TestACPCancellationAgentHelper$")
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	type result struct {
		response providers.ExecuteResponse
		err      error
	}
	results := make(chan result, 2)
	for _, id := range []providers.ID{"first-acp", "second-acp"} {
		id := id
		go func() {
			response, executeErr := adapter.Execute(context.Background(), catalog.Descriptor{Provider: providers.Provider{
				ID: id, ExecutionKind: providers.ExecutionKindACP, Command: "test-agent",
			}}, providers.ExecuteRequest{
				ProviderID: id, WorkingDirectory: os.TempDir(),
				Prompt:      []providers.ContentPart{{Kind: providers.ContentKindText, Text: "run"}},
				Environment: successTestEnvironment(string(id)),
			})
			results <- result{response: response, err: executeErr}
		}()
	}
	seen := map[string]bool{}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("Execute() error = %v", got.err)
		}
		if got.response.Session == nil || got.response.Content == "" {
			t.Fatalf("response = %#v", got.response)
		}
		seen[got.response.Session.ID] = true
	}
	if starts.Load() != 2 || !seen["first-acp-session"] || !seen["second-acp-session"] {
		t.Fatalf("starts=%d sessions=%v", starts.Load(), seen)
	}
}

func cancellationTestEnvironment(ready string) []providers.EnvironmentEntry {
	entries := make([]providers.EnvironmentEntry, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		name, content, ok := strings.Cut(value, "=")
		if ok && name != "" {
			entries = append(entries, providers.EnvironmentEntry{Name: name, Value: content})
		}
	}
	return append(entries,
		providers.EnvironmentEntry{Name: cancellationHelperEnvironment, Value: "1"},
		providers.EnvironmentEntry{Name: "YOU_TEST_ACP_HELPER_MODE", Value: "cancel"},
		providers.EnvironmentEntry{Name: "YOU_TEST_ACP_READY_FILE", Value: ready},
	)
}

func successTestEnvironment(id string) []providers.EnvironmentEntry {
	entries := cancellationTestEnvironment("")
	for index := range entries {
		if entries[index].Name == "YOU_TEST_ACP_HELPER_MODE" {
			entries[index].Value = "success"
		}
	}
	return append(entries, providers.EnvironmentEntry{Name: "YOU_TEST_ACP_SESSION", Value: id + "-session"})
}

func waitForTestFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestACPCancellationAgentHelper(t *testing.T) {
	if os.Getenv(cancellationHelperEnvironment) != "1" {
		return
	}
	agent := &cancellationAgent{}
	connection := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	helperConnection = connection
	<-connection.Done()
}

type cancellationAgent struct{}

func (*cancellationAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{ProtocolVersion: acpsdk.ProtocolVersionNumber}, nil
}
func (*cancellationAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	sessionID := os.Getenv("YOU_TEST_ACP_SESSION")
	if sessionID == "" {
		sessionID = "cancel-session"
	}
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(sessionID)}, nil
}
func (a *cancellationAgent) Prompt(ctx context.Context, request acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	if os.Getenv("YOU_TEST_ACP_HELPER_MODE") == "success" {
		if err := helperConnection.SessionUpdate(ctx, acpsdk.SessionNotification{SessionId: request.SessionId, Update: acpsdk.UpdateAgentMessageText(string(request.SessionId))}); err != nil {
			return acpsdk.PromptResponse{}, err
		}
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
	}
	if err := os.WriteFile(os.Getenv("YOU_TEST_ACP_READY_FILE"), []byte("ready"), 0o600); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	<-ctx.Done()
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
}
func (*cancellationAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}
func (*cancellationAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}
func (*cancellationAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}
func (*cancellationAgent) CloseSession(context.Context, acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	return acpsdk.CloseSessionResponse{}, nil
}
func (*cancellationAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}
func (*cancellationAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}
func (*cancellationAgent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}
func (*cancellationAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

var _ acpsdk.Agent = (*cancellationAgent)(nil)

var helperConnection *acpsdk.AgentSideConnection

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
