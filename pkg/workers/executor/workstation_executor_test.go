package executor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

type wsMockExecutor struct {
	dispatch interfaces.WorkstationExecutionRequest
	called   bool
	result   interfaces.WorkResult
	err      error
}

type dispatchCapturingExecutor struct {
	dispatch    interfaces.WorkstationExecutionRequest
	called      bool
	deadline    time.Time
	hasDeadline bool
	result      interfaces.WorkResult
	err         error
}

func (m *wsMockExecutor) Execute(_ context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.called = true
	m.dispatch = d
	return m.result, m.err
}

func (m *dispatchCapturingExecutor) Execute(ctx context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.called = true
	m.dispatch = d
	m.deadline, m.hasDeadline = ctx.Deadline()
	return m.result, m.err
}

func newTestWorkstationExecutor(runtimeConfig interfaces.RuntimeConfigLookup, executor WorkstationRequestExecutor) *WorkstationExecutor {
	return &WorkstationExecutor{
		RuntimeConfig: runtimeConfig,
		Executor:      executor,
		Renderer:      &DefaultPromptRenderer{},
	}
}

func TestWorkstationExecutor_ModelWorkstation_RendersPromptAndDelegates(t *testing.T) {
	mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "You are a helpful assistant."},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Process work {{ (index .Inputs 0).WorkID }}"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(interfaces.Token{
			ID:    "tok-1",
			Color: interfaces.TokenColor{WorkID: "work-1", WorkTypeID: "code-changes"},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if result.Output != "done" {
		t.Fatalf("Output = %q, want %q", result.Output, "done")
	}
	if mock.dispatch.SystemPrompt != "You are a helpful assistant." {
		t.Fatalf("system prompt not set")
	}
	if mock.dispatch.UserMessage != "Process work work-1" {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this workstation execution contract test keeps canonical runtime field assertions together on the worker seam.
func TestWorkstationExecutor_ModelWorkstationUsesCanonicalWorkstationRuntimeFields(t *testing.T) {
	projectRoot := t.TempDir()

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(canonicalWorkstationRuntimeConfig(projectRoot), mock)

	start := time.Now()
	result, err := we.Execute(context.Background(), canonicalWorkstationDispatch())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if mock.dispatch.WorkerType != "canonical-worker" {
		t.Fatalf("worker type = %q, want canonical worker binding", mock.dispatch.WorkerType)
	}
	if mock.dispatch.ProjectID != "agent-factory" {
		t.Fatalf("project ID = %q, want canonical dispatch project context", mock.dispatch.ProjectID)
	}
	if mock.dispatch.SystemPrompt != "canonical system" {
		t.Fatalf("system prompt = %q, want canonical worker body", mock.dispatch.SystemPrompt)
	}
	if mock.dispatch.UserMessage != "Review work-1 for agent-factory" {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
	if mock.dispatch.OutputSchema != `{"type":"object"}` {
		t.Fatalf("output schema = %q", mock.dispatch.OutputSchema)
	}
	if mock.dispatch.WorkingDirectory != filepath.Join(projectRoot, "repo", "feature-runtime") {
		t.Fatalf("working directory = %q", mock.dispatch.WorkingDirectory)
	}
	if mock.dispatch.Worktree != "worktrees/feature-runtime" {
		t.Fatalf("worktree = %q", mock.dispatch.Worktree)
	}
	if mock.dispatch.EnvVars["PROJECT"] != "agent-factory" || mock.dispatch.EnvVars["BRANCH"] != "feature-runtime" {
		t.Fatalf("env vars = %#v", mock.dispatch.EnvVars)
	}
	if !mock.hasDeadline {
		t.Fatal("expected workstation timeout to set executor deadline")
	}
	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation timeout range", remaining)
	}
}

func TestWorkstationExecutor_ModelWorkstation_PreservesDistinctMultiInputCanonicalContent(t *testing.T) {
	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type: interfaces.WorkstationTypeModel,
					PromptTemplate: `{{ (index .Inputs 0).WorkID }}:{{ range (index .Inputs 0).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}
{{ (index .Inputs 1).WorkID }}:{{ range (index .Inputs 1).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}`,
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-multi-content",
		TransitionID:    "t-multi-content",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(
			interfaces.Token{
				ID: "tok-text",
				Color: interfaces.TokenColor{
					WorkID: "work-text",
					Content: []interfaces.WorkContentPart{
						{Type: interfaces.WorkContentPartTypeText, Text: "plan"},
					},
					Payload: []byte("plan"),
				},
			},
			interfaces.Token{
				ID: "tok-mixed",
				Color: interfaces.TokenColor{
					WorkID: "work-mixed",
					Content: []interfaces.WorkContentPart{
						{Type: interfaces.WorkContentPartTypeText, Text: "caption"},
						{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
					},
					Payload: []byte("caption"),
				},
			},
		),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if !strings.Contains(mock.dispatch.UserMessage, "work-text: [text=plan]") {
		t.Fatalf("rendered prompt = %q, want first input content preserved", mock.dispatch.UserMessage)
	}
	if !strings.Contains(mock.dispatch.UserMessage, "work-mixed: [text=caption] [image=fixtures/mockup.png]") {
		t.Fatalf("rendered prompt = %q, want second input mixed content preserved", mock.dispatch.UserMessage)
	}

	inputTokens := executionRequestInputTokens(mock.dispatch)
	if len(inputTokens) != 2 {
		t.Fatalf("forwarded input token count = %d, want 2", len(inputTokens))
	}
	if inputTokens[0].Color.WorkID != "work-text" || len(inputTokens[0].Color.Content) != 1 {
		t.Fatalf("first forwarded input = %#v, want text input preserved", inputTokens[0].Color)
	}
	if inputTokens[1].Color.WorkID != "work-mixed" || len(inputTokens[1].Color.Content) != 2 {
		t.Fatalf("second forwarded input = %#v, want mixed-content input preserved", inputTokens[1].Color)
	}
	if inputTokens[1].Color.Content[1].Type != interfaces.WorkContentPartTypeImage || inputTokens[1].Color.Content[1].File != "fixtures/mockup.png" {
		t.Fatalf("second forwarded input content = %#v, want ordered image part", inputTokens[1].Color.Content)
	}
}

func TestWorkstationExecutor_ResolveWorkstationExecutionContext_AppliesResolvedRuntimeFields(t *testing.T) {
	projectRoot := t.TempDir()
	we := newTestWorkstationExecutor(canonicalWorkstationRuntimeConfig(projectRoot), &wsMockExecutor{})
	workstationDef, ok := we.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected review workstation")
	}

	resolved, failed := we.resolveWorkstationExecutionContext(
		canonicalWorkstationDispatch(),
		workstationDef,
		time.Now(),
		logging.NoopLogger{},
	)
	if failed != nil {
		t.Fatalf("unexpected failed result: %#v", failed)
	}

	if resolved.ProjectID != "agent-factory" {
		t.Fatalf("project ID = %q", resolved.ProjectID)
	}
	if resolved.WorkingDirectory != filepath.Join(projectRoot, "repo", "feature-runtime") {
		t.Fatalf("working directory = %q", resolved.WorkingDirectory)
	}
	if resolved.Worktree != "worktrees/feature-runtime" {
		t.Fatalf("worktree = %q", resolved.Worktree)
	}
	if resolved.EnvVars["PROJECT"] != "agent-factory" || resolved.EnvVars["BRANCH"] != "feature-runtime" {
		t.Fatalf("env vars = %#v", resolved.EnvVars)
	}
}

func TestWorkstationExecutor_ResolvesRelativeWorkingDirectoryAgainstRuntimeConfigFactoryDirectory(t *testing.T) {
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath: wantDir,
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: ".",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-relative",
		TransitionID:    "t-relative",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != wantDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantDir)
	}
	if mock.dispatch.UserMessage != "Work from "+wantDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_ResolvesRelativeWorkingDirectoryAgainstRuntimeBaseDirectoryOverride(t *testing.T) {
	factoryDir := t.TempDir()
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath:     factoryDir,
			RuntimeBasePath: wantDir,
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: ".",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-relative-runtime-base",
		TransitionID:    "t-relative-runtime-base",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != wantDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantDir)
	}
	if mock.dispatch.UserMessage != "Work from "+wantDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_ResolvesPortableRootedWorkingDirectoryAgainstRuntimeBaseDirectoryOverride(t *testing.T) {
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			RuntimeBasePath: wantDir,
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: "/worktrees/feature-abc",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-portable-rooted",
		TransitionID:    "t-portable-rooted",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	expectedDir := filepath.Join(wantDir, "worktrees", "feature-abc")
	if mock.dispatch.WorkingDirectory != expectedDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, expectedDir)
	}
	if mock.dispatch.UserMessage != "Work from "+expectedDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_PreservesExistingUnixAbsoluteWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix absolute path semantics do not apply on Windows")
	}
	absoluteDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			RuntimeBasePath: t.TempDir(),
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: filepath.ToSlash(absoluteDir),
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-unix-absolute",
		TransitionID:    "t-unix-absolute",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != filepath.Clean(absoluteDir) {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, filepath.Clean(absoluteDir))
	}
	if mock.dispatch.UserMessage != "Work from "+filepath.Clean(absoluteDir) {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_LoadedRuntimeConfigRuntimeBaseDirOverrideDrivesRelativeExecutionPath(t *testing.T) {
	factoryDir := t.TempDir()
	runtimeBaseDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())
	writeRuntimeLookupFixture(t, factoryDir)

	runtimeCfg, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	runtimeCfg.SetRuntimeBaseDir(runtimeBaseDir)

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(runtimeCfg, mock)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-loaded-runtime-base",
		TransitionID:    "t-loaded-runtime-base",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		ProjectID:       "agent-factory",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, filepath.Join(runtimeBaseDir, "workspace"))
	}
	if mock.dispatch.UserMessage != "Work from "+filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func setTestWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})
}

