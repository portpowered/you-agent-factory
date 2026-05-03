package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestLoadWorkerConfig_ModelWorker(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
executorProvider: LOCAL_CLAUDE
resources:
  - name: gpu:1
    capacity: 1
timeout: 10m
---

You are a software engineer. Write tests for all new code.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != interfaces.WorkerTypeModel {
		t.Errorf("expected type %s, got %s", interfaces.WorkerTypeModel, cfg.Type)
	}
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", cfg.Model)
	}
	if cfg.ModelProvider != "claude" {
		t.Errorf("expected model provider claude, got %s", cfg.ModelProvider)
	}
	if cfg.ExecutorProvider != "LOCAL_CLAUDE" {
		t.Errorf("expected executor provider LOCAL_CLAUDE, got %s", cfg.ExecutorProvider)
	}
	if len(cfg.Resources) != 1 || cfg.Resources[0].Name != "gpu:1" || cfg.Resources[0].Capacity != 1 {
		t.Errorf("expected resources [{gpu:1 1}], got %v", cfg.Resources)
	}
	if cfg.TimeoutDuration() != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %s", cfg.TimeoutDuration())
	}
	if cfg.Body != "You are a software engineer. Write tests for all new code." {
		t.Errorf("unexpected body: %q", cfg.Body)
	}
}

func TestLoadWorkerConfig_ScriptWorker(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: SCRIPT_WORKER
command: ./scripts/inpaint.py
args: ["--input", "{{input_path}}", "--output", "{{output_path}}"]
resources:
  - name: gpu:1
    capacity: 1
timeout: 30m
---

Inpainting worker. Runs the inpaint.py script.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != interfaces.WorkerTypeScript {
		t.Errorf("expected type %s, got %s", interfaces.WorkerTypeScript, cfg.Type)
	}
	if cfg.Command != "./scripts/inpaint.py" {
		t.Errorf("expected command ./scripts/inpaint.py, got %s", cfg.Command)
	}
	if len(cfg.Args) != 4 {
		t.Errorf("expected 4 args, got %d", len(cfg.Args))
	}
}

func TestLoadWorkerConfig_RejectsRetiredAliases(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		want        string
	}{
		{
			name: "provider",
			frontmatter: `type: MODEL_WORKER
model: gpt-5.4
provider: script_wrap`,
			want: "frontmatter.provider is not supported; use executorProvider",
		},
		{
			name: "model provider",
			frontmatter: `type: MODEL_WORKER
model: gpt-5.4
model_provider: claude`,
			want: "frontmatter.model_provider is not supported; use modelProvider",
		},
		{
			name: "session id",
			frontmatter: `type: MODEL_WORKER
model: gpt-5.4
session_id: retired-session`,
			want: "frontmatter.session_id is not supported; remove sessionId; provider sessions are runtime-owned",
		},
		{
			name: "stop token",
			frontmatter: `type: MODEL_WORKER
model: gpt-5.4
stop_token: COMPLETE`,
			want: "frontmatter.stop_token is not supported; use stopToken",
		},
		{
			name: "skip permissions",
			frontmatter: `type: MODEL_WORKER
model: gpt-5.4
skip_permissions: true`,
			want: "frontmatter.skip_permissions is not supported; use skipPermissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			agentsMD := "---\n" + tt.frontmatter + "\n---\nRejected alias.\n"
			if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadWorkerConfig(dir)
			if err == nil {
				t.Fatal("expected retired worker alias to be rejected")
			}
			if got := err.Error(); got == "" || !containsAll(got, tt.want) {
				t.Fatalf("expected %q in error, got %v", tt.want, err)
			}
		})
	}
}

func TestLoadWorkstationConfig_ModelWorkstation(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: swe
limits:
  maxRetries: 3
  maxExecutionTime: 30m
resources:
  - name: reviewer-slot
    capacity: 1
stopWords:
  - "<COMPLETE>"
---

At this workstation, you write design documents.

Given the following request:
{{ .Payload }}
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != interfaces.WorkstationTypeModel {
		t.Errorf("expected type %s, got %s", interfaces.WorkstationTypeModel, cfg.Type)
	}
	if cfg.WorkerTypeName != "swe" {
		t.Errorf("expected worker swe, got %s", cfg.WorkerTypeName)
	}
	if cfg.Limits.MaxRetries != 3 {
		t.Errorf("expected maxRetries 3, got %d", cfg.Limits.MaxRetries)
	}
	if cfg.Limits.MaxExecutionTime != "30m" {
		t.Errorf("expected maxExecutionTime 30m, got %s", cfg.Limits.MaxExecutionTime)
	}
	if cfg.Timeout != "" {
		t.Errorf("expected timeout field to remain empty, got %s", cfg.Timeout)
	}
	if len(cfg.Resources) != 1 || cfg.Resources[0].Name != "reviewer-slot" || cfg.Resources[0].Capacity != 1 {
		t.Fatalf("expected canonical resources [{reviewer-slot 1}], got %#v", cfg.Resources)
	}
	if len(cfg.StopWords) != 1 || cfg.StopWords[0] != "<COMPLETE>" {
		t.Fatalf("expected canonical stopWords [<COMPLETE>], got %#v", cfg.StopWords)
	}
	// PromptTemplate should be the body since no PromptFile was specified.
	if cfg.PromptTemplate == "" {
		t.Error("expected non-empty prompt template from body")
	}
}

