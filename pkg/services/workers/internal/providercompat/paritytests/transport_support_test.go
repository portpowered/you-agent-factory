package providerparity

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const parityTransportSessionID = "session-parity-transport"

const (
	transportJSONRecordResponseEvent    = "response_event"
	transportJSONRecordInvocationResult = "invocation_result"
)

// TransportParityOutcome is one fixture run with published response events and
// the terminal invocation result used for CLI/API transport parity proofs.
type TransportParityOutcome struct {
	Events           []factorysessions.FactoryResponseEvent
	Terminal         TerminalResult
	InvocationResult interfaces.FactoryInvocationResult
}

// RunTransportParity executes one fixture, publishes adapter drafts through the
// session response-event store, and returns transport-neutral parity inputs.
func RunTransportParity(ctx context.Context, fixture Fixture) (TransportParityOutcome, error) {
	terminal, err := RunTerminal(ctx, fixture)
	if err != nil {
		return TransportParityOutcome{}, err
	}
	events, err := publishDrafts(terminal.Drafts)
	if err != nil {
		return TransportParityOutcome{}, fmt.Errorf("publish fixture %q drafts: %w", fixture.ID, err)
	}
	return TransportParityOutcome{
		Events:           events,
		Terminal:         terminal,
		InvocationResult: invocationResultFromTerminal(fixture, terminal),
	}, nil
}

// AssertCLIAPITransportParity proves decoded CLI NDJSON and API SSE transports
// agree on FactoryResponseEvent values and terminal InvocationResponse outcomes.
func AssertCLIAPITransportParity(outcome TransportParityOutcome) error {
	if len(outcome.Events) == 0 {
		return fmt.Errorf("fixture produced no publishable response events")
	}
	apiRecords, err := EncodeTransportAPIRecords(outcome.Events)
	if err != nil {
		return fmt.Errorf("encode API records: %w", err)
	}
	cliLines, err := EncodeTransportCLINDJSON(outcome.Events, outcome.InvocationResult)
	if err != nil {
		return fmt.Errorf("encode CLI NDJSON: %w", err)
	}
	apiEvents, err := DecodeTransportAPIRecords(apiRecords)
	if err != nil {
		return fmt.Errorf("decode API records: %w", err)
	}
	cliEvents, cliInvocation, err := DecodeTransportCLINDJSON(cliLines)
	if err != nil {
		return fmt.Errorf("decode CLI NDJSON: %w", err)
	}
	if err := assertEventSequencesEqual(apiEvents, cliEvents); err != nil {
		return fmt.Errorf("response-event transport parity: %w", err)
	}
	apiInvocation := apisurface.InvocationResponseFromResult(outcome.InvocationResult)
	if !reflect.DeepEqual(cliInvocation, apiInvocation) {
		return fmt.Errorf("invocation transport parity: CLI = %#v, API = %#v", cliInvocation, apiInvocation)
	}
	return nil
}

// EncodeTransportAPIRecords serializes events the same way runtime SSE consumers
// receive them through FactoryResponseEventRecord payloads.
func EncodeTransportAPIRecords(events []factorysessions.FactoryResponseEvent) ([]apisurface.FactoryResponseEventRecord, error) {
	records := make([]apisurface.FactoryResponseEventRecord, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("marshal response event sequence %d: %w", event.Sequence, err)
		}
		records = append(records, apisurface.FactoryResponseEventRecord{
			Sequence: event.Sequence,
			Kind:     string(event.Kind),
			Data:     data,
		})
	}
	return records, nil
}

// DecodeTransportAPIRecords decodes API SSE data payloads back to response events.
func DecodeTransportAPIRecords(records []apisurface.FactoryResponseEventRecord) ([]factorysessions.FactoryResponseEvent, error) {
	decoded := make([]factorysessions.FactoryResponseEvent, 0, len(records))
	for _, record := range records {
		var event factorysessions.FactoryResponseEvent
		if err := json.Unmarshal(record.Data, &event); err != nil {
			return nil, fmt.Errorf("decode API record sequence %d: %w", record.Sequence, err)
		}
		if err := factorysessions.ValidateFactoryResponseEvent(event); err != nil {
			return nil, fmt.Errorf("validate API record sequence %d: %w", record.Sequence, err)
		}
		decoded = append(decoded, event)
	}
	return decoded, nil
}

