package functional_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

func agentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testutil.MustRepoPath(t, rel)
}

func clearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}

func setFactoryProject(t *testing.T, dir string, project string) {
	t.Helper()

	updateFactoryProject(t, dir, func(config map[string]any) {
		config["project"] = project
	})
}

func updateFactoryProject(t *testing.T, dir string, update func(map[string]any)) {
	t.Helper()

	factoryPath := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse factory config: %v", err)
	}
	update(config)
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(factoryPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}
}
