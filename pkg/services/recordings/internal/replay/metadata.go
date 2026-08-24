// Package replay owns metadata-only reads for the versioned replay artifacts.
package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
)

// LoadMetadata reads only the identity-bearing metadata of either replay
// artifact version. V2 returns after validating its header; legacy JSON scans
// event contexts one at a time so the event history is never retained.
func LoadMetadata(
	openFile func(string) (io.ReadCloser, error),
	path string,
) (recordingcontracts.ReplayInputMetadata, error) {
	if openFile == nil {
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("replay artifact metadata opener is required")
	}
	file, err := openFile(path)
	if err != nil {
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("open replay artifact %q: %w", path, err)
	}
	reader := bufio.NewReader(file)
	firstLine, readErr := reader.ReadBytes('\n')
	_ = file.Close()
	if readErr != nil && readErr != io.EOF {
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("read replay artifact header %q: %w", path, readErr)
	}
	if len(firstLine) == 0 {
		if readErr != nil {
			return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("read replay artifact header %q: %w", path, readErr)
		}
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("read replay artifact header %q: empty replay artifact", path)
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(firstLine), &envelope); err == nil && envelope.SchemaVersion == ReplayV2SchemaVersion {
		var header ReplayV2Header
		if err := json.Unmarshal(bytes.TrimSpace(firstLine), &header); err != nil {
			return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("parse replay v2 header %q: %w", path, err)
		}
		if err := validateReplayV2Header(header); err != nil {
			return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("parse replay v2 header %q: %w", path, err)
		}
		return recordingcontracts.ReplayInputMetadata{FactorySessionID: strings.TrimSpace(header.SessionID)}, nil
	}
	return loadReplayV1Metadata(openFile, path)
}

func loadReplayV1Metadata(
	openFile func(string) (io.ReadCloser, error),
	path string,
) (recordingcontracts.ReplayInputMetadata, error) {
	file, err := openFile(path)
	if err != nil {
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("open legacy replay artifact %q: %w", path, err)
	}
	defer file.Close()
	sessionID, err := decodeReplayV1Metadata(file)
	if err != nil {
		return recordingcontracts.ReplayInputMetadata{}, fmt.Errorf("decode legacy replay artifact %q: %w", path, err)
	}
	return recordingcontracts.ReplayInputMetadata{FactorySessionID: sessionID}, nil
}

func decodeReplayV1Metadata(reader io.Reader) (string, error) {
	decoder := json.NewDecoder(reader)
	if err := expectJSONDelimiter(decoder, '{', "legacy replay artifact must contain a JSON object"); err != nil {
		return "", err
	}
	sessionID, err := replayV1ObjectSessionID(decoder)
	if err != nil {
		return "", err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", err
	}
	return sessionID, nil
}

func replayV1ObjectSessionID(decoder *json.Decoder) (string, error) {
	var sessionID string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return "", err
		}
		keyText, ok := key.(string)
		if !ok {
			return "", fmt.Errorf("legacy replay artifact contains a non-string field name")
		}
		candidate, err := replayV1ObjectFieldSessionID(decoder, keyText)
		if err != nil {
			return "", err
		}
		if err := mergeReplaySessionID(&sessionID, candidate); err != nil {
			return "", err
		}
	}
	if err := expectJSONDelimiter(decoder, '}', "legacy replay artifact object is not terminated"); err != nil {
		return "", err
	}
	return sessionID, nil
}

func replayV1ObjectFieldSessionID(decoder *json.Decoder, key string) (string, error) {
	if key != "events" {
		return "", skipJSONValue(decoder)
	}
	return replayV1EventsSessionID(decoder)
}

func replayV1EventsSessionID(decoder *json.Decoder) (string, error) {
	if err := expectJSONDelimiter(decoder, '[', "events must be an array"); err != nil {
		return "", err
	}
	var sessionID string
	for decoder.More() {
		candidate, err := replayV1EventSessionID(decoder)
		if err != nil {
			return "", err
		}
		if err := mergeReplaySessionID(&sessionID, candidate); err != nil {
			return "", err
		}
	}
	if err := expectJSONDelimiter(decoder, ']', "events array is not terminated"); err != nil {
		return "", err
	}
	return sessionID, nil
}

func replayV1EventSessionID(decoder *json.Decoder) (string, error) {
	if err := expectJSONDelimiter(decoder, '{', "event must be an object"); err != nil {
		return "", err
	}
	var sessionID string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return "", err
		}
		keyText, ok := key.(string)
		if !ok {
			return "", fmt.Errorf("event contains a non-string field name")
		}
		candidate, err := replayV1EventFieldSessionID(decoder, keyText)
		if err != nil {
			return "", err
		}
		if err := mergeReplaySessionID(&sessionID, candidate); err != nil {
			return "", err
		}
	}
	if err := expectJSONDelimiter(decoder, '}', "event object is not terminated"); err != nil {
		return "", err
	}
	return sessionID, nil
}

func replayV1EventFieldSessionID(decoder *json.Decoder, key string) (string, error) {
	if key != "context" {
		return "", skipJSONValue(decoder)
	}
	return replayV1ContextSessionID(decoder)
}

func replayV1ContextSessionID(decoder *json.Decoder) (string, error) {
	if err := expectJSONDelimiter(decoder, '{', "event context must be an object"); err != nil {
		return "", err
	}
	var sessionID string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return "", err
		}
		keyText, ok := key.(string)
		if !ok {
			return "", fmt.Errorf("event context contains a non-string field name")
		}
		candidate, err := replayV1ContextFieldSessionID(decoder, keyText)
		if err != nil {
			return "", err
		}
		if err := mergeReplaySessionID(&sessionID, candidate); err != nil {
			return "", err
		}
	}
	if err := expectJSONDelimiter(decoder, '}', "event context is not terminated"); err != nil {
		return "", err
	}
	return sessionID, nil
}

func replayV1ContextFieldSessionID(decoder *json.Decoder, key string) (string, error) {
	if key != "sessionId" {
		return "", skipJSONValue(decoder)
	}
	value, err := decoder.Token()
	if err != nil {
		return "", err
	}
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("event context sessionId must be a string")
	}
}

func mergeReplaySessionID(current *string, candidate string) error {
	if candidate == "" {
		return nil
	}
	if *current != "" && *current != candidate {
		return fmt.Errorf("legacy replay input contains multiple Factory Session UUIDs")
	}
	*current = candidate
	return nil
}

func expectJSONDelimiter(decoder *json.Decoder, want json.Delim, message string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != want {
		return fmt.Errorf("%s", message)
	}
	return nil
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	_, err = decoder.Token()
	return err
}

func requireJSONEOF(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unexpected trailing JSON token %v", token)
}
