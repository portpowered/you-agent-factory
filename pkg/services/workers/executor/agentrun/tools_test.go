package agentrun

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type localToolFileSystem struct{}

func (localToolFileSystem) Abs(path string) (string, error)            { return filepath.Abs(path) }
func (localToolFileSystem) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (localToolFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localToolFileSystem) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localToolFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localToolFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func TestPolicyToolExecutor_DisabledDeniesToolCalls(t *testing.T) {
	t.Parallel()

	recorder := NewToolDiagnosticRecorder()
	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyDisabled, t.TempDir(), recorder)
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"note.txt"}`,
	})
	if !errors.Is(err, ErrToolPolicyDenied) {
		t.Fatalf("Execute error = %v, want ErrToolPolicyDenied", err)
	}
	metadata := toolDiagnosticsMetadata(interfaces.AgentToolPolicyDisabled, recorder)
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
	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyReadOnly, dir, recorder)
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
	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyEnabled, dir, recorder)
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
	metadata := toolDiagnosticsMetadata(interfaces.AgentToolPolicyEnabled, recorder)
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

	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyReadOnly, dir, NewToolDiagnosticRecorder())
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

	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyReadOnly, t.TempDir(), NewToolDiagnosticRecorder())
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"../secret.txt"}`,
	})
	if err == nil {
		t.Fatal("expected path escape rejection")
	}
}

func TestPolicyToolExecutor_UsesInjectedFileSystem(t *testing.T) {
	t.Parallel()

	files := &recordingToolFileSystem{content: []byte("injected content")}
	executor := NewPolicyToolExecutor(
		files,
		interfaces.AgentToolPolicyReadOnly,
		"workspace",
		NewToolDiagnosticRecorder(),
	)
	response, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc-injected",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"note.txt"}`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if response.Content != "injected content" {
		t.Fatalf("content = %q, want injected content", response.Content)
	}
	wantPath := filepath.Join(files.absoluteBase(), "note.txt")
	if files.readPath != wantPath {
		t.Fatalf("ReadFile path = %q, want %q", files.readPath, wantPath)
	}
}

func TestPolicyToolExecutor_MissingFileSystemFailsClosed(t *testing.T) {
	t.Parallel()

	executor := NewPolicyToolExecutor(
		nil,
		interfaces.AgentToolPolicyReadOnly,
		"workspace",
		NewToolDiagnosticRecorder(),
	)
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc-missing-filesystem",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"note.txt"}`,
	})
	if !errors.Is(err, ErrToolFileSystemRequired) {
		t.Fatalf("Execute error = %v, want missing filesystem failure", err)
	}
}

func TestPolicyToolExecutor_FailureDiagnosticsExcludeAbsolutePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		toolName   string
		arguments  string
		wantPath   string
		wantReason string
		setup      func(t *testing.T, dir string)
	}{
		{
			name:       "read missing file",
			toolName:   ToolNameReadFile,
			arguments:  `{"path":"missing.txt"}`,
			wantPath:   "missing.txt",
			wantReason: "not_found",
		},
		{
			name:       "list missing directory",
			toolName:   ToolNameListDirectory,
			arguments:  `{"path":"missing-dir"}`,
			wantPath:   "missing-dir",
			wantReason: "not_found",
		},
		{
			name:       "write when parent path is a file",
			toolName:   ToolNameWriteFile,
			arguments:  `{"path":"nested/out.txt","content":"data"}`,
			wantPath:   "nested/out.txt",
			wantReason: "operation_failed",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "nested"), []byte("blocker"), 0o644); err != nil {
					t.Fatalf("WriteFile blocker: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			caseDir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, caseDir)
			}
			localRecorder := NewToolDiagnosticRecorder()
			localExecutor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyEnabled, caseDir, localRecorder)
			_, err := localExecutor.Execute(context.Background(), messages.ToolCall{
				ID:        "tc1",
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err == nil {
				t.Fatal("expected tool failure")
			}
			metadata := toolDiagnosticsMetadata(interfaces.AgentToolPolicyEnabled, localRecorder)
			diagnostics := metadata[DiagnosticToolDiagnostics]
			if diagnostics == "" {
				t.Fatalf("tool diagnostics = %#v, want failure summary", metadata)
			}
			if strings.Contains(diagnostics, caseDir) {
				t.Fatalf("tool diagnostics leak absolute working directory %q: %q", caseDir, diagnostics)
			}
			if !strings.Contains(diagnostics, "path="+tc.wantPath) {
				t.Fatalf("tool diagnostics = %q, want relative path %q", diagnostics, tc.wantPath)
			}
			if !strings.Contains(diagnostics, "reason="+tc.wantReason) {
				t.Fatalf("tool diagnostics = %q, want reason %q", diagnostics, tc.wantReason)
			}
		})
	}
}

