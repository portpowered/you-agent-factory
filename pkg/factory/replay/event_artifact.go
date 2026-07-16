package replay

import (
	"encoding/json"
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
)

const (
	replayRunStartedEventID  = "factory-event/run-started"
	replayRunFinishedEventID = "factory-event/run-finished"
	replayMetadataReplayKey  = "replayKey"
)

func artifactForStorage(artifact *interfaces.ReplayArtifact) (*interfaces.ReplayArtifact, error) {
	if artifact == nil {
		return nil, fmt.Errorf("replay artifact is required")
	}
	out := *artifact
	out.Events = append([]interfaces.FactoryEvent(nil), artifact.Events...)
	out.Factory = artifact.Factory.Clone()
	assignEventSequences(out.Events)
	if err := canonicalizeRunRequestEventPayloads(&out); err != nil {
		return nil, err
	}
	if err := hydrateArtifactFromEvents(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func canonicalizeRunRequestEventPayloads(artifact *interfaces.ReplayArtifact) error {
	for index := range artifact.Events {
		event := &artifact.Events[index]
		if event.Type != interfaces.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := runStartedPayloadFromEvent(*event)
		if err != nil {
			return err
		}
		if payload.RecordedAt.IsZero() {
			payload.RecordedAt = artifact.RecordedAt
		}
		if payload.RecordedAt.IsZero() {
			payload.RecordedAt = event.Context.EventTime
		}
		encodedPayload, err := encodeRunRequestEventPayload(payload)
		if err != nil {
			return fmt.Errorf("encode run started event %q payload: %w", event.Id, err)
		}
		event.Payload = encodedPayload
	}
	return nil
}

func encodeRunRequestEventPayload(payload interfaces.RunRequestEventPayload) (json.RawMessage, error) {
	diagnostics, err := runRequestDiagnosticsEventEnvelope(payload.Diagnostics)
	if err != nil {
		return nil, err
	}
	eventPayload := struct {
		Diagnostics *runRequestDiagnosticsEnvelope `json:"diagnostics,omitempty"`
		Factory     *interfaces.FactorySnapshot    `json:"factory"`
		RecordedAt  time.Time                      `json:"recordedAt"`
		WallClock   *interfaces.RunEventWallClock  `json:"wallClock,omitempty"`
	}{
		Diagnostics: diagnostics,
		Factory:     payload.Factory,
		RecordedAt:  payload.RecordedAt,
		WallClock:   payload.WallClock,
	}
	encoded, err := json.Marshal(eventPayload)
	if err != nil {
		return nil, fmt.Errorf("encode run request event payload: %w", err)
	}
	return encoded, nil
}

// NewEventLogArtifact creates a replay artifact shell whose first event carries
// the detached Factory-owned snapshot.
func NewEventLogArtifact(recordedAt time.Time, factorySnapshot *interfaces.FactorySnapshot, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) (*interfaces.ReplayArtifact, error) {
	event, err := runStartedEventFromSnapshot(recordedAt, factorySnapshot, wallClock, diagnostics)
	if err != nil {
		return nil, err
	}
	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        []interfaces.FactoryEvent{event},
		Factory:       factorySnapshot,
		WallClock:     wallClock,
		Diagnostics:   diagnostics,
	}
	assignEventSequences(artifact.Events)
	return artifact, nil
}

func runStartedEventFromSnapshot(recordedAt time.Time, factorySnapshot *interfaces.FactorySnapshot, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) (interfaces.FactoryEvent, error) {
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	payload := interfaces.RunRequestEventPayload{
		RecordedAt:  recordedAt,
		Factory:     factorySnapshot.Clone(),
		WallClock:   runEventWallClock(wallClock),
		Diagnostics: replayDiagnosticsPtr(diagnostics),
	}
	encodedPayload, err := encodeRunRequestEventPayload(payload)
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("encode run started event payload: %w", err)
	}
	return interfaces.FactoryEvent{
		Id:            replayRunStartedEventID,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeRunRequest,
		Context: interfaces.FactoryEventContext{
			EventTime: recordedAt,
			Tick:      0,
		},
		Payload: encodedPayload,
	}, nil
}

func runFinishedEvent(finishedAt time.Time, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) interfaces.FactoryEvent {
	state := interfaces.FactoryStateCompleted
	payload := interfaces.RunResponseEventPayload{
		State:       &state,
		WallClock:   runEventWallClock(wallClock),
		Diagnostics: replayDiagnosticsPtr(diagnostics),
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("encode run finished event payload: %v", err))
	}
	return interfaces.FactoryEvent{
		Id:            replayRunFinishedEventID,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeRunResponse,
		Context: interfaces.FactoryEventContext{
			EventTime: finishedAt,
		},
		Payload: encodedPayload,
	}
}

