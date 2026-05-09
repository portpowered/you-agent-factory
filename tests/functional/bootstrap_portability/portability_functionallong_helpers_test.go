//go:build functionallong

package bootstrap_portability

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFatFactoryJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write fat factory.json: %v", err)
	}
}

func writeFactoryTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
