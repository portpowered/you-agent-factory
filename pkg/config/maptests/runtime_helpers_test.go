package maptests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func writeRuntimeFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}

func writeRuntimeWorkerAgentsMD(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workerDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(worker AGENTS.md): %v", err)
	}
}

func writeRuntimeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName, content string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workstationDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(workstation AGENTS.md): %v", err)
	}
}
