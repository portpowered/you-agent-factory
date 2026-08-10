package climanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

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
	if err := ValidateRootContract(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate root contract: %w", err)
	}
	if err := ValidatePlacementContract(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate placement contract: %w", err)
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
	if err := decodeManifest(raw, &manifest); err != nil {
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

func decodeManifest(raw []byte, manifest *Manifest) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := rejectDuplicateJSONKeys(decoder, "$"); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return fmt.Errorf("decode trailing JSON: %w", err)
		}
		return fmt.Errorf("unexpected trailing JSON value %v", token)
	}
	return json.Unmarshal(raw, manifest)
}

func rejectDuplicateJSONKeys(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key at %s is not a string", path)
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate object key %q at %s", key, path)
				}
				seen[key] = struct{}{}
				if err := rejectDuplicateJSONKeys(decoder, path+"."+key); err != nil {
					return err
				}
			}
			return expectJSONDelimiter(decoder, '}', path)
		case '[':
			index := 0
			for decoder.More() {
				if err := rejectDuplicateJSONKeys(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
				index++
			}
			return expectJSONDelimiter(decoder, ']', path)
		case '}', ']':
			return fmt.Errorf("unexpected JSON delimiter %q at %s", delimiter, path)
		}
	}
	return nil
}

func expectJSONDelimiter(decoder *json.Decoder, want json.Delim, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected JSON delimiter %q at %s, got %v", want, path, token)
	}
	return nil
}

// CommandByID returns one command record or an error when the stable ID is absent.
func (m Manifest) CommandByID(id string) (Command, error) {
	record, ok := m.Commands[id]
	if !ok {
		return Command{}, fmt.Errorf("production manifest missing command %q", id)
	}
	return record, nil
}
