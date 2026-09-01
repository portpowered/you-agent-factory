package inference_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestWSRFT003ProviderNeutralLifecycleWorksWithoutProviderSession replays the
// sanitized Antigravity final-only fixture through the customer process
// boundary. It proves the provider-neutral output has honest final-only
// fidelity and that neither the inference event nor response stream invents a
// Provider Session reference that the provider did not emit.
//
// WSR-FT-003: provider-independent opening/terminal history and no fabricated
// Provider Session reference.
// golden: tests/functional/internal/support/testdata/provider-sessions/agy/final-only-success/manifest.json
func TestWSRFT003ProviderNeutralLifecycleWorksWithoutProviderSession(t *testing.T) {
	t.Parallel()
	loaded := loadOpeningRecordFixture(t, "agy", "final-only-success")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", sharedInferenceWithExecutorProvider(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, loaded.Process.Model),
		"ANTIGRAVITY",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"WSR-FT-003 provider-neutral lifecycle"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})
	_, listed, factoryEvents, responseEvents, workerEvents := runSharedInferenceFactoryWithStreams(
		t,
		dir,
		sharedInferenceScenario{commandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want 1", runner.CallCount())
	}
	assertAgyOpeningRecordProviderNeutrality(t, factoryEvents)
	assertAgyFinalOnlyOpeningRecord(t, responseEvents)
	assertAgyWorkerSessionHistory(t, workerEvents)
	functionalevidence.Covers(t,
		"rest/streamWorkerSessionEventsByWorkerSessionId",
		"sse/streamWorkerSessionEventsByWorkerSessionId",
	)
}

func loadOpeningRecordFixture(
	t *testing.T,
	fixtureProvider string,
	caseName string,
) support.ProviderSessionCase {
	t.Helper()
	caseDir := filepath.Join(
		testutil.MustRepoRoot(t),
		filepath.FromSlash(support.ProviderSessionFixturePath(fixtureProvider, caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q/%q): %v", fixtureProvider, caseName, err)
	}
	return loaded
}

func assertAgyOpeningRecordProviderNeutrality(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse && event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		observation, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode Antigravity inference response: %v", err)
		}
		if observation.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		if observation.ProviderSession != nil &&
			observation.ProviderSession.Id != nil &&
			strings.TrimSpace(*observation.ProviderSession.Id) != "" {
			t.Fatalf("Antigravity final-only response fabricated Provider Session id: %#v", observation.ProviderSession)
		}
		if observation.Response == nil || !strings.Contains(*observation.Response, "COMPLETE") {
			t.Fatalf("Antigravity response = %#v, want successful COMPLETE-bearing final output", observation.Response)
		}
		return
	}
	t.Fatalf("Factory Event history omitted successful Antigravity inference response: %#v", events)
}

func assertAgyFinalOnlyOpeningRecord(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("response-event history is empty")
	}
	completedMessage := false
	for _, event := range events {
		if event.ProviderSessionRef != nil && strings.TrimSpace(*event.ProviderSessionRef) != "" {
			t.Fatalf("Antigravity response event fabricated Provider Session reference: %#v", event)
		}
		if event.Kind == factoryapi.FactoryResponseEventKindMessage {
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseDelta:
				t.Fatalf("Antigravity final-only response fabricated MESSAGE/DELTA: %#v", event)
			case factoryapi.FactoryResponseEventPhaseCompleted:
				if event.Provenance.Provider != string(modelprovider.ProviderAntigravity) ||
					event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot ||
					event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityLossless {
					t.Fatalf("Antigravity final message provenance = %#v, want provider-attributed normalized snapshot", event.Provenance)
				}
				payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
				if err != nil {
					t.Fatalf("decode Antigravity final message: %v", err)
				}
				encoded, err := json.Marshal(payload)
				if err != nil || !strings.Contains(string(encoded), "COMPLETE") {
					t.Fatalf("Antigravity final message = %#v, want COMPLETE-bearing snapshot", payload)
				}
				completedMessage = true
			}
		}
	}
	if !completedMessage {
		t.Fatalf("Antigravity response stream omitted final authoritative message: %#v", events)
	}
}

