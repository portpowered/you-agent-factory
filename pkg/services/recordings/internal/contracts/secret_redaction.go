package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RecordingSecretProvenance identifies why a value is classified as secret at
// the Recordings boundary. The value is deliberately a closed vocabulary: a
// caller cannot turn an unrecognized classification into a plaintext write.
type RecordingSecretProvenance string

const (
	RecordingSecretProvenanceDeclared RecordingSecretProvenance = "DECLARED_SECRET"
)

// RecordingSecret identifies one already-classified value by its JSON Pointer
// location. Recordings never compares the value at the location with another
// value; the explicit classification is the only redaction decision.
type RecordingSecret struct {
	JSONPointer string                    `json:"jsonPointer"`
	Provenance  RecordingSecretProvenance `json:"provenance"`
}

// RecordingRedactionRequest is the pure input to the Recordings-owned
// write-boundary transformation. Secrets contains only provenance and
// locations; it intentionally does not duplicate or hash the classified
// values.
type RecordingRedactionRequest struct {
	Payload json.RawMessage   `json:"payload"`
	Secrets []RecordingSecret `json:"secrets,omitempty"`
}

// RecordingRedactedValue is the stable typed value persisted in place of a
// declared secret. It carries no original value, digest, prefix, suffix, or
// length.
type RecordingRedactedValue struct {
	Redacted   bool                      `json:"redacted"`
	Provenance RecordingSecretProvenance `json:"provenance"`
}

// RecordingRedactionResult contains the detached safe payload and the number
// of classified locations replaced by the typed redaction value.
type RecordingRedactionResult struct {
	Payload       json.RawMessage
	RedactedCount int
}

var (
	ErrInvalidRecordingRedactionRequest = errors.New("invalid recording redaction request")
	ErrInvalidRecordingSecretProvenance = errors.New("invalid recording secret provenance")
	ErrInvalidRecordingSecretPath       = errors.New("invalid recording secret path")
	ErrRecordingSecretPathNotFound      = errors.New("recording secret path not found")
	ErrDuplicateRecordingSecretPath     = errors.New("duplicate recording secret path")
)

// Validate reports whether value is the one valid persisted redaction marker.
func (value RecordingRedactedValue) Validate() error {
	if !value.Redacted || value.Provenance != RecordingSecretProvenanceDeclared {
		return ErrInvalidRecordingSecretProvenance
	}
	return nil
}

// MarshalJSON prevents an invalid marker from becoming a persistence value.
func (value RecordingRedactedValue) MarshalJSON() ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	type persistedValue RecordingRedactedValue
	return json.Marshal(persistedValue(value))
}

// UnmarshalJSON validates the stable marker when a caller decodes one from a
// recording. It never includes the encoded value in an error.
func (value *RecordingRedactedValue) UnmarshalJSON(data []byte) error {
	type persistedValue RecordingRedactedValue
	var decoded persistedValue
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ErrInvalidRecordingSecretProvenance
	}
	validated := RecordingRedactedValue(decoded)
	if err := validated.Validate(); err != nil {
		return err
	}
	*value = validated
	return nil
}

// RedactDeclaredSecrets replaces every explicitly classified JSON Pointer in
// request.Payload before the caller serializes or publishes the result. An
// empty Secrets list returns a detached copy of the original JSON bytes.
func RedactDeclaredSecrets(request RecordingRedactionRequest) (RecordingRedactionResult, error) {
	paths, err := validateRecordingRedactionRequest(request)
	if err != nil {
		return RecordingRedactionResult{}, err
	}
	if len(paths) == 0 {
		return RecordingRedactionResult{Payload: append(json.RawMessage(nil), request.Payload...)}, nil
	}

	document, err := decodeRecordingJSON(request.Payload)
	if err != nil {
		return RecordingRedactionResult{}, err
	}
	marker := RecordingRedactedValue{
		Redacted:   true,
		Provenance: RecordingSecretProvenanceDeclared,
	}
	for _, path := range paths {
		document, err = replaceRecordingJSONAt(document, path, marker)
		if err != nil {
			return RecordingRedactionResult{}, err
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return RecordingRedactionResult{}, fmt.Errorf("encode redacted recording payload: %w", err)
	}
	return RecordingRedactionResult{
		Payload:       encoded,
		RedactedCount: len(paths),
	}, nil
}

// RedactCanonicalEvents returns a detached event slice whose classified
// payload values have been replaced before an enclosing recording is
// serialized. The optional map is keyed by event index, and each pointer is
// relative to that event's Payload. Provenance is not copied into the result.
func RedactCanonicalEvents(
	events []CanonicalEvent,
	provenance ...map[int][]RecordingSecret,
) ([]CanonicalEvent, int, error) {
	redacted := make([]CanonicalEvent, len(events))
	redactedCount := 0
	var eventProvenance map[int][]RecordingSecret
	if len(provenance) > 0 {
		eventProvenance = provenance[0]
	}
	for index, secrets := range eventProvenance {
		if len(secrets) == 0 {
			continue
		}
		if index < 0 || index >= len(events) {
			return nil, 0, fmt.Errorf("%w: event %d", ErrRecordingSecretPathNotFound, index)
		}
	}
	for index, event := range events {
		redacted[index] = event
		secrets := eventProvenance[index]
		if len(secrets) == 0 {
			continue
		}
		result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
			Payload: []byte(event.Payload), Secrets: secrets,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("redact recording event %d: %w", index, err)
		}
		redacted[index].Payload = string(result.Payload)
		redactedCount += result.RedactedCount
	}
	return redacted, redactedCount, nil
}

