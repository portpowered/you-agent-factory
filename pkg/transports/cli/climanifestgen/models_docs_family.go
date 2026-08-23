package climanifestgen

import (
	"fmt"
	"slices"
)

// ModelsDocsFamilyCommandIDs are the stable command IDs the generator may emit
// for the B11 models/docs cutover family.
var ModelsDocsFamilyCommandIDs = []string{
	"you.docs",
	"you.models",
	"you.models.list",
	"you.models.inspect",
	"you.models.invoke",
	"you.models.pull",
	"you.models.remove",
}

// IsModelsDocsFamilyCommandID reports whether id belongs to the models/docs family.
func IsModelsDocsFamilyCommandID(id string) bool {
	return slices.Contains(ModelsDocsFamilyCommandIDs, id)
}

// AssertModelsDocsFamilyCommandID returns an error when id is outside the
// models/docs family scope.
func AssertModelsDocsFamilyCommandID(id string) error {
	if IsModelsDocsFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the models/docs family %v",
		id,
		ModelsDocsFamilyCommandIDs,
	)
}
