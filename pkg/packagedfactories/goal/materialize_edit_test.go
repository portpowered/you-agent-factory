package goal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestEditedMaterializedPackagedGoalFactoryChangesNextLoad(t *testing.T) {
	factoryDir := materializePackagedGoalFactory(t, t.TempDir())
	initialWorker := loadPackagedGoalWorker(t, factoryDir)
	if initialWorker.Body == "" {
		t.Fatal("expected initial materialized goal worker body")
	}

	editedBody := "You are the customer-edited @you/goal built-in.\n"
	editMaterializedWorkerBody(t, factoryDir, "goal-executor", editedBody)

	editedWorker := loadPackagedGoalWorker(t, factoryDir)
	if editedWorker.Body != strings.TrimSpace(editedBody) {
		t.Fatalf("edited worker body = %q, want %q", editedWorker.Body, strings.TrimSpace(editedBody))
	}
	if editedWorker.Body == initialWorker.Body {
		t.Fatalf("edited worker body = %q, want change from initial materialized content", editedWorker.Body)
	}
}

func loadPackagedGoalWorker(t *testing.T, factoryDir string) *interfaces.WorkerConfig {
	t.Helper()
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factoryDir, err)
	}
	worker, ok := loaded.Worker("goal-executor")
	if !ok {
		t.Fatal("expected materialized goal-executor worker")
	}
	return worker
}

func editMaterializedWorkerBody(t *testing.T, factoryDir, workerName, body string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, interfaces.WorkersDir, workerName, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(workerPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(materialized worker body): %v", err)
	}
}
