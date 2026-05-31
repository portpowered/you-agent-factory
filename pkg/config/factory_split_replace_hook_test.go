package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReplaceFactorySplitLayout_ValidationFailureLeavesFactoryUnchanged(t *testing.T) {
	targetDir := t.TempDir()
	initial := []byte(`{
		"name": "alpha",
		"id": "alpha",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err := replaceFactorySplitLayout(targetDir, rollbackSplitReplacePayload(t, "broken"), factorySplitLayoutReplaceHooks{
		afterStageWrite: func(stagingDir string) error {
			path := filepath.Join(stagingDir, interfaces.WorkstationsDir, "execute-broken", interfaces.FactoryAgentsFileName)
			return os.WriteFile(path, []byte("---\ntype: [\n"), 0o644)
		},
	})
	if err == nil {
		t.Fatal("expected validation failure before commit")
	}
	if got := err.Error(); !strings.Contains(got, `validate factory`) || !strings.Contains(got, "AGENTS.md missing closing frontmatter delimiter") {
		t.Fatalf("expected load-time validation error, got %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(targetDir, interfaces.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("ReadFile(factory.json): %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after failed replace = %q, want unchanged payload", got)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, interfaces.WorkersDir)); !os.IsNotExist(statErr) {
		t.Fatalf("expected no workers directory after failed replace, got stat err=%v", statErr)
	}
}

func rollbackSplitReplacePayload(t *testing.T, project string) []byte {
	t.Helper()
	return []byte(`{
		"name": "` + project + `",
		"id": "` + project + `",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-` + project + `","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
}
