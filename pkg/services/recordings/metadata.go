package recordings

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	metadataFactoryHash       = "factory_hash"
	metadataWorkersHash       = "workers_hash"
	metadataWorkstationsHash  = "workstations_hash"
	metadataRuntimeConfigHash = "runtime_config_hash"
)

// MetadataMismatchWarning describes replay metadata that differs from the
// currently selected Factory Definition.
type MetadataMismatchWarning struct {
	Key      string
	Artifact string
	Current  string
}

// FactoryMetadataWarnings compares the metadata recorded with a replay against
// the current Factory Definition. The replay remains authoritative.
func FactoryMetadataWarnings(
	artifactFactory *factorydefinitions.FactorySnapshot,
	currentFactory *factorydefinitions.FactorySnapshot,
) []MetadataMismatchWarning {
	artifactMetadata := factorySnapshotMetadata(artifactFactory)
	currentMetadata := factorySnapshotMetadata(currentFactory)
	keys := []string{
		metadataFactoryHash,
		metadataWorkersHash,
		metadataWorkstationsHash,
		metadataRuntimeConfigHash,
	}
	warnings := make([]MetadataMismatchWarning, 0, len(keys))
	for _, key := range keys {
		artifactValue := artifactMetadata[key]
		currentValue := currentMetadata[key]
		if artifactValue == "" || currentValue == "" || artifactValue == currentValue {
			continue
		}
		warnings = append(warnings, MetadataMismatchWarning{
			Key: key, Artifact: artifactValue, Current: currentValue,
		})
	}
	return warnings
}

func factorySnapshotMetadata(snapshot *factorydefinitions.FactorySnapshot) map[string]string {
	if snapshot == nil {
		return nil
	}
	var boundary struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(*snapshot, &boundary); err != nil {
		return nil
	}
	return boundary.Metadata
}
