package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

// RunnableWorkCommandIDs returns contracted runnable command IDs for the work
// family in stable sorted order.
func RunnableWorkCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.WorkFamilyCommandIDs))
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
			return nil, err
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyWorkRunnableCoverage fails when any contracted runnable work-family
// command ID lacks a registered handwritten handler.
func (r *Registry) VerifyWorkRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableWorkCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"work runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}