func writeRuntimeLookupFixture(t *testing.T, factoryDir string) {
	t.Helper()

	writeRuntimeLookupFactoryJSON(t, factoryDir, map[string]any{
		"id": "agent-factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":    "standard",
				"worker":  "worker-a",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			},
		},
	})
	writeRuntimeLookupAgentsMD(t, filepath.Join(factoryDir, "workers", "worker-a"), `---
type: MODEL_WORKER
model: gpt-5.4
---
System prompt.
`)
	writeRuntimeLookupAgentsMD(t, filepath.Join(factoryDir, "workstations", "standard"), `---
type: MODEL_WORKSTATION
worker: worker-a
workingDirectory: workspace
---
Work from {{ .Context.WorkDir }}
`)
}

func writeRuntimeLookupFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = filepath.Base(factoryDir)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeRuntimeLookupAgentsMD(t *testing.T, dir string, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryAgentsFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func canonicalWorkstationRuntimeConfig(factoryDir string) staticRuntimeConfig {
	return staticRuntimeConfig{
		FactoryPath: factoryDir,
		Workers: map[string]*interfaces.WorkerConfig{
			"canonical-worker": {Type: interfaces.WorkerTypeModel, Body: "canonical system", Timeout: "1h"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Type:             interfaces.WorkstationTypeModel,
				WorkerTypeName:   "canonical-worker",
				PromptTemplate:   `Review {{ (index .Inputs 0).WorkID }} for {{ .Context.Project }}`,
				OutputSchema:     `{"type":"object"}`,
				Limits:           interfaces.WorkstationLimits{MaxExecutionTime: "75ms"},
				StopWords:        []string{"DONE"},
				WorkingDirectory: `/repo/{{ index (index .Inputs 0).Tags "branch" }}`,
				Worktree:         `worktrees/{{ index (index .Inputs 0).Tags "branch" }}`,
				Env: map[string]string{
					"PROJECT": "{{ .Context.Project }}",
					"BRANCH":  `{{ index (index .Inputs 0).Tags "branch" }}`,
				},
			},
		},
	}
}

func canonicalWorkstationDispatch() interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      "d-canonical",
		TransitionID:    "t-review",
		WorkerType:      "stale-worker",
		WorkstationName: "review",
		ProjectID:       "agent-factory",
		InputTokens: InputTokens(interfaces.Token{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"branch": "feature-runtime"},
			},
		}),
	}
}

