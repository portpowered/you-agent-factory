package definitions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const initFactoryWorkType = "task"

// TestFactoryInitCreatesRunnablePortableScaffold proves public Factory init
// materializes the default portable scaffold and runs seeded starter Work to
// task:complete through a controlled provider edge using public Work and
// session observations.
func TestFactoryInitCreatesRunnablePortableScaffold(t *testing.T) {
	t.Parallel()
	server := sharedDefinitionsInitServer(t)

	workspaceDir := filepath.Join(t.TempDir(), "init-workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create init workspace: %v", err)
	}

	sessionID := initFactoryViaSessionCreate(t, server.baseURL, workspaceDir)
	t.Cleanup(func() { closeDefinitionsFactorySession(t, server.baseURL, sessionID) })

	factoryRoot := filepath.Join(workspaceDir, factorydefinitions.FactoryDir)
	assertPortableInitScaffoldLayout(t, factoryRoot)

	testutil.WriteSeedFile(t, factoryRoot, initFactoryWorkType, []byte(`{"title": "init factory runnable scaffold"}`))

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Task processed successfully."},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		factoryRoot,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)
	assertInitWorkReachedTerminalState(t, listed, "complete")

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.CallCount("processor"))
	}
}

// TestFactoryInitIsIdempotent proves a second public Factory init against the
// same workspace preserves customer-edited scaffold files instead of rewriting
// generated starter content.
func TestFactoryInitIsIdempotent(t *testing.T) {
	t.Parallel()
	server := sharedDefinitionsInitServer(t)

	workspaceDir := filepath.Join(t.TempDir(), "init-idempotent-workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create init workspace: %v", err)
	}

	process := sharedDefinitionsInitProcess(t)
	firstSessionID := initFactoryViaSessionCreateWithProcess(t, process, server.baseURL, workspaceDir)
	t.Cleanup(func() { closeDefinitionsFactorySession(t, server.baseURL, firstSessionID) })

	factoryRoot := filepath.Join(workspaceDir, factorydefinitions.FactoryDir)
	customPath := filepath.Join(factoryRoot, "workers", "processor", "AGENTS.md")
	original, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read processor AGENTS.md: %v", err)
	}
	const customerMarker = "Customer-edited processor instructions for idempotent init."
	custom := strings.Replace(string(original), "Complete the task.", customerMarker, 1)
	if err := os.WriteFile(customPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("write customer-edited processor AGENTS.md: %v", err)
	}

	secondSessionID := initFactoryViaSessionCreateWithProcess(t, process, server.baseURL, workspaceDir)
	t.Cleanup(func() { closeDefinitionsFactorySession(t, server.baseURL, secondSessionID) })

	got, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read processor AGENTS.md after re-init: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("processor AGENTS.md after re-init = %q, want preserved customer content %q", got, custom)
	}

	assertPortableInitScaffoldLayout(t, factoryRoot)
}

// TestFactoryInitFailureRoutingProducesFailedWork proves provider execution
// failure on the default initialized portable scaffold routes seeded Work to
// task:failed through public Work and session observations instead of complete.
func TestFactoryInitFailureRoutingProducesFailedWork(t *testing.T) {
	t.Parallel()
	server := sharedDefinitionsInitServer(t)

	workspaceDir := filepath.Join(t.TempDir(), "init-failure-workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create init workspace: %v", err)
	}

	sessionID := initFactoryViaSessionCreate(t, server.baseURL, workspaceDir)
	t.Cleanup(func() { closeDefinitionsFactorySession(t, server.baseURL, sessionID) })

	factoryRoot := filepath.Join(workspaceDir, factorydefinitions.FactoryDir)
	assertPortableInitScaffoldLayout(t, factoryRoot)

	testutil.WriteSeedFile(t, factoryRoot, initFactoryWorkType, []byte(`{"title": "init factory failure routing"}`))

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"processor": {
			{Error: errors.New("provider execution failed")},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		factoryRoot,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)
	assertInitWorkReachedTerminalState(t, listed, "failed")

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.CallCount("processor"))
	}
}

func initHostFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory-definitions-init-host",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func initFactoryViaSessionCreate(t *testing.T, serverURL, workspaceDir string) string {
	t.Helper()
	return initFactoryViaSessionCreateWithProcess(t, sharedDefinitionsInitProcess(t), serverURL, workspaceDir)
}

func initFactoryViaSessionCreateWithProcess(
	t *testing.T,
	process support.Process,
	serverURL string,
	workspaceDir string,
) string {
	t.Helper()

	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", serverURL,
		"session", "create",
		"--dir", workspaceDir,
		"--init-new-factory",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workspaceDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(session create --init-new-factory) error = %v; stdout=%q stderr=%q",
			err, inputs.Stdout(), inputs.Stderr(),
		)
	}

	var created factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &created); err != nil {
		t.Fatalf("decode session create JSON: %v\noutput:\n%s", err, inputs.Stdout())
	}
	if created.Session == nil || strings.TrimSpace(created.Session.Id) == "" {
		t.Fatalf("session create response missing session id: %#v", created)
	}
	if created.Session.FolderPath != workspaceDir {
		t.Fatalf("session folder path = %q, want %q", created.Session.FolderPath, workspaceDir)
	}
	return created.Session.Id
}

func assertPortableInitScaffoldLayout(t *testing.T, factoryRoot string) {
	t.Helper()

	expectedFiles := []string{
		factorydefinitions.FactoryConfigFile,
		filepath.Join("workers", "README.md"),
		filepath.Join("workers", "processor", "AGENTS.md"),
		filepath.Join("workstations", "README.md"),
		filepath.Join("workstations", "process", "AGENTS.md"),
		filepath.Join("inputs", "README.md"),
	}
	for _, relative := range expectedFiles {
		path := filepath.Join(factoryRoot, relative)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist under initialized Factory root", relative)
		}
	}

	defaultInbox := filepath.Join(
		factoryRoot,
		factorydefinitions.InputsDir,
		factorydefinitions.DefaultFactoryInputType,
		factorydefinitions.DefaultChannelName,
	)
	info, err := os.Stat(defaultInbox)
	if err != nil {
		t.Fatalf("expected default task inbox %s: %v", defaultInbox, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", defaultInbox)
	}

	factoriesCatchAll := filepath.Join(factoryRoot, "factories")
	if _, err := os.Stat(factoriesCatchAll); err == nil {
		t.Error("expected factories/ catch-all directory to NOT be created by Factory init")
	}
}

func assertInitWorkReachedTerminalState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalState string,
) {
	t.Helper()

	for _, state := range []string{"init", "complete", "failed"} {
		want := 0
		if state == terminalState {
			want = 1
		}
		location := support.WorkCustomerLocation(initFactoryWorkType, state)
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s work count = %d, want %d", location, got, want)
		}
	}
}
