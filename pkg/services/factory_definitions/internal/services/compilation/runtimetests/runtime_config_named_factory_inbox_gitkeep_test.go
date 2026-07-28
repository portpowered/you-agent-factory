package runtimetests

import (
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPersistNamedFactory_EnsuresCanonicalInputInboxGitkeepsOnCreate(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	assertInputInboxGitkeepFile(t, factoryDir, "BATCH")
	assertInputInboxGitkeepFile(t, factoryDir, "task")
}

func TestReplaceNamedFactory_EnsuresCanonicalInputInboxGitkeepsWhenAbsentBeforeReplace(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryDir := filepath.Join(rootDir, "alpha")
	removeInputInboxGitkeep(t, factoryDir, "BATCH")
	removeInputInboxGitkeep(t, factoryDir, "task")

	if _, err := factorydefinitioncomposition.ReplaceNamedFactory(rootDir, "alpha", namedFactoryPayloadWithBundledFiles(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}

	assertInputInboxGitkeepFile(t, factoryDir, "BATCH")
	assertInputInboxGitkeepFile(t, factoryDir, "task")
}

func TestReplaceNamedFactory_PreservesBatchInboxGitkeepAcrossReplace(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryDir := filepath.Join(rootDir, "alpha")
	batchGitkeep := filepath.Join(factoryDir, factorydefinitions.InputsDir, "BATCH", factorydefinitions.DefaultChannelName, ".gitkeep")
	if err := os.MkdirAll(filepath.Dir(batchGitkeep), 0o755); err != nil {
		t.Fatalf("MkdirAll(batch inbox): %v", err)
	}
	if err := os.WriteFile(batchGitkeep, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(batch .gitkeep): %v", err)
	}

	staleStarter := filepath.Join(factoryDir, factorydefinitions.InputsDir, "task", factorydefinitions.DefaultChannelName, "stale.md")
	if err := os.WriteFile(staleStarter, []byte("stale starter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale starter): %v", err)
	}

	replacePayload := namedFactoryPayloadWithBundledFiles(t, "alpha")
	if _, err := factorydefinitioncomposition.ReplaceNamedFactory(rootDir, "alpha", replacePayload, ownerFactoryDefinitionValidator()); err != nil {
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

	bundledStarter := filepath.Join(factoryDir, factorydefinitions.InputsDir, "task", factorydefinitions.DefaultChannelName, "starter.md")
	assertRuntimeFactoryFileContent(t, bundledStarter, "starter work\n")
}

func assertInputInboxGitkeepFile(t *testing.T, factoryDir, channel string) {
	t.Helper()

	path := filepath.Join(factoryDir, factorydefinitions.InputsDir, channel, factorydefinitions.DefaultChannelName, ".gitkeep")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: %v", channel, factorydefinitions.DefaultChannelName, err)
	}
	if info.IsDir() {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: got directory, want regular file", channel, factorydefinitions.DefaultChannelName)
	}
	if info.Size() != 0 {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: size=%d, want empty sentinel", channel, factorydefinitions.DefaultChannelName, info.Size())
	}
}

func removeInputInboxGitkeep(t *testing.T, factoryDir, channel string) {
	t.Helper()

	path := filepath.Join(factoryDir, factorydefinitions.InputsDir, channel, factorydefinitions.DefaultChannelName, ".gitkeep")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Remove(%s): %v", path, err)
	}
}