func TestWorkstationExecutor_LogicalMove_DoesNotCallExecutor(t *testing.T) {
	mock := &wsMockExecutor{}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical": {Type: interfaces.WorkstationTypeLogical},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{DispatchID: "d-1", TransitionID: "t-logical", WorkstationName: "logical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called")
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
}

func TestWorkstationExecutor_ExecutorError_ReturnsFailedResult(t *testing.T) {
	mock := &wsMockExecutor{err: errors.New("connection timeout")}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "executor failed: connection timeout" {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestWorkstationExecutor_ClassifierTrimsLabelAndIgnoresNonFailureOutcomeKinds(t *testing.T) {
	mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Output: "  approved \n"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-classifier-trim",
		TransitionID:    "t-classifier-trim",
		WorkerType:      "worker-a",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if result.Output != "approved" {
		t.Fatalf("Output = %q, want approved", result.Output)
	}
}

func TestWorkstationExecutor_ClassifierRejectsJSONStringLabel(t *testing.T) {
	mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "\"needs_review\""}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-classifier-json-string",
		TransitionID:    "t-classifier-json-string",
		WorkerType:      "worker-a",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != `classifier output invalid: expected plain string label (raw output: "\"needs_review\"")` {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestWorkstationExecutor_ClassifierRejectsEmptyOrNonStringOutput(t *testing.T) {
	testCases := []struct {
		name   string
		output string
	}{
		{name: "empty", output: " \n\t "},
		{name: "json object", output: `{"label":"approved"}`},
		{name: "json number", output: `123`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: tc.output}}
			we := newTestWorkstationExecutor(
				staticRuntimeConfig{
					Workers: map[string]*interfaces.WorkerConfig{
						"worker-a": {Body: "system"},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
					},
				},
				mock,
			)

			result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
				DispatchID:      "d-classifier-invalid",
				TransitionID:    "t-classifier-invalid",
				WorkerType:      "worker-a",
				WorkstationName: "classifier",
				InputTokens:     InputTokens(interfaces.Token{ID: "tok-1"}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Outcome != interfaces.OutcomeFailed {
				t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
			}
			if !strings.HasPrefix(result.Error, "classifier output invalid:") {
				t.Fatalf("Error = %q, want classifier output invalid prefix", result.Error)
			}
			if strings.TrimSpace(tc.output) != "" && !strings.Contains(result.Error, "raw output:") {
				t.Fatalf("Error = %q, want raw output evidence", result.Error)
			}
		})
	}
}

func TestWorkstationExecutor_PromptRenderFailure_ReturnsFailedResult(t *testing.T) {
	mock := &wsMockExecutor{}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"broken": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "{{ .InvalidSyntax"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-prompt-failure",
		TransitionID:    "t-prompt-failure",
		WorkerType:      "worker-a",
		WorkstationName: "broken",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called when prompt rendering fails")
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if !strings.HasPrefix(result.Error, "prompt render failed:") {
		t.Fatalf("Error = %q, want prompt render failed prefix", result.Error)
	}
}

func TestWorkstationExecutor_ResolvesWorkerAndWorkstationPerDispatch(t *testing.T) {
	mock := &wsMockExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system-a"},
				"worker-b": {Body: "system-b"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"review-a": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Review {{ (index .Inputs 0).WorkID }}"},
				"review-b": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Inspect {{ (index .Inputs 0).WorkID }}"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	first, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "review-a",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, interfaces.OutcomeAccepted)
	}
	if got := mock.dispatch.SystemPrompt; got != "system-a" {
		t.Fatalf("first system prompt = %q", got)
	}
	if got := mock.dispatch.UserMessage; got != "Review work-1" {
		t.Fatalf("first user message = %q", got)
	}

	second, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-2",
		TransitionID:    "t-2",
		WorkerType:      "worker-b",
		WorkstationName: "review-b",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-2", Color: interfaces.TokenColor{WorkID: "work-2"}}),
	})
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, interfaces.OutcomeAccepted)
	}
	if got := mock.dispatch.SystemPrompt; got != "system-b" {
		t.Fatalf("second system prompt = %q", got)
	}
	if got := mock.dispatch.UserMessage; got != "Inspect work-2" {
		t.Fatalf("second user message = %q", got)
	}
}

