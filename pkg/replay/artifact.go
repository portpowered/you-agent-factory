package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	// CurrentSchemaVersion is the only replay artifact schema version this
	// package can currently load.
	CurrentSchemaVersion = "agent-factory.replay.v1"

	replayArtifactReplaceAttempts = 20
	replayArtifactReplaceDelay    = 10 * time.Millisecond
)

// Save validates and writes an artifact as indented JSON.
func Save(path string, artifact *interfaces.ReplayArtifact) error {
	data, err := MarshalArtifact(artifact)
	if err != nil {
		return err
	}
	if err := writeReplayArtifactFile(path, data); err != nil {
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

func writeReplayArtifactFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create replay artifact directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create replay artifact temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write replay artifact temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync replay artifact temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close replay artifact temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err == nil {
		cleanupTemp = false
		return nil
	} else if runtime.GOOS != "windows" {
		return fmt.Errorf("replace replay artifact with temp file: %w; temp artifact left at %s", err, tmpPath)
	}

	// Windows readers can briefly block deletion while the recorder streams
	// updates and consumers poll Load. Keep the completed temp file recoverable
	// while retrying the replace.
	var replaceErr error
	for attempt := 0; attempt < replayArtifactReplaceAttempts; attempt++ {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			replaceErr = fmt.Errorf("remove previous replay artifact before replace: %w", err)
		} else if err := os.Rename(tmpPath, path); err != nil {
			replaceErr = fmt.Errorf("replace replay artifact from temp file: %w", err)
		} else {
			cleanupTemp = false
			return nil
		}
		time.Sleep(replayArtifactReplaceDelay)
	}
	return fmt.Errorf("%w; temp artifact left at %s", replaceErr, tmpPath)
}

// Load reads, decodes, and validates a replay artifact before returning it to
// runtime replay code.
func Load(path string) (*interfaces.ReplayArtifact, error) {
	data, err := readReplayArtifactFile(path)
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

func readReplayArtifactFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil || runtime.GOOS != "windows" {
		return data, err
	}

	lastErr := err
	for attempt := 0; attempt < replayArtifactReplaceAttempts; attempt++ {
		time.Sleep(replayArtifactReplaceDelay)
		data, err = os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

func unmarshalReplayArtifact(data []byte) (*interfaces.ReplayArtifact, error) {
	var artifact interfaces.ReplayArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

// Validate rejects artifacts that cannot be safely used as replay input.
func Validate(artifact *interfaces.ReplayArtifact) error {
	if err := validateReplayEventEnvelope(artifact); err != nil {
		return err
	}
	if !generatedFactoryHasConfig(artifact.Factory) {
		return errors.New("replay artifact factory is required")
	}
	return nil
}

const (
	legacyDispatchRunnerIDField                 = "runnerId"
	legacyDispatchRunnerSelectionSourceField    = "runnerSelectionSource"
	dispatchProviderModelProviderField          = "modelProvider"
	dispatchProviderModelProviderSelectionField = "modelProviderSelectionSource"
)

func normalizeDispatchProviderEvents(events []factoryapi.FactoryEvent) error {
	for index := range events {
		if err := normalizeDispatchProviderEvent(&events[index]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDispatchProviderEvent(event *factoryapi.FactoryEvent) error {
	if event == nil {
		return nil
	}
	switch event.Type {
	case factoryapi.FactoryEventTypeDispatchRequest:
		return normalizeDispatchRequestProviderMetadata(event)
	case factoryapi.FactoryEventTypeDispatchQueued:
		return normalizeDispatchQueuedProviderMetadata(event)
	default:
		return nil
	}
}

func normalizeDispatchRequestProviderMetadata(event *factoryapi.FactoryEvent) error {
	payloadJSON, err := event.Payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal dispatch request payload for event %q: %w", event.Id, err)
	}
	var raw struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return fmt.Errorf("decode dispatch request payload for event %q: %w", event.Id, err)
	}
	if raw.Metadata == nil {
		return nil
	}
	if err := normalizeLegacyDispatchProviderFields(raw.Metadata); err != nil {
		return fmt.Errorf("normalize dispatch request metadata for event %q: %w", event.Id, err)
	}
	return rewriteDispatchRequestPayload(event, raw.Metadata)
}

func normalizeDispatchQueuedProviderMetadata(event *factoryapi.FactoryEvent) error {
	payloadJSON, err := event.Payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal dispatch queued payload for event %q: %w", event.Id, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return fmt.Errorf("decode dispatch queued payload for event %q: %w", event.Id, err)
	}
	if err := normalizeLegacyDispatchProviderFields(raw); err != nil {
		return fmt.Errorf("normalize dispatch queued payload for event %q: %w", event.Id, err)
	}
	return rewriteDispatchQueuedPayload(event, raw)
}

func normalizeLegacyDispatchProviderFields(fields map[string]any) error {
	if fields == nil {
		return nil
	}
	if _, hasModelProvider := fields[dispatchProviderModelProviderField]; hasModelProvider {
		delete(fields, legacyDispatchRunnerIDField)
		delete(fields, legacyDispatchRunnerSelectionSourceField)
		return nil
	}
	legacyRunnerID, hasRunnerID := stringFieldValue(fields, legacyDispatchRunnerIDField)
	if !hasRunnerID {
		return nil
	}
	public, err := interfaces.PublicModelProviderFromLegacyRunnerID(legacyRunnerID)
	if err != nil {
		return err
	}
	fields[dispatchProviderModelProviderField] = string(public)
	delete(fields, legacyDispatchRunnerIDField)

	if legacySource, ok := stringFieldValue(fields, legacyDispatchRunnerSelectionSourceField); ok {
		publicSource := interfaces.PublicModelProviderSelectionSourceFromLegacyRunnerSelectionSource(legacySource)
		fields[dispatchProviderModelProviderSelectionField] = string(publicSource)
		delete(fields, legacyDispatchRunnerSelectionSourceField)
	}
	return nil
}

func rewriteDispatchRequestPayload(event *factoryapi.FactoryEvent, metadata map[string]any) error {
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch request payload for event %q: %w", event.Id, err)
	}
	if len(metadata) == 0 {
		payload.Metadata = nil
	} else {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("encode dispatch request metadata for event %q: %w", event.Id, err)
		}
		var normalized factoryapi.DispatchRequestEventMetadata
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			return fmt.Errorf("decode normalized dispatch request metadata for event %q: %w", event.Id, err)
		}
		payload.Metadata = &normalized
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		return fmt.Errorf("encode dispatch request payload for event %q: %w", event.Id, err)
	}
	event.Payload = union
	return nil
}

func rewriteDispatchQueuedPayload(event *factoryapi.FactoryEvent, raw map[string]any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode dispatch queued payload for event %q: %w", event.Id, err)
	}
	var payload factoryapi.DispatchQueuedEventPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return fmt.Errorf("decode normalized dispatch queued payload for event %q: %w", event.Id, err)
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchQueuedEventPayload(payload); err != nil {
		return fmt.Errorf("encode dispatch queued payload for event %q: %w", event.Id, err)
	}
	event.Payload = union
	return nil
}

func stringFieldValue(fields map[string]any, key string) (string, bool) {
	value, ok := fields[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}
