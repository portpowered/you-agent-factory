// Package replay owns Factory-event artifact construction, reduction, and
// deterministic replay behavior.
package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

const (
	// CurrentSchemaVersion is the only replay artifact schema version this
	// package can currently load.
	CurrentSchemaVersion = interfaces.ReplayV1SourceFormat
)

const unavailableHistoricalFailureMessage = "Failure details were not recorded in this historical event."

func normalizeHistoricalFailureDetails(data []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if payload, ok := root["payload"].(map[string]any); ok {
		normalizeHistoricalFailureObject(payload)
	}
	if events, ok := root["events"].([]any); ok {
		for _, value := range events {
			event, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if payload, ok := event["payload"].(map[string]any); ok {
				normalizeHistoricalFailureObject(payload)
			}
		}
	}
	return json.Marshal(root)
}

func normalizeHistoricalFailureObject(object map[string]any) {
	detail, validCanonical := validHistoricalFailureDetail(object["failureDetail"])
	legacyReason, hasLegacyReason := trimmedString(object["failureReason"])
	legacyMessage, hasLegacyMessage := trimmedString(object["failureMessage"])
	errorClass, hasErrorClass := trimmedString(object["errorClass"])
	delete(object, "failureReason")
	delete(object, "failureMessage")
	delete(object, "errorClass")
	if validCanonical {
		object["failureDetail"] = detail
		return
	}
	if !hasLegacyReason && !hasLegacyMessage && !hasErrorClass {
		return
	}
	reason := "unknown"
	if hasLegacyReason {
		reason = normalizedHistoricalFailureReason(legacyReason)
	} else if hasErrorClass {
		reason = normalizedHistoricalFailureReason(errorClass)
	}
	if !hasLegacyMessage {
		legacyMessage = unavailableHistoricalFailureMessage
	}
	object["failureDetail"] = map[string]any{"reason": reason, "message": legacyMessage}
}

func validHistoricalFailureDetail(value any) (map[string]any, bool) {
	detail, ok := value.(map[string]any)
	if !ok || len(detail) != 2 {
		return nil, false
	}
	reason, hasReason := trimmedString(detail["reason"])
	message, hasMessage := trimmedString(detail["message"])
	if !hasReason || !hasMessage {
		return nil, false
	}
	if !isCanonicalFailureReason(reason) {
		return nil, false
	}
	return map[string]any{"reason": reason, "message": message}, true
}

func normalizedHistoricalFailureReason(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	if isCanonicalFailureReason(normalized) {
		return normalized
	}
	return "unknown"
}

func isCanonicalFailureReason(reason string) bool {
	switch reason {
	case "auth_failure",
		"misconfigured",
		"permanent_bad_request",
		"internal_server_error",
		"throttled",
		"timeout",
		"unknown":
		return true
	default:
		return false
	}
}

func trimmedString(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

// Save validates and writes an artifact as indented JSON.
func Save(storage platformreplay.Storage, path string, artifact *interfaces.ReplayArtifact) error {
	if storage == nil {
		return fmt.Errorf("replay artifact storage is required")
	}
	data, err := MarshalArtifact(artifact)
	if err != nil {
		return err
	}
	if err := storage.WriteFile(path, data); err != nil {
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
// runtime replay code. The Factory Definitions boundary decoder is supplied by
// composition so Recordings does not depend on Config's concrete adapter.
func Load(
	storage platformreplay.Storage,
	path string,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
) (*interfaces.ReplayArtifact, error) {
	if storage == nil {
		return nil, fmt.Errorf("replay artifact storage is required")
	}
	data, err := storage.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replay artifact %q: %w", path, err)
	}

	artifact, err := unmarshalReplayArtifact(data)
	if err != nil {
		return nil, fmt.Errorf("parse replay artifact %q: %w", path, err)
	}
	if err := hydrateArtifactFromEventsAtBoundary(artifact, decodeFactorySnapshot); err != nil {
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
