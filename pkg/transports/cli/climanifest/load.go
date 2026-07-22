package climanifest

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
)

const (
	ProductionManifestPath    = "contracts/cli/commands.json"
	CompatibilityManifestPath = "contracts/cli/deprecated-commands.json"
)

// LoadProduction decodes the committed production CLI command manifest.
func LoadProduction(store generatedartifacts.SourceStore, path string) (Manifest, error) {
	manifest, err := load(store, path, "production", true)
	if err != nil {
		return Manifest{}, err
	}
	if err := ValidateRunSubmitFamily(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate run/submit family: %w", err)
	}
	return manifest, nil
}

// LoadCompatibility decodes the separately classified compatibility command manifest.
func LoadCompatibility(store generatedartifacts.SourceStore, path string) (Manifest, error) {
	return load(store, path, "compatibility", false)
}

func load(store generatedartifacts.SourceStore, path, label string, requireCommands bool) (Manifest, error) {
	if store == nil {
		return Manifest{}, fmt.Errorf("CLI command manifest source store is required")
	}
	raw, err := store.Read(path)
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
	if requireCommands && len(manifest.Commands) == 0 {
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
