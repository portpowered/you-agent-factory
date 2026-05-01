package guards_batch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/agent-factory/pkg/workers"
)

func writeAgentConfig(t *testing.T, dir, workerName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workers", workerName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create worker config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertArgsContainSequence(t *testing.T, args, want []string) {
	t.Helper()

	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return
		}
	}

	t.Fatalf("expected args %v to contain sequence %v", args, want)
}

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()

	corpus, err := workers.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("workers.LoadProviderErrorCorpus() error = %v", err)
	}
	entry, ok := corpus.Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}