func assertAgyWorkerSessionHistory(t *testing.T, events []factoryapi.WorkerSessionEvent) {
	t.Helper()
	records := make([]factoryapi.WorkerSessionEvent, 0, len(events))
	var summary *factoryapi.WorkerSessionReplaySummary
	for _, event := range events {
		if event.ReplaySummary != nil {
			summary = event.ReplaySummary
		}
		if string(event.Delivery) == "REPLAY_SUMMARY" {
			continue
		}
		if event.Event.Position == 0 {
			t.Fatalf("Antigravity Worker Session emitted a non-record frame: %#v", event)
		}
		records = append(records, event)
	}
	if len(records) == 0 {
		t.Fatal("Antigravity Worker Session history is empty")
	}
	if summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(records)) {
		t.Fatalf("Antigravity Worker Session replay summary = %#v, want complete summary for %d records", summary, len(records))
	}
	if records[0].Event.SourceType == "factory_event" {
		assertCanonicalFactoryWorkerSessionHistory(t, records, "antigravity", false)
		return
	}

	opening := records[0]
	if opening.Event.Position != 1 || opening.Event.SourceType != "worker_session_lifecycle" ||
		workerEventString(opening, "kind") != "SESSION" || workerEventString(opening, "phase") != "STARTED" {
		t.Fatalf("Antigravity Worker Session opening = %#v, want lifecycle SESSION/STARTED at position 1", opening)
	}
	if workerEventString(opening, "status") != "STARTING" || workerEventString(opening, "startedAt") == "" {
		t.Fatalf("Antigravity Worker Session opening payload = %#v, want STARTING with startedAt", opening.Event.Payload)
	}
	if _, exists := opening.Event.Payload["providerSession"]; exists || hasProviderSession(opening) {
		t.Fatalf("Antigravity Worker Session opening fabricated Provider Session reference: %#v", opening)
	}
	assertLifecycleProvenance(t, opening, "", "STARTING")

	workerSessionID := opening.WorkerSessionId
	if workerSessionID == "" || len(opening.WorkIds) == 0 || opening.WorkIds[0] == "" {
		t.Fatalf("Antigravity Worker Session opening correlation = %#v, want Worker Session and Work identities", opening)
	}
	assertAgyWorkerSessionRecords(t, records, opening)
}

func assertAgyWorkerSessionRecords(t *testing.T, records []factoryapi.WorkerSessionEvent, opening factoryapi.WorkerSessionEvent) {
	t.Helper()
	workerSessionID := opening.WorkerSessionId
	providerBindingIndex := -1
	firstProviderOutputIndex := -1
	terminalIndex := -1
	seenSourceEvents := make(map[string]struct{})
	lastSourceSequences := make(map[string]int64)
	for index, event := range records {
		if event.WorkerSessionId != workerSessionID || !sameWorkIDs(event.WorkIds, opening.WorkIds) {
			t.Fatalf("Antigravity Worker Session frame[%d] correlation = %#v, want Worker Session %q and Work IDs %#v", index, event, workerSessionID, opening.WorkIds)
		}
		if index > 0 && event.Event.Position <= records[index-1].Event.Position {
			t.Fatalf("Antigravity Worker Session positions are not increasing: frame[%d]=%d previous=%d", index, event.Event.Position, records[index-1].Event.Position)
		}
		key := fmt.Sprintf("%s|%s|%d|%s", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, event.Event.SourceEventId)
		if _, exists := seenSourceEvents[key]; exists {
			t.Fatalf("Antigravity Worker Session duplicated source event key %q", key)
		}
		seenSourceEvents[key] = struct{}{}
		if previous, exists := lastSourceSequences[event.Event.SourceType+"|"+event.Event.SourceId]; exists && event.Event.SourceSequence <= previous {
			t.Fatalf("Antigravity Worker Session source sequence regressed for %s/%s: %d after %d", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, previous)
		}
		lastSourceSequences[event.Event.SourceType+"|"+event.Event.SourceId] = event.Event.SourceSequence

		kind, phase := workerEventString(event, "kind"), workerEventString(event, "phase")
		if !legalAgyWorkerEvent(kind, phase) {
			t.Fatalf("Antigravity Worker Session frame[%d] has illegal normalized pair %s/%s: %#v", index, kind, phase, event.Event.Payload)
		}
		if event.Event.SourceType == "worker_session_lifecycle" && phase == "UPDATED" && workerEventProvider(event) == "antigravity" {
			if providerBindingIndex != -1 {
				t.Fatalf("Antigravity Worker Session history has multiple provider bindings: %#v", records)
			}
			providerBindingIndex = index
			assertLifecycleProvenance(t, event, "antigravity", "STARTING")
		}
		if event.Event.SourceType == "worker_observation" {
			if firstProviderOutputIndex == -1 {
				firstProviderOutputIndex = index
			}
			if workerEventProvider(event) != "antigravity" {
				t.Fatalf("Antigravity provider output attribution = %#v, want antigravity", event.Event.Payload)
			}
			switch kind {
			case "MESSAGE":
				if workerEventProvenanceString(event, "delivery") != "NATIVE_FINAL" ||
					workerEventProvenanceString(event, "representation") != "SNAPSHOT" || workerEventProvenanceString(event, "fidelity") != "FINAL_ONLY" {
					t.Fatalf("Antigravity final message provenance = %#v, want NATIVE_FINAL/SNAPSHOT/FINAL_ONLY", event.Event.Payload)
				}
				if phase == "DELTA" {
					t.Fatalf("Antigravity final-only Worker Session fabricated MESSAGE/DELTA: %#v", event.Event.Payload)
				}
			case "RUN":
				if workerEventProvenanceString(event, "delivery") != "SYNTHESIZED" ||
					workerEventProvenanceString(event, "representation") != "NOTIFICATION" || workerEventProvenanceString(event, "fidelity") != "LIFECYCLE_ONLY" {
					t.Fatalf("Antigravity lifecycle provenance = %#v, want SYNTHESIZED/NOTIFICATION/LIFECYCLE_ONLY", event.Event.Payload)
				}
			}
		}
		if event.Event.SourceType == "worker_session_lifecycle" && phase == "COMPLETED" {
			if terminalIndex != -1 {
				t.Fatalf("Antigravity Worker Session history has multiple terminal lifecycle records: %#v", records)
			}
			terminalIndex = index
			if kind != "SESSION" || workerEventString(event, "status") != "COMPLETED" || string(event.Delivery) != "TERMINAL_REPLAY" {
				t.Fatalf("Antigravity Worker Session terminal = %#v, want SESSION/COMPLETED TERMINAL_REPLAY", event)
			}
			assertLifecycleProvenance(t, event, "antigravity", "COMPLETED")
		}
		if hasProviderSession(event) {
			t.Fatalf("Antigravity Worker Session fabricated Provider Session reference: %#v", event)
		}
	}
	if providerBindingIndex == -1 || firstProviderOutputIndex == -1 || providerBindingIndex >= firstProviderOutputIndex {
		t.Fatalf("Antigravity Worker Session provider binding/output order = binding %d, output %d; history=%#v", providerBindingIndex, firstProviderOutputIndex, records)
	}
	if terminalIndex != len(records)-1 {
		t.Fatalf("Antigravity Worker Session terminal index = %d, want last record %d", terminalIndex, len(records)-1)
	}
}

