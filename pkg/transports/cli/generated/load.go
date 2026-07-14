package generated

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

//go:embed representative_family.json
var representativeFamilyJSON []byte

//go:embed factory_config_init_family.json
var factoryConfigInitFamilyJSON []byte

// RepresentativeFamilyManifest returns generated §4.3 metadata for the
// representative root/session-show command family.
func RepresentativeFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(representativeFamilyJSON, "representative-family")
}

// FactoryConfigInitFamilyManifest returns generated §4.3 metadata for the
// factory/config/init command family.
func FactoryConfigInitFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(factoryConfigInitFamilyJSON, "factory/config/init family")
}

func parseRepresentativeFamilyManifest(payload []byte) (climanifest.Manifest, error) {
	return parseFamilyManifest(payload, "representative-family")
}

func parseFamilyManifest(payload []byte, familyLabel string) (climanifest.Manifest, error) {
	var manifest climanifest.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return climanifest.Manifest{}, fmt.Errorf("decode generated %s metadata: %w", familyLabel, err)
	}
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("generated %s metadata missing rootPath", familyLabel)
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("generated %s metadata missing commands", familyLabel)
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

// FactoryConfigInitCommandByID returns one generated factory/config/init command record.
func FactoryConfigInitCommandByID(id string) (climanifest.Command, error) {
	manifest, err := FactoryConfigInitFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}
