package climanifestgen

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

// ExtractRepresentativeFamily returns manifest metadata for exactly the
// representative root/session-show command IDs declared in commands.json.
func ExtractRepresentativeFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "representative", RepresentativeFamilyCommandIDs)
}

// ExtractWorkFamily returns manifest metadata for exactly the work
// inspection/control command IDs declared in commands.json.
func ExtractWorkFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "work", WorkFamilyCommandIDs)
}

func extractFamily(
	manifest climanifest.Manifest,
	familyLabel string,
	commandIDs []string,
) (climanifest.Manifest, error) {
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing rootPath")
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing commands")
	}

	commands := make(map[string]climanifest.Command, len(commandIDs))
	for _, id := range commandIDs {
		record, ok := manifest.Commands[id]
		if !ok {
			return climanifest.Manifest{}, fmt.Errorf(
				"production manifest missing %s-family command %q",
				familyLabel,
				id,
			)
		}
		commands[id] = record
	}

	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands:      commands,
	}, nil
}