func legalAgyWorkerEvent(kind, phase string) bool {
	switch kind {
	case "SESSION":
		return phase == "STARTED" || phase == "UPDATED" || phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED"
	case "RUN":
		return phase == "STARTED" || phase == "UPDATED" || phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED"
	case "PROGRESS":
		return phase == "UPDATED"
	case "MESSAGE", "TOOL", "REASONING", "FILE_CHANGE", "PLAN", "USAGE", "ERROR":
		return phase == "STARTED" || phase == "DELTA" || phase == "UPDATED" || phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED"
	default:
		return false
	}
}

func workerEventPayload(event factoryapi.WorkerSessionEvent) map[string]interface{} {
	if nested, ok := event.Event.Payload["payload"].(map[string]interface{}); ok {
		return nested
	}
	return event.Event.Payload
}

func workerEventString(event factoryapi.WorkerSessionEvent, key string) string {
	if value, ok := workerEventPayload(event)[key].(string); ok {
		return value
	}
	if value, ok := event.Event.Payload[key].(string); ok {
		return value
	}
	return ""
}

func workerEventProvider(event factoryapi.WorkerSessionEvent) string {
	return workerEventProvenanceString(event, "provider")
}

func workerEventProvenanceString(event factoryapi.WorkerSessionEvent, key string) string {
	provenance, ok := event.Event.Payload["provenance"].(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := provenance[key].(string)
	return value
}

func hasProviderSession(event factoryapi.WorkerSessionEvent) bool {
	return event.ProviderSession.Provider != "" || event.ProviderSession.Kind != "" || event.ProviderSession.Id != ""
}

func assertLifecycleProvenance(t *testing.T, event factoryapi.WorkerSessionEvent, provider, status string) {
	t.Helper()
	if workerEventProvider(event) != provider || workerEventProvenanceString(event, "delivery") != "SYNTHESIZED" ||
		workerEventProvenanceString(event, "representation") != "NOTIFICATION" || workerEventProvenanceString(event, "fidelity") != "LIFECYCLE_ONLY" {
		t.Fatalf("lifecycle provenance = %#v, want provider=%q and SYNTHESIZED/NOTIFICATION/LIFECYCLE_ONLY", event.Event.Payload, provider)
	}
	if status != "" && workerEventString(event, "status") != status {
		t.Fatalf("lifecycle status = %q, want %q; payload=%#v", workerEventString(event, "status"), status, event.Event.Payload)
	}
}

func sameWorkIDs(left, right []string) bool {
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

func assertProviderWorkerSessionHistory(t *testing.T, events []factoryapi.WorkerSessionEvent, provider string, providerSessionExpected bool) {
	t.Helper()
	records := make([]factoryapi.WorkerSessionEvent, 0, len(events))
	var summary *factoryapi.WorkerSessionReplaySummary
	for _, event := range events {
		if event.ReplaySummary != nil {
			summary = event.ReplaySummary
		}
		if string(event.Delivery) == "REPLAY_SUMMARY" {
			continue
		}
		if event.Event.Position == 0 {
			t.Fatalf("%s Worker Session emitted a non-record frame: %#v", provider, event)
		}
		records = append(records, event)
	}
	if len(records) == 0 || summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(records)) {
		t.Fatalf("%s Worker Session replay = records=%d summary=%#v, want complete opening-to-terminal history", provider, len(records), summary)
	}
	if records[0].Event.SourceType == "factory_event" {
		assertCanonicalFactoryWorkerSessionHistory(t, records, provider, providerSessionExpected)
		return
	}

	opening := records[0]
	if opening.Event.Position != 1 || opening.Event.SourceType != "worker_session_lifecycle" ||
		workerEventString(opening, "kind") != "SESSION" || workerEventString(opening, "phase") != "STARTED" {
		t.Fatalf("%s Worker Session opening = %#v, want lifecycle SESSION/STARTED at position 1", provider, opening)
	}
	if opening.WorkerSessionId == "" || len(opening.WorkIds) == 0 {
		t.Fatalf("%s Worker Session opening correlation = %#v, want Worker Session and Work identities", provider, opening)
	}
	providerBindingIndex := -1
	firstProviderOutputIndex := -1
	terminalIndex := -1
	providerReferenceSeen := false
	seenSourceEvents := make(map[string]struct{})
	lastSourceSequences := make(map[string]int64)
	for index, event := range records {
		if event.WorkerSessionId != opening.WorkerSessionId || !sameWorkIDs(event.WorkIds, opening.WorkIds) {
			t.Fatalf("%s Worker Session frame[%d] correlation = %#v, want Worker Session %q and Work IDs %#v", provider, index, event, opening.WorkerSessionId, opening.WorkIds)
		}
		if index > 0 && event.Event.Position <= records[index-1].Event.Position {
			t.Fatalf("%s Worker Session positions are not increasing: frame[%d]=%d previous=%d", provider, index, event.Event.Position, records[index-1].Event.Position)
		}
		key := fmt.Sprintf("%s|%s|%d|%s", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, event.Event.SourceEventId)
		if _, exists := seenSourceEvents[key]; exists {
			t.Fatalf("%s Worker Session duplicated source event key %q", provider, key)
		}
		seenSourceEvents[key] = struct{}{}
		sourceKey := event.Event.SourceType + "|" + event.Event.SourceId
		if previous, exists := lastSourceSequences[sourceKey]; exists && event.Event.SourceSequence <= previous {
			t.Fatalf("%s Worker Session source sequence regressed for %s: %d after %d", provider, sourceKey, event.Event.SourceSequence, previous)
		}
		lastSourceSequences[sourceKey] = event.Event.SourceSequence
		kind, phase := workerEventString(event, "kind"), workerEventString(event, "phase")
		if !legalAgyWorkerEvent(kind, phase) {
			t.Fatalf("%s Worker Session frame[%d] has illegal normalized pair %s/%s: %#v", provider, index, kind, phase, event.Event.Payload)
		}
		if hasProviderSession(event) {
			if !providerSessionExpected || event.ProviderSession.Provider != provider || event.ProviderSession.Id == "" {
				t.Fatalf("%s Worker Session provider reference = %#v, expected=%t", provider, event.ProviderSession, providerSessionExpected)
			}
			providerReferenceSeen = true
		}
		if event.Event.SourceType == "worker_session_lifecycle" && phase == "UPDATED" && workerEventProvider(event) == provider {
			if providerBindingIndex != -1 {
				t.Fatalf("%s Worker Session history has multiple provider bindings: %#v", provider, records)
			}
			providerBindingIndex = index
		}
		if event.Event.SourceType != "worker_session_lifecycle" {
			if firstProviderOutputIndex == -1 {
				firstProviderOutputIndex = index
			}
			if workerEventProvider(event) != provider || workerEventProvenanceString(event, "representation") != "NOTIFICATION" ||
				workerEventProvenanceString(event, "fidelity") == "" || workerEventProvenanceString(event, "fidelity") == "LIFECYCLE_ONLY" {
				t.Fatalf("%s provider output provenance = %#v, want provider-attributed non-lifecycle fidelity", provider, event.Event.Payload)
			}
		}
		if event.Event.SourceType == "worker_session_lifecycle" && (phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED") {
			if terminalIndex != -1 || kind != "SESSION" || string(event.Delivery) != "TERMINAL_REPLAY" {
				t.Fatalf("%s Worker Session terminal = %#v, want exactly one SESSION terminal replay", provider, event)
			}
			terminalIndex = index
		}
	}
	if providerBindingIndex == -1 && workerEventProvider(opening) != provider {
		t.Fatalf("%s Worker Session never established provider identity before output", provider)
	}
	if firstProviderOutputIndex == -1 || (providerBindingIndex != -1 && providerBindingIndex >= firstProviderOutputIndex) {
		t.Fatalf("%s Worker Session provider ordering = binding %d, output %d", provider, providerBindingIndex, firstProviderOutputIndex)
	}
	if terminalIndex != len(records)-1 {
		t.Fatalf("%s Worker Session terminal index = %d, want last record %d", provider, terminalIndex, len(records)-1)
	}
	if providerSessionExpected != providerReferenceSeen {
		t.Fatalf("%s Worker Session provider reference seen = %t, want %t", provider, providerReferenceSeen, providerSessionExpected)
	}
}

func assertCanonicalFactoryWorkerSessionHistory(
	t *testing.T,
	records []factoryapi.WorkerSessionEvent,
	provider string,
	providerSessionExpected bool,
) {
	t.Helper()
	if len(records) < 2 {
		t.Fatalf("%s canonical Worker Session history = %#v, want opening and terminal Factory events", provider, records)
	}
	opening := records[0]
	if opening.Event.SchemaId != "DISPATCH_REQUEST" {
		t.Fatalf("%s canonical Worker Session opening schema = %q, want DISPATCH_REQUEST", provider, opening.Event.SchemaId)
	}
	if opening.WorkerSessionId == "" || len(opening.WorkIds) == 0 || opening.WorkIds[0] == "" {
		t.Fatalf("%s canonical Worker Session opening correlation = %#v, want Worker Session and Work identities", provider, opening)
	}

	seenSourceEvents := make(map[string]struct{}, len(records))
	lastPosition := int64(0)
	for index, event := range records {
		if event.WorkerSessionId != opening.WorkerSessionId || !sameWorkIDs(event.WorkIds, opening.WorkIds) {
			t.Fatalf("%s canonical Worker Session frame[%d] correlation = %#v, want Worker Session %q and Work IDs %#v", provider, index, event, opening.WorkerSessionId, opening.WorkIds)
		}
		if event.Event.Position <= lastPosition {
			t.Fatalf("%s canonical Worker Session positions are not increasing: frame[%d]=%d previous=%d", provider, index, event.Event.Position, lastPosition)
		}
		lastPosition = event.Event.Position
		key := fmt.Sprintf("%s|%s|%d|%s", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, event.Event.SourceEventId)
		if _, exists := seenSourceEvents[key]; exists {
			t.Fatalf("%s canonical Worker Session duplicated source event key %q", provider, key)
		}
		seenSourceEvents[key] = struct{}{}
		if event.Event.SourceType != "factory_event" {
			t.Fatalf("%s canonical Worker Session source type = %q, want factory_event", provider, event.Event.SourceType)
		}
		if providerSessionExpected {
			if event.ProviderSession.Provider != provider || event.ProviderSession.Kind != "session_id" || event.ProviderSession.Id == "" {
				t.Fatalf("%s canonical Worker Session provider reference = %#v, want exact provider session", provider, event.ProviderSession)
			}
		} else if hasProviderSession(event) {
			t.Fatalf("%s canonical Worker Session fabricated Provider Session reference: %#v", provider, event)
		}
	}

	terminal := records[len(records)-1]
	if terminal.Event.SchemaId != "DISPATCH_RESPONSE" || string(terminal.Delivery) != "TERMINAL_REPLAY" {
		t.Fatalf("%s canonical Worker Session terminal = %#v, want DISPATCH_RESPONSE TERMINAL_REPLAY", provider, terminal)
	}
}