func hydrateArtifactFromEvents(artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return fmt.Errorf("replay artifact is required")
	}
	for _, event := range artifact.Events {
		switch event.Type {
		case interfaces.FactoryEventTypeRunRequest:
			payload, err := runStartedPayloadFromEvent(event)
			if err != nil {
				return err
			}
			artifact.Factory = payload.Factory.Clone()
			artifact.RecordedAt = payload.RecordedAt
			artifact.WallClock = replayWallClockFromRunEvent(payload.WallClock)
			artifact.Diagnostics = replayDiagnosticsFromRunEvent(payload.Diagnostics)
		case interfaces.FactoryEventTypeRunResponse:
			var payload interfaces.RunResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				return fmt.Errorf("decode run finished event %q: %w", event.Id, err)
			}
			if wallClock := replayWallClockFromRunEvent(payload.WallClock); wallClock != nil {
				artifact.WallClock = wallClock
			}
			if diagnostics := replayDiagnosticsFromRunEvent(payload.Diagnostics); len(diagnostics.Notes) > 0 || len(diagnostics.Workers) > 0 {
				artifact.Diagnostics = diagnostics
			}
		}
	}
	return nil
}

func runStartedPayloadFromEvent(event interfaces.FactoryEvent) (interfaces.RunRequestEventPayload, error) {
	var raw struct {
		RecordedAt  time.Time                      `json:"recordedAt"`
		WallClock   *interfaces.RunEventWallClock  `json:"wallClock"`
		Diagnostics *runRequestDiagnosticsEnvelope `json:"diagnostics"`
	}
	if err := event.DecodePayload(&raw); err != nil {
		return interfaces.RunRequestEventPayload{}, fmt.Errorf("decode run started event %q: %w", event.Id, err)
	}
	factorySnapshot, err := runStartedFactorySnapshotFromEvent(event)
	if err != nil {
		return interfaces.RunRequestEventPayload{}, err
	}
	diagnostics, err := replayDiagnosticsFromRunRequestEnvelope(raw.Diagnostics)
	if err != nil {
		return interfaces.RunRequestEventPayload{}, fmt.Errorf("decode run started event %q diagnostics: %w", event.Id, err)
	}
	return interfaces.RunRequestEventPayload{
		RecordedAt:  raw.RecordedAt,
		WallClock:   raw.WallClock,
		Diagnostics: diagnostics,
		Factory:     factorySnapshot,
	}, nil
}

type runRequestDiagnosticsEnvelope struct {
	Notes   []string                   `json:"notes,omitempty"`
	Workers map[string]json.RawMessage `json:"workers,omitempty"`
}

func runRequestDiagnosticsEventEnvelope(diagnostics *interfaces.ReplayDiagnostics) (*runRequestDiagnosticsEnvelope, error) {
	if diagnostics == nil {
		return nil, nil
	}
	envelope := &runRequestDiagnosticsEnvelope{
		Notes:   append([]string(nil), diagnostics.Notes...),
		Workers: make(map[string]json.RawMessage, len(diagnostics.Workers)),
	}
	for key, diagnostic := range diagnostics.Workers {
		payload, err := workerdiagnostics.SafeWorkDiagnosticsEventPayload(&diagnostic)
		if err != nil {
			return nil, fmt.Errorf("encode worker %q diagnostics: %w", key, err)
		}
		envelope.Workers[key] = payload
	}
	if len(envelope.Workers) == 0 {
		envelope.Workers = nil
	}
	return envelope, nil
}

func replayDiagnosticsFromRunRequestEnvelope(envelope *runRequestDiagnosticsEnvelope) (*interfaces.ReplayDiagnostics, error) {
	if envelope == nil {
		return nil, nil
	}
	diagnostics := &interfaces.ReplayDiagnostics{
		Notes:   append([]string(nil), envelope.Notes...),
		Workers: make(map[string]workerdiagnostics.SafeWorkDiagnostics, len(envelope.Workers)),
	}
	for key, payload := range envelope.Workers {
		decoded, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("worker %q: %w", key, err)
		}
		if decoded != nil {
			diagnostics.Workers[key] = *decoded
		}
	}
	return diagnostics, nil
}

