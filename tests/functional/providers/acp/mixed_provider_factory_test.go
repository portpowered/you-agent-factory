package acp_test

import (
	"os"
	"path/filepath"
	"testing"
)

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
