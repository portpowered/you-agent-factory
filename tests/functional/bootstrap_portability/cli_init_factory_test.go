package bootstrap_portability

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

var retiredInitFactoryContractFields = []string{`"work_types"`, `"work_type"`, `"on_failure"`}
var retiredInitWorkerContractFields = []string{"model_provider:", "provider:", "stop_token:", "skip_permissions:", "concurrency:", "sessionId:"}

// TestInitFactory_StructureIsValid verifies that the init command creates the
// correct directory structure with all required files:
//
//	factory.json               — workflow definition with workers section
//	workers/processor/AGENTS.md — MODEL_WORKER definition
//	workstations/process/AGENTS.md — MODEL_WORKSTATION prompt template
//	inputs/task/default/       — preseed directory for the "task" work type
//
// No "factories/" subdirectory should be created.
func TestInitFactory_StructureIsValid(t *testing.T) {
	dir := t.TempDir()

	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify expected files exist.
	expectedFiles := []string{
		"factory.json",
		filepath.Join("workers", "README.md"),
		filepath.Join("workers", "processor", "AGENTS.md"),
		filepath.Join("workstations", "README.md"),
		filepath.Join("workstations", "process", "AGENTS.md"),
		filepath.Join("inputs", "README.md"),
	}
	for _, f := range expectedFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}

	// Verify expected directories exist.
	expectedDirs := []string{
		filepath.Join("inputs", initcmd.DefaultFactoryInputType, "default"),
	}
	for _, d := range expectedDirs {
		path := filepath.Join(dir, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}

	// Verify "factories/" subdirectory is NOT created.
	factoriesPath := filepath.Join(dir, "factories")
	if _, err := os.Stat(factoriesPath); err == nil {
		t.Error("expected 'factories/' directory to NOT be created by init")
	}

}

// TestInitFactory_EndToEnd exercises the full init → run → complete lifecycle:
//
//  1. Run cli.Init on a temporary directory.
//  2. Write a seed work item into the generated inputs/task/default/ directory.
//  3. Start the factory service with a mock provider.
//  4. Verify the work item flows through the pipeline to task:complete.
//
// This confirms that the init command generates a fully functional factory
// that can be used as-is without any manual file creation.
func TestInitFactory_EndToEnd(t *testing.T) {
	dir := t.TempDir()

	// Step 1: Run Init to generate the factory structure.
	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	assertGeneratedInitScaffoldCanonical(t, dir, "gpt-5-codex", "codex")

	// Step 2: Write a seed file into the generated preseed directory.
	testutil.WriteSeedFile(t, dir, initcmd.DefaultFactoryInputType, []byte(`{"title": "init factory e2e test"}`))
	assertCanonicalStarterInboxState(t, dir)

	// Step 3: Start the factory with a mock provider that returns a successful response.
	// The init-generated worker has no stop_token, so any non-error response is accepted.
	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Step 4: Run until all tokens reach a terminal state.
	h.RunUntilComplete(t, 15*time.Second)

	// Assert the work item reached the terminal "complete" state.
	h.Assert().
		HasTokenInPlace(initcmd.DefaultFactoryInputType + ":complete").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":init").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":failed").
		TokenCount(1)

	// Verify the mock provider was called exactly once (one workstation in the pipeline).
	if provider.CallCount("processor") != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount("processor"))
	}

	// Verify the provider received a non-empty user message rendered from the
	// workstation AGENTS.md template.
	calls := provider.Calls("processor")
	if len(calls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}
	if calls[0].UserMessage == "" {
		t.Error("expected non-empty user message from rendered workstations/process/AGENTS.md template")
	}
	assertInitProviderRequest(t, calls[0], "gpt-5-codex", "codex")
}

func TestInitFactory_ClaudeEndToEndUsesClaudeStarterWorker(t *testing.T) {
	dir := t.TempDir()

	if err := initcmd.Init(initcmd.InitConfig{Dir: dir, Executor: "claude"}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	assertGeneratedInitScaffoldCanonical(t, dir, "claude-sonnet-4-20250514", "claude")

	testutil.WriteSeedFile(t, dir, initcmd.DefaultFactoryInputType, []byte(`{"title": "claude init factory e2e test"}`))
	assertCanonicalStarterInboxState(t, dir)

	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Claude task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace(initcmd.DefaultFactoryInputType + ":complete").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":init").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":failed").
		TokenCount(1)

	if provider.CallCount("processor") != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount("processor"))
	}

	calls := provider.Calls("processor")
	if len(calls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}
	assertInitProviderRequest(t, calls[0], "claude-sonnet-4-20250514", "claude")
}

