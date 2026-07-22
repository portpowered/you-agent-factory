package authoredlayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestWriteAgentsFileCreatesDirectoryAndWritesContent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "workers", "reviewer")
	content := []byte("---\ntype: INFERENCE_WORKER\n---\n")

	writeAgentsFile := NewAgentsFileWriter(localTestFileSystem{})
	if err := writeAgentsFile(dir, content); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	written, err := os.ReadFile(
		filepath.Join(dir, factorydefinitions.FactoryAgentsFileName),
	)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(written) != string(content) {
		t.Fatalf("content = %q, want %q", written, content)
	}
}

func TestWriterFailsClosedWithoutFileSystemOrInboxEnsurer(t *testing.T) {
	newWriter := func(fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem, ensureInbox factorydefinitions.InputInboxSentinelEnsurer) *Writer {
		return NewWriter(
			func(factorydefinitions.FactoryWorkerConfig) ([]byte, error) { return nil, nil },
			func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error) { return nil, nil },
			func(string) []byte { return nil },
			func(string, []byte) error { return nil },
			func(_, value string) (string, error) { return value, nil },
			func(_, value string) (string, error) { return value, nil },
			fileSystem,
			ensureInbox,
		)
	}
	if err := newWriter(nil, nil).WritePrepared("unused", nil, "test", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "writer filesystem is required") {
		t.Fatalf("WritePrepared() without filesystem error = %v", err)
	}
	fileSystem := localTestFileSystem{}
	if err := newWriter(fileSystem, nil).WritePrepared("unused", nil, "test", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "sentinel ensurer is required") {
		t.Fatalf("WritePrepared() without inbox ensurer error = %v", err)
	}
}
