package workers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	factory_context "github.com/portpowered/agent-factory/pkg/factory/context"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestResolveTemplateFields_WorkingDirectory(t *testing.T) {
	tokens := []interfaces.Token{
		{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"branch": "feature-xyz", "worktree": "/tmp/wt-1"},
			},
		},
	}

	resolved, err := ResolveTemplateFields(
		`/worktrees/{{ index (index .Inputs 0).Tags "branch" }}`,
		nil,
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.WorkingDirectory != "/worktrees/feature-xyz" {
		t.Errorf("expected /worktrees/feature-xyz, got %s", resolved.WorkingDirectory)
	}
}

func TestResolveTemplateFields_Env(t *testing.T) {
	tokens := []interfaces.Token{
		{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"project": "inventory-service", "branch": "main"},
			},
		},
	}

	resolved, err := ResolveTemplateFields(
		"",
		map[string]string{
			"PROJECT_NAME": `{{ index (index .Inputs 0).Tags "project" }}`,
			"GIT_BRANCH":   `{{ index (index .Inputs 0).Tags "branch" }}`,
			"STATIC_VAR":   "literal-value",
		},
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.WorkingDirectory != "" {
		t.Errorf("expected empty working directory, got %s", resolved.WorkingDirectory)
	}

	if resolved.Env["PROJECT_NAME"] != "inventory-service" {
		t.Errorf("expected inventory-service, got %s", resolved.Env["PROJECT_NAME"])
	}
	if resolved.Env["GIT_BRANCH"] != "main" {
		t.Errorf("expected main, got %s", resolved.Env["GIT_BRANCH"])
	}
	if resolved.Env["STATIC_VAR"] != "literal-value" {
		t.Errorf("expected literal-value, got %s", resolved.Env["STATIC_VAR"])
	}
}

func TestResolveTemplateFields_ProjectVariableUsesTag(t *testing.T) {
	tokens := []interfaces.Token{
		{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags: map[string]string{
					"branch":  "feature/runtime-project",
					"project": "billing-api",
				},
			},
		},
	}

	resolved, err := ResolveTemplateFields(
		`/workspaces/{{ (index .Inputs 0).Project }}/{{ index (index .Inputs 0).Tags "branch" }}`,
		map[string]string{
			"PROJECT": "{{ (index .Inputs 0).Project }}",
		},
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.WorkingDirectory != "/workspaces/billing-api/feature/runtime-project" {
		t.Errorf("expected billing-api working directory, got %s", resolved.WorkingDirectory)
	}
	if resolved.Env["PROJECT"] != "billing-api" {
		t.Errorf("expected PROJECT=billing-api, got %s", resolved.Env["PROJECT"])
	}
}

func TestResolveTemplateFields_ProjectVariableFallsBackToContextThenNeutralDefault(t *testing.T) {
	tokens := []interfaces.Token{
		{ID: "tok-1", Color: interfaces.TokenColor{
			WorkID: "work-1",
			Tags: map[string]string{
				factory_context.ProjectTagKey: "token-project",
			},
		}},
	}

	resolved, err := ResolveTemplateFields(
		"",
		map[string]string{
			"PROJECT":         "{{ .Context.Project }}",
			"CONTEXT_PROJECT": "{{ .Context.Project }}",
		},
		tokens,
		&factory_context.FactoryContext{ProjectID: "analytics-platform"},
	)
	if err != nil {
		t.Fatalf("unexpected context fallback error: %v", err)
	}
	if resolved.Env["PROJECT"] != "analytics-platform" {
		t.Fatalf("expected explicit context project to win, got %s", resolved.Env["PROJECT"])
	}
	if resolved.Env["CONTEXT_PROJECT"] != "analytics-platform" {
		t.Fatalf("expected context project in template context, got %s", resolved.Env["CONTEXT_PROJECT"])
	}

	tokens[0].Color.Tags = nil
	resolved, err = ResolveTemplateFields(
		"",
		map[string]string{
			"PROJECT":         "{{ .Context.Project }}",
			"CONTEXT_PROJECT": "{{ .Context.Project }}",
		},
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected neutral fallback error: %v", err)
	}
	if resolved.Env["PROJECT"] != factory_context.DefaultProjectID {
		t.Fatalf("expected neutral project fallback %q, got %s", factory_context.DefaultProjectID, resolved.Env["PROJECT"])
	}
	if resolved.Env["CONTEXT_PROJECT"] != factory_context.DefaultProjectID {
		t.Fatalf("expected neutral context project fallback %q, got %s", factory_context.DefaultProjectID, resolved.Env["CONTEXT_PROJECT"])
	}
}

