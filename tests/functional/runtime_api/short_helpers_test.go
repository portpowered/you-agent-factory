//go:build !functionallong

package runtime_api

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
