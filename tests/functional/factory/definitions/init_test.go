package definitions

import (
	"encoding/json"
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
// task:complete through mock workers using public Work and session observations.
func TestFactoryInitCreatesRunnablePortableScaffold(t *testing.T) {
	hostFactoryDir := support.ScaffoldFactory(t, initHostFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostFactoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	workspaceDir := filepath.Join(t.TempDir(), "init-workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create init workspace: %v", err)
	}

	initFactoryViaSessionCreate(t, server.URL(), workspaceDir)

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

func initFactoryViaSessionCreate(t *testing.T, serverURL, workspaceDir string) {
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
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
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
