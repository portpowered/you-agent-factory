package climanifest

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	ProductionManifestPath    = "contracts/cli/commands.json"
	CompatibilityManifestPath = "contracts/cli/deprecated-commands.json"
)

// LoadProduction decodes the committed production CLI command manifest.
func LoadProduction(path string) (Manifest, error) {
	return load(path, "production")
}

// LoadCompatibility decodes the separately classified compatibility command manifest.
func LoadCompatibility(path string) (Manifest, error) {
	return load(path, "compatibility")
}

func load(path, label string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read %s CLI command manifest %s: %w", label, path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode %s CLI command manifest: %w", label, err)
	}
	if manifest.RootPath == "" {
		return Manifest{}, fmt.Errorf("%s CLI command manifest missing rootPath", label)
	}
	if len(manifest.Commands) == 0 {
		return Manifest{}, fmt.Errorf("%s CLI command manifest missing commands", label)
	}
	return manifest, nil
}

// CommandByID returns one command record or an error when the stable ID is absent.
func (m Manifest) CommandByID(id string) (Command, error) {
	record, ok := m.Commands[id]
	if !ok {
		return Command{}, fmt.Errorf("production manifest missing command %q", id)
	}
	return record, nil
}