func TestLoadWorkstationConfig_PreservesNonSuccessRouteArrays(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: swe
inputs:
  - workType: story
    state: init
outputs:
  - workType: story
    state: complete
onContinue:
  - workType: story
    state: retry
  - workType: story
    state: complete
onRejection:
  - workType: story
    state: init
  - workType: story
    state: review
onFailure:
  - workType: story
    state: failed
  - workType: story
    state: review
---

Route work based on execution outcome.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(cfg.OnContinue); got != 2 {
		t.Fatalf("onContinue length = %d, want 2", got)
	}
	if got := len(cfg.OnRejection); got != 2 {
		t.Fatalf("onRejection length = %d, want 2", got)
	}
	if got := len(cfg.OnFailure); got != 2 {
		t.Fatalf("onFailure length = %d, want 2", got)
	}
	if cfg.OnContinue[0].StateName != "retry" || cfg.OnContinue[1].StateName != "complete" {
		t.Fatalf("unexpected onContinue routes: %#v", cfg.OnContinue)
	}
	if cfg.OnRejection[0].StateName != "init" || cfg.OnRejection[1].StateName != "review" {
		t.Fatalf("unexpected onRejection routes: %#v", cfg.OnRejection)
	}
	if cfg.OnFailure[0].StateName != "failed" || cfg.OnFailure[1].StateName != "review" {
		t.Fatalf("unexpected onFailure routes: %#v", cfg.OnFailure)
	}
}

type workstationRetiredAliasCase struct {
	name        string
	frontmatter string
	want        string
}

var workstationRetiredAliasCases = []workstationRetiredAliasCase{
	{
		name: "runtime type",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
runtime_type: MODEL_WORKSTATION`,
		want: "frontmatter.runtime_type is not supported; use type",
	},
	{
		name: "timeout",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
timeout: 30m`,
		want: "frontmatter.timeout is not supported; use limits.maxExecutionTime",
	},
	{
		name: "stop token",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
stop_token: DONE`,
		want: "frontmatter.stop_token is not supported; use stopWords",
	},
	{
		name: "resource usage",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
resource_usage:
  - reviewer-slot`,
		want: "frontmatter.resource_usage is not supported; use resources",
	},
	{
		name: "prompt file",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
prompt_file: prompt.md`,
		want: "frontmatter.prompt_file is not supported; use promptFile",
	},
	{
		name: "on continue alias",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
on_continue:
  - workType: story
    state: retry`,
		want: "frontmatter.on_continue is not supported; use onContinue",
	},
	{
		name: "cron trigger at start",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
cron:
  schedule: "*/5 * * * *"
  trigger_at_start: true`,
		want: "frontmatter.cron.trigger_at_start is not supported; use triggerAtStart",
	},
	{
		name: "cron expiry window",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
cron:
  schedule: "*/5 * * * *"
  expiry_window: 45s`,
		want: "frontmatter.cron.expiry_window is not supported; use expiryWindow",
	},
	{
		name: "input work type",
		frontmatter: `type: LOGICAL_MOVE
inputs:
  - work_type: story
    state: ready`,
		want: "frontmatter.inputs[0].work_type is not supported; use workType",
	},
	{
		name: "parent input",
		frontmatter: `type: LOGICAL_MOVE
inputs:
  - workType: story
    state: ready
    guard:
      type: ALL_CHILDREN_COMPLETE
      parent_input: parent`,
		want: "frontmatter.inputs[0].guard.parent_input is not supported; use parentInput",
	},
	{
		name: "max visits",
		frontmatter: `type: LOGICAL_MOVE
guards:
  - type: VISIT_COUNT
    workstation: swe
    max_visits: 2`,
		want: "frontmatter.guards[0].max_visits is not supported; use maxVisits",
	},
	{
		name: "on continue route entry alias",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
onContinue:
  - work_type: story
    state: retry`,
		want: "frontmatter.onContinue[0].work_type is not supported; use workType",
	},
	{
		name: "on rejection route entry alias",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
onRejection:
  - work_type: story
    state: rejected`,
		want: "frontmatter.onRejection[0].work_type is not supported; use workType",
	},
	{
		name: "on failure route entry alias",
		frontmatter: `type: MODEL_WORKSTATION
worker: swe
onFailure:
  - work_type: story
    state: failed`,
		want: "frontmatter.onFailure[0].work_type is not supported; use workType",
	},
}

func TestLoadWorkstationConfig_RejectsRetiredAliases(t *testing.T) {
	for _, tt := range workstationRetiredAliasCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assertLoadWorkstationConfigRejectsRetiredAlias(t, tt.frontmatter, tt.want)
		})
	}
}

