package parityfixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	Events           []responseevents.FactoryResponseEvent
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
	store := responseeventstore.NewSessionResponseEventStore(parityTransportSessionID)
	events, err := publishDrafts(store, terminal.Drafts)
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
func EncodeTransportAPIRecords(events []responseevents.FactoryResponseEvent) ([]apisurface.FactoryResponseEventRecord, error) {
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
func DecodeTransportAPIRecords(records []apisurface.FactoryResponseEventRecord) ([]responseevents.FactoryResponseEvent, error) {
	decoded := make([]responseevents.FactoryResponseEvent, 0, len(records))
	for _, record := range records {
		var event responseevents.FactoryResponseEvent
		if err := json.Unmarshal(record.Data, &event); err != nil {
			return nil, fmt.Errorf("decode API record sequence %d: %w", record.Sequence, err)
		}
		if err := responseevents.ValidateEvent(event); err != nil {
			return nil, fmt.Errorf("validate API record sequence %d: %w", record.Sequence, err)
		}
		decoded = append(decoded, event)
	}
	return decoded, nil
}

// EncodeTransportCLINDJSON serializes response events and the terminal invocation
// result using the CLI response-stream NDJSON record envelope.
func EncodeTransportCLINDJSON(
	events []responseevents.FactoryResponseEvent,
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
func DecodeTransportCLINDJSON(lines []string) ([]responseevents.FactoryResponseEvent, factoryapi.InvocationResponse, error) {
	if len(lines) == 0 {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("CLI NDJSON is empty")
	}
	events := make([]responseevents.FactoryResponseEvent, 0, len(lines)-1)
	for index, line := range lines[:len(lines)-1] {
		var record transportCLIResponseEventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, factoryapi.InvocationResponse{}, fmt.Errorf("decode CLI response_event line %d: %w", index, err)
		}
		if record.RecordType != transportJSONRecordResponseEvent {
			return nil, factoryapi.InvocationResponse{}, fmt.Errorf("CLI line %d recordType = %q, want %q", index, record.RecordType, transportJSONRecordResponseEvent)
		}
		if err := responseevents.ValidateEvent(record.Event); err != nil {
			return nil, factoryapi.InvocationResponse{}, fmt.Errorf("validate CLI response_event line %d: %w", index, err)
		}
		events = append(events, record.Event)
	}
	var finalRecord transportCLIInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("decode CLI invocation_result: %w", err)
	}
	if finalRecord.RecordType != transportJSONRecordInvocationResult {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("final CLI recordType = %q, want %q", finalRecord.RecordType, transportJSONRecordInvocationResult)
	}
	return events, finalRecord.Invocation, nil
}

// EncodeTransportSSEFrame serializes one event using the HTTP SSE wire format.
func EncodeTransportSSEFrame(event responseevents.FactoryResponseEvent) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal SSE event: %w", err)
	}
	return fmt.Sprintf("id: %s\ndata: %s\n\n", strconv.FormatInt(event.Sequence, 10), data), nil
}

// DecodeTransportSSEFrame decodes one HTTP SSE response-event frame.
func DecodeTransportSSEFrame(frame string) (responseevents.FactoryResponseEvent, error) {
	idLine, dataLine, err := parseSSEFrame(frame)
	if err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("decode SSE data: %w", err)
	}
	if idLine != strconv.FormatInt(event.Sequence, 10) {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want sequence %d", idLine, event.Sequence)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("validate SSE event: %w", err)
	}
	return event, nil
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
	store *responseeventstore.SessionResponseEventStore,
	drafts []responseevents.Draft,
) ([]responseevents.FactoryResponseEvent, error) {
	published := make([]responseevents.FactoryResponseEvent, 0, len(drafts))
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			return nil, fmt.Errorf("draft[%d]: %w", index, err)
		}
		stored, err := store.Publish(responseevents.FactoryResponseEvent{
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
		})
		if err != nil {
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
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: terminal.Response.Content},
		},
		SessionID: parityTransportSessionID,
	}
}

func assertEventSequencesEqual(left, right []responseevents.FactoryResponseEvent) error {
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

func assertEventsConsumerParityEqual(want, got responseevents.FactoryResponseEvent) error {
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
		if event.Kind != responseevents.KindMessage || event.Phase != responseevents.PhaseDelta {
			continue
		}
		messageDeltaCount++
		if event.Provenance.Fidelity == responseevents.FidelityFinalOnly {
			return fmt.Errorf("message delta claims final-only fidelity: %#v", event)
		}
		if event.Provenance.Delivery != responseevents.DeliveryNativeStream {
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
		if event.Kind != responseevents.KindMessage {
			continue
		}
		switch event.Phase {
		case responseevents.PhaseDelta:
			return fmt.Errorf("snapshot-only fixture emitted message delta: %#v", event)
		case responseevents.PhaseCompleted:
			messageSnapshotCount++
			if event.Provenance.Fidelity == responseevents.FidelityFinalOnly {
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
		if event.Kind != responseevents.KindMessage {
			continue
		}
		switch event.Phase {
		case responseevents.PhaseDelta:
			return fmt.Errorf("final-only fixture emitted message delta: %#v", event)
		case responseevents.PhaseCompleted:
			messageFinalCount++
			if event.Provenance.Fidelity != responseevents.FidelityFinalOnly {
				return fmt.Errorf("completed message fidelity = %q, want %q: %#v", event.Provenance.Fidelity, responseevents.FidelityFinalOnly, event)
			}
			if event.Provenance.Delivery != responseevents.DeliveryNativeFinal {
				return fmt.Errorf("completed message delivery = %q, want %q: %#v", event.Provenance.Delivery, responseevents.DeliveryNativeFinal, event)
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
		if event.Kind != responseevents.KindMessage {
			continue
		}
		switch event.Phase {
		case responseevents.PhaseDelta:
			return fmt.Errorf("partial-stream fixture emitted message delta: %#v", event)
		case responseevents.PhaseCompleted:
			messageSnapshotCount++
			if event.Provenance.Fidelity == responseevents.FidelityFinalOnly {
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
	RecordType string                              `json:"recordType"`
	Event      responseevents.FactoryResponseEvent `json:"event"`
}

type transportCLIInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}
