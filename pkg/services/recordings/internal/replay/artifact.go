// Package replay owns Factory-event artifact construction, reduction, and
// deterministic replay behavior.
package replay

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
)

const (
	// CurrentSchemaVersion is the only replay artifact schema version this
	// package can currently load.
	CurrentSchemaVersion = interfaces.ReplayV1SourceFormat
	// ReplayV2SchemaVersion identifies the append-only JSONL recording format.
	ReplayV2SchemaVersion = "agent-factory.replay.v2"

	replayV2RecordHeader   = "header"
	replayV2RecordEvent    = "event"
	replayV2RecordTerminal = "terminal"
	maxReplayV2LineBytes   = 16 * 1024 * 1024
)

// ReplayV2FactoryIdentity is the stable Factory identity repeated in a v2
// header. The complete Factory snapshot remains in the run-started event so
// replay values are sourced from the same canonical event history as v1.
type ReplayV2FactoryIdentity struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	SourceDirectory  string `json:"sourceDirectory"`
}

// ReplayV2Header is the first record in an append-only replay artifact.
type ReplayV2Header struct {
	RecordType      string                  `json:"recordType"`
	SchemaVersion   string                  `json:"schemaVersion"`
	RecordedAt      time.Time               `json:"recordedAt"`
	SessionID       string                  `json:"sessionId"`
	FactoryIdentity ReplayV2FactoryIdentity `json:"factoryIdentity"`
	Hashes          map[string]string       `json:"hashes"`
}

// ReplayV2FlushDiagnostics contains only safe terminal persistence facts. In
// particular, it deliberately excludes failure messages, targets, and event
// payloads, which may contain customer data or secrets.
type ReplayV2FlushDiagnostics struct {
	FailureCount int      `json:"failureCount,omitempty"`
	FailureCodes []string `json:"failureCodes,omitempty"`
}

// ReplayV2TerminalRecord is the final normal-completion framing record.
type ReplayV2TerminalRecord struct {
	RecordType       string                   `json:"recordType"`
	FinishedAt       time.Time                `json:"finishedAt"`
	TerminalState    string                   `json:"terminalState"`
	FlushDiagnostics ReplayV2FlushDiagnostics `json:"flushDiagnostics"`
}

// ReplayV2Stream is the validated complete prefix of one v2 JSONL artifact.
// TruncatedTail is true when the final physical line was incomplete; all
// preceding complete records remain available in Header and Events.
type ReplayV2Stream struct {
	Header        ReplayV2Header
	Events        []interfaces.FactoryEvent
	Terminal      *ReplayV2TerminalRecord
	TruncatedTail bool
}

// ReplayReadMetadata reports version-specific framing facts without changing
// the normalized ReplayArtifact returned to runtime replay consumers.
type ReplayReadMetadata struct {
	SchemaVersion string
	V2            *ReplayV2Stream
}

type replayV2EventRecord struct {
	RecordType string                  `json:"recordType"`
	Event      interfaces.FactoryEvent `json:"event"`
}

// MarshalReplayV2Header encodes one complete v2 header line. It is pure and
// does not perform any filesystem operation or mutate the supplied artifact.
func MarshalReplayV2Header(
	artifact *interfaces.ReplayArtifact,
	sessionID string,
) ([]byte, error) {
	if artifact == nil {
		return nil, fmt.Errorf("replay artifact is required")
	}
	recordedAt := artifact.RecordedAt
	if recordedAt.IsZero() && len(artifact.Events) > 0 {
		recordedAt = artifact.Events[0].Context.EventTime
	}
	if recordedAt.IsZero() && artifact.WallClock != nil {
		recordedAt = artifact.WallClock.StartedAt
	}
	if strings.TrimSpace(sessionID) == "" && len(artifact.Events) > 0 {
		if value := artifact.Events[0].Context.SessionID; value != nil {
			sessionID = *value
		}
	}
	header := ReplayV2Header{
		RecordType:      replayV2RecordHeader,
		SchemaVersion:   ReplayV2SchemaVersion,
		RecordedAt:      recordedAt.UTC(),
		SessionID:       strings.TrimSpace(sessionID),
		FactoryIdentity: replayV2FactoryIdentityFromSnapshot(artifact.Factory),
		Hashes:          replayV2HashesFromSnapshot(artifact.Factory),
	}
	if err := validateReplayV2Header(header); err != nil {
		return nil, err
	}
	return marshalReplayV2Line(header)
}

