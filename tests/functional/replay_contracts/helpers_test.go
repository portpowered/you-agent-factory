package replay_contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}
