package bootstrap_portability

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

const defaultInitFactoryWorkType = "task"

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

	support.RunInitCommand(t, dir)

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
		filepath.Join("inputs", defaultInitFactoryWorkType, "default"),
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
	support.RunInitCommand(t, dir)
	assertGeneratedInitScaffoldCanonical(t, dir, "codex")

	// Step 2: Write a seed file into the generated preseed directory.
	testutil.WriteSeedFile(t, dir, defaultInitFactoryWorkType, []byte(`{"title": "init factory e2e test"}`))
	assertCanonicalStarterInboxState(t, dir)

	// Step 3: Start the factory with a mock provider that returns a successful response.
	// The init-generated worker has no stop_token, so any non-error response is accepted.
	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertInitSessionPlaces(t, session, listed, "complete")

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
	assertInitProviderRequest(t, calls[0], "", "codex")
}

func TestInitFactory_ClaudeEndToEndUsesClaudeStarterWorker(t *testing.T) {
	dir := t.TempDir()

	support.RunInitCommand(t, dir, "--executor", "claude")
	assertGeneratedInitScaffoldCanonical(t, dir, "claude")

	testutil.WriteSeedFile(t, dir, defaultInitFactoryWorkType, []byte(`{"title": "claude init factory e2e test"}`))
	assertCanonicalStarterInboxState(t, dir)

	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Claude task processed successfully."},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertInitSessionPlaces(t, session, listed, "complete")

	if provider.CallCount("processor") != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount("processor"))
	}

	calls := provider.Calls("processor")
	if len(calls) == 0 {
		t.Fatal("expected at least 1 provider call")
	}
	assertInitProviderRequest(t, calls[0], "", "claude")
}

// TestInitFactory_FailureRouting verifies that the init-generated factory
// correctly routes work to task:failed when the provider returns an error.
func TestInitFactory_FailureRouting(t *testing.T) {
	dir := t.TempDir()

	support.RunInitCommand(t, dir)

	testutil.WriteSeedFile(t, dir, defaultInitFactoryWorkType, []byte(`{"title": "failing task"}`))

	// Provider returns an error — triggers OutcomeFailed → task:failed.
	work := map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "something went wrong", Error: errors.New("provider execution failed")},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertInitSessionPlaces(t, session, listed, "failed")
}

func assertInitSessionPlaces(t *testing.T, session factoryapi.FactorySession, listed factoryapi.ListWorkResponse, terminalState string) {
	t.Helper()
	for _, state := range []string{"init", "complete", "failed"} {
		want := 0
		if state == terminalState {
			want = 1
		}
		placeID := defaultInitFactoryWorkType + ":" + state
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	for _, usage := range session.Runtime.Usage.Resources {
		if usage.Name == "agent-slot" {
			if usage.Available != 1 || usage.Total != 1 {
				t.Errorf("agent-slot resource usage = %#v, want 1 available and total", usage)
			}
			return
		}
	}
	t.Errorf("session usage missing agent-slot resource")
}

// TestInitFactory_Idempotent verifies that running Init twice on the same
// directory does not overwrite existing files.
func TestInitFactory_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First init.
	support.RunInitCommand(t, dir)

	// Write a custom file into the worker dir to verify it's preserved.
	customContent := []byte("custom worker content")
	customPath := filepath.Join(dir, "workers", "processor", "AGENTS.md")
	if err := os.WriteFile(customPath, customContent, 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	// Second init.
	support.RunInitCommand(t, dir)

	// Verify the custom file was NOT overwritten.
	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom file: %v", err)
	}
	if string(data) != string(customContent) {
		t.Error("expected Init to not overwrite existing AGENTS.md files")
	}
}

func assertInitProviderRequest(t *testing.T, req workerexecution.ProviderInferenceRequest, wantModel, wantProvider string) {
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

	canonicalInputDir := filepath.Join(dir, "inputs", defaultInitFactoryWorkType, "default")
	entries, err := os.ReadDir(canonicalInputDir)
	if err != nil {
		t.Fatalf("read canonical starter inbox: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected canonical starter inbox %q to contain a seeded work item", canonicalInputDir)
	}
}

func assertGeneratedInitScaffoldCanonical(t *testing.T, dir, wantProvider string) {
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
	inputsReadmeBytes, err := os.ReadFile(filepath.Join(dir, "inputs", "README.md"))
	if err != nil {
		t.Fatalf("read generated inputs README.md: %v", err)
	}
	inputsReadme := string(inputsReadmeBytes)
	for _, expected := range []string{
		"inputs/task/default/",
		"Seed your starter work by adding files to this inbox",
	} {
		if !strings.Contains(inputsReadme, expected) {
			t.Fatalf("generated inputs README.md should contain %q:\n%s", expected, inputsReadme)
		}
	}

	workerAgentsBytes, err := os.ReadFile(filepath.Join(dir, "workers", "processor", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read generated worker AGENTS.md: %v", err)
	}
	workerAgents := string(workerAgentsBytes)
	for _, expected := range []string{
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
	if strings.Contains(workerAgents, "model:") {
		t.Fatalf("generated worker AGENTS.md should not contain a default model:\n%s", workerAgents)
	}
}