func runStartedFactorySnapshotFromEvent(event interfaces.FactoryEvent) (*interfaces.FactorySnapshot, error) {
	payloadJSON := event.Payload

	var raw struct {
		Factory json.RawMessage `json:"factory"`
	}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return nil, fmt.Errorf("decode run started event %q payload envelope: %w", event.Id, err)
	}
	if len(raw.Factory) == 0 {
		return nil, fmt.Errorf("run started event %q factory is required", event.Id)
	}

	normalizedFactoryJSON, err := normalizeLegacyReplayFactoryBoundary(raw.Factory)
	if err != nil {
		return nil, fmt.Errorf("normalize run started event %q factory boundary: %w", event.Id, err)
	}

	factorySnapshot, err := validatedFactorySnapshotFromOpenAPIJSON(normalizedFactoryJSON)
	if err != nil {
		return nil, fmt.Errorf("decode run started event %q factory boundary: %w", event.Id, err)
	}
	return factorySnapshot, nil
}

func normalizeLegacyReplayFactoryBoundary(data json.RawMessage) ([]byte, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	root, ok := raw.(map[string]any)
	if !ok {
		return []byte(data), nil
	}

	workstations, ok := root["workstations"].([]any)
	if !ok {
		return json.Marshal(root)
	}
	for _, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalizeLegacyReplayTransitionRoutes(workstation)
		if definition, ok := workstation["definition"].(map[string]any); ok {
			normalizeLegacyReplayTransitionRoutes(definition)
		}
	}
	return json.Marshal(root)
}

func normalizeLegacyReplayTransitionRoutes(workstation map[string]any) {
	for _, key := range []string{"onContinue", "onFailure", "onRejection"} {
		value, ok := workstation[key]
		if !ok {
			continue
		}
		if route, ok := value.(map[string]any); ok {
			workstation[key] = []any{route}
		}
	}
	if _, hasBody := workstation["body"]; !hasBody {
		if promptTemplate, ok := workstation["promptTemplate"]; ok {
			workstation["body"] = promptTemplate
		}
	}
	delete(workstation, "promptTemplate")
}

func assignEventSequences(events []interfaces.FactoryEvent) {
	for i := range events {
		events[i].SchemaVersion = interfaces.FactoryEventSchemaVersionV1
		events[i].Context.Sequence = i
		if events[i].Context.EventTime.IsZero() {
			events[i].Context.EventTime = time.Now().UTC()
		}
	}
}

func replayDiagnosticsPtr(diagnostics interfaces.ReplayDiagnostics) *interfaces.ReplayDiagnostics {
	detached := replayDiagnosticsFromRunEvent(&diagnostics)
	return &detached
}

func replayDiagnosticsFromRunEvent(diagnostics *interfaces.ReplayDiagnostics) interfaces.ReplayDiagnostics {
	if diagnostics == nil {
		return interfaces.ReplayDiagnostics{}
	}
	workers := make(map[string]workerdiagnostics.SafeWorkDiagnostics, len(diagnostics.Workers))
	for key, value := range diagnostics.Workers {
		cloned := workerdiagnostics.CloneSafeWorkDiagnostics(&value)
		if cloned != nil {
			workers[key] = *cloned
		}
	}
	return interfaces.ReplayDiagnostics{
		Notes:   append([]string(nil), diagnostics.Notes...),
		Workers: workers,
	}
}

func runEventWallClock(wallClock *interfaces.ReplayWallClockMetadata) *interfaces.RunEventWallClock {
	if wallClock == nil {
		return nil
	}
	return &interfaces.RunEventWallClock{
		StartedAt:  timePtrIfNotZero(wallClock.StartedAt),
		FinishedAt: timePtrIfNotZero(wallClock.FinishedAt),
	}
}

func replayWallClockFromRunEvent(wallClock *interfaces.RunEventWallClock) *interfaces.ReplayWallClockMetadata {
	if wallClock == nil {
		return nil
	}
	return &interfaces.ReplayWallClockMetadata{
		StartedAt:  timeValue(wallClock.StartedAt),
		FinishedAt: timeValue(wallClock.FinishedAt),
	}
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstString(values *[]string) string {
	if values == nil || len(*values) == 0 {
		return ""
	}
	return (*values)[0]
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(*values))
	copy(out, *values)
	return out
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func stringPtrIfNotEmpty[T ~string](value T) *T {
	if value == "" {
		return nil
	}
	return &value
}

func intPtrIfNonZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func int64PtrIfNonZero(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func float64PtrIfNonZero(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func slicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := make([]T, len(values))
	copy(out, values)
	return &out
}
