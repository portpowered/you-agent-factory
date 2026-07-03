package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWriteSeedExecutionFile_WritesUnderExecutionChannel(t *testing.T) {
	dir := t.TempDir()
	WriteSeedExecutionFile(t, dir, "chapter", "exec-test", []byte(`{"title":"seed"}`))

	gotPath := filepath.Join(dir, interfaces.InputsDir, "chapter", "exec-test")
	entries, err := os.ReadDir(gotPath)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("file count = %d, want 1", len(entries))
	}
	if entries[0].IsDir() {
		t.Fatalf("expected file, got directory %q", entries[0].Name())
	}
}
