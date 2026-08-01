package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

type OpenEventStreamFile func(string) (io.ReadCloser, error)
type InspectAdjacentFactoryPath func(string) (fs.FileInfo, error)

const maxEventStreamLineBytes = 16 * 1024 * 1024

// EventStreamArtifactResult captures the parsed replay artifact plus a small
// amount of recovery metadata for partially written SSE logs.
type EventStreamArtifactResult struct {
	Artifact              *interfaces.ReplayArtifact
	ParsedEvents          int
	SkippedTrailingBlocks int
}

type eventStreamArtifactBuilder struct {
	blockIndex            int
	dataLines             []string
	events                []interfaces.FactoryEvent
	skippedTrailingBlocks int
}

const legacyEventStreamCronPlaceholderSchedule = "* * * * *"

// ArtifactFromEventStream parses an SSE-style event stream whose payloads are
// canonical FactoryEvent JSON documents and returns a hydrated replay artifact.
// If the stream ends with a truncated final event block, that final block is
// skipped so long as at least one complete event was already recovered.
func ArtifactFromEventStream(
	r io.Reader,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
) (*EventStreamArtifactResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventStreamLineBytes)
	builder := eventStreamArtifactBuilder{}

	for scanner.Scan() {
		switch line := scanner.Text(); {
		case line == "":
			if err := builder.flushBlock(false); err != nil {
				return nil, err
			}
		default:
			builder.appendLine(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan event stream: %w", err)
	}
	if err := builder.flushBlock(true); err != nil {
		return nil, err
	}
	if len(builder.events) == 0 {
		return nil, fmt.Errorf("event stream contained no replayable events")
	}
	if err := normalizeEventStreamRunRequestFactories(
		builder.events,
		decodeFactorySnapshot,
	); err != nil {
		return nil, err
	}

	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		Events:        append([]interfaces.FactoryEvent(nil), builder.events...),
	}
	if err := hydrateArtifactFromEventsAtBoundary(artifact, decodeFactorySnapshot); err != nil {
		return nil, err
	}
	if err := Validate(artifact); err != nil {
		return nil, err
	}
	return &EventStreamArtifactResult{
		Artifact:              artifact,
		ParsedEvents:          len(builder.events),
		SkippedTrailingBlocks: builder.skippedTrailingBlocks,
	}, nil
}

func (b *eventStreamArtifactBuilder) appendLine(line string) {
	switch {
	case strings.HasPrefix(line, "data: "):
		b.dataLines = append(b.dataLines, line[6:])
	case strings.HasPrefix(line, "data:"):
		b.dataLines = append(b.dataLines, strings.TrimLeft(line[5:], " \t"))
	}
}

func (b *eventStreamArtifactBuilder) flushBlock(atEOF bool) error {
	if len(b.dataLines) == 0 {
		return nil
	}
	b.blockIndex++
	payload := strings.Join(b.dataLines, "\n")
	b.dataLines = b.dataLines[:0]

	normalized, err := normalizeHistoricalFailureDetails([]byte(payload))
	if err != nil {
		return b.decodeBlockError(atEOF, err)
	}
	var event interfaces.FactoryEvent
	if err := json.Unmarshal(normalized, &event); err != nil {
		return b.decodeBlockError(atEOF, err)
	}
	if event.Id == "" || event.Type == "" {
		return b.decodeBlockError(atEOF, fmt.Errorf("required replay event fields missing"))
	}
	if event.SchemaVersion == "" {
		event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	}
	b.events = append(b.events, event)
	return nil
}

func (b *eventStreamArtifactBuilder) decodeBlockError(atEOF bool, err error) error {
	if atEOF && len(b.events) > 0 {
		b.skippedTrailingBlocks++
		return nil
	}
	return fmt.Errorf("decode event stream block %d: %w", b.blockIndex, err)
}

// ArtifactFromEventStreamFile opens and parses a saved event stream file into a
// replay artifact.
func ArtifactFromEventStreamFile(
	path string,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
	loadAdjacentFactory factorydefinitionswire.FactorySnapshotDirectoryLoader,
	openFile OpenEventStreamFile,
	inspectPath InspectAdjacentFactoryPath,
) (*EventStreamArtifactResult, error) {
	if openFile == nil || inspectPath == nil {
		return nil, fmt.Errorf("event stream file operations are required")
	}
	file, err := openFile(path)
	if err != nil {
		return nil, fmt.Errorf("open event stream %q: %w", path, err)
	}
	defer file.Close()

	result, err := ArtifactFromEventStream(file, decodeFactorySnapshot)
	if err != nil {
		return nil, fmt.Errorf("parse event stream %q: %w", path, err)
	}
	if err := hydrateArtifactFromAdjacentFactory(
		path,
		result.Artifact,
		loadAdjacentFactory,
		inspectPath,
	); err != nil {
		return nil, fmt.Errorf("hydrate replay artifact from adjacent factory for %q: %w", path, err)
	}
	return result, nil
}