// MarshalReplayV2Event encodes one complete v2 event line.
func MarshalReplayV2Event(event interfaces.FactoryEvent) ([]byte, error) {
	if event.SchemaVersion != interfaces.FactoryEventSchemaVersionV1 {
		return nil, fmt.Errorf("replay v2 event %q has unsupported schemaVersion %q", event.Id, event.SchemaVersion)
	}
	if event.Id == "" || event.Type == "" || event.Context.EventTime.IsZero() {
		return nil, fmt.Errorf("replay v2 event requires id, type, and eventTime")
	}
	if !json.Valid(event.Payload) {
		return nil, fmt.Errorf("replay v2 event %q payload is invalid JSON", event.Id)
	}
	return marshalReplayV2Line(replayV2EventRecord{
		RecordType: replayV2RecordEvent,
		Event:      event,
	})
}

// MarshalReplayV2Terminal encodes one complete v2 terminal line. Callers pass
// already-classified safe diagnostics rather than raw errors.
func MarshalReplayV2Terminal(
	finishedAt time.Time,
	terminalState string,
	diagnostics ReplayV2FlushDiagnostics,
) ([]byte, error) {
	if finishedAt.IsZero() {
		return nil, fmt.Errorf("replay v2 terminal finishedAt is required")
	}
	if strings.TrimSpace(terminalState) == "" {
		return nil, fmt.Errorf("replay v2 terminal terminalState is required")
	}
	diagnostics.FailureCodes = safeReplayV2FailureCodes(diagnostics.FailureCodes)
	if diagnostics.FailureCount < len(diagnostics.FailureCodes) {
		diagnostics.FailureCount = len(diagnostics.FailureCodes)
	}
	return marshalReplayV2Line(ReplayV2TerminalRecord{
		RecordType:       replayV2RecordTerminal,
		FinishedAt:       finishedAt.UTC(),
		TerminalState:    strings.TrimSpace(terminalState),
		FlushDiagnostics: diagnostics,
	})
}

func marshalReplayV2Line(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal replay v2 record: %w", err)
	}
	return append(data, '\n'), nil
}