func TestResolveTemplateFields_InvalidTemplate(t *testing.T) {
	tokens := []interfaces.Token{
		{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}},
	}

	_, err := ResolveTemplateFields(
		`{{ .InvalidSyntax`,
		nil,
		tokens,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "working_directory") {
		t.Errorf("error should mention working_directory: %s", err.Error())
	}
}

func TestResolveTemplateFields_MissingTagKey(t *testing.T) {
	tokens := []interfaces.Token{
		{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"existing": "value"},
			},
		},
	}

	// index returns empty string for missing keys, not an error — this is Go map behavior.
	resolved, err := ResolveTemplateFields(
		`/worktrees/{{ index (index .Inputs 0).Tags "missing_key" }}`,
		nil,
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The template resolves to "/worktrees/" since the missing key returns empty string.
	if resolved.WorkingDirectory != "/worktrees/" {
		t.Errorf("expected /worktrees/, got %s", resolved.WorkingDirectory)
	}
}

func TestResolveTemplateFields_WorkIDAndPayload(t *testing.T) {
	tokens := []interfaces.Token{
		{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID:     "work-42",
				WorkTypeID: "stories",
				Payload:    []byte("some payload"),
			},
		},
	}

	resolved, err := ResolveTemplateFields(
		`/work/{{ (index .Inputs 0).WorkID }}/{{ (index .Inputs 0).WorkTypeID }}`,
		map[string]string{
			"WORK_ID": "{{ (index .Inputs 0).WorkID }}",
		},
		tokens,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.WorkingDirectory != "/work/work-42/stories" {
		t.Errorf("expected /work/work-42/stories, got %s", resolved.WorkingDirectory)
	}
	if resolved.Env["WORK_ID"] != "work-42" {
		t.Errorf("expected work-42, got %s", resolved.Env["WORK_ID"])
	}
}