// EncodeTransportCLINDJSON serializes response events and the terminal invocation
// result using the CLI response-stream NDJSON record envelope.
func EncodeTransportCLINDJSON(
	events []factorysessions.FactoryResponseEvent,
	invocation interfaces.FactoryInvocationResult,
) ([]string, error) {
	lines := make([]string, 0, len(events)+1)
	for _, event := range events {
		encoded, err := json.Marshal(transportCLIResponseEventRecord{
			RecordType: transportJSONRecordResponseEvent,
			Event:      event,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal CLI response_event sequence %d: %w", event.Sequence, err)
		}
		lines = append(lines, string(encoded))
	}
	finalEncoded, err := json.Marshal(transportCLIInvocationResultRecord{
		RecordType: transportJSONRecordInvocationResult,
		Invocation: apisurface.InvocationResponseFromResult(invocation),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal CLI invocation_result: %w", err)
	}
	lines = append(lines, string(finalEncoded))
	return lines, nil
}

// DecodeTransportCLINDJSON decodes CLI response-stream NDJSON records.
func DecodeTransportCLINDJSON(lines []string) ([]factorysessions.FactoryResponseEvent, factoryapi.InvocationResponse, error) {
	if len(lines) == 0 {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("CLI NDJSON is empty")
	}
	events := make([]factorysessions.FactoryResponseEvent, 0, len(lines)-1)
	for index, line := range lines[:len(lines)-1] {
		record, err := decodeTransportCLIResponseEventLine(line, index)
		if err != nil {
			return nil, factoryapi.InvocationResponse{}, err
		}
		events = append(events, record.Event)
	}
	finalRecord, err := decodeTransportCLIInvocationResultLine(lines[len(lines)-1])
	if err != nil {
		return nil, factoryapi.InvocationResponse{}, err
	}
	return events, finalRecord.Invocation, nil
}

func decodeTransportCLIRecordHeader(line string, label string) (string, error) {
	var header struct {
		RecordType string `json:"recordType"`
	}
	if err := json.Unmarshal([]byte(line), &header); err != nil {
		return "", fmt.Errorf("decode CLI %s record header: %w", label, err)
	}
	for _, retired := range []string{"progress", "compaction", "primary_result", "stream_gap"} {
		if header.RecordType == retired {
			return "", fmt.Errorf("unsupported retired private CLI NDJSON recordType %q", header.RecordType)
		}
	}
	return header.RecordType, nil
}

func decodeTransportCLIResponseEventLine(line string, index int) (transportCLIResponseEventRecord, error) {
	recordType, err := decodeTransportCLIRecordHeader(line, fmt.Sprintf("response_event line %d", index))
	if err != nil {
		return transportCLIResponseEventRecord{}, err
	}
	if recordType != transportJSONRecordResponseEvent {
		return transportCLIResponseEventRecord{}, fmt.Errorf("CLI line %d recordType = %q, want %q", index, recordType, transportJSONRecordResponseEvent)
	}
	var record transportCLIResponseEventRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return transportCLIResponseEventRecord{}, fmt.Errorf("decode CLI response_event line %d: %w", index, err)
	}
	if err := factorysessions.ValidateFactoryResponseEvent(record.Event); err != nil {
		return transportCLIResponseEventRecord{}, fmt.Errorf("validate CLI response_event line %d: %w", index, err)
	}
	return record, nil
}

func decodeTransportCLIInvocationResultLine(line string) (transportCLIInvocationResultRecord, error) {
	recordType, err := decodeTransportCLIRecordHeader(line, "invocation_result")
	if err != nil {
		return transportCLIInvocationResultRecord{}, err
	}
	if recordType != transportJSONRecordInvocationResult {
		return transportCLIInvocationResultRecord{}, fmt.Errorf("final CLI recordType = %q, want %q", recordType, transportJSONRecordInvocationResult)
	}
	var finalRecord transportCLIInvocationResultRecord
	if err := json.Unmarshal([]byte(line), &finalRecord); err != nil {
		return transportCLIInvocationResultRecord{}, fmt.Errorf("decode CLI invocation_result: %w", err)
	}
	return finalRecord, nil
}

// EncodeTransportSSEFrame serializes one event using the HTTP SSE wire format.
func EncodeTransportSSEFrame(event factorysessions.FactoryResponseEvent) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal SSE event: %w", err)
	}
	return fmt.Sprintf("id: %s\ndata: %s\n\n", strconv.FormatInt(event.Sequence, 10), data), nil
}

