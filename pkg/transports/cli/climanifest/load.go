package climanifest

import (
	"encoding/json"
	"fmt"
	"os"
)

const ProductionManifestPath = "contracts/cli/commands.json"

// LoadProduction decodes the committed production CLI command manifest.
func LoadProduction(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read production CLI command manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode production CLI command manifest: %w", err)
	}
	if manifest.RootPath == "" {
		return Manifest{}, fmt.Errorf("production CLI command manifest missing rootPath")
	}
	if len(manifest.Commands) == 0 {
		return Manifest{}, fmt.Errorf("production CLI command manifest missing commands")
	}
	if err := ValidateRunSubmitFamily(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate run/submit family: %w", err)
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
