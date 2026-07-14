package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

// RunnableFactoryConfigInitCommandIDs returns contracted runnable command IDs for
// the factory/config/init family in stable sorted order.
func RunnableFactoryConfigInitCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	for _, commandID := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID(commandID); err != nil {
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

// VerifyFactoryConfigInitRunnableCoverage fails when any contracted runnable
// factory/config/init command ID lacks a registered handwritten handler.
func (r *Registry) VerifyFactoryConfigInitRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableFactoryConfigInitCommandIDs(manifest)
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
			"factory/config/init runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}