// SaveArtifactFromEventStreamFile converts an event stream file into the
// canonical replay artifact JSON format.
func SaveArtifactFromEventStreamFile(
	storage platformreplay.Storage,
	eventStreamPath string,
	artifactPath string,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
	loadAdjacentFactory factorydefinitionswire.FactorySnapshotDirectoryLoader,
	openFile OpenEventStreamFile,
	inspectPath InspectAdjacentFactoryPath,
) (*EventStreamArtifactResult, error) {
	result, err := ArtifactFromEventStreamFile(
		eventStreamPath,
		decodeFactorySnapshot,
		loadAdjacentFactory,
		openFile,
		inspectPath,
	)
	if err != nil {
		return nil, err
	}
	if err := Save(storage, artifactPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("save replay artifact from event stream %q: %w", eventStreamPath, err)
	}
	return result, nil
}

func normalizeEventStreamRunRequestFactories(
	events []interfaces.FactoryEvent,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
) error {
	for index := range events {
		event := &events[index]
		if event.Type != interfaces.FactoryEventTypeRunRequest {
			continue
		}

		payload, err := runStartedPayloadFromEventAtBoundary(
			*event,
			decodeFactorySnapshot,
		)
		if err != nil {
			return err
		}
		if err := normalizeLegacyCronFactorySnapshot(payload.Factory); err != nil {
			return fmt.Errorf("decode run started event %q factory: %w", event.Id, err)
		}
		encodedPayload, err := encodeRunRequestEventPayload(payload)
		if err != nil {
			return fmt.Errorf("normalize run started event payload: %w", err)
		}
		event.Payload = encodedPayload
	}
	return nil
}

func hydrateArtifactFromAdjacentFactory(
	eventStreamPath string,
	artifact *interfaces.ReplayArtifact,
	loadFactory factorydefinitionswire.FactorySnapshotDirectoryLoader,
	inspectPath InspectAdjacentFactoryPath,
) error {
	if artifact == nil {
		return nil
	}
	factoryDir, ok := adjacentFactoryDir(eventStreamPath, inspectPath)
	if !ok {
		return nil
	}
	if loadFactory == nil {
		return nil
	}
	authored, err := loadFactory(factoryDir)
	if err != nil || authored == nil {
		return nil
	}
	merged, err := mergeFactorySnapshotsMissingRuntimeFields(artifact.Factory, authored)
	if err != nil {
		return err
	}
	if err := rewriteArtifactFactoryEvents(artifact, merged); err != nil {
		return err
	}
	artifact.Factory = merged
	return nil
}

func adjacentFactoryDir(eventStreamPath string, inspectPath InspectAdjacentFactoryPath) (string, bool) {
	if inspectPath == nil {
		return "", false
	}
	candidates := []string{
		filepath.Dir(eventStreamPath),
		filepath.Dir(filepath.Dir(eventStreamPath)),
	}
	for _, dir := range candidates {
		if dir == "" || dir == "." {
			continue
		}
		if _, err := inspectPath(filepath.Join(dir, interfaces.FactoryConfigFile)); err == nil {
			return dir, true
		}
	}
	return "", false
}

func normalizeLegacyCronFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	factory, err := factorySnapshotObject(snapshot)
	if err != nil {
		return err
	}
	workstations, ok, err := objectArrayField(factory, "workstations")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	for _, workstation := range workstations {
		if rawStringField(workstation, "behavior") == "CRON" && rawFieldMissing(workstation, "cron") {
			workstation["cron"] = json.RawMessage(`{"schedule":"` + legacyEventStreamCronPlaceholderSchedule + `"}`)
		}
	}
	encodedWorkstations, err := json.Marshal(workstations)
	if err != nil {
		return fmt.Errorf("encode normalized replay workstations: %w", err)
	}
	factory["workstations"] = encodedWorkstations
	updated, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		return fmt.Errorf("capture normalized replay factory: %w", err)
	}
	*snapshot = *updated
	return nil
}

func mergeFactorySnapshotsMissingRuntimeFields(recorded, authored *interfaces.FactorySnapshot) (*interfaces.FactorySnapshot, error) {
	merged, err := factorySnapshotObject(recorded)
	if err != nil {
		return nil, err
	}
	authoredObject, err := factorySnapshotObject(authored)
	if err != nil {
		return nil, err
	}
	copyMissingFields(merged, authoredObject, "factoryDirectory", "sourceDirectory", "id")
	copyEmptyFields(merged, authoredObject, "metadata", "inputTypes")
	if err := mergeNamedRuntimeEntries(merged, authoredObject, "workers", mergeRuntimeWorkerFields); err != nil {
		return nil, err
	}
	if err := mergeNamedRuntimeEntries(merged, authoredObject, "workstations", mergeRuntimeWorkstationFields); err != nil {
		return nil, err
	}
	snapshot, err := interfaces.NewFactorySnapshot(merged)
	if err != nil {
		return nil, fmt.Errorf("capture hydrated replay factory: %w", err)
	}
	return snapshot, nil
}

