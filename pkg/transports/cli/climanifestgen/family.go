package climanifestgen

import (
	"fmt"
	"slices"
)

// RepresentativeFamilyCommandIDs are the only stable command IDs the generator
// may emit for the B10 representative root/session-show cutover family.
var RepresentativeFamilyCommandIDs = []string{
	"you",
	"you.session",
	"you.session.show",
}

// IsRepresentativeFamilyCommandID reports whether id belongs to the representative family.
func IsRepresentativeFamilyCommandID(id string) bool {
	return slices.Contains(RepresentativeFamilyCommandIDs, id)
}

// AssertRepresentativeFamilyCommandID returns an error when id is outside the
// representative family scope.
func AssertRepresentativeFamilyCommandID(id string) error {
	if IsRepresentativeFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the representative family %v",
		id,
		RepresentativeFamilyCommandIDs,
	)
}