func TestResolveTemplateFields_EnvInvalidTemplate(t *testing.T) {
	tokens := []interfaces.Token{
		{ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"}},
	}

	_, err := ResolveTemplateFields(
		"",
		map[string]string{
			"GOOD": "static",
			"BAD":  `{{ .BadSyntax`,
		},
		tokens,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid env template")
	}
	if !strings.Contains(err.Error(), "env[BAD]") {
		t.Errorf("error should mention env[BAD]: %s", err.Error())
	}
}

func TestApplyResolvedFields_OverridesWorkingDirectory(t *testing.T) {
	base := &factory_context.FactoryContext{
		WorkDirectory: "/original/path",
		EnvVars:       map[string]string{"EXISTING": "keep"},
	}

	resolved := &ResolvedFields{
		WorkingDirectory: "/resolved/path",
		Env:              map[string]string{"NEW_VAR": "new-value"},
	}

	result := applyResolvedFields(base, resolved)

	if result.WorkDirectory != "/resolved/path" {
		t.Errorf("expected /resolved/path, got %s", result.WorkDirectory)
	}
	// Original context should not be mutated.
	if base.WorkDirectory != "/original/path" {
		t.Error("original context was mutated")
	}
	// Existing env vars should be preserved.
	if result.EnvVars["EXISTING"] != "keep" {
		t.Errorf("existing env var lost: %v", result.EnvVars)
	}
	if result.EnvVars["NEW_VAR"] != "new-value" {
		t.Errorf("new env var not set: %v", result.EnvVars)
	}
	if _, ok := base.EnvVars["NEW_VAR"]; ok {
		t.Errorf("original env vars were mutated: %v", base.EnvVars)
	}
	result.EnvVars["EXISTING"] = "changed"
	if base.EnvVars["EXISTING"] != "keep" {
		t.Errorf("original env vars share result map: %v", base.EnvVars)
	}
}

func TestApplyResolvedFields_NilBase(t *testing.T) {
	resolved := &ResolvedFields{
		WorkingDirectory: "/new/path",
		Env:              map[string]string{"KEY": "val"},
	}

	result := applyResolvedFields(nil, resolved)

	if result.WorkDirectory != "/new/path" {
		t.Errorf("expected /new/path, got %s", result.WorkDirectory)
	}
	if result.EnvVars["KEY"] != "val" {
		t.Errorf("expected val, got %s", result.EnvVars["KEY"])
	}
}

func TestApplyResolvedFields_NilResolved(t *testing.T) {
	base := &factory_context.FactoryContext{WorkDirectory: "/base"}
	result := applyResolvedFields(base, nil)
	if result != base {
		t.Error("expected original base returned when resolved is nil")
	}
}

func TestWorkstationExecutor_ParameterizedWorkingDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	setTestWorkingDirectory(t, projectRoot)

	mock := &wsMockExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	}

	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Name:             "standard",
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "do work",
					WorkingDirectory: `/worktrees/{{ index (index .Inputs 0).Tags "branch" }}`,
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	dispatch := interfaces.WorkDispatch{
		TransitionID: "t-1",
		WorkerType:   "worker-a",
		InputTokens: InputTokens(interfaces.Token{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"branch": "feature-abc"},
			},
		}),
		WorkstationName: "standard",
	}

	result, err := we.Execute(context.Background(), dispatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Errorf("expected ACCEPTED, got %s", result.Outcome)
	}

	// Verify the working directory was resolved and applied.
	if mock.dispatch.WorkingDirectory != filepath.Join(projectRoot, "worktrees", "feature-abc") {
		t.Fatalf("expected working directory /worktrees/feature-abc, got %q", mock.dispatch.WorkingDirectory)
	}
}

func TestWorkstationExecutor_ParameterizedEnv(t *testing.T) {
	mock := &wsMockExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	}

	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:           interfaces.WorkstationTypeModel,
					PromptTemplate: "do work",
					Env: map[string]string{
						"PROJECT": `{{ index (index .Inputs 0).Tags "project" }}`,
					},
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	dispatch := interfaces.WorkDispatch{
		TransitionID: "t-1",
		WorkerType:   "worker-a",
		InputTokens: InputTokens(interfaces.Token{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"project": "myapp"},
			},
		}),
		WorkstationName: "standard",
	}

	result, err := we.Execute(context.Background(), dispatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Errorf("expected ACCEPTED, got %s", result.Outcome)
	}

	if mock.dispatch.EnvVars["PROJECT"] != "myapp" {
		t.Errorf("expected myapp, got %s", mock.dispatch.EnvVars["PROJECT"])
	}
}

func TestWorkstationExecutor_ParameterizedFieldError(t *testing.T) {
	mock := &wsMockExecutor{}

	we := &WorkstationExecutor{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Name:             "standard",
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "do work",
					WorkingDirectory: `{{ .InvalidSyntax`,
				},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	dispatch := interfaces.WorkDispatch{
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(interfaces.Token{
			ID: "tok-1", Color: interfaces.TokenColor{WorkID: "work-1"},
		}),
	}

	result, err := we.Execute(context.Background(), dispatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.called {
		t.Fatal("executor should not be called when parameterized field resolution fails")
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Errorf("expected FAILED, got %s", result.Outcome)
	}

	if !strings.Contains(result.Error, "parameterized field resolution failed") {
		t.Errorf("error should mention parameterized field resolution: %s", result.Error)
	}
}
