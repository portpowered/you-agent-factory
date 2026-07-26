package climanifestgen

import (
	"fmt"
	"slices"
)

// FactoryConfigInitFamilyCommandIDs are the stable command IDs the generator may
// emit for the B11 factory/config/init cutover family.
var FactoryConfigInitFamilyCommandIDs = []string{
	"you.config",
	"you.factory",
	"you.factory.config",
	"you.factory.config.expand",
	"you.factory.config.flatten",
	"you.factory.config.validate",
	"you.factory.create",
	"you.factory.delete",
	"you.factory.list",
	"you.factory.query",
	"you.factory.replace-current",
	"you.factory.update",
	"you.init",
}

// IsFactoryConfigInitFamilyCommandID reports whether id belongs to the
// factory/config/init family.
func IsFactoryConfigInitFamilyCommandID(id string) bool {
	return slices.Contains(FactoryConfigInitFamilyCommandIDs, id)
}

// AssertFactoryConfigInitFamilyCommandID returns an error when id is outside the
// factory/config/init family scope.
func AssertFactoryConfigInitFamilyCommandID(id string) error {
	if IsFactoryConfigInitFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the factory/config/init family %v",
		id,
		FactoryConfigInitFamilyCommandIDs,
	)
}