func TestToolFailureReason_ClassifiesStableCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "not found", err: fs.ErrNotExist, want: "not_found"},
		{name: "permission denied", err: fs.ErrPermission, want: "permission_denied"},
		{name: "path required", err: errors.New("tool path is required"), want: "path_required"},
		{name: "path must be relative", err: errors.New("tool path must be relative to the agent working directory"), want: "path_must_be_relative"},
		{name: "path escape", err: errors.New("tool path cannot escape the agent working directory"), want: "path_escape_denied"},
		{name: "invalid arguments", err: errors.New("read_file arguments must be JSON with path: invalid"), want: "invalid_arguments"},
		{name: "working directory unavailable", err: errors.New("agent working directory is unavailable"), want: "working_directory_unavailable"},
		{name: "unsupported tool", err: errors.New("agent tool is not supported: custom"), want: "tool_not_supported"},
		{name: "generic failure", err: errors.New("unexpected"), want: "operation_failed"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := toolFailureReason(tc.err); got != tc.want {
				t.Fatalf("toolFailureReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolFailureDetail_UsesRelativePathAndReason(t *testing.T) {
	t.Parallel()

	got := toolFailureDetail(ToolNameReadFile, `{"path":"notes/missing.txt"}`, fs.ErrNotExist)
	want := "path=notes/missing.txt reason=not_found"
	if got != want {
		t.Fatalf("toolFailureDetail() = %q, want %q", got, want)
	}
}

func TestToolRelativePathFromArguments_ExtractsWritePath(t *testing.T) {
	t.Parallel()

	got := toolRelativePathFromArguments(`{"path":"nested/out.txt","content":"data"}`)
	if got != "nested/out.txt" {
		t.Fatalf("toolRelativePathFromArguments() = %q, want nested/out.txt", got)
	}
}

func TestToolRelativePathFromArguments_OmitsUnsafePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		arguments string
	}{
		{name: "absolute path", arguments: `{"path":"/Users/test/secret.txt"}`},
		{name: "path escape", arguments: `{"path":"../secret.txt"}`},
		{name: "empty path", arguments: `{"path":""}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := toolRelativePathFromArguments(tc.arguments); got != "" {
				t.Fatalf("toolRelativePathFromArguments() = %q, want empty for unsafe path", got)
			}
		})
	}
}

func TestPolicyToolExecutor_AbsolutePathArgumentOmitsPathFromDiagnostics(t *testing.T) {
	t.Parallel()

	absolutePath := filepath.Join(t.TempDir(), "secret.txt")
	recorder := NewToolDiagnosticRecorder()
	executor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyReadOnly, t.TempDir(), recorder)
	_, err := executor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: fmt.Sprintf(`{"path":%q}`, absolutePath),
	})
	if err == nil {
		t.Fatal("expected absolute path rejection")
	}

	metadata := toolDiagnosticsMetadata(interfaces.AgentToolPolicyReadOnly, recorder)
	diagnostics := metadata[DiagnosticToolDiagnostics]
	if diagnostics == "" {
		t.Fatalf("tool diagnostics = %#v, want failure summary", metadata)
	}
	if strings.Contains(diagnostics, absolutePath) {
		t.Fatalf("tool diagnostics leak absolute path argument %q: %q", absolutePath, diagnostics)
	}
	if strings.Contains(diagnostics, "path=") {
		t.Fatalf("tool diagnostics should omit path for rejected absolute argument: %q", diagnostics)
	}
	if !strings.Contains(diagnostics, "reason=path_must_be_relative") {
		t.Fatalf("tool diagnostics = %q, want path_must_be_relative reason", diagnostics)
	}
	if strings.Contains(err.Error(), absolutePath) {
		t.Fatalf("tool runtime error leaks absolute path argument %q: %q", absolutePath, err.Error())
	}
}

func TestSanitizeToolDiagnosticDetail_TruncatesLongValues(t *testing.T) {
	t.Parallel()

	longDetail := strings.Repeat("x", toolDiagnosticMaxLen+10)
	got := sanitizeToolDiagnosticDetail(longDetail)
	if len(got) <= toolDiagnosticMaxLen {
		t.Fatalf("sanitizeToolDiagnosticDetail() = %d chars, want truncation beyond %d", len(got), toolDiagnosticMaxLen)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("sanitizeToolDiagnosticDetail() = %q, want ellipsis suffix", got)
	}
}

func TestToolDefinitionsForPolicy_ExposesSupportedTools(t *testing.T) {
	t.Parallel()

	readOnly := toolDefinitionsForPolicy(interfaces.AgentToolPolicyReadOnly)
	if len(readOnly) != 2 {
		t.Fatalf("read-only tools = %d, want 2", len(readOnly))
	}
	enabled := toolDefinitionsForPolicy(interfaces.AgentToolPolicyEnabled)
	if len(enabled) != 3 {
		t.Fatalf("enabled tools = %d, want 3", len(enabled))
	}
	if toolDefinitionsForPolicy(interfaces.AgentToolPolicyDisabled) != nil {
		t.Fatal("disabled policy should not expose tool definitions")
	}
}

func TestLibraryHarnessAdapter_EnabledToolsRegistersExecutor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
	result, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage:  "hello",
		Inferencer:   staticInferencer{response: "done"},
		ToolPolicy:   interfaces.AgentToolPolicyReadOnly,
		WorkingDir:   dir,
		ToolRecorder: NewToolDiagnosticRecorder(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", result.FinalText)
	}
}

func TestLibraryHarnessAdapter_EnabledToolsRequireFileSystem(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter(nil)
	_, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{response: "done"},
		ToolPolicy:  interfaces.AgentToolPolicyReadOnly,
	})
	if !errors.Is(err, ErrToolFileSystemRequired) {
		t.Fatalf("Execute error = %v, want ErrToolFileSystemRequired", err)
	}
}

func TestLibraryHarnessAdapter_DisabledRunsNoToolsMode(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
	result, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{response: "done"},
		ToolPolicy:  interfaces.AgentToolPolicyDisabled,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", result.FinalText)
	}
}

func TestAgentRunToolFailure_SanitizedFailureMessageThroughDispatchProjection(t *testing.T) {
	t.Parallel()

	caseDir := t.TempDir()
	recorder := NewToolDiagnosticRecorder()
	toolExecutor := NewPolicyToolExecutor(localToolFileSystem{}, interfaces.AgentToolPolicyEnabled, caseDir, recorder)
	_, toolErr := toolExecutor.Execute(context.Background(), messages.ToolCall{
		ID:        "tc1",
		Name:      ToolNameReadFile,
		Arguments: `{"path":"missing.txt"}`,
	})
	if toolErr == nil {
		t.Fatal("expected tool failure")
	}

	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-tool-fail",
		TransitionID:    "execute",
		WorkstationName: "Execute",
	}
	result := agentRunFailureWorkResult(
		dispatch,
		toolErr,
		time.Second,
		interfaces.AgentToolPolicyEnabled,
		recorder,
	)
	if strings.Contains(result.Error, caseDir) {
		t.Fatalf("work result error leaks absolute working directory %q: %q", caseDir, result.Error)
	}

	workItem := work.FactoryWorkItem{
		ID:          "work-tool-fail",
		WorkTypeID:  "task",
		DisplayName: "Tool failure story",
		TraceID:     "trace-tool-fail",
		PlaceID:     "task:init",
	}
	completedAt := time.Unix(1, 0).UTC()
	state := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			workItem.ID: workItem,
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID:     dispatch.DispatchID,
			TransitionID:   dispatch.TransitionID,
			Workstation:    interfaces.FactoryWorkstationRef{ID: "execute", Name: "Execute"},
			WorkItemIDs:    []string{workItem.ID},
			TraceIDs:       []string{workItem.TraceID},
			StartedAt:      time.Unix(0, 0).UTC(),
			CompletedAt:    completedAt,
			DurationMillis: 1000,
			Result: interfaces.WorkstationResult{
				Outcome: string(workerexecution.OutcomeFailed),
				Error:   result.Error,
				FailureDetail: &workerexecution.FailureDetail{
					Reason:  workerexecution.WorkFailureTypeUnknown,
					Message: result.Error,
				},
			},
		}},
	}
	projection := recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
	if projection.WorkstationRequestsByDispatchId == nil {
		t.Fatal("expected workstation request projection")
	}
	view := (*projection.WorkstationRequestsByDispatchId)[dispatch.DispatchID]
	if view.Response == nil || view.Response.FailureDetail == nil {
		t.Fatalf("projected response = %#v, want failure message", view.Response)
	}
	if strings.Contains(view.Response.FailureDetail.Message, caseDir) {
		t.Fatalf("projected failure message leaks absolute working directory %q: %q", caseDir, view.Response.FailureDetail.Message)
	}
}

func TestFailureClassForError_ToolRuntimeFailure(t *testing.T) {
	t.Parallel()

	err := newToolRuntimeError(ToolNameReadFile, `{"path":"missing.txt"}`, fs.ErrNotExist)
	if got := failureClassForError(err); got != FailureClassToolRuntime {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassToolRuntime)
	}
}

func TestAgentRunExecutor_ToolRuntimeFailureSurfacesFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: newToolRuntimeError(ToolNameReadFile, `{"path":"missing.txt"}`, fs.ErrNotExist)}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentToolsConfig{
						Policy: interfaces.AgentToolPolicyReadOnly,
					},
				},
			},
		},
		&stubRunner{}, nil,

		harness, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassToolRuntime {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassToolRuntime)
	}
	if strings.Contains(result.Error, "open /") {
		t.Fatalf("work result error leaks absolute path details: %q", result.Error)
	}
}

func TestAgentRunExecutor_ToolPolicyViolationSurfacesFailureClass(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{err: ErrToolPolicyDenied}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentToolsConfig{
						Policy: interfaces.AgentToolPolicyReadOnly,
					},
				},
			},
		},
		&stubRunner{}, nil,

		harness, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticFailureClass] != FailureClassToolPolicy {
		t.Fatalf("failure class = %#v, want %s", result.Diagnostics, FailureClassToolPolicy)
	}
}

type recordingToolFileSystem struct {
	content  []byte
	readPath string
}

func (*recordingToolFileSystem) absoluteBase() string {
	return filepath.Join(string(filepath.Separator), "injected-workspace")
}

func (files *recordingToolFileSystem) Abs(string) (string, error) {
	return files.absoluteBase(), nil
}

func (files *recordingToolFileSystem) Stat(path string) (fs.FileInfo, error) {
	return toolFileInfo{directory: path == files.absoluteBase()}, nil
}

func (files *recordingToolFileSystem) ReadFile(path string) ([]byte, error) {
	files.readPath = path
	return append([]byte(nil), files.content...), nil
}

func (*recordingToolFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, errors.New("unexpected ReadDir")
}

func (*recordingToolFileSystem) MkdirAll(string, fs.FileMode) error {
	return errors.New("unexpected MkdirAll")
}

func (*recordingToolFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	return errors.New("unexpected WriteFile")
}

type toolFileInfo struct{ directory bool }

func (toolFileInfo) Name() string       { return "fixture" }
func (toolFileInfo) Size() int64        { return 0 }
func (toolFileInfo) Mode() fs.FileMode  { return 0 }
func (toolFileInfo) ModTime() time.Time { return time.Time{} }
func (info toolFileInfo) IsDir() bool   { return info.directory }
func (toolFileInfo) Sys() any           { return nil }

func TestAgentRunExecutor_SuccessIncludesToolPolicyDiagnostics(t *testing.T) {
	t.Parallel()

	harness := &recordingHarnessAdapter{
		result: HarnessResult{FinalText: "final answer"},
	}
	executor := NewAgentRunExecutorWithDependencies(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {
					Type: interfaces.WorkerTypeAgent,
					AgentTools: &interfaces.AgentToolsConfig{
						Policy: interfaces.AgentToolPolicyDisabled,
					},
				},
			},
		},
		&stubRunner{response: "unused"}, nil,

		harness, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRunRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.Metadata[DiagnosticToolPolicy] != interfaces.AgentToolPolicyDisabled {
		t.Fatalf("tool policy diagnostics = %#v", result.Diagnostics)
	}
	if harness.lastInput.ToolPolicy != interfaces.AgentToolPolicyDisabled {
		t.Fatalf("harness tool policy = %q, want DISABLED", harness.lastInput.ToolPolicy)
	}
}
