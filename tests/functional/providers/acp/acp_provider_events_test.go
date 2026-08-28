package acp_test

import (
	"fmt"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertResponseEventsStayOutOfFactoryReplay(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	responseKinds := map[string]bool{"MESSAGE": true, "REASONING": true, "TOOL": true, "FILE_CHANGE": true, "PLAN": true, "USAGE": true, "ERROR": true}
	for _, event := range events {
		if responseKinds[string(event.Type)] {
			t.Fatalf("response event kind %q leaked into canonical Factory replay", event.Type)
		}
	}
}

func assertACPResponseEventSequence(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	want := []struct {
		kind  string
		phase string
	}{
		{kind: "REASONING", phase: "STARTED"},
		{kind: "REASONING", phase: "DELTA"},
		{kind: "PLAN", phase: "UPDATED"},
		{kind: "USAGE", phase: "UPDATED"},
		{kind: "FILE_CHANGE", phase: "UPDATED"},
		{kind: "REASONING", phase: "COMPLETED"},
		{kind: "MESSAGE", phase: "COMPLETED"},
		{kind: "RUN", phase: "COMPLETED"},
	}
	got := make([]factoryapi.FactoryResponseEvent, 0, len(events))
	var previous int64
	for _, event := range events {
		if event.Provenance.Provider != "cursor-acp" {
			continue
		}
		if event.Sequence <= previous {
			t.Fatalf("response event sequence = %d after %d", event.Sequence, previous)
		}
		previous = event.Sequence
		got = append(got, event)
		if event.ProviderSessionRef == nil || *event.ProviderSessionRef != "acp-session-functional-1" {
			t.Fatalf("response event ProviderSessionRef = %#v", event.ProviderSessionRef)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("ACP response event kinds = %v, want %v (all events: %#v)", got, want, events)
	}
	for index := range want {
		if string(got[index].Kind) != want[index].kind || string(got[index].Phase) != want[index].phase {
			t.Fatalf("ACP response events[%d] = %s/%s, want %s/%s", index, got[index].Kind, got[index].Phase, want[index].kind, want[index].phase)
		}
	}
	message := got[len(got)-2]
	if message.Kind != "MESSAGE" || message.Phase != "COMPLETED" || message.Provenance.Representation != "SNAPSHOT" {
		t.Fatalf("authoritative ACP response event = %#v, want completed MESSAGE snapshot", message)
	}
	final := got[len(got)-1]
	if final.Kind != "RUN" || final.Phase != "COMPLETED" {
		t.Fatalf("terminal ACP response event = %#v, want completed RUN", final)
	}
}

func assertACPWorkerSessionHistory(t *testing.T, events []factoryapi.WorkerSessionEvent) {
	t.Helper()
	records := make([]factoryapi.WorkerSessionEvent, 0, len(events))
	var summary *factoryapi.WorkerSessionReplaySummary
	for _, event := range events {
		if event.ReplaySummary != nil {
			summary = event.ReplaySummary
		}
		if string(event.Delivery) == "REPLAY_SUMMARY" {
			summary = event.ReplaySummary
			continue
		}
		if event.Event.Position == 0 {
			t.Fatalf("ACP Worker Session emitted a non-record frame: %#v", event)
		}
		records = append(records, event)
	}
	if len(records) == 0 || summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(records)) {
		t.Fatalf("ACP Worker Session replay = records=%d summary=%#v, want complete history", len(records), summary)
	}
	if records[0].Event.SourceType == "factory_event" {
		assertCanonicalACPWorkerSessionHistory(t, records)
		return
	}
	opening := records[0]
	if opening.Event.Position != 1 || opening.Event.SourceType != "worker_session_lifecycle" ||
		acpWorkerString(opening, "kind") != "SESSION" || acpWorkerString(opening, "phase") != "STARTED" {
		t.Fatalf("ACP Worker Session opening = %#v, want lifecycle SESSION/STARTED at position 1", opening)
	}
	if acpWorkerString(opening, "status") != "STARTING" || acpWorkerString(opening, "startedAt") == "" {
		t.Fatalf("ACP Worker Session opening payload = %#v, want STARTING with startedAt", opening.Event.Payload)
	}
	if opening.WorkerSessionId == "" || len(opening.WorkIds) == 0 {
		t.Fatalf("ACP Worker Session opening correlation = %#v, want Worker Session and Work identities", opening)
	}
	if opening.ProviderSession.Provider != "cursor-acp" || opening.ProviderSession.Kind != "session_id" || opening.ProviderSession.Id == "" {
		t.Fatalf("ACP Worker Session opening provider reference = %#v, want cursor-acp session_id identity", opening.ProviderSession)
	}

	providerBindingIndex := -1
	firstProviderOutputIndex := -1
	terminalIndex := -1
	seenSourceEvents := make(map[string]struct{})
	lastSourceSequences := make(map[string]int64)
	for index, event := range records {
		if event.WorkerSessionId != opening.WorkerSessionId || !sameACPWorkIDs(event.WorkIds, opening.WorkIds) {
			t.Fatalf("ACP Worker Session frame[%d] correlation = %#v, want Worker Session %q and Work IDs %#v", index, event, opening.WorkerSessionId, opening.WorkIds)
		}
		if index > 0 && event.Event.Position <= records[index-1].Event.Position {
			t.Fatalf("ACP Worker Session positions are not increasing: frame[%d]=%d previous=%d", index, event.Event.Position, records[index-1].Event.Position)
		}
		key := fmt.Sprintf("%s|%s|%d|%s", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, event.Event.SourceEventId)
		if _, exists := seenSourceEvents[key]; exists {
			t.Fatalf("ACP Worker Session duplicated source event key %q", key)
		}
		seenSourceEvents[key] = struct{}{}
		sourceKey := event.Event.SourceType + "|" + event.Event.SourceId
		if previous, exists := lastSourceSequences[sourceKey]; exists && event.Event.SourceSequence <= previous {
			t.Fatalf("ACP Worker Session source sequence regressed for %s: %d after %d", sourceKey, event.Event.SourceSequence, previous)
		}
		lastSourceSequences[sourceKey] = event.Event.SourceSequence
		if event.ProviderSession.Provider != "cursor-acp" || event.ProviderSession.Kind != "session_id" || event.ProviderSession.Id == "" {
			t.Fatalf("ACP Worker Session frame[%d] provider reference = %#v, want cursor-acp session_id identity", index, event.ProviderSession)
		}

		kind, phase := acpWorkerString(event, "kind"), acpWorkerString(event, "phase")
		if !legalACPWorkerEvent(kind, phase) {
			t.Fatalf("ACP Worker Session frame[%d] has illegal normalized pair %s/%s: %#v", index, kind, phase, event.Event.Payload)
		}
		if event.Event.SourceType == "worker_session_lifecycle" && phase == "UPDATED" && acpWorkerProvider(event) == "cursor-acp" {
			if providerBindingIndex != -1 {
				t.Fatalf("ACP Worker Session history has multiple provider bindings: %#v", records)
			}
			providerBindingIndex = index
			assertACPLifecycleProvenance(t, event, "cursor-acp", "STARTING")
		}
		if event.Event.SourceType == "worker_observation" {
			if firstProviderOutputIndex == -1 {
				firstProviderOutputIndex = index
			}
			if acpWorkerProvider(event) != "cursor-acp" || acpWorkerProvenance(event, "delivery") != "NATIVE_STREAM" ||
				acpWorkerProvenance(event, "representation") != "NOTIFICATION" || acpWorkerProvenance(event, "fidelity") != "NORMALIZED" {
				t.Fatalf("ACP Worker Session provider output provenance = %#v, want cursor-acp/NATIVE_STREAM/NOTIFICATION/NORMALIZED", event.Event.Payload)
			}
		}
		if event.Event.SourceType == "worker_session_lifecycle" && phase == "COMPLETED" {
			if terminalIndex != -1 || kind != "SESSION" || string(event.Delivery) != "TERMINAL_REPLAY" || acpWorkerString(event, "status") != "COMPLETED" {
				t.Fatalf("ACP Worker Session terminal = %#v, want exactly one SESSION/COMPLETED TERMINAL_REPLAY", event)
			}
			terminalIndex = index
			assertACPLifecycleProvenance(t, event, "cursor-acp", "COMPLETED")
		}
	}
	if providerBindingIndex == -1 || firstProviderOutputIndex == -1 || providerBindingIndex >= firstProviderOutputIndex {
		t.Fatalf("ACP Worker Session provider binding/output order = binding %d, output %d", providerBindingIndex, firstProviderOutputIndex)
	}
	if terminalIndex != len(records)-1 {
		t.Fatalf("ACP Worker Session terminal index = %d, want last record %d", terminalIndex, len(records)-1)
	}
}

func assertCanonicalACPWorkerSessionHistory(t *testing.T, records []factoryapi.WorkerSessionEvent) {
	t.Helper()
	if len(records) < 2 {
		t.Fatalf("canonical ACP Worker Session history = %#v, want opening and terminal", records)
	}
	opening := records[0]
	if opening.Event.SchemaId != "DISPATCH_REQUEST" || opening.WorkerSessionId == "" || len(opening.WorkIds) == 0 {
		t.Fatalf("canonical ACP opening = %#v, want correlated DISPATCH_REQUEST", opening)
	}
	providerResponse := false
	for index, event := range records {
		if event.Event.SourceType != "factory_event" {
			t.Fatalf("canonical ACP frame[%d] source type = %q, want factory_event", index, event.Event.SourceType)
		}
		if event.WorkerSessionId != opening.WorkerSessionId || !sameACPWorkIDs(event.WorkIds, opening.WorkIds) {
			t.Fatalf("canonical ACP frame[%d] correlation = %#v, want Worker Session %q and Work IDs %#v", index, event, opening.WorkerSessionId, opening.WorkIds)
		}
		if index > 0 && event.Event.Position <= records[index-1].Event.Position {
			t.Fatalf("canonical ACP positions are not increasing: frame[%d]=%d previous=%d", index, event.Event.Position, records[index-1].Event.Position)
		}
		if event.ProviderSession.Provider != "cursor-acp" || event.ProviderSession.Kind != "session_id" || event.ProviderSession.Id == "" {
			t.Fatalf("canonical ACP frame[%d] provider reference = %#v, want cursor-acp session identity", index, event.ProviderSession)
		}
		providerResponse = providerResponse || event.Event.SchemaId == "MODEL_RESPONSE"
	}
	if !providerResponse {
		t.Fatalf("canonical ACP history omitted MODEL_RESPONSE: %#v", records)
	}
	terminal := records[len(records)-1]
	if terminal.Event.SchemaId != "DISPATCH_RESPONSE" || terminal.Delivery != "TERMINAL_REPLAY" {
		t.Fatalf("canonical ACP terminal = %#v, want DISPATCH_RESPONSE TERMINAL_REPLAY", terminal)
	}
}

func legalACPWorkerEvent(kind, phase string) bool {
	switch kind {
	case "SESSION", "RUN":
		return phase == "STARTED" || phase == "UPDATED" || phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED"
	case "PROGRESS":
		return phase == "UPDATED"
	case "MESSAGE", "TOOL", "REASONING", "FILE_CHANGE", "PLAN", "USAGE", "ERROR":
		return phase == "STARTED" || phase == "DELTA" || phase == "UPDATED" || phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED"
	default:
		return false
	}
}

func acpWorkerPayload(event factoryapi.WorkerSessionEvent) map[string]interface{} {
	if nested, ok := event.Event.Payload["payload"].(map[string]interface{}); ok {
		return nested
	}
	return event.Event.Payload
}

func acpWorkerString(event factoryapi.WorkerSessionEvent, key string) string {
	if value, ok := acpWorkerPayload(event)[key].(string); ok {
		return value
	}
	if value, ok := event.Event.Payload[key].(string); ok {
		return value
	}
	return ""
}

func acpWorkerProvider(event factoryapi.WorkerSessionEvent) string {
	return acpWorkerProvenance(event, "provider")
}

func acpWorkerProvenance(event factoryapi.WorkerSessionEvent, key string) string {
	provenance, ok := event.Event.Payload["provenance"].(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := provenance[key].(string)
	return value
}

func assertACPLifecycleProvenance(t *testing.T, event factoryapi.WorkerSessionEvent, provider, status string) {
	t.Helper()
	if acpWorkerProvider(event) != provider || acpWorkerProvenance(event, "delivery") != "SYNTHESIZED" ||
		acpWorkerProvenance(event, "representation") != "NOTIFICATION" || acpWorkerProvenance(event, "fidelity") != "LIFECYCLE_ONLY" ||
		acpWorkerString(event, "status") != status {
		t.Fatalf("ACP Worker Session lifecycle provenance = %#v, want %s/%s SYNTHESIZED/NOTIFICATION/LIFECYCLE_ONLY", event.Event.Payload, provider, status)
	}
}

func sameACPWorkIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
