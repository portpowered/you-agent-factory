package generated

import (
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

// RepresentativeFamilyManifest returns generated §4.3 metadata for the
// representative root/session-show command family.
func RepresentativeFamilyManifest() (climanifest.Manifest, error) {
	return representativeFamilyManifestValue(), nil
}

// SessionFamilyManifest returns generated metadata for the complete canonical
// Factory Session command family.
func SessionFamilyManifest() (climanifest.Manifest, error) {
	return sessionFamilyManifestValue(), nil
}

// WorkFamilyManifest returns generated §4.3 metadata for the work
// inspection/control command family.
func WorkFamilyManifest() (climanifest.Manifest, error) {
	return workFamilyManifestValue(), nil
}

// FactoryConfigInitFamilyManifest returns generated §4.3 metadata for the
// factory/config/init command family.
func FactoryConfigInitFamilyManifest() (climanifest.Manifest, error) {
	return factoryConfigInitFamilyManifestValue(), nil
}

// ModelsDocsFamilyManifest returns generated §4.3 metadata for the models/docs
// command family.
func ModelsDocsFamilyManifest() (climanifest.Manifest, error) {
	return modelsDocsFamilyManifestValue(), nil
}

// RunSubmitFamilyManifest returns generated metadata for the run/submit family.
func RunSubmitFamilyManifest() (climanifest.Manifest, error) {
	return runSubmitFamilyManifestValue(), nil
}

func MCPFamilyManifest() (climanifest.Manifest, error) {
	return mcpFamilyManifestValue(), nil
}

// CommandByID returns one generated representative-family command record.
func CommandByID(id string) (climanifest.Command, error) {
	manifest, err := RepresentativeFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}

// WorkCommandByID returns one generated work-family command record.
func WorkCommandByID(id string) (climanifest.Command, error) {
	manifest, err := WorkFamilyManifest()
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