func replayV2FactoryIdentityFromSnapshot(snapshot *interfaces.FactorySnapshot) ReplayV2FactoryIdentity {
	identity := ReplayV2FactoryIdentity{
		ID:               "unknown",
		Name:             "unknown",
		FactoryDirectory: "unknown",
		SourceDirectory:  "unknown",
	}
	if snapshot == nil {
		return identity
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(*snapshot, &object); err != nil {
		return identity
	}
	identity.ID = replayV2StringField(object, "id")
	identity.Name = replayV2StringField(object, "name")
	identity.FactoryDirectory = replayV2StringField(object, "factoryDirectory")
	identity.SourceDirectory = replayV2StringField(object, "sourceDirectory")
	if identity.ID == "" {
		identity.ID = identity.Name
	}
	if identity.Name == "" {
		identity.Name = identity.ID
	}
	if identity.FactoryDirectory == "" {
		identity.FactoryDirectory = identity.SourceDirectory
	}
	if identity.SourceDirectory == "" {
		identity.SourceDirectory = identity.FactoryDirectory
	}
	identity.ID = nonEmptyReplayV2Metadata(identity.ID)
	identity.Name = nonEmptyReplayV2Metadata(identity.Name)
	identity.FactoryDirectory = nonEmptyReplayV2Metadata(identity.FactoryDirectory)
	identity.SourceDirectory = nonEmptyReplayV2Metadata(identity.SourceDirectory)
	return identity
}

func replayV2HashesFromSnapshot(snapshot *interfaces.FactorySnapshot) map[string]string {
	var snapshotData []byte
	if snapshot != nil {
		snapshotData = []byte(*snapshot)
	}
	var object struct {
		Metadata map[string]string `json:"metadata"`
	}
	if len(snapshotData) > 0 {
		_ = json.Unmarshal(snapshotData, &object)
	}
	hashes := make(map[string]string, 4)
	for _, key := range []string{
		metadataFactoryHash,
		metadataWorkersHash,
		metadataWorkstationsHash,
		metadataRuntimeConfigHash,
	} {
		value := strings.TrimSpace(object.Metadata[key])
		if value == "" {
			value = replayV2SnapshotHash(snapshotData, key)
		}
		hashes[key] = value
	}
	return hashes
}

func replayV2SnapshotHash(snapshot []byte, key string) string {
	digest := sha256.Sum256(append([]byte(key+":"), snapshot...))
	return fmt.Sprintf("sha256:%x", digest)
}

func nonEmptyReplayV2Metadata(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func validateReplayV2Header(header ReplayV2Header) error {
	if header.RecordType != replayV2RecordHeader {
		return fmt.Errorf("replay v2 header recordType must be %q", replayV2RecordHeader)
	}
	if header.SchemaVersion != ReplayV2SchemaVersion {
		return fmt.Errorf("unsupported replay v2 header schemaVersion %q", header.SchemaVersion)
	}
	if header.RecordedAt.IsZero() {
		return fmt.Errorf("replay v2 header recordedAt is required")
	}
	if _, err := uuid.Parse(strings.TrimSpace(header.SessionID)); err != nil {
		return fmt.Errorf("replay v2 header sessionId must be a UUID: %w", err)
	}
	if strings.TrimSpace(header.FactoryIdentity.ID) == "" ||
		strings.TrimSpace(header.FactoryIdentity.Name) == "" ||
		strings.TrimSpace(header.FactoryIdentity.FactoryDirectory) == "" ||
		strings.TrimSpace(header.FactoryIdentity.SourceDirectory) == "" {
		return fmt.Errorf("replay v2 header factoryIdentity requires id, name, factoryDirectory, and sourceDirectory")
	}
	for _, key := range []string{
		metadataFactoryHash,
		metadataWorkersHash,
		metadataWorkstationsHash,
		metadataRuntimeConfigHash,
	} {
		if strings.TrimSpace(header.Hashes[key]) == "" {
			return fmt.Errorf("replay v2 header hashes requires %q", key)
		}
	}
	return nil
}

func replayV2StringField(object map[string]json.RawMessage, key string) string {
	var value string
	if err := json.Unmarshal(object[key], &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func safeReplayV2FailureCodes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || !safeReplayV2Code(code) {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

func safeReplayV2Code(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

// IsReplayV2Artifact recognizes the v2 framing version from the first
// complete physical line without interpreting later records.
func IsReplayV2Artifact(data []byte) bool {
	firstLine := data
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		firstLine = data[:index]
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(firstLine), &envelope); err != nil {
		return false
	}
	return envelope.SchemaVersion == ReplayV2SchemaVersion
}

// ParseReplayV2 validates the JSONL framing and returns its complete prefix.
// A syntactically incomplete final line is recoverable; malformed complete
// records, invalid order, duplicate framing, and missing headers are errors.
func ParseReplayV2(data []byte) (*ReplayV2Stream, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("replay v2 artifact is empty")
	}
	stream := &ReplayV2Stream{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), maxReplayV2LineBytes)
	finalLineMayBeIncomplete := len(data) > 0 && data[len(data)-1] != '\n'
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		truncated, err := parseReplayV2Line(
			stream,
			bytes.TrimSpace(scanner.Bytes()),
			lineNumber,
			finalLineMayBeIncomplete,
		)
		if err != nil {
			return nil, err
		}
		if truncated {
			stream.TruncatedTail = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan replay v2 artifact: %w", err)
	}
	if stream.Header.SchemaVersion == "" {
		return nil, fmt.Errorf("replay v2 header is required")
	}
	return stream, nil
}

func parseReplayV2Line(
	stream *ReplayV2Stream,
	line []byte,
	lineNumber int,
	finalLineMayBeIncomplete bool,
) (bool, error) {
	if len(line) == 0 {
		return false, fmt.Errorf("replay v2 line %d is empty", lineNumber)
	}
	var envelope struct {
		RecordType string `json:"recordType"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		if finalLineMayBeIncomplete && stream.Header.SchemaVersion != "" && stream.Terminal == nil {
			return true, nil
		}
		return false, fmt.Errorf("replay v2 line %d is malformed: %w", lineNumber, err)
	}
	switch envelope.RecordType {
	case replayV2RecordHeader:
		return false, parseReplayV2Header(stream, line, lineNumber)
	case replayV2RecordEvent:
		return false, parseReplayV2Event(stream, line, lineNumber)
	case replayV2RecordTerminal:
		return false, parseReplayV2Terminal(stream, line, lineNumber)
	default:
		return false, fmt.Errorf("replay v2 line %d has unsupported recordType %q", lineNumber, envelope.RecordType)
	}
}

func parseReplayV2Header(
	stream *ReplayV2Stream,
	line []byte,
	lineNumber int,
) error {
	if lineNumber != 1 || stream.Header.SchemaVersion != "" {
		return fmt.Errorf("replay v2 header must appear exactly once as the first line")
	}
	var header ReplayV2Header
	if err := json.Unmarshal(line, &header); err != nil {
		return fmt.Errorf("replay v2 header is malformed: %w", err)
	}
	if err := validateReplayV2Header(header); err != nil {
		return err
	}
	stream.Header = header
	return nil
}

func parseReplayV2Event(
	stream *ReplayV2Stream,
	line []byte,
	lineNumber int,
) error {
	if stream.Header.SchemaVersion == "" {
		return fmt.Errorf("replay v2 event appears before header at line %d", lineNumber)
	}
	if stream.Terminal != nil {
		return fmt.Errorf("replay v2 event appears after terminal record at line %d", lineNumber)
	}
	var record replayV2EventRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return fmt.Errorf("replay v2 event line %d is malformed: %w", lineNumber, err)
	}
	if !validReplayV2Event(record.Event, len(stream.Events)) {
		return fmt.Errorf("replay v2 event line %d has invalid identity, order, schema, timestamp, or payload", lineNumber)
	}
	if replayV2HasEventID(stream.Events, record.Event.Id) {
		return fmt.Errorf("replay v2 event line %d duplicates event %q", lineNumber, record.Event.Id)
	}
	stream.Events = append(stream.Events, record.Event)
	return nil
}

func validReplayV2Event(event interfaces.FactoryEvent, sequence int) bool {
	return event.SchemaVersion == interfaces.FactoryEventSchemaVersionV1 &&
		event.Id != "" && event.Type != "" &&
		event.Context.Sequence == sequence &&
		!event.Context.EventTime.IsZero() && json.Valid(event.Payload)
}

func replayV2HasEventID(events []interfaces.FactoryEvent, id string) bool {
	for _, event := range events {
		if event.Id == id {
			return true
		}
	}
	return false
}

func parseReplayV2Terminal(
	stream *ReplayV2Stream,
	line []byte,
	lineNumber int,
) error {
	if stream.Header.SchemaVersion == "" {
		return fmt.Errorf("replay v2 terminal appears before header at line %d", lineNumber)
	}
	if stream.Terminal != nil {
		return fmt.Errorf("replay v2 terminal appears more than once")
	}
	var terminal ReplayV2TerminalRecord
	if err := json.Unmarshal(line, &terminal); err != nil {
		return fmt.Errorf("replay v2 terminal is malformed: %w", err)
	}
	if terminal.FinishedAt.IsZero() || strings.TrimSpace(terminal.TerminalState) == "" {
		return fmt.Errorf("replay v2 terminal requires finishedAt and terminalState")
	}
	stream.Terminal = &terminal
	return nil
}

// DecodeReplayV2 converts a validated v2 stream into the normalized replay
// artifact used by the existing reducer. It never writes or rewrites input.
func DecodeReplayV2(
	data []byte,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
) (*interfaces.ReplayArtifact, *ReplayV2Stream, error) {
	stream, err := ParseReplayV2(data)
	if err != nil {
		return nil, nil, err
	}
	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    stream.Header.RecordedAt,
		Events:        append([]interfaces.FactoryEvent(nil), stream.Events...),
	}
	if len(artifact.Events) > 0 {
		if err := hydrateArtifactFromEventsAtBoundary(artifact, decodeFactorySnapshot); err != nil {
			return nil, nil, err
		}
		if err := Validate(artifact); err != nil {
			return nil, nil, err
		}
	}
	if stream.Terminal != nil {
		wallClock := artifact.WallClock
		if wallClock == nil {
			wallClock = &interfaces.ReplayWallClockMetadata{}
		}
		if wallClock.StartedAt.IsZero() {
			wallClock.StartedAt = stream.Header.RecordedAt
		}
		wallClock.FinishedAt = stream.Terminal.FinishedAt
		artifact.WallClock = wallClock
	}
	return artifact, stream, nil
}

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
	trimmed := strings.TrimSpace(value)
	if trimmed == "EXPECTED_ARTIFACTS_UNSATISFIED" {
		return trimmed
	}
	normalized := strings.ToLower(trimmed)
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	if normalized == "expected_artifacts_unsatisfied" {
		return "EXPECTED_ARTIFACTS_UNSATISFIED"
	}
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
		"unknown",
		"EXPECTED_ARTIFACTS_UNSATISFIED":
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
func Save(
	storage platformreplay.Storage,
	path string,
	artifact *interfaces.ReplayArtifact,
	declaredSecrets ...[]recordingcontracts.RecordingSecret,
) error {
	if storage == nil {
		return fmt.Errorf("replay artifact storage is required")
	}
	data, err := MarshalArtifact(artifact, declaredSecrets...)
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
func MarshalArtifact(
	artifact *interfaces.ReplayArtifact,
	declaredSecrets ...[]recordingcontracts.RecordingSecret,
) ([]byte, error) {
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
	if len(declaredSecrets) > 0 {
		result, err := recordingcontracts.RedactDeclaredSecrets(recordingcontracts.RecordingRedactionRequest{
			Payload: data, Secrets: flattenRecordingSecretGroups(declaredSecrets),
		})
		if err != nil {
			return nil, fmt.Errorf("redact replay artifact: %w", err)
		}
		var indented bytes.Buffer
		if err := json.Indent(&indented, result.Payload, "", "  "); err != nil {
			return nil, fmt.Errorf("format redacted replay artifact: %w", err)
		}
		data = indented.Bytes()
	}
	return append(data, '\n'), nil
}

func flattenRecordingSecretGroups(
	groups [][]recordingcontracts.RecordingSecret,
) []recordingcontracts.RecordingSecret {
	var total int
	for _, group := range groups {
		total += len(group)
	}
	secrets := make([]recordingcontracts.RecordingSecret, 0, total)
	for _, group := range groups {
		secrets = append(secrets, group...)
	}
	return secrets
}

// Load reads, decodes, and validates a replay artifact before returning it to
// runtime replay code. The Factory Definitions boundary decoder is supplied by
// composition so Recordings does not depend on Config's concrete adapter.
func Load(
	storage platformreplay.Storage,
	path string,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
) (*interfaces.ReplayArtifact, error) {
	artifact, _, err := LoadWithMetadata(storage, path, decodeFactorySnapshot)
	return artifact, err
}

// LoadWithMetadata reads either a historical v1 JSON artifact or a v2 JSONL
// artifact and returns the normalized replay model plus framing metadata.
// Neither version is modified by this operation.
func LoadWithMetadata(
	storage platformreplay.Storage,
	path string,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
) (*interfaces.ReplayArtifact, ReplayReadMetadata, error) {
	if storage == nil {
		return nil, ReplayReadMetadata{}, fmt.Errorf("replay artifact storage is required")
	}
	data, err := storage.ReadFile(path)
	if err != nil {
		return nil, ReplayReadMetadata{}, fmt.Errorf("read replay artifact %q: %w", path, err)
	}
	if IsReplayV2Artifact(data) {
		artifact, stream, err := DecodeReplayV2(data, decodeFactorySnapshot)
		if err != nil {
			return nil, ReplayReadMetadata{}, fmt.Errorf("parse replay artifact %q: %w", path, err)
		}
		return artifact, ReplayReadMetadata{
			SchemaVersion: ReplayV2SchemaVersion,
			V2:            stream,
		}, nil
	}

	artifact, err := unmarshalReplayArtifact(data)
	if err != nil {
		return nil, ReplayReadMetadata{}, fmt.Errorf("parse replay artifact %q: %w", path, err)
	}
	if err := hydrateArtifactFromEventsAtBoundary(artifact, decodeFactorySnapshot); err != nil {
		return nil, ReplayReadMetadata{}, err
	}
	if err := Validate(artifact); err != nil {
		return nil, ReplayReadMetadata{}, err
	}
	return artifact, ReplayReadMetadata{SchemaVersion: artifact.SchemaVersion}, nil
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