// DecodeTransportSSEFrame decodes one HTTP SSE response-event frame.
func DecodeTransportSSEFrame(frame string) (factorysessions.FactoryResponseEvent, error) {
	idLine, dataLine, err := parseSSEFrame(frame)
	if err != nil {
		return factorysessions.FactoryResponseEvent{}, err
	}
	var event factorysessions.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("decode SSE data: %w", err)
	}
	if idLine != strconv.FormatInt(event.Sequence, 10) {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want sequence %d", idLine, event.Sequence)
	}
	if err := factorysessions.ValidateFactoryResponseEvent(event); err != nil {
		return factorysessions.FactoryResponseEvent{}, fmt.Errorf("validate SSE event: %w", err)
	}
	return event, nil
}

// AssertObservableToolLifecycle verifies published response events include a
// correlated tool start and completion with stable tool identity fields.
func AssertObservableToolLifecycle(events []factorysessions.FactoryResponseEvent) error {
	var tracker toolLifecycleTracker
	for _, event := range events {
		if err := tracker.observe(event); err != nil {
			return err
		}
	}
	return tracker.finalize()
}

type toolLifecycleTracker struct {
	started    bool
	completed  bool
	toolCallID string
	toolName   string
}

func (t *toolLifecycleTracker) observe(event factorysessions.FactoryResponseEvent) error {
	if event.Kind != factorysessions.ResponseEventKindTool {
		return nil
	}
	switch event.Phase {
	case factorysessions.ResponseEventPhaseStarted:
		return t.observeStarted(event)
	case factorysessions.ResponseEventPhaseCompleted:
		return t.observeCompleted(event)
	default:
		return nil
	}
}

func (t *toolLifecycleTracker) finalize() error {
	if !t.started || !t.completed {
		return fmt.Errorf("tool lifecycle events = started %t completed %t, want both", t.started, t.completed)
	}
	return nil
}

func (t *toolLifecycleTracker) observeStarted(event factorysessions.FactoryResponseEvent) error {
	payload, err := decodeToolPayload(event, "started")
	if err != nil {
		return err
	}
	if err := requireToolIdentity(payload, "started"); err != nil {
		return err
	}
	if t.toolCallID == "" {
		t.toolCallID = payload.ToolCallID
		t.toolName = payload.ToolName
	} else if payload.ToolCallID != t.toolCallID || payload.ToolName != t.toolName {
		return fmt.Errorf("tool started identity drift: %#v", payload)
	}
	t.started = true
	return nil
}

func (t *toolLifecycleTracker) observeCompleted(event factorysessions.FactoryResponseEvent) error {
	payload, err := decodeToolPayload(event, "completed")
	if err != nil {
		return err
	}
	if err := requireToolIdentity(payload, "completed"); err != nil {
		return err
	}
	if t.toolCallID != "" && (payload.ToolCallID != t.toolCallID || payload.ToolName != t.toolName) {
		return fmt.Errorf("tool completed identity mismatch: %#v", payload)
	}
	t.completed = true
	return nil
}

func decodeToolPayload(event factorysessions.FactoryResponseEvent, label string) (factorysessions.ResponseEventTool, error) {
	var payload factorysessions.ResponseEventTool
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return payload, fmt.Errorf("decode tool %s payload: %w", label, err)
	}
	return payload, nil
}

func requireToolIdentity(payload factorysessions.ResponseEventTool, label string) error {
	if payload.ToolCallID == "" || payload.ToolName == "" {
		return fmt.Errorf("tool %s missing identity: %#v", label, payload)
	}
	return nil
}

// AssertTruthfulStreamingFidelity verifies adapter events claim only the fixture
// fidelity class's truthful streaming capabilities.
func AssertTruthfulStreamingFidelity(fixture Fixture, outcome TransportParityOutcome) error {
	switch fixture.FidelityClass {
	case FidelityFullStream:
		return assertFullStreamFidelity(outcome)
	case FidelityPartialStream:
		return assertPartialStreamFidelity(outcome)
	case FidelitySnapshotOnly:
		return assertSnapshotOnlyFidelity(outcome)
	case FidelityFinalOnly:
		return assertFinalOnlyFidelity(outcome)
	default:
		return fmt.Errorf("unsupported fidelity class %q", fixture.FidelityClass)
	}
}

