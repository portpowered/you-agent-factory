// Package replay owns Factory-event artifact construction, reduction, and
// deterministic replay behavior.
package replay

import (
	"encoding/json"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
)

const (
	// CurrentSchemaVersion is the only replay artifact schema version this
	// package can currently load.
	CurrentSchemaVersion = "agent-factory.replay.v1"
)

const unavailableHistoricalFailureMessage = "Failure details were not recorded in this historical event."

func normalizeHistoricalFailureDetails(data []byte) ([]byte, error) {
	return platformreplay.NormalizeHistoricalFailureDetails(data)
}

// Save validates and writes an artifact as indented JSON.
func Save(path string, artifact *interfaces.ReplayArtifact) error {
	data, err := MarshalArtifact(artifact)
	if err != nil {
		return err
	}
	if err := platformreplay.WriteFile(path, data); err != nil {
		return fmt.Errorf("write replay artifact %q: %w", path, err)
	}
	return nil
}

// MarshalArtifact validates and serializes a replay artifact in the canonical
// indented JSON format used by artifact files.
func MarshalArtifact(artifact *interfaces.ReplayArtifact) ([]byte, error) {
	storageArtifact, err := artifactForStorage(artifact)
	if err != nil {
		return nil, err
	}
	if err := Validate(storageArtifact); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(storageArtifact, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal replay artifact: %w", err)
	}
	return append(data, '\n'), nil
}

// Load reads, decodes, and validates a replay artifact before returning it to
// runtime replay code.
func Load(path string) (*interfaces.ReplayArtifact, error) {
	data, err := platformreplay.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replay artifact %q: %w", path, err)
	}

	artifact, err := unmarshalReplayArtifact(data)
	if err != nil {
		return nil, fmt.Errorf("parse replay artifact %q: %w", path, err)
	}
	if err := hydrateArtifactFromEvents(artifact); err != nil {
		return nil, err
	}
	if err := Validate(artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func unmarshalReplayArtifact(data []byte) (*interfaces.ReplayArtifact, error) {
	normalized, err := normalizeHistoricalFailureDetails(data)
	if err != nil {
		return nil, err
	}
	var artifact interfaces.ReplayArtifact
	if err := json.Unmarshal(normalized, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

// Validate rejects artifacts that cannot be safely used as replay input.
func Validate(artifact *interfaces.ReplayArtifact) error {
	if err := validateReplayEventEnvelope(artifact); err != nil {
		return err
	}
	if !factorySnapshotHasConfig(artifact.Factory) {
		return errors.New("replay artifact factory is required")
	}
	return nil
}

func factorySnapshotHasConfig(snapshot *interfaces.FactorySnapshot) bool {
	if snapshot == nil {
		return false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(*snapshot, &object); err != nil {
		return false
	}
	for _, field := range []string{"workTypes", "resources", "workers", "workstations", "inputTypes", "id", "factoryDirectory", "sourceDirectory", "metadata"} {
		if _, ok := object[field]; ok {
			return true
		}
	}
	return false
}