func mergeRuntimeWorkerFields(worker, authored map[string]json.RawMessage) {
	copyMissingFields(worker, authored, "type", "command", "modelProvider", "executorProvider", "timeout", "stopToken", "skipPermissions", "body")
	copyEmptyFields(worker, authored, "args", "resources")
}

func mergeRuntimeWorkstationFields(workstation, authored map[string]json.RawMessage) {
	copyMissingFields(workstation, authored,
		"id", "behavior", "type", "onFailure", "onContinue", "onRejection",
		"cron", "limits", "worktree", "workingDirectory", "promptFile", "body",
	)
	copyBlankStringFields(workstation, authored, "worker")
	copyEmptyFields(workstation, authored, "inputs", "outputs", "resources", "guards", "stopWords")
}

func mergeNamedRuntimeEntries(factory, authored map[string]json.RawMessage, field string, merge func(map[string]json.RawMessage, map[string]json.RawMessage)) error {
	entries, ok, err := objectArrayField(factory, field)
	if err != nil || !ok {
		return err
	}
	authoredEntries, ok, err := objectArrayField(authored, field)
	if err != nil || !ok {
		return err
	}
	authoredByName := make(map[string]map[string]json.RawMessage, len(authoredEntries))
	for _, entry := range authoredEntries {
		authoredByName[rawStringField(entry, "name")] = entry
	}
	for _, entry := range entries {
		if authoredEntry, found := authoredByName[rawStringField(entry, "name")]; found {
			merge(entry, authoredEntry)
		}
	}
	encodedEntries, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode replay factory %s: %w", field, err)
	}
	factory[field] = encodedEntries
	return nil
}

func factorySnapshotObject(snapshot *interfaces.FactorySnapshot) (map[string]json.RawMessage, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("replay artifact factory is required")
	}
	var object map[string]json.RawMessage
	if err := snapshot.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode replay artifact factory: %w", err)
	}
	return object, nil
}

func objectArrayField(object map[string]json.RawMessage, field string) ([]map[string]json.RawMessage, bool, error) {
	raw, ok := object[field]
	if !ok || string(raw) == "null" {
		return nil, false, nil
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, false, fmt.Errorf("decode replay factory %s: %w", field, err)
	}
	return entries, true, nil
}

func copyMissingFields(target, source map[string]json.RawMessage, fields ...string) {
	for _, field := range fields {
		if sourceValue, ok := source[field]; rawFieldMissing(target, field) && ok && len(sourceValue) > 0 && string(sourceValue) != "null" {
			target[field] = cloneRawMessage(sourceValue)
		}
	}
}

func copyEmptyFields(target, source map[string]json.RawMessage, fields ...string) {
	for _, field := range fields {
		if sourceValue, ok := source[field]; rawFieldEmpty(target, field) && ok && len(sourceValue) > 0 && string(sourceValue) != "null" {
			target[field] = cloneRawMessage(sourceValue)
		}
	}
}

func copyBlankStringFields(target, source map[string]json.RawMessage, fields ...string) {
	for _, field := range fields {
		if rawStringField(target, field) == "" {
			if sourceValue, ok := source[field]; ok && len(sourceValue) > 0 && string(sourceValue) != "null" {
				target[field] = cloneRawMessage(sourceValue)
			}
		}
	}
}

func rawFieldMissing(object map[string]json.RawMessage, field string) bool {
	raw, ok := object[field]
	return !ok || len(raw) == 0 || string(raw) == "null"
}

func rawFieldEmpty(object map[string]json.RawMessage, field string) bool {
	if rawFieldMissing(object, field) {
		return true
	}
	raw := object[field]
	var collection any
	if err := json.Unmarshal(raw, &collection); err != nil {
		return false
	}
	switch value := collection.(type) {
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

func rawStringField(object map[string]json.RawMessage, field string) string {
	var value string
	_ = json.Unmarshal(object[field], &value)
	return value
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func rewriteArtifactFactoryEvents(artifact *interfaces.ReplayArtifact, snapshot *interfaces.FactorySnapshot) error {
	for i := range artifact.Events {
		event := &artifact.Events[i]
		switch event.Type {
		case interfaces.FactoryEventTypeRunRequest:
			var payload interfaces.RunRequestEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				return fmt.Errorf("decode run request event %q: %w", event.Id, err)
			}
			payload.Factory = snapshot.Clone()
			encodedPayload, err := encodeRunRequestEventPayload(payload)
			if err != nil {
				return fmt.Errorf("rewrite run request factory payload: %w", err)
			}
			event.Payload = encodedPayload
		case interfaces.FactoryEventTypeInitialStructureRequest:
			var payload map[string]json.RawMessage
			if err := event.DecodePayload(&payload); err != nil {
				return fmt.Errorf("decode initial structure event %q: %w", event.Id, err)
			}
			if payload == nil {
				return fmt.Errorf("decode initial structure event %q: payload object is required", event.Id)
			}
			payload["factory"] = append(json.RawMessage(nil), (*snapshot)...)
			encodedPayload, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("rewrite initial structure factory payload: %w", err)
			}
			event.Payload = encodedPayload
		}
	}
	return nil
}
