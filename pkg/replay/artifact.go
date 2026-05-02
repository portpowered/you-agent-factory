package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

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
