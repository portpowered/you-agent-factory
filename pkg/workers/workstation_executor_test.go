package workers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

type wsMockExecutor struct {
	dispatch interfaces.WorkstationExecutionRequest
	called   bool
	result   interfaces.WorkResult
	err      error
}

type deadlineCapturingExecutor struct {
	deadline    time.Time
	hasDeadline bool
}

type dispatchCapturingExecutor struct {
	dispatch    interfaces.WorkstationExecutionRequest
	called      bool
	deadline    time.Time
	hasDeadline bool
	result      interfaces.WorkResult
	err         error
}

type contextBlockingExecutor struct{}

func (m *wsMockExecutor) Execute(_ context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.called = true
	m.dispatch = d
	return m.result, m.err
}

func (m *deadlineCapturingExecutor) Execute(ctx context.Context, _ interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.deadline, m.hasDeadline = ctx.Deadline()
	return interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}, nil
}

func (m *dispatchCapturingExecutor) Execute(ctx context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.called = true
	m.dispatch = d
	m.deadline, m.hasDeadline = ctx.Deadline()
	return m.result, m.err
}

func (m *contextBlockingExecutor) Execute(ctx context.Context, _ interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	<-ctx.Done()
	return interfaces.WorkResult{}, ctx.Err()
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
		"project": "agent-factory",
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

func TestWorkstationExecutor_AppliesWorkstationExecutionTimeout(t *testing.T) {
	mock := &wsMockExecutor{
		err: context.DeadlineExceeded,
	}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}
}

func TestWorkstationExecutor_WorkstationExecutionLimitSetsTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "worker-a",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 20*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation execution-limit range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutPrefersWorkstationLimit(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "90m"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Limits:         interfaces.WorkstationLimits{MaxExecutionTime: "50ms"},
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 20*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation timeout range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutFallsBackToWorkerTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want worker timeout range", remaining)
	}
}

func TestWorkstationExecutor_ExplicitPositiveTimeoutOverridesDefaults(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Timeout: "1h"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "75ms"}},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want explicit workstation timeout range", remaining)
	}
}

func TestWorkstationExecutor_ScriptWorkerTimeoutDefaultsToTwoHours(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"timed": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-timeout",
		TransitionID:    "t-timeout",
		WorkerType:      "script-worker",
		WorkstationName: "timed",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 119*time.Minute || remaining > 121*time.Minute {
		t.Fatalf("deadline offset = %v, want approximately 2h", remaining)
	}
}

func TestWorkstationExecutor_ZeroTimeoutDefaultsToTwoHours(t *testing.T) {
	tests := []struct {
		name              string
		workerDef         *interfaces.WorkerConfig
		workstationConfig *interfaces.FactoryWorkstationConfig
	}{
		{
			name:              "worker_zero",
			workerDef:         &interfaces.WorkerConfig{Type: interfaces.WorkerTypeScript, Timeout: "0s"},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
		},
		{
			name:              "workstation_zero",
			workerDef:         &interfaces.WorkerConfig{Type: interfaces.WorkerTypeScript},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "0s"}},
		},
		{
			name:              "legacy_timeout_alias_zero",
			workerDef:         &interfaces.WorkerConfig{Type: interfaces.WorkerTypeScript},
			workstationConfig: &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work", Timeout: "0s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &deadlineCapturingExecutor{}
			we := &WorkstationExecutor{
				RuntimeConfig: staticRuntimeConfig{
					Workers: map[string]*interfaces.WorkerConfig{
						"script-worker": tt.workerDef,
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"timed": tt.workstationConfig,
					},
				},
				Executor: mock,
				Renderer: &DefaultPromptRenderer{},
			}

			start := time.Now()
			_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
				DispatchID:      "d-timeout",
				TransitionID:    "t-timeout",
				WorkerType:      "script-worker",
				WorkstationName: "timed",
				InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !mock.hasDeadline {
				t.Fatal("expected timeout-derived deadline on executor context")
			}

			remaining := mock.deadline.Sub(start)
			if remaining < 119*time.Minute || remaining > 121*time.Minute {
				t.Fatalf("deadline offset = %v, want approximately 2h", remaining)
			}
		})
	}
}

func TestWorkstationExecutor_ModelWorkerTimeoutFallsBackToWorkerTimeout(t *testing.T) {
	mock := &deadlineCapturingExecutor{}
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "75ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.hasDeadline {
		t.Fatal("expected timeout-derived deadline on executor context")
	}

	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want worker timeout range", remaining)
	}
}

func TestWorkstationExecutor_ModelWorkerTimeoutCancelsLongRunningExecutor(t *testing.T) {
	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Timeout: "20ms"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		Executor: &contextBlockingExecutor{},
		Renderer: &DefaultPromptRenderer{},
	}

	start := time.Now()
	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-timeout",
		TransitionID:    "t-model-timeout",
		WorkerType:      "model-worker",
		WorkstationName: "standard",
		InputTokens:     InputTokens(interfaces.Token{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("execution elapsed = %v, want cancellation before 1s", elapsed)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want execution timeout", result.Error)
	}
	if result.ProviderFailure == nil || result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure = %#v, want timeout metadata", result.ProviderFailure)
	}
}
