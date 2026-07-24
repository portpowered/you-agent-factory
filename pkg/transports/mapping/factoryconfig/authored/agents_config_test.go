package authored

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func containsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}

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

func TestLoadWorkerConfig_PreservesExtensionModelProviderFromYAMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKER
modelProvider: customer.provider-v2
---

Use the registered extension provider.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkerConfig: %v", err)
	}
	if got := cfg.ModelProvider; got != "customer.provider-v2" {
		t.Fatalf("modelProvider = %q, want preserved extension identity", got)
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

func TestLoadWorkstationConfig_NormalizesSameTraceIDInputGuard(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: LOGICAL_MOVE
inputs:
  - workType: planItem
    state: ready
  - workType: taskItem
    state: ready
    guard:
      type: SAME_TRACE_ID
      matchInput: planItem
outputs:
  - workType: taskItem
    state: matched
---

Join plan and task items by authored trace identity.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Inputs) != 2 || cfg.Inputs[1].Guard == nil {
		t.Fatalf("expected same-trace guard to load, got %#v", cfg.Inputs)
	}
	if cfg.Inputs[1].Guard.Type != interfaces.GuardTypeSameTraceID || cfg.Inputs[1].Guard.MatchInput != "planItem" {
		t.Fatalf("expected same-trace guard to normalize, got %#v", cfg.Inputs[1].Guard)
	}
	if cfg.Inputs[1].Guard.ParentInput != "" || cfg.Inputs[1].Guard.SpawnedBy != "" {
		t.Fatalf("expected same-trace guard to keep parent-aware fields empty, got %#v", cfg.Inputs[1].Guard)
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

func TestLoadWorkerConfig_OpenCodeAgent(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: opencode
openCodeAgent: reviewer
---

Use the reviewer agent profile.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OpenCodeAgent != "reviewer" {
		t.Fatalf("OpenCodeAgent = %q, want reviewer", cfg.OpenCodeAgent)
	}
}

func TestLoadWorkerConfig_RejectsBlankOpenCodeAgent(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: opencode
openCodeAgent: "   "
---
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkerConfig(dir)
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent must be a non-empty string") {
		t.Fatalf("expected blank openCodeAgent validation error, got %v", err)
	}
}

func TestLoadWorkstationConfig_OpenCodeAgent(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: swe
openCodeAgent: implementer
---
Workstation prompt.
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OpenCodeAgent != "implementer" {
		t.Fatalf("OpenCodeAgent = %q, want implementer", cfg.OpenCodeAgent)
	}
}

func TestLoadWorkstationConfig_RejectsBlankOpenCodeAgent(t *testing.T) {
	dir := t.TempDir()
	agentsMD := `---
type: MODEL_WORKSTATION
worker: swe
openCodeAgent: ""
---
`
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkstationConfig(dir)
	if err == nil || !strings.Contains(err.Error(), "openCodeAgent must be a non-empty string") {
		t.Fatalf("expected blank openCodeAgent validation error, got %v", err)
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
