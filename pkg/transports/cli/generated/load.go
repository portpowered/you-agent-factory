package generated

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

//go:embed representative_family.json
var representativeFamilyJSON []byte

//go:embed models_docs_family.json
var modelsDocsFamilyJSON []byte

// RepresentativeFamilyManifest returns generated §4.3 metadata for the
// representative root/session-show command family.
func RepresentativeFamilyManifest() (climanifest.Manifest, error) {
	return parseRepresentativeFamilyManifest(representativeFamilyJSON)
}

// ModelsDocsFamilyManifest returns generated §4.3 metadata for the models/docs
// command family.
func ModelsDocsFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(modelsDocsFamilyJSON, "models/docs")
}

func parseRepresentativeFamilyManifest(payload []byte) (climanifest.Manifest, error) {
	return parseFamilyManifest(payload, "representative")
}

func parseFamilyManifest(payload []byte, familyName string) (climanifest.Manifest, error) {
	var manifest climanifest.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return climanifest.Manifest{}, fmt.Errorf("decode generated %s-family metadata: %w", familyName, err)
	}
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("generated %s-family metadata missing rootPath", familyName)
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("generated %s-family metadata missing commands", familyName)
	}
	return manifest, nil
}

// CommandByID returns one generated representative-family command record.
func CommandByID(id string) (climanifest.Command, error) {
	manifest, err := RepresentativeFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}
