package bootstrap_portability

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRelativeWorkingDirectoryFactoryConfig(t *testing.T, factoryDir string) {
	t.Helper()

	config := `{
  "name": "factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "worker-a" }
  ],
  "workstations": [
    {
      "name": "process",
      "behavior": "STANDARD",
      "worker": "worker-a",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" },
      "workingDirectory": ".claude/worktrees/{{ (index .Inputs 0).Name }}",
      "worktree": "{{ (index .Inputs 0).Name }}"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeRelativeWorkingDirectoryWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