// RedactPortableArtifact applies event-local provenance before
// portable-artifact bytes are produced. The returned artifact is detached and
// contains no provenance handoff.
func RedactPortableArtifact(artifact PortableArtifact) (PortableArtifact, int, error) {
	redactedEvents, redactedCount, err := RedactCanonicalEvents(
		artifact.Events, artifact.SecretProvenance,
	)
	if err != nil {
		return PortableArtifact{}, 0, err
	}
	redacted := artifact
	redacted.Events = redactedEvents
	redacted.SecretProvenance = nil
	return redacted, redactedCount, nil
}

// RedactPortableRecording applies document-level provenance to a portable
// recording and updates its existing bounded secret-redaction count. The
// result contains no original value or provenance handoff.
func RedactPortableRecording(recording PortableRecording) (PortableRecording, int, error) {
	secrets := cloneRecordingSecrets(recording.SecretProvenance)
	recording.SecretProvenance = nil
	if len(secrets) == 0 {
		return recording, 0, nil
	}
	payload, err := json.Marshal(recording)
	if err != nil {
		return PortableRecording{}, 0, fmt.Errorf("encode portable recording for redaction: %w", err)
	}
	result, err := RedactDeclaredSecrets(RecordingRedactionRequest{
		Payload: payload, Secrets: secrets,
	})
	if err != nil {
		return PortableRecording{}, 0, fmt.Errorf("redact portable recording: %w", err)
	}
	var redacted PortableRecording
	if err := json.Unmarshal(result.Payload, &redacted); err != nil {
		return PortableRecording{}, 0, fmt.Errorf("decode redacted portable recording: %w", err)
	}
	redacted.SecretProvenance = nil
	if redacted.Result != nil && len(redacted.Result.PrimaryResult) > 0 {
		digest := sha256.Sum256(compactPortableRecordingJSON(redacted.Result.PrimaryResult))
		redacted.Result.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
	}
	redacted.Redaction.SecretsRedacted = saturatingPortableRecordingSecretsRedacted(
		recordedSecretsCount(recording.Redaction.SecretsRedacted), int64(result.RedactedCount),
	)
	return redacted, result.RedactedCount, nil
}

func recordedSecretsCount(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func cloneRecordingSecrets(secrets []RecordingSecret) []RecordingSecret {
	if len(secrets) == 0 {
		return nil
	}
	return append([]RecordingSecret(nil), secrets...)
}

func validateRecordingRedactionRequest(request RecordingRedactionRequest) ([][]string, error) {
	if len(request.Payload) == 0 || !json.Valid(request.Payload) {
		return nil, ErrInvalidRecordingRedactionRequest
	}
	paths := make([][]string, 0, len(request.Secrets))
	seen := make(map[string]struct{}, len(request.Secrets))
	for index, secret := range request.Secrets {
		if secret.Provenance != RecordingSecretProvenanceDeclared {
			return nil, fmt.Errorf("%w: classified value %d", ErrInvalidRecordingSecretProvenance, index)
		}
		path, err := parseRecordingJSONPointer(secret.JSONPointer)
		if err != nil {
			return nil, fmt.Errorf("%w: classified value %d", ErrInvalidRecordingSecretPath, index)
		}
		key, err := json.Marshal(path)
		if err != nil {
			return nil, ErrInvalidRecordingSecretPath
		}
		if _, exists := seen[string(key)]; exists {
			return nil, fmt.Errorf("%w: classified value %d", ErrDuplicateRecordingSecretPath, index)
		}
		seen[string(key)] = struct{}{}
		paths = append(paths, path)
	}
	return paths, nil
}

func decodeRecordingJSON(payload json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, ErrInvalidRecordingRedactionRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidRecordingRedactionRequest
	}
	return document, nil
}

func parseRecordingJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, ErrInvalidRecordingSecretPath
	}
	parts := strings.Split(pointer[1:], "/")
	decoded := make([]string, len(parts))
	for index, part := range parts {
		value, err := decodeRecordingJSONPointerToken(part)
		if err != nil {
			return nil, err
		}
		decoded[index] = value
	}
	return decoded, nil
}

func decodeRecordingJSONPointerToken(token string) (string, error) {
	if strings.IndexByte(token, '~') < 0 {
		return token, nil
	}
	var builder strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			builder.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", ErrInvalidRecordingSecretPath
		}
		index++
		switch token[index] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", ErrInvalidRecordingSecretPath
		}
	}
	return builder.String(), nil
}

func replaceRecordingJSONAt(document any, path []string, replacement RecordingRedactedValue) (any, error) {
	if len(path) == 0 {
		return replacement, nil
	}
	switch value := document.(type) {
	case map[string]any:
		child, ok := value[path[0]]
		if !ok {
			return nil, ErrRecordingSecretPathNotFound
		}
		replaced, err := replaceRecordingJSONAt(child, path[1:], replacement)
		if err != nil {
			return nil, err
		}
		value[path[0]] = replaced
		return document, nil
	case []any:
		index, err := recordingJSONArrayIndex(path[0], len(value))
		if err != nil {
			return nil, err
		}
		replaced, err := replaceRecordingJSONAt(value[index], path[1:], replacement)
		if err != nil {
			return nil, err
		}
		value[index] = replaced
		return document, nil
	default:
		return nil, ErrRecordingSecretPathNotFound
	}
}

func recordingJSONArrayIndex(segment string, length int) (int, error) {
	if segment == "" || (len(segment) > 1 && segment[0] == '0') {
		return 0, ErrInvalidRecordingSecretPath
	}
	index, err := strconv.Atoi(segment)
	if err != nil || index < 0 || index >= length {
		return 0, ErrRecordingSecretPathNotFound
	}
	return index, nil
}
