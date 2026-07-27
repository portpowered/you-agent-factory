package runtime_lifecycle

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestOneShotRunJoinsRuntimeLifecycle proves one-shot completion joins the owned runtime.
func TestOneShotRunJoinsRuntimeLifecycle(t *testing.T) {
	factoryRoot := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	testutil.WriteSeedFile(t, factoryRoot, "task", []byte(`{"title": "one-shot lifecycle"}`))
	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "One-shot done. COMPLETE"},
	)

	process := support.BuildProcess(t, serviceedges.Edges{ProviderOverride: provider})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", factoryRoot,
		"--quiet",
		"--no-record",
	})
	inputs.Input.WorkingDirectory = factoryRoot
	inputs.Input.Env = isolatedLifecycleEnvironment(t.TempDir())
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(one-shot run) error = %v; stderr=%q", err, inputs.Stderr())
	}
	if provider.CallCount() != 1 {
		t.Fatalf("one-shot provider calls = %d, want 1", provider.CallCount())
	}
}

func isolatedLifecycleEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}