// TestInitFactory_FailureRouting verifies that the init-generated factory
// correctly routes work to task:failed when the provider returns an error.
func TestInitFactory_FailureRouting(t *testing.T) {
	dir := t.TempDir()

	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	testutil.WriteSeedFile(t, dir, initcmd.DefaultFactoryInputType, []byte(`{"title": "failing task"}`))

	// Provider returns an error — triggers OutcomeFailed → task:failed.
	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "something went wrong", Error: errors.New("provider execution failed")},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	// Token should end in failed state since the provider returned an error.
	h.Assert().
		HasTokenInPlace(initcmd.DefaultFactoryInputType + ":failed").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":init").
		HasNoTokenInPlace(initcmd.DefaultFactoryInputType + ":complete").
		TokenCount(1)
}

// TestInitFactory_Idempotent verifies that running Init twice on the same
// directory does not overwrite existing files.
func TestInitFactory_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First init.
	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}

	// Write a custom file into the worker dir to verify it's preserved.
	customContent := []byte("custom worker content")
	customPath := filepath.Join(dir, "workers", "processor", "AGENTS.md")
	if err := os.WriteFile(customPath, customContent, 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	// Second init.
	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}

	// Verify the custom file was NOT overwritten.
	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom file: %v", err)
	}
	if string(data) != string(customContent) {
		t.Error("expected Init to not overwrite existing AGENTS.md files")
	}
}

func assertInitProviderRequest(t *testing.T, req interfaces.ProviderInferenceRequest, wantModel, wantProvider string) {
	t.Helper()

	if req.Model != wantModel {
		t.Fatalf("provider request model = %q, want %q", req.Model, wantModel)
	}
	if req.ModelProvider != wantProvider {
		t.Fatalf("provider request model provider = %q, want %q", req.ModelProvider, wantProvider)
	}
	if req.SystemPrompt == "" {
		t.Fatal("expected provider request system prompt to be populated from worker AGENTS.md")
	}
	if !strings.Contains(req.SystemPrompt, "You are the processor. Complete the task.") {
		t.Fatalf("provider request system prompt = %q, want default processor instructions", req.SystemPrompt)
	}
	if req.UserMessage == "" {
		t.Fatal("expected provider request user message to be populated from workstation AGENTS.md")
	}
}

func assertCanonicalStarterInboxState(t *testing.T, dir string) {
	t.Helper()

	canonicalInputDir := filepath.Join(dir, "inputs", initcmd.DefaultFactoryInputType, "default")
	entries, err := os.ReadDir(canonicalInputDir)
	if err != nil {
		t.Fatalf("read canonical starter inbox: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected canonical starter inbox %q to contain a seeded work item", canonicalInputDir)
	}
}

func assertGeneratedInitScaffoldCanonical(t *testing.T, dir, wantModel, wantProvider string) {
	t.Helper()

	factoryJSONBytes, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read generated factory.json: %v", err)
	}
	factoryJSON := string(factoryJSONBytes)
	for _, expected := range []string{`"workType"`, `"onFailure"`} {
		if !strings.Contains(factoryJSON, expected) {
			t.Fatalf("generated factory.json should contain %q:\n%s", expected, factoryJSON)
		}
	}
	for _, retired := range retiredInitFactoryContractFields {
		if strings.Contains(factoryJSON, retired) {
			t.Fatalf("generated factory.json should not contain retired %q:\n%s", retired, factoryJSON)
		}
	}

	workerAgentsBytes, err := os.ReadFile(filepath.Join(dir, "workers", "processor", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read generated worker AGENTS.md: %v", err)
	}
	workerAgents := string(workerAgentsBytes)
	for _, expected := range []string{
		"model: " + wantModel,
		"modelProvider: " + strings.ToUpper(wantProvider),
		"executorProvider: SCRIPT_WRAP",
		"skipPermissions: true",
		"timeout: 1h",
		"You are the processor. Complete the task.",
	} {
		if !strings.Contains(workerAgents, expected) {
			t.Fatalf("generated worker AGENTS.md should contain %q:\n%s", expected, workerAgents)
		}
	}
	for _, retired := range retiredInitWorkerContractFields {
		if strings.Contains(workerAgents, retired) {
			t.Fatalf("generated worker AGENTS.md should not contain retired %q:\n%s", retired, workerAgents)
		}
	}
}
