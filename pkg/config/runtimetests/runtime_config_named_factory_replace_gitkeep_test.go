package runtimetests

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReplaceNamedFactory_PreservesBatchInboxGitkeepAcrossReplace(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryDir := filepath.Join(rootDir, "alpha")
	batchGitkeep := filepath.Join(factoryDir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName, ".gitkeep")
	if err := os.MkdirAll(filepath.Dir(batchGitkeep), 0o755); err != nil {
		t.Fatalf("MkdirAll(batch inbox): %v", err)
	}
	if err := os.WriteFile(batchGitkeep, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(batch .gitkeep): %v", err)
	}

	staleStarter := filepath.Join(factoryDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "stale.md")
	if err := os.WriteFile(staleStarter, []byte("stale starter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale starter): %v", err)
	}

	replacePayload := namedFactoryPayloadWithBundledFiles(t, "alpha")
	if _, err := ReplaceNamedFactory(rootDir, "alpha", replacePayload); err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}

	if _, err := os.Stat(batchGitkeep); err != nil {
		t.Fatalf("inputs/BATCH/default/.gitkeep after replace: %v", err)
	}
	if info, err := os.Stat(batchGitkeep); err == nil && info.IsDir() {
		t.Fatalf("inputs/BATCH/default/.gitkeep after replace: got directory, want regular file")
	}

	if _, err := os.Stat(staleStarter); !os.IsNotExist(err) {
		t.Fatalf("stale input file after replace: stat err=%v, want absent", err)
	}

	bundledStarter := filepath.Join(factoryDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "starter.md")
	assertRuntimeFactoryFileContent(t, bundledStarter, "starter work\n")
}