func TestResolveModelOperationBindings_UsesInputThenConfigThenDefaultAndRecordsSource(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{
			{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Selector: &interfaces.ModelOperationBindingSelector{
					Role: "voice",
				},
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
			{
				Slot: "style",
				DefaultContent: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "neutral",
					Slot: "style",
				}},
			},
		},
	}
	worker := &interfaces.WorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", Required: true},
				{Name: "voice"},
				{Name: "style"},
				{Name: "optional"},
			},
		}},
	}
	inputs := []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Slot: "ignored", Label: "utterance", Text: "first"},
				{Type: interfaces.WorkContentPartTypeText, Slot: "text", Label: "utterance", Text: "second"},
			},
		},
	}}

	got, err := resolveModelOperationBindings(workstation, worker, inputs)
	if err != nil {
		t.Fatalf("resolveModelOperationBindings: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("binding count = %d, want 4", len(got))
	}
	if got[0].Source != interfaces.ModelOperationBindingSourceInput || got[0].Content[0].Text != "first" {
		t.Fatalf("text binding = %#v, want first input match", got[0])
	}
	if got[1].Source != interfaces.ModelOperationBindingSourceConfig || string(got[1].Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want config fallback", got[1])
	}
	if got[2].Source != interfaces.ModelOperationBindingSourceDefault || got[2].Content[0].Text != "neutral" {
		t.Fatalf("style binding = %#v, want default fallback", got[2])
	}
	if got[3].Source != interfaces.ModelOperationBindingSourceOmitted || len(got[3].Content) != 0 {
		t.Fatalf("optional binding = %#v, want omitted", got[3])
	}
}

func TestResolveModelOperationBindings_ImplicitlyMatchesBySlotAndRejectsMissingRequiredInput(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
	}
	worker := &interfaces.WorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", Required: true},
			},
		}},
	}

	got, err := resolveModelOperationBindings(workstation, worker, []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Slot: "text",
				Text: "hello",
			}},
		},
	}})
	if err != nil {
		t.Fatalf("resolveModelOperationBindings implicit slot: %v", err)
	}
	if len(got) != 1 || got[0].Source != interfaces.ModelOperationBindingSourceInput || got[0].Content[0].Text != "hello" {
		t.Fatalf("implicit slot binding = %#v, want input text", got)
	}

	_, err = resolveModelOperationBindings(workstation, worker, nil)
	if err == nil {
		t.Fatal("expected missing required input slot to fail")
	}
}