func assertLoadWorkstationConfigRejectsRetiredAlias(t *testing.T, frontmatter string, want string) {
	t.Helper()

	dir := t.TempDir()
	agentsMD := "---\n" + frontmatter + "\n---\nRejected alias.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkstationConfig(dir)
	if err == nil {
		t.Fatal("expected retired workstation alias to be rejected")
	}
	if got := err.Error(); got == "" || !containsAll(got, want) {
		t.Fatalf("expected %q in error, got %v", want, err)
	}
}

func TestLoadWorkstationConfig_LogicalMove(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: LOGICAL_MOVE
---

Aggregation point. Collects completed work items.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != interfaces.WorkstationTypeLogical {
		t.Errorf("expected type %s, got %s", interfaces.WorkstationTypeLogical, cfg.Type)
	}
	if cfg.WorkerTypeName != "" {
		t.Errorf("expected no worker for LOGICAL_MOVE, got %s", cfg.WorkerTypeName)
	}
}

func TestLoadWorkstationConfig_NormalizesCanonicalPublicEnums(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
behavior: CRON
type: MODEL_WORKSTATION
worker: swe
guards:
  - type: VISIT_COUNT
    workstation: swe
    maxVisits: 2
inputs:
  - workType: story
    state: init
    guard:
      type: ALL_CHILDREN_COMPLETE
      parentInput: parent
---

Handle scheduled work.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Kind != interfaces.WorkstationKindCron {
		t.Fatalf("expected kind %s, got %s", interfaces.WorkstationKindCron, cfg.Kind)
	}
	if len(cfg.Guards) != 1 || cfg.Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("expected visit_count guard, got %#v", cfg.Guards)
	}
	if len(cfg.Inputs) != 1 || cfg.Inputs[0].Guard == nil || cfg.Inputs[0].Guard.Type != interfaces.GuardTypeAllChildrenComplete {
		t.Fatalf("expected all_children_complete input guard, got %#v", cfg.Inputs)
	}
}

func TestLoadWorkstationConfig_NormalizesSameNameInputGuard(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: LOGICAL_MOVE
inputs:
  - workType: planItem
    state: ready
  - workType: taskItem
    state: ready
    guard:
      type: SAME_NAME
      matchInput: planItem
outputs:
  - workType: taskItem
    state: matched
---

Join plan and task items by authored name.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Inputs) != 2 || cfg.Inputs[1].Guard == nil {
		t.Fatalf("expected same-name guard to load, got %#v", cfg.Inputs)
	}
	if cfg.Inputs[1].Guard.Type != interfaces.GuardTypeSameName || cfg.Inputs[1].Guard.MatchInput != "planItem" {
		t.Fatalf("expected same-name guard to normalize, got %#v", cfg.Inputs[1].Guard)
	}
	if cfg.Inputs[1].Guard.ParentInput != "" || cfg.Inputs[1].Guard.SpawnedBy != "" {
		t.Fatalf("expected same-name guard to keep parent-aware fields empty, got %#v", cfg.Inputs[1].Guard)
	}
}

func TestLoadWorkstationConfig_NormalizesMatchesFieldsWorkstationGuard(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: matcher
guards:
  - type: MATCHES_FIELDS
    matchConfig:
      inputKey: .Name
inputs:
  - workType: asset
    state: ready
outputs:
  - workType: asset
    state: matched
---

Match assets by resolved field.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Guards) != 1 || cfg.Guards[0].Type != interfaces.GuardTypeMatchesFields {
		t.Fatalf("expected matches-fields guard to load, got %#v", cfg.Guards)
	}
	if cfg.Guards[0].MatchConfig == nil || cfg.Guards[0].MatchConfig.InputKey != ".Name" {
		t.Fatalf("expected matches-fields matchConfig.inputKey=.Name, got %#v", cfg.Guards[0].MatchConfig)
	}
}

func TestLoadWorkstationConfig_WithPromptFile(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: swe
promptFile: prompt.md
---

This body should be ignored for prompt template.
`
	promptContent := "Custom prompt: {{ .WorkID }}"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(promptContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PromptTemplate != promptContent {
		t.Errorf("expected prompt template from file, got %q", cfg.PromptTemplate)
	}
}

func TestLoadWorkerConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadWorkerConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing AGENTS.md")
	}
}

func TestLoadWorkerConfig_InvalidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	// Missing closing delimiter
	agentsMD := `---
type: MODEL_WORKER
model: test
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkerConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing closing frontmatter delimiter")
	}
}

func TestLoadWorkerConfig_MissingOptionalFields(t *testing.T) {
	dir := t.TempDir()
	// Minimal frontmatter — only type
	agentsMD := `---
type: MODEL_WORKER
---

Minimal worker.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != interfaces.WorkerTypeModel {
		t.Errorf("expected type %s, got %s", interfaces.WorkerTypeModel, cfg.Type)
	}
	if cfg.Model != "" {
		t.Errorf("expected empty model, got %s", cfg.Model)
	}
	if cfg.TimeoutDuration() != 0 {
		t.Errorf("expected zero timeout, got %s", cfg.TimeoutDuration())
	}
	if cfg.Body != "Minimal worker." {
		t.Errorf("unexpected body: %q", cfg.Body)
	}
}
