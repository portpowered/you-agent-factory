package climanifestgen

import (
	"fmt"
	"slices"
)

// ProvidersFamilyCommandIDs are the stable command IDs emitted for the
// provider capability-discovery family.
var ProvidersFamilyCommandIDs = []string{
	"you.providers",
	"you.providers.list",
}

// IsProvidersFamilyCommandID reports whether id belongs to the providers
// capability-discovery family.
func IsProvidersFamilyCommandID(id string) bool {
	return slices.Contains(ProvidersFamilyCommandIDs, id)
}

// AssertProvidersFamilyCommandID rejects command IDs outside the providers
// capability-discovery family.
func AssertProvidersFamilyCommandID(id string) error {
	if IsProvidersFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the providers family %v",
		id,
		ProvidersFamilyCommandIDs,
	)
}
