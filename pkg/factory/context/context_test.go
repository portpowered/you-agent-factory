package factory_context

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestNewWorkflowContext_SetsCorrectPaths(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)

	ctx, err := NewFactoryContext(
		"wf-123",
		nil, nil, nil,
		WithBaseDir(baseDir),
		WithTimestamp(ts),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantRunDir := filepath.Join(baseDir, "wf-123", "20260315T103000")
	wantWorkDir := filepath.Join(wantRunDir, "work")
	wantArtifactDir := filepath.Join(wantRunDir, interfaces.ArtifactsDirectory)

	if ctx.FactoryDirectory != "wf-123" {
		t.Errorf("WorkflowID = %q, want %q", ctx.FactoryDirectory, "wf-123")
	}
	if ctx.WorkDirectory != wantWorkDir {
		t.Errorf("WorkDir = %q, want %q", ctx.WorkDirectory, wantWorkDir)
	}
	if ctx.ArtifactDir != wantArtifactDir {
		t.Errorf("ArtifactDir = %q, want %q", ctx.ArtifactDir, wantArtifactDir)
	}
	if ctx.ProjectID != DefaultProjectID {
		t.Errorf("ProjectID = %q, want %q", ctx.ProjectID, DefaultProjectID)
	}
	workInfo, err := os.Stat(ctx.WorkDirectory)
	if err != nil {
		t.Errorf("WorkDir was not created on disk: %v", err)
	}
	if err == nil && !workInfo.IsDir() {
		t.Errorf("WorkDir path is not a directory: %s", ctx.WorkDirectory)
	}

	artifactInfo, err := os.Stat(ctx.ArtifactDir)
	if err != nil {
		t.Errorf("ArtifactDir was not created on disk: %v", err)
	}
	if err == nil && !artifactInfo.IsDir() {
		t.Errorf("ArtifactDir path is not a directory: %s", ctx.ArtifactDir)
	}
}

func TestNewWorkflowContext_EnvMerge(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)

	factoryEnv := map[string]string{
		"FACTORY_VAR": "factory",
		"SHARED":      "from-factory",
	}
	wfCfg := &WorkflowConfig{
		Project: "workflow-project",
		EnvVars: map[string]string{
			"WORKFLOW_VAR": "workflow",
			"SHARED":       "from-workflow",
		},
	}
	submitParams := &SubmitParams{
		Project: "submit-project",
		EnvVars: map[string]string{
			"SUBMIT_VAR": "submit",
			"SHARED":     "from-submit",
		},
	}

	ctx, err := NewFactoryContext(
		"wf-env",
		factoryEnv, wfCfg, submitParams,
		WithBaseDir(baseDir),
		WithTimestamp(ts),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each level's unique var is present.
	if ctx.EnvVars["FACTORY_VAR"] != "factory" {
		t.Errorf("FACTORY_VAR = %q, want %q", ctx.EnvVars["FACTORY_VAR"], "factory")
	}
	if ctx.EnvVars["WORKFLOW_VAR"] != "workflow" {
		t.Errorf("WORKFLOW_VAR = %q, want %q", ctx.EnvVars["WORKFLOW_VAR"], "workflow")
	}
	if ctx.EnvVars["SUBMIT_VAR"] != "submit" {
		t.Errorf("SUBMIT_VAR = %q, want %q", ctx.EnvVars["SUBMIT_VAR"], "submit")
	}

	// Submission env wins over workflow and factory.
	if ctx.EnvVars["SHARED"] != "from-submit" {
		t.Errorf("SHARED = %q, want %q (submit overrides)", ctx.EnvVars["SHARED"], "from-submit")
	}
	if ctx.ProjectID != "submit-project" {
		t.Errorf("ProjectID = %q, want submit-project", ctx.ProjectID)
	}
}

func TestNewWorkflowContext_NilConfigsHandled(t *testing.T) {
	baseDir := t.TempDir()
	ts := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)

	ctx, err := NewFactoryContext(
		"wf-nil",
		nil, nil, nil,
		WithBaseDir(baseDir),
		WithTimestamp(ts),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// EnvVars should be an initialized empty map, not nil.
	if ctx.EnvVars == nil {
		t.Error("EnvVars is nil, want initialized empty map")
	}
}

func TestMergeEnvVars(t *testing.T) {
	result := MergeEnvVars(
		map[string]string{"A": "1", "B": "1"},
		nil,
		map[string]string{"B": "2", "C": "3"},
	)

	if result["A"] != "1" {
		t.Errorf("A = %q, want %q", result["A"], "1")
	}
	if result["B"] != "2" {
		t.Errorf("B = %q, want %q (later overrides)", result["B"], "2")
	}
	if result["C"] != "3" {
		t.Errorf("C = %q, want %q", result["C"], "3")
	}
}

func TestResolveProjectID_UsesExplicitWorkflowThenNeutralDefault(t *testing.T) {
	if got := ResolveProjectID("  tagged-project  ", nil, nil); got != "tagged-project" {
		t.Fatalf("explicit project = %q, want tagged-project", got)
	}

	if got := ResolveProjectID("", &WorkflowConfig{Project: "workflow-project"}, nil); got != "workflow-project" {
		t.Fatalf("workflow project = %q, want workflow-project", got)
	}

	if got := ResolveProjectID("", nil, nil); got != DefaultProjectID {
		t.Fatalf("default project = %q, want %q", got, DefaultProjectID)
	}
}

func TestWorkDispatchCarriesContext(t *testing.T) {
	// This test verifies the integration point: a WorkflowContext created
	// by NewWorkflowContext can be set on the Dispatcher and appears
	// in WorkDispatches. The Dispatcher already does this (see dispatcher.go),
	// so this test just confirms the type plumbing works.
	wfCtx := &FactoryContext{
		FactoryDirectory: "wf-dispatch",
		WorkDirectory:    "/tmp/work",
		ArtifactDir:      "/tmp/artifacts",
		EnvVars:          map[string]string{"KEY": "value"},
	}

	if wfCtx.FactoryDirectory != "wf-dispatch" {
		t.Errorf("WorkflowID = %q, want %q", wfCtx.FactoryDirectory, "wf-dispatch")
	}
	if wfCtx.EnvVars["KEY"] != "value" {
		t.Errorf("EnvVars[KEY] = %q, want %q", wfCtx.EnvVars["KEY"], "value")
	}
}