func publishDrafts(
	drafts []factorysessions.ResponseEventDraft,
) ([]factorysessions.FactoryResponseEvent, error) {
	published := make([]factorysessions.FactoryResponseEvent, 0, len(drafts))
	for index, draft := range drafts {
		if err := factorysessions.ValidateResponseEventDraft(draft); err != nil {
			return nil, fmt.Errorf("draft[%d]: %w", index, err)
		}
		stored := factorysessions.FactoryResponseEvent{
			SchemaVersion:      factorysessions.ResponseEventSchemaVersionV1,
			EventID:            fmt.Sprintf("evt-parity-%d", index+1),
			FactorySessionID:   parityTransportSessionID,
			Sequence:           int64(index + 1),
			RecordedAt:         time.Unix(0, int64(index+1)).UTC(),
			RunID:              draft.RunID,
			Kind:               draft.Kind,
			Phase:              draft.Phase,
			Provenance:         draft.Provenance,
			Payload:            append([]byte(nil), draft.Payload...),
			DispatchID:         draft.DispatchID,
			TurnID:             draft.TurnID,
			ItemID:             draft.ItemID,
			ParentItemID:       draft.ParentItemID,
			ProviderSessionRef: draft.ProviderSessionRef,
		}
		if err := factorysessions.ValidateFactoryResponseEvent(stored); err != nil {
			return nil, fmt.Errorf("publish draft[%d]: %w", index, err)
		}
		published = append(published, stored)
	}
	return published, nil
}

func invocationResultFromTerminal(fixture Fixture, terminal TerminalResult) interfaces.FactoryInvocationResult {
	return interfaces.FactoryInvocationResult{
		RequestID: "req-parity-" + fixture.ID,
		TraceID:   "trace-parity-" + fixture.ID,
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: terminal.Response.Content},
		},
		SessionID: parityTransportSessionID,
	}
}

func assertEventSequencesEqual(left, right []factorysessions.FactoryResponseEvent) error {
	if len(left) != len(right) {
		return fmt.Errorf("event count = %d, want %d", len(left), len(right))
	}
	for index := range left {
		if err := assertEventsConsumerParityEqual(left[index], right[index]); err != nil {
			return fmt.Errorf("event[%d]: %w", index, err)
		}
	}
	return nil
}

func assertEventsConsumerParityEqual(want, got factorysessions.FactoryResponseEvent) error {
	if !jsonValuesEqual(want.Payload, got.Payload) {
		return fmt.Errorf("payload mismatch:\nwant=%s\ngot=%s", want.Payload, got.Payload)
	}
	want.Payload = nil
	got.Payload = nil
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("envelope mismatch:\nwant=%#v\ngot=%#v", want, got)
	}
	return nil
}

func jsonValuesEqual(left, right json.RawMessage) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func assertFullStreamFidelity(outcome TransportParityOutcome) error {
	capabilities := outcome.Terminal.Capabilities
	if !capabilities.NativeStreaming || !capabilities.MessageDeltas {
		return fmt.Errorf("capabilities = %#v, want native streaming with message deltas", capabilities)
	}
	if capabilities.FinalOnly {
		return fmt.Errorf("full-stream adapter must not claim final-only capabilities")
	}
	var messageDeltaCount int
	for _, event := range outcome.Events {
		if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseDelta {
			continue
		}
		messageDeltaCount++
		if event.Provenance.Fidelity == factorysessions.ResponseEventFidelityFinalOnly {
			return fmt.Errorf("message delta claims final-only fidelity: %#v", event)
		}
		if event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
			return fmt.Errorf("message delta delivery = %q, want native stream", event.Provenance.Delivery)
		}
	}
	if messageDeltaCount == 0 {
		return fmt.Errorf("full-stream fixture produced no message delta events")
	}
	return nil
}

