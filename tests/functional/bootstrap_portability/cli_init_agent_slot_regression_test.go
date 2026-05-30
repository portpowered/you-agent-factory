package bootstrap_portability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	"github.com/portpowered/infinite-you/pkg/service"
)

// TestInitFactory_AgentSlotResourceMismatchRegression documents that a default
// init directory whose processor worker declares agent-slot without a matching
// factory.json resources pool cannot start with mock workers.
//
// Story factory-new-tab-and-init-fix-002 aligns the embedded canonical scaffold
// so this mismatch no longer exists on fresh init output.
func TestInitFactory_AgentSlotResourceMismatchRegression(t *testing.T) {
	dir := t.TempDir()

	if err := initcmd.Init(initcmd.InitConfig{Dir: dir}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if err := writeProcessorAgentSlotResource(dir); err != nil {
		t.Fatalf("write processor agent-slot resource: %v", err)
	}

	factoryJSON, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	if strings.Contains(string(factoryJSON), "agent-slot") {
		t.Fatalf("regression fixture requires factory.json without agent-slot resources pool; got:\n%s", factoryJSON)
	}

	err = buildInitFactoryService(dir)
	if err == nil {
		t.Fatal("expected factory service startup to fail when processor declares agent-slot without factory resources")
	}
	assertAgentSlotResourceMismatchError(t, err)
}

func writeProcessorAgentSlotResource(dir string) error {
	workerPath := filepath.Join(dir, "workers", "processor", "AGENTS.md")
	data, err := os.ReadFile(workerPath)
	if err != nil {
		return err
	}
	body := string(data)
	if strings.Contains(body, "agent-slot") {
		return nil
	}

	const resourceBlock = `resources:
  - name: agent-slot
    capacity: 1
`
	needle := "skipPermissions: true\n"
	if !strings.Contains(body, needle) {
		return os.WriteFile(workerPath, []byte(`---
type: MODEL_WORKER
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
`+resourceBlock+`---
You are the processor. Complete the task.
`), 0o644)
	}
	return os.WriteFile(workerPath, []byte(strings.Replace(body, needle, needle+resourceBlock, 1)), 0o644)
}

func buildInitFactoryService(dir string) error {
	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     dir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	return err
}

func assertAgentSlotResourceMismatchError(t *testing.T, err error) {
	t.Helper()

	msg := err.Error()
	switch {
	case strings.Contains(msg, "agent-slot") && strings.Contains(msg, "non-existent resource"):
		return
	case strings.Contains(msg, "agent-slot:available"):
		return
	default:
		t.Fatalf("expected agent-slot resource mismatch error, got: %v", err)
	}
}
