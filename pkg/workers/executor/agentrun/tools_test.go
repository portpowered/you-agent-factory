package agentrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPolicyToolExecutor_DisabledDeniesToolCalls(t *testing.T) {
	t.Parallel()

	recorder := NewToolDiagnosticRecorder()
	executor := NewPolicyToolExecutor(interfaces.AgentWorkerToolPolicyDisabled, t.TempDir(), recorder)
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:   "tc1",
		Name: ToolNameReadFile,
		Arguments: `{"path":"note.txt"}`,
	})
	if !errors.Is(err, ErrToolPolicyDenied) {
		t.Fatalf("Execute error = %v, want ErrToolPolicyDenied", err)
	}
	metadata := toolDiagnosticsMetadata(interfaces.AgentWorkerToolPolicyDisabled, recorder)
	if metadata[DiagnosticToolDiagnostics] == "" {
		t.Fatalf("tool diagnostics = %#v, want denied summary", metadata)
	}
}

func TestPolicyToolExecutor_ReadOnlyAllowsReadAndDeniesWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	recorder := NewToolDiagnosticRecorder()
	executor := NewPolicyToolExecutor(interfaces.AgentWorkerToolPolicyReadOnly, dir, recorder)
	response, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"note.txt"}`,
	})
	if err != nil {
		t.Fatalf("read_file Execute: %v", err)
	}
	if response.Content != "hello" {
		t.Fatalf("read_file content = %q, want hello", response.Content)
	}

	_, err = executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc2",
		Name:      ToolNameWriteFile,
		Arguments: `{"path":"out.txt","content":"nope"}`,
	})
	if !errors.Is(err, ErrToolPolicyDenied) {
		t.Fatalf("write_file error = %v, want ErrToolPolicyDenied", err)
	}
}

func TestPolicyToolExecutor_EnabledAllowsWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder := NewToolDiagnosticRecorder()
	executor := NewPolicyToolExecutor(interfaces.AgentWorkerToolPolicyEnabled, dir, recorder)
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameWriteFile,
		Arguments: `{"path":"out.txt","content":"saved"}`,
	})
	if err != nil {
		t.Fatalf("write_file Execute: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "saved" {
		t.Fatalf("written content = %q, want saved", string(data))
	}
	metadata := toolDiagnosticsMetadata(interfaces.AgentWorkerToolPolicyEnabled, recorder)
	if metadata[DiagnosticToolCallCount] != "2" {
		t.Fatalf("tool call count = %q, want 2 start+success events recorded", metadata[DiagnosticToolCallCount])
	}
}

func TestPolicyToolExecutor_ReadOnlyListDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	executor := NewPolicyToolExecutor(interfaces.AgentWorkerToolPolicyReadOnly, dir, NewToolDiagnosticRecorder())
	response, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameListDirectory,
		Arguments: `{"path":"."}`,
	})
	if err != nil {
		t.Fatalf("list_directory Execute: %v", err)
	}
	if response.Content != "a.txt" {
		t.Fatalf("list_directory content = %q, want a.txt", response.Content)
	}
}

func TestPolicyToolExecutor_RejectsPathEscape(t *testing.T) {
	t.Parallel()

	executor := NewPolicyToolExecutor(interfaces.AgentWorkerToolPolicyReadOnly, t.TempDir(), NewToolDiagnosticRecorder())
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"../secret.txt"}`,
	})
	if err == nil {
		t.Fatal("expected path escape rejection")
	}
}

func TestLibraryHarnessAdapter_DisabledRunsNoToolsMode(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter()
	result, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{response: "done"},
		ToolPolicy:  interfaces.AgentWorkerToolPolicyDisabled,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", result.FinalText)
	}
}

func TestFailureClassForError_ToolRuntimeFailure(t *testing.T) {
	t.Parallel()

	err := errors.New("read_file failed: open missing.txt: no such file or directory")
	if got := failureClassForError(err); got != FailureClassToolRuntime {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassToolRuntime)
	}
}

func TestAgentRunExecutor_ToolRuntimeFailureSurfacesFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: errors.New("read_file failed: open missing.txt: no such file or directory")}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentWorkerToolsConfig{
						Policy: interfaces.AgentWorkerToolPolicyReadOnly,
					},
				},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassToolRuntime {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassToolRuntime)
	}
}

func TestAgentRunExecutor_ToolPolicyViolationSurfacesFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: ErrToolPolicyDenied}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentWorkerToolsConfig{
						Policy: interfaces.AgentWorkerToolPolicyReadOnly,
					},
				},
			},
		},
		&stubRunner{},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassToolPolicy {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassToolPolicy)
	}
}

func TestAgentRunExecutor_SuccessIncludesToolPolicyDiagnostics(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{
		result: HarnessResult{FinalText: "final answer"},
	}
	executor := NewAgentRunExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentWorkerToolsConfig{
						Policy: interfaces.AgentWorkerToolPolicyDisabled,
					},
				},
			},
		},
		&stubRunner{response: "unused"},
		WithAgentRunHarness(harness),
	)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticToolPolicy] != interfaces.AgentWorkerToolPolicyDisabled {
		t.Fatalf("tool policy diagnostics = %#v", result.Diagnostics)
	}
	if harness.lastInput.ToolPolicy != interfaces.AgentWorkerToolPolicyDisabled {
		t.Fatalf("harness tool policy = %q, want DISABLED", harness.lastInput.ToolPolicy)
	}
}
