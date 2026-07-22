package factorysession_test

import (
	"encoding/json"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this consumer test keeps fake-service projection assertions together across apisurface mappers.
func TestFakeServiceConsumer_ProjectsFixtureThroughApisurfaceMappers(t *testing.T) {
	scenario := findScenario(t, loadDurableFixtureCatalog(t), "petri-succeeded-one-dispatch")
	sessionFixture, ok := scenario["session"].(map[string]any)
	if !ok {
		t.Fatal("missing session fixture")
	}
	started := asyncStartFromSessionFixture(sessionFixture)
	mappedStart := factorysession.AsyncStartResponseToAPI(started)
	if mappedStart.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q", mappedStart.SessionId)
	}
	if mappedStart.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", mappedStart.Status)
	}

	read := sessionReadFromFixture(sessionFixture)
	mappedRead := factorysession.SessionReadResponseToAPI(read)
	if mappedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("read status = %q", mappedRead.Status)
	}

	dispatches := listDispatchesFromFixture(scenario)
	mappedDispatches := factorysession.ListDispatchesResponseToAPI(dispatches)
	if len(mappedDispatches.Dispatches) != 1 || mappedDispatches.Dispatches[0].Id != "disp-petri-success-001" {
		t.Fatalf("dispatches = %#v", mappedDispatches.Dispatches)
	}

	resultFixture, ok := scenario["result"].(map[string]any)
	if !ok {
		t.Fatal("missing result fixture")
	}
	result := resultFromFixture(resultFixture)
	mappedResult := factorysession.ResultResponseToAPI(result)
	if mappedResult.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", mappedResult.ResultStatus)
	}
	if mappedResult.ArtifactRefs == nil || len(*mappedResult.ArtifactRefs) != 1 || (*mappedResult.ArtifactRefs)[0].Id != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", mappedResult.ArtifactRefs)
	}

	eventRows, ok := scenario["events"].([]any)
	if !ok {
		eventRows = []any{map[string]any{
			"id":       "session-completed/dur-sess-petri-success-001",
			"type":     "factory.session.completed",
			"sequence": 1,
		}}
	}
	events := eventReadResultFromFixture(t, eventRows)
	mappedEvents := factorysession.EventReadResponseToAPI(events)
	if len(mappedEvents) == 0 {
		t.Fatal("terminal events missing")
	}
}

func TestFakeServiceConsumer_ProjectsCanonicalFixtureEventsForRunningApprovalTerminal(t *testing.T) {
	cases := []struct {
		scenarioID string
		eventCount int
	}{
		{"javascript-running-n-dispatch", 2},
		{"javascript-succeeded-two-dispatch", 3},
		{"javascript-awaiting-approval", 2},
	}
	for _, tc := range cases {
		t.Run(tc.scenarioID, func(t *testing.T) {
			scenario := findScenario(t, loadDurableFixtureCatalog(t), tc.scenarioID)
			eventRows, ok := scenario["events"].([]any)
			if !ok {
				t.Fatal("missing events fixture")
			}
			events := eventReadResultFromFixture(t, eventRows)
			mapped := factorysession.EventReadResponseToAPI(events)
			if len(mapped) != tc.eventCount {
				t.Fatalf("mapped events = %d, want %d", len(mapped), tc.eventCount)
			}
			for index, event := range mapped {
				if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
					t.Fatalf("event[%d] schemaVersion = %q, want agent-factory.event.v1", index, event.SchemaVersion)
				}
			}
		})
	}
}

func eventReadResultFromFixture(t *testing.T, events []any) factorysessionexecution.EventReadResult {
	t.Helper()
	rawEvents := make([]json.RawMessage, 0, len(events))
	for index, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event[%d]: %v", index, err)
		}
		rawEvents = append(rawEvents, raw)
	}
	return factorysessionexecution.EventReadResult{Events: rawEvents}
}

func asyncStartFromSessionFixture(session map[string]any) factorysessionexecution.AsyncStartResult {
	read := sessionReadFromFixture(session)
	return factorysessionexecution.AsyncStartResult{
		SessionID:        read.SessionID,
		Status:           string(read.Status),
		OrchestratorKind: read.OrchestratorKind,
		Dialect:          read.Dialect,
		ResolvedSource:   read.ResolvedSource,
		SourceHash:       read.SourceHash,
		Policy:           read.Policy,
		Links:            read.Links,
	}
}

func TestInvocationRequestFromAPI_PreservesPublicInputAtDomainBoundary(t *testing.T) {
	requestID := "request-1"
	sourceKind := factoryapi.InvocationInputSourceKindText
	timeoutMillis := int64(2500)
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: "hello",
	}); err != nil {
		t.Fatalf("FromWorkTextContentPart: %v", err)
	}
	content := factoryapi.WorkContent{part}
	args := map[string]any{"prompt": "hello"}

	mapped := factorysession.InvocationRequestFromAPI(factoryapi.InvocationRequest{
		Args: &args, Content: &content, RequestId: &requestID,
		SourceKind: &sourceKind, TimeoutMillis: &timeoutMillis,
	})
	args["prompt"] = "mutated"
	requestID = "mutated"
	timeoutMillis = 1
	if !mapped.ContentProvided || len(mapped.Content) != 1 || mapped.Content[0].Text != "hello" {
		t.Fatalf("mapped content = %#v, provided=%v", mapped.Content, mapped.ContentProvided)
	}
	if mapped.RequestID == nil || *mapped.RequestID != "request-1" || mapped.SourceKind == nil || string(*mapped.SourceKind) != "text" {
		t.Fatalf("mapped identity/source = %#v", mapped)
	}
	if mapped.Args == nil || (*mapped.Args)["prompt"] != "hello" || mapped.TimeoutMillis == nil || *mapped.TimeoutMillis != 2500 {
		t.Fatalf("mapped args/timeout = %#v", mapped)
	}
}
