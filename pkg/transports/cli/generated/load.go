package generated

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

//go:embed representative_family.json
var representativeFamilyJSON []byte

// RepresentativeFamilyManifest returns generated §4.3 metadata for the
// representative root/session-show command family.
func RepresentativeFamilyManifest() (climanifest.Manifest, error) {
	return parseRepresentativeFamilyManifest(representativeFamilyJSON)
}

func parseRepresentativeFamilyManifest(payload []byte) (climanifest.Manifest, error) {
	var manifest climanifest.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return climanifest.Manifest{}, fmt.Errorf("decode generated representative-family metadata: %w", err)
	}
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("generated representative-family metadata missing rootPath")
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("generated representative-family metadata missing commands")
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
