package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

// RunnableModelsDocsCommandIDs returns contracted runnable command IDs for the
// models/docs family in stable sorted order.
func RunnableModelsDocsCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.ModelsDocsFamilyCommandIDs))
	for _, commandID := range climanifestgen.ModelsDocsFamilyCommandIDs {
		if err := climanifestgen.AssertModelsDocsFamilyCommandID(commandID); err != nil {
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

// VerifyModelsDocsRunnableCoverage fails when any contracted runnable models/docs
// command ID lacks a registered handwritten handler.
func (r *Registry) VerifyModelsDocsRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableModelsDocsCommandIDs(manifest)
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
			"models/docs runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}
