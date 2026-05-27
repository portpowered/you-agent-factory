package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPersistNamedFactory_RollsBackStagedLayoutWhenLoadRuntimeConfigFails(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", rollbackNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	_, err := persistNamedFactory(rootDir, "broken", rollbackNamedFactoryPayload(t, "broken"), namedFactoryPersistOptions{}, namedFactoryPersistHooks{
		afterWrite: func(stagingDir string) error {
			path := filepath.Join(stagingDir, interfaces.WorkstationsDir, "execute-broken", interfaces.FactoryAgentsFileName)
			return os.WriteFile(path, []byte("---\ntype: [\n"), 0o644)
		},
	})
	if err == nil {
		t.Fatal("expected staged named-factory validation failure")
	}
	if got := err.Error(); !rollbackContainsAll(got, `validate factory "broken" config`, "load workstation", "AGENTS.md missing closing frontmatter delimiter") {
		t.Fatalf("expected load-time validation error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("expected failed staged factory directory to be absent, got stat err=%v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(current after staged failure): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "alpha") {
		t.Fatalf("FactoryDir after staged failure = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "alpha"))
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project after staged failure = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func rollbackNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}

func rollbackContainsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}
