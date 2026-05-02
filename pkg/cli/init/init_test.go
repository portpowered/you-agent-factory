package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var retiredInitFactoryJSONFields = []string{`"work_types"`, `"work_type"`, `"on_failure"`}
var retiredInitWorkerFrontmatterFields = []string{"model_provider:", "provider:", "stop_token:", "skip_permissions:", "concurrency:", "sessionId:"}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func requireContainsAll(t *testing.T, label, content string, expected []string) {
	t.Helper()

	for _, fragment := range expected {
		if !strings.Contains(content, fragment) {
			t.Fatalf("%s missing %q:\n%s", label, fragment, content)
		}
	}
}

func requireOmitsAll(t *testing.T, label, content string, disallowed []string) {
	t.Helper()

	for _, fragment := range disallowed {
		if strings.Contains(content, fragment) {
			t.Fatalf("%s should not contain %q:\n%s", label, fragment, content)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-cli-init-fixture review=2026-07-19 removal=split-directory-skeleton-and-file-content-assertions-before-next-cli-init-change
func TestInit_CreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	err := Init(InitConfig{Dir: base})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, d := range initDirs {
		path := filepath.Join(base, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}

		readmePath := filepath.Join(path, "README.md")
		if _, err := os.Stat(readmePath); err != nil {
			t.Errorf("expected README.md in %s: %v", d, err)
		}
	}

	factoryConfigPath := filepath.Join(base, "factory.json")
	data, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("expected factory.json to be created: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("factory.json is not valid JSON: %v", err)
	}
	if _, ok := cfg["workTypes"]; !ok {
		t.Error("factory.json missing 'workTypes' field")
	}
	if _, ok := cfg["work_types"]; ok {
		t.Error("factory.json should not include retired 'work_types' field")
	}
	if _, ok := cfg["workstations"]; !ok {
		t.Error("factory.json missing 'workstations' field")
	}
	factoryJSON := string(data)
	if !strings.Contains(factoryJSON, `"workType"`) {
		t.Fatalf("generated factory.json = %q, want canonical workType keys", factoryJSON)
	}
	if !strings.Contains(factoryJSON, `"onFailure"`) {
		t.Fatalf("generated factory.json = %q, want canonical onFailure key", factoryJSON)
	}
	if strings.Contains(factoryJSON, `"work_type"`) {
		t.Fatalf("generated factory.json = %q, should not contain retired work_type keys", factoryJSON)
	}
	if strings.Contains(factoryJSON, `"on_failure"`) {
		t.Fatalf("generated factory.json = %q, should not contain retired on_failure key", factoryJSON)
	}

	if _, ok := cfg["workers"]; !ok {
		t.Error("factory.json missing 'workers' field")
	}

	defaultInputDir := filepath.Join(base, "inputs", "tasks", "default")
	info, err := os.Stat(defaultInputDir)
	if err != nil {
		t.Fatalf("expected inputs/tasks/default/ to be created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected inputs/tasks/default/ to be a directory")
	}

	workerAgentsPath := filepath.Join(base, "workers", "processor", "AGENTS.md")
	if _, err := os.Stat(workerAgentsPath); os.IsNotExist(err) {
		t.Error("expected workers/processor/AGENTS.md to be created")
	}
	workerAgents, err := os.ReadFile(workerAgentsPath)
	if err != nil {
		t.Fatalf("read generated worker AGENTS.md: %v", err)
	}
	if !strings.Contains(string(workerAgents), "model: gpt-5-codex") {
		t.Fatalf("generated worker AGENTS.md = %q, want model: gpt-5-codex", string(workerAgents))
	}
	if !strings.Contains(string(workerAgents), "modelProvider: CODEX") {
		t.Fatalf("generated worker AGENTS.md = %q, want modelProvider: CODEX", string(workerAgents))
	}
	if !strings.Contains(string(workerAgents), "executorProvider: SCRIPT_WRAP") {
		t.Fatalf("generated worker AGENTS.md = %q, want executorProvider: SCRIPT_WRAP", string(workerAgents))
	}
	if strings.Contains(string(workerAgents), "model_provider: codex") {
		t.Fatalf("generated worker AGENTS.md = %q, should not contain model_provider", string(workerAgents))
	}
	if strings.Contains(string(workerAgents), "provider: script_wrap") {
		t.Fatalf("generated worker AGENTS.md = %q, should not contain retired provider", string(workerAgents))
	}
	if strings.Contains(string(workerAgents), "concurrency:") {
		t.Fatalf("generated worker AGENTS.md = %q, should not contain retired concurrency", string(workerAgents))
	}
	if !strings.Contains(string(workerAgents), "timeout: 1h") {
		t.Fatalf("generated worker AGENTS.md = %q, want timeout: 1h", string(workerAgents))
	}
	if !strings.Contains(string(workerAgents), "skipPermissions: true") {
		t.Fatalf("generated worker AGENTS.md = %q, want skipPermissions: true", string(workerAgents))
	}
	if !strings.Contains(string(workerAgents), defaultProcessorSystemBody) {
		t.Fatalf("generated worker AGENTS.md = %q, want default processor system prompt", string(workerAgents))
	}
	if strings.Contains(string(workerAgents), "skip_permissions: true") {
		t.Fatalf("generated worker AGENTS.md = %q, should not contain skip_permissions", string(workerAgents))
	}
	if strings.Contains(string(workerAgents), "timeout: 2h") {
		t.Fatal("generated worker AGENTS.md should not use the subprocess fallback as the emitted default")
	}
	assertInitScaffoldFilesCanonical(t, base, "gpt-5-codex", "codex")

	workstationAgentsPath := filepath.Join(base, "workstations", "process", "AGENTS.md")
	if _, err := os.Stat(workstationAgentsPath); os.IsNotExist(err) {
		t.Error("expected workstations/process/AGENTS.md to be created")
	}

	factoriesDir := filepath.Join(base, "factories")
	if _, err := os.Stat(factoriesDir); err == nil {
		t.Error("expected 'factories/' directory to NOT be created")
	}
}

func TestInit_ClaudeExecutorCreatesClaudeWorkerScaffold(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	if err := Init(InitConfig{Dir: base, Executor: string(StarterExecutorClaude)}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	workerAgentsPath := filepath.Join(base, "workers", "processor", "AGENTS.md")
	workerAgents, err := os.ReadFile(workerAgentsPath)
	if err != nil {
		t.Fatalf("read generated worker AGENTS.md: %v", err)
	}

	contents := string(workerAgents)
	if !strings.Contains(contents, "model: claude-sonnet-4-20250514") {
		t.Fatalf("generated worker AGENTS.md = %q, want Claude model scaffold", contents)
	}
	if !strings.Contains(contents, "modelProvider: CLAUDE") {
		t.Fatalf("generated worker AGENTS.md = %q, want modelProvider: CLAUDE", contents)
	}
	if !strings.Contains(contents, "executorProvider: SCRIPT_WRAP") {
		t.Fatalf("generated worker AGENTS.md = %q, want executorProvider: SCRIPT_WRAP", contents)
	}
	if !strings.Contains(contents, defaultProcessorSystemBody) {
		t.Fatalf("generated worker AGENTS.md = %q, want default processor system prompt", contents)
	}
	if strings.Contains(contents, "model: gpt-5-codex") {
		t.Fatalf("generated worker AGENTS.md = %q, should not include default codex model", contents)
	}
	assertInitScaffoldFilesCanonical(t, base, "claude-sonnet-4-20250514", "claude")
}

func TestInit_LoadRuntimeConfigForDefaultScaffold(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	if err := Init(InitConfig{Dir: base}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	assertInitRuntimeConfig(t, base, "gpt-5-codex", "codex")
}

func TestInit_LoadRuntimeConfigForClaudeScaffold(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	if err := Init(InitConfig{Dir: base, Executor: string(StarterExecutorClaude)}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	assertInitRuntimeConfig(t, base, "claude-sonnet-4-20250514", "claude")
}

func TestInit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	if err := Init(InitConfig{Dir: base}); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := Init(InitConfig{Dir: base}); err != nil {
		t.Fatalf("second init: %v", err)
	}
}

func TestInit_DoesNotOverwriteExistingFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	if err := Init(InitConfig{Dir: base}); err != nil {
		t.Fatalf("first init: %v", err)
	}

	factoryConfigPath := filepath.Join(base, "factory.json")
	customContent := `{"custom": true}`
	if err := os.WriteFile(factoryConfigPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("write custom factory.json: %v", err)
	}

	if err := Init(InitConfig{Dir: base}); err != nil {
		t.Fatalf("second init: %v", err)
	}

	data, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if string(data) != customContent {
		t.Errorf("factory.json was overwritten; want %q, got %q", customContent, string(data))
	}
}

// portos:func-length-exception owner=agent-factory reason=ralph-init-topology-fixture review=2026-07-21 removal=split-runtime-load-assertions-from-file-layout-checks-before-next-ralph-init-topology-change
func TestInit_RalphTypeCreatesDistinctScaffold(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "ralph-factory")

	if err := Init(InitConfig{Dir: base, Type: string(RalphScaffoldType)}); err != nil {
		t.Fatalf("Init Ralph scaffold: %v", err)
	}

	factoryConfigPath := filepath.Join(base, "factory.json")
	data, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("read Ralph factory.json: %v", err)
	}
	factoryJSON := string(data)
	for _, expected := range []string{
		`"name": "request"`,
		`"name": "story"`,
		`"name": "plan-request"`,
		`"name": "execute-story"`,
		`"name": "execute-story-loop-breaker"`,
		`"workingDirectory":`,
		`"onContinue"`,
		`"maxVisits": 8`,
	} {
		if !strings.Contains(factoryJSON, expected) {
			t.Fatalf("Ralph factory.json missing %q:\n%s", expected, factoryJSON)
		}
	}
	for _, disallowed := range []string{`"name": "tasks"`, `"name": "process"`} {
		if strings.Contains(factoryJSON, disallowed) {
			t.Fatalf("Ralph factory.json should not contain %q:\n%s", disallowed, factoryJSON)
		}
	}
	requireOmitsAll(t, "Ralph factory.json", factoryJSON, []string{`"work_type"`, `"on_failure"`, `"on_rejection"`})

	for _, path := range []string{
		filepath.Join(base, "workers", "planner", "AGENTS.md"),
		filepath.Join(base, "workers", "executor", "AGENTS.md"),
		filepath.Join(base, "workstations", "plan-request", "AGENTS.md"),
		filepath.Join(base, "workstations", "execute-story", "AGENTS.md"),
		filepath.Join(base, "inputs", RalphFactoryInputType, "default"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected Ralph scaffold path %s: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(base, "workstations", "execute-story-loop-breaker", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected topology-only loop-breaker to omit split AGENTS.md, got err=%v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(base, nil)
	if err != nil {
		t.Fatalf("generated Ralph scaffold should load through runtime config: %v", err)
	}
	if len(loaded.FactoryConfig().WorkTypes) != 2 {
		t.Fatalf("loaded Ralph scaffold workTypes = %d, want 2", len(loaded.FactoryConfig().WorkTypes))
	}
	if len(loaded.FactoryConfig().Workstations) != 3 {
		t.Fatalf("loaded Ralph scaffold workstations = %d, want 3", len(loaded.FactoryConfig().Workstations))
	}

	planner, ok := loaded.Workstation("plan-request")
	if !ok {
		t.Fatal("expected plan-request workstation to load")
	}
	if planner.Kind != "" && planner.Kind != interfaces.WorkstationKindStandard {
		t.Fatalf("plan-request kind = %q, want empty/default standard", planner.Kind)
	}
	if planner.WorkingDirectory != "." {
		t.Fatalf("plan-request workingDirectory = %q, want %q", planner.WorkingDirectory, ".")
	}

	executor, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected execute-story workstation to load")
	}
	if executor.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("execute-story kind = %q, want %q", executor.Kind, interfaces.WorkstationKindRepeater)
	}
	if executor.WorkingDirectory != "." {
		t.Fatalf("execute-story workingDirectory = %q, want %q", executor.WorkingDirectory, ".")
	}
	if executor.OnContinue == nil || executor.OnContinue.WorkTypeName != "story" || executor.OnContinue.StateName != "init" {
		t.Fatalf("execute-story onContinue = %#v, want story:init", executor.OnContinue)
	}

	loopBreaker, ok := loaded.Workstation("execute-story-loop-breaker")
	if !ok {
		t.Fatal("expected execute-story-loop-breaker workstation to load")
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop-breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("loop-breaker guards = %d, want 1", len(loopBreaker.Guards))
	}
	if guard := loopBreaker.Guards[0]; guard.Type != interfaces.GuardTypeVisitCount || guard.Workstation != "execute-story" || guard.MaxVisits != 8 {
		t.Fatalf("loop-breaker guard = %#v, want VISIT_COUNT on execute-story max 8", guard)
	}

	for _, unexpected := range []string{"review-story", "thoughts", "cron"} {
		if _, ok := loaded.Workstation(unexpected); ok {
			t.Fatalf("did not expect Ralph scaffold workstation %q", unexpected)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=ralph-init-content-contract-fixture review=2026-07-21 removal=split-worker-prompt-readme-contract-assertions-before-next-ralph-init-content-change
func TestInit_RalphScaffoldTemplatesUsePublicContractAndArtifactFlow(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "ralph-factory")

	if err := Init(InitConfig{Dir: base, Type: string(RalphScaffoldType)}); err != nil {
		t.Fatalf("Init Ralph scaffold: %v", err)
	}

	for _, workerName := range []string{"planner", "executor"} {
		workerPath := filepath.Join(base, "workers", workerName, "AGENTS.md")
		workerBody := readFileString(t, workerPath)
		requireContainsAll(t, workerPath, workerBody, []string{
			"type: MODEL_WORKER",
			"modelProvider: CODEX",
			"executorProvider: SCRIPT_WRAP",
			`stopToken: "<COMPLETE>"`,
			"skipPermissions: true",
		})
		requireOmitsAll(t, workerPath, workerBody, []string{
			"model_provider:",
			"provider:",
			"stop_token:",
			"skip_permissions:",
			"concurrency:",
			"sessionId:",
		})
	}

	planPromptPath := filepath.Join(base, "workstations", "plan-request", "AGENTS.md")
	planPrompt := readFileString(t, planPromptPath)
	requireContainsAll(t, planPromptPath, planPrompt, []string{
		`"prd.md"`,
		`"prd.json"`,
		`"progress.txt"`,
		`"branchName"`,
		"project description, requested changes, and customer intent",
		"acceptance criteria, notes",
		".Context.WorkDir",
		".Context.Project",
		`Tags "branch"`,
		`"passes: false"`,
		"product-neutral",
		`"<COMPLETE>"`,
	})

	executePromptPath := filepath.Join(base, "workstations", "execute-story", "AGENTS.md")
	executePrompt := readFileString(t, executePromptPath)
	requireContainsAll(t, executePromptPath, executePrompt, []string{
		`"prd.json"`,
		`"prd.md"`,
		`"progress.txt"`,
		"highest-priority user story",
		`"passes" is "false"`,
		`"passes: true"`,
		".Context.WorkDir",
		".Context.Project",
		`Tags "branch"`,
		`"<CONTINUE>"`,
		`"<COMPLETE>"`,
	})

	workstationsReadmePath := filepath.Join(base, "workstations", "README.md")
	workstationsReadme := readFileString(t, workstationsReadmePath)
	requireContainsAll(t, workstationsReadmePath, workstationsReadme, []string{
		"prd.md",
		"prd.json",
		"progress.txt",
		"reviewer, ideation, and cron",
	})

	inputsReadmePath := filepath.Join(base, "inputs", "README.md")
	inputsReadme := readFileString(t, inputsReadmePath)
	requireContainsAll(t, inputsReadmePath, inputsReadme, []string{
		"Example request payload",
		"document processing service",
		"product-neutral",
	})

	scaffoldReadmePath := filepath.Join(base, "README.md")
	scaffoldReadme := readFileString(t, scaffoldReadmePath)
	requireContainsAll(t, scaffoldReadmePath, scaffoldReadme, []string{
		"agent-factory init --type ralph --dir ralph-factory",
		"agent-factory run --dir ralph-factory",
		"ralph-factory/inputs/request/default/release-planning-loop.md",
		"prd.md",
		"prd.json",
		"progress.txt",
		"reviewer, thoughts or ideation, and cron",
		"<COMPLETE>",
	})
}

func TestInit_RalphTypePreservesExistingGeneratedFilesOnRerun(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "ralph-factory")

	if err := Init(InitConfig{Dir: base, Type: string(RalphScaffoldType)}); err != nil {
		t.Fatalf("Init Ralph scaffold: %v", err)
	}

	customFiles := map[string]string{
		filepath.Join(base, "README.md"):    "# Custom Ralph README\n",
		filepath.Join(base, "factory.json"): `{"custom":"factory"}`,
		filepath.Join(base, "workers", "planner", "AGENTS.md"): `---
type: MODEL_WORKER
model: custom-planner
---`,
		filepath.Join(base, "workstations", "execute-story", "AGENTS.md"): `---
type: MODEL_WORKSTATION
---
custom execute prompt
`,
		filepath.Join(base, "workstations", "README.md"): "# Custom workstation notes\n",
	}

	for path, body := range customFiles {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write custom file %s: %v", path, err)
		}
	}

	if err := Init(InitConfig{Dir: base, Type: string(RalphScaffoldType)}); err != nil {
		t.Fatalf("rerun Ralph init: %v", err)
	}

	for path, want := range customFiles {
		if got := readFileString(t, path); got != want {
			t.Fatalf("%s was overwritten on rerun; got %q want %q", path, got, want)
		}
	}
}

func TestInit_InvalidExecutorFailsBeforeFileGeneration(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "factory")

	err := Init(InitConfig{Dir: base, Executor: "invalid-provider"})
	if err == nil {
		t.Fatal("expected invalid executor to fail")
	}
	if !strings.Contains(err.Error(), `unsupported init executor "invalid-provider"`) {
		t.Fatalf("error = %q, want invalid executor message", err.Error())
	}
	if !strings.Contains(err.Error(), "codex, claude") {
		t.Fatalf("error = %q, want supported executor list", err.Error())
	}
	if _, statErr := os.Stat(base); !os.IsNotExist(statErr) {
		t.Fatalf("expected invalid executor to fail before creating files, stat err = %v", statErr)
	}
}
func TestInit_UnsupportedTypeReturnsDeterministicError(t *testing.T) {
	err := Init(InitConfig{Dir: t.TempDir(), Type: "unsupported"})
	if err == nil {
		t.Fatal("expected unsupported scaffold type to fail")
	}
	if got, want := err.Error(), `unsupported scaffold type "unsupported" (supported: default, ralph)`; got != want {
		t.Fatalf("Init error = %q, want %q", got, want)
	}
}

func assertInitRuntimeConfig(t *testing.T, base, wantModel, wantProvider string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(base, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("processor")
	if !ok {
		t.Fatal("expected processor worker definition to load")
	}
	if workerDef.Type != "MODEL_WORKER" {
		t.Fatalf("worker type = %q, want MODEL_WORKER", workerDef.Type)
	}
	if workerDef.Model != wantModel {
		t.Fatalf("worker model = %q, want %q", workerDef.Model, wantModel)
	}
	if workerDef.ModelProvider != wantProvider {
		t.Fatalf("worker model provider = %q, want %q", workerDef.ModelProvider, wantProvider)
	}
	if workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("worker executor provider = %q, want script_wrap", workerDef.ExecutorProvider)
	}
	if workerDef.Body != defaultProcessorSystemBody {
		t.Fatalf("worker body = %q, want %q", workerDef.Body, defaultProcessorSystemBody)
	}
	if workerDef.Concurrency != 0 || workerDef.SessionID != "" {
		t.Fatalf("worker config leaked retired public runtime fields: %#v", workerDef)
	}

	workstationDef, ok := loaded.Workstation("process")
	if !ok {
		t.Fatal("expected process workstation definition to load")
	}
	if workstationDef.Type != "MODEL_WORKSTATION" {
		t.Fatalf("workstation type = %q, want MODEL_WORKSTATION", workstationDef.Type)
	}
	if workstationDef.WorkerTypeName != "processor" {
		t.Fatalf("workstation worker = %q, want processor", workstationDef.WorkerTypeName)
	}
	if workstationDef.PromptTemplate == "" {
		t.Fatal("expected process workstation prompt template to load")
	}
}

func assertInitScaffoldFilesCanonical(t *testing.T, base, wantModel, wantProvider string) {
	t.Helper()

	factoryConfigPath := filepath.Join(base, "factory.json")
	factoryJSONBytes, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("read generated factory.json: %v", err)
	}
	factoryJSON := string(factoryJSONBytes)
	if !strings.Contains(factoryJSON, `"workType"`) {
		t.Fatalf("generated factory.json = %q, want canonical workType key", factoryJSON)
	}
	if !strings.Contains(factoryJSON, `"onFailure"`) {
		t.Fatalf("generated factory.json = %q, want canonical onFailure key", factoryJSON)
	}
	for _, retired := range retiredInitFactoryJSONFields {
		if strings.Contains(factoryJSON, retired) {
			t.Fatalf("generated factory.json should not contain retired %q:\n%s", retired, factoryJSON)
		}
	}

	workerAgentsPath := filepath.Join(base, "workers", "processor", "AGENTS.md")
	workerAgentsBytes, err := os.ReadFile(workerAgentsPath)
	if err != nil {
		t.Fatalf("read generated worker AGENTS.md: %v", err)
	}
	workerAgents := string(workerAgentsBytes)
	for _, expected := range []string{
		"model: " + wantModel,
		"modelProvider: " + strings.ToUpper(wantProvider),
		"executorProvider: SCRIPT_WRAP",
		"timeout: 1h",
		"skipPermissions: true",
		defaultProcessorSystemBody,
	} {
		if !strings.Contains(workerAgents, expected) {
			t.Fatalf("generated worker AGENTS.md should contain %q:\n%s", expected, workerAgents)
		}
	}
	for _, retired := range retiredInitWorkerFrontmatterFields {
		if strings.Contains(workerAgents, retired) {
			t.Fatalf("generated worker AGENTS.md should not contain retired %q:\n%s", retired, workerAgents)
		}
	}
}
