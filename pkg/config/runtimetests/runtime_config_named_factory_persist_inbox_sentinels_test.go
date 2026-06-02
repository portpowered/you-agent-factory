package runtimetests

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPersistNamedFactory_EnsuresCanonicalInputInboxGitkeepsOnCreate(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	assertInputInboxGitkeepFile(t, factoryDir, "BATCH")
	assertInputInboxGitkeepFile(t, factoryDir, "task")
}

func TestReplaceNamedFactory_EnsuresCanonicalInputInboxGitkeepsWhenAbsentBeforeReplace(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryDir := filepath.Join(rootDir, "alpha")
	removeInputInboxGitkeep(t, factoryDir, "BATCH")
	removeInputInboxGitkeep(t, factoryDir, "task")

	if _, err := ReplaceNamedFactory(rootDir, "alpha", namedFactoryPayloadWithBundledFiles(t, "alpha")); err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}

	assertInputInboxGitkeepFile(t, factoryDir, "BATCH")
	assertInputInboxGitkeepFile(t, factoryDir, "task")
}

func assertInputInboxGitkeepFile(t *testing.T, factoryDir, channel string) {
	t.Helper()

	path := filepath.Join(factoryDir, interfaces.InputsDir, channel, interfaces.DefaultChannelName, ".gitkeep")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: %v", channel, interfaces.DefaultChannelName, err)
	}
	if info.IsDir() {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: got directory, want regular file", channel, interfaces.DefaultChannelName)
	}
	if info.Size() != 0 {
		t.Fatalf("inputs/%s/%s/.gitkeep after materialization: size=%d, want empty sentinel", channel, interfaces.DefaultChannelName, info.Size())
	}
}

func removeInputInboxGitkeep(t *testing.T, factoryDir, channel string) {
	t.Helper()

	path := filepath.Join(factoryDir, interfaces.InputsDir, channel, interfaces.DefaultChannelName, ".gitkeep")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Remove(%s): %v", path, err)
	}
}