func assertSnapshotOnlyFidelity(outcome TransportParityOutcome) error {
	capabilities := outcome.Terminal.Capabilities
	if !capabilities.NativeStreaming || !capabilities.MessageSnapshots {
		return fmt.Errorf("capabilities = %#v, want native streaming with message snapshots", capabilities)
	}
	if capabilities.MessageDeltas {
		return fmt.Errorf("snapshot-only adapter must not claim message deltas")
	}
	if capabilities.FinalOnly {
		return fmt.Errorf("snapshot-only adapter must not claim final-only capabilities")
	}
	var messageSnapshotCount int
	for _, event := range outcome.Events {
		if event.Kind != factorysessions.ResponseEventKindMessage {
			continue
		}
		switch event.Phase {
		case factorysessions.ResponseEventPhaseDelta:
			return fmt.Errorf("snapshot-only fixture emitted message delta: %#v", event)
		case factorysessions.ResponseEventPhaseCompleted:
			messageSnapshotCount++
			if event.Provenance.Fidelity == factorysessions.ResponseEventFidelityFinalOnly {
				return fmt.Errorf("message snapshot claims final-only fidelity: %#v", event)
			}
		}
	}
	if messageSnapshotCount == 0 {
		return fmt.Errorf("snapshot-only fixture produced no completed message snapshots")
	}
	return nil
}

func assertFinalOnlyFidelity(outcome TransportParityOutcome) error {
	capabilities := outcome.Terminal.Capabilities
	if capabilities.NativeStreaming || capabilities.MessageDeltas || !capabilities.MessageSnapshots || !capabilities.FinalOnly {
		return fmt.Errorf("capabilities = %#v, want final-only without native streaming", capabilities)
	}
	var messageFinalCount int
	for _, event := range outcome.Events {
		if event.Kind != factorysessions.ResponseEventKindMessage {
			continue
		}
		switch event.Phase {
		case factorysessions.ResponseEventPhaseDelta:
			return fmt.Errorf("final-only fixture emitted message delta: %#v", event)
		case factorysessions.ResponseEventPhaseCompleted:
			messageFinalCount++
			if event.Provenance.Fidelity != factorysessions.ResponseEventFidelityFinalOnly {
				return fmt.Errorf("completed message fidelity = %q, want %q: %#v", event.Provenance.Fidelity, factorysessions.ResponseEventFidelityFinalOnly, event)
			}
			if event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeFinal {
				return fmt.Errorf("completed message delivery = %q, want %q: %#v", event.Provenance.Delivery, factorysessions.ResponseEventDeliveryNativeFinal, event)
			}
		}
	}
	if messageFinalCount == 0 {
		return fmt.Errorf("final-only fixture produced no completed message events")
	}
	return nil
}

func assertPartialStreamFidelity(outcome TransportParityOutcome) error {
	capabilities := outcome.Terminal.Capabilities
	if !capabilities.NativeStreaming || capabilities.MessageDeltas {
		return fmt.Errorf("capabilities = %#v, want native streaming without message deltas", capabilities)
	}
	if capabilities.FinalOnly {
		return fmt.Errorf("partial-stream adapter must not claim final-only capabilities")
	}
	var messageSnapshotCount int
	for _, event := range outcome.Events {
		if event.Kind != factorysessions.ResponseEventKindMessage {
			continue
		}
		switch event.Phase {
		case factorysessions.ResponseEventPhaseDelta:
			return fmt.Errorf("partial-stream fixture emitted message delta: %#v", event)
		case factorysessions.ResponseEventPhaseCompleted:
			messageSnapshotCount++
			if event.Provenance.Fidelity == factorysessions.ResponseEventFidelityFinalOnly {
				return fmt.Errorf("message snapshot claims final-only fidelity: %#v", event)
			}
		}
	}
	if messageSnapshotCount == 0 {
		return fmt.Errorf("partial-stream fixture produced no completed message snapshots")
	}
	return nil
}

func parseSSEFrame(frame string) (idLine, dataLine string, err error) {
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "id: "):
			idLine = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			dataLine = strings.TrimPrefix(line, "data: ")
		default:
			return "", "", fmt.Errorf("unexpected SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		return "", "", fmt.Errorf("SSE frame id=%q data=%q, want exactly one of each", idLine, dataLine)
	}
	return idLine, dataLine, nil
}

type transportCLIResponseEventRecord struct {
	RecordType string                               `json:"recordType"`
	Event      factorysessions.FactoryResponseEvent `json:"event"`
}

type transportCLIInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}
