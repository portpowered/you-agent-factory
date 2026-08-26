package acp_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryMixesACPAndScriptWrapWorkersWithoutCrossRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	factory := `{
  "name":"mixed-acp-native",
  "workTypes":[
    {"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]},
    {"name":"native","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}
  ],
  "workers":[{"name":"worker"},{"name":"native-worker"}],
  "workstations":[
    {"name":"process-acp","worker":"worker","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}],"onFailure":[{"workType":"task","state":"failed"}]},
    {"name":"process-native","worker":"native-worker","inputs":[{"workType":"native","state":"init"}],"outputs":[{"workType":"native","state":"done"}],"onFailure":[{"workType":"native","state":"failed"}]}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(factory), 0o600); err != nil {
		t.Fatalf("write mixed Factory: %v", err)
	}
	writeACPWorker(t, dir, "cursor-acp")
	writeWorkerDefinition(t, dir, "native-worker", "SCRIPT_WRAP")
	writeWorkstationDefinition(t, dir, "process-acp")
	writeWorkstationDefinition(t, dir, "process-native")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP branch"}`))
	testutil.WriteSeedFile(t, dir, "native", []byte(`{"title":"native branch"}`))
	var starts atomic.Int32
	legacy := &legacyProvider{response: providers.ExecuteResult{Content: "native COMPLETE"}}
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, functionalACPFixture("1")),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderOverride:              legacy,
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("ACP completed work = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "native:done"); got != 1 {
		t.Fatalf("native completed work = %d, want 1", got)
	}
	if starts.Load() != 1 || legacy.calls.Load() != 1 {
		t.Fatalf("ACP starts=%d legacy calls=%d, want one each", starts.Load(), legacy.calls.Load())
	}
}

func writeWorkerDefinition(t *testing.T, factoryDir, name, executorProvider string) {
	t.Helper()
	dir := filepath.Join(factoryDir, "workers", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create Worker directory: %v", err)
	}
	definition := "---\nexecutorProvider: " + executorProvider + "\nmodel: test-model\nstopToken: COMPLETE\ntype: MODEL_WORKER\n---\n\nMixed provider Worker.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(definition), 0o600); err != nil {
		t.Fatalf("write Worker definition: %v", err)
	}
}

func writeWorkstationDefinition(t *testing.T, factoryDir, name string) {
	t.Helper()
	dir := filepath.Join(factoryDir, "workstations", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create Workstation directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\n\nMixed provider Workstation.\n"), 0o600); err != nil {
		t.Fatalf("write Workstation definition: %v", err)
	}
}
