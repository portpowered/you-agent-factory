package factorysession_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestDurableSessionMapperRoundTrip_AllFixtureResponses(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			rawScenario := findScenario(t, catalog, scenario.ID)

			if asyncResponse, ok := rawScenario["asyncResponse"].(map[string]any); ok {
				assertAsyncStartMapperRoundTrip(t, scenario.ID, asyncResponse)
			}
			if session, ok := rawScenario["session"].(map[string]any); ok {
				assertSessionReadMapperRoundTrip(t, scenario.ID, session)
			}
			if listSummary, ok := rawScenario["listSummary"].(map[string]any); ok {
				assertListSummaryMapperRoundTrip(t, scenario.ID, listSummary)
			}
			if result, ok := rawScenario["result"].(map[string]any); ok {
				assertResultMapperRoundTrip(t, scenario.ID, result)
			}
			if dispatches, ok := rawScenario["dispatches"].([]any); ok && len(dispatches) > 0 {
				assertDispatchListMapperRoundTrip(t, scenario.ID, rawScenario, dispatches)
			}
			if dispatchDetail, ok := rawScenario["dispatchDetail"].(map[string]any); ok {
				assertDispatchDetailMapperRoundTrip(t, scenario.ID, dispatchDetail)
			}
			if artifacts, ok := rawScenario["artifacts"].([]any); ok && len(artifacts) > 0 {
				assertArtifactListMapperRoundTrip(t, scenario.ID, rawScenario, artifacts)
			}
			if artifactDetail, ok := rawScenario["artifactDetail"].(map[string]any); ok {
				assertArtifactDetailMapperRoundTrip(t, scenario.ID, artifactDetail)
			}
			if lifecycleControl, ok := rawScenario["lifecycleControl"].(map[string]any); ok {
				assertLifecycleControlMapperRoundTrip(t, scenario.ID, lifecycleControl)
			}
			if events, ok := rawScenario["events"].([]any); ok && len(events) > 0 {
				assertFixtureEventsMapperRoundTrip(t, scenario.ID, events)
			}
		})
	}
}

func TestDurableSessionMapperRoundTrip_CanonicalFixtureEventsThroughFakeService(t *testing.T) {
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
			rawScenario := findScenario(t, loadDurableFixtureCatalog(t), tc.scenarioID)
			events, ok := rawScenario["events"].([]any)
			if !ok {
				t.Fatal("missing events")
			}
			if len(events) != tc.eventCount {
				t.Fatalf("events = %d, want %d", len(events), tc.eventCount)
			}
			rootEvents := eventReadResultFromFixture(t, events)
			mapped := factorysession.EventReadResponseToAPI(rootEvents)
			if len(mapped) != tc.eventCount {
				t.Fatalf("mapped events = %d, want %d", len(mapped), tc.eventCount)
			}
			for index, event := range mapped {
				if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
					t.Fatalf("event[%d] schemaVersion = %q, want agent-factory.event.v1", index, event.SchemaVersion)
				}
				if event.Id == "" || event.Type == "" {
					t.Fatalf("event[%d] = %#v, want canonical id and type", index, event)
				}
			}
		})
	}
}

func TestDurableSessionMapperRoundTrip_DispatchCountCoverage(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	cases := []struct {
		id            string
		dispatchCount string
		orchestrator  string
	}{
		{"petri-running-one-dispatch", "ONE", "PETRI"},
		{"javascript-paused-two-dispatch", "TWO", "JAVASCRIPT"},
		{"javascript-running-n-dispatch", "N", "JAVASCRIPT"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			scenario := findScenario(t, catalog, tc.id)
			tags, ok := scenario["tags"].(map[string]any)
			if !ok {
				t.Fatal("missing tags")
			}
			if stringValue(tags, "dispatchCount") != tc.dispatchCount {
				t.Fatalf("dispatchCount = %q, want %q", stringValue(tags, "dispatchCount"), tc.dispatchCount)
			}
			if stringValue(tags, "orchestrator") != tc.orchestrator {
				t.Fatalf("orchestrator = %q, want %q", stringValue(tags, "orchestrator"), tc.orchestrator)
			}
			dispatches, ok := scenario["dispatches"].([]any)
			if !ok {
				t.Fatal("missing dispatches")
			}
			switch tc.dispatchCount {
			case "ONE":
				if len(dispatches) != 1 {
					t.Fatalf("dispatch count = %d, want 1", len(dispatches))
				}
			case "TWO":
				if len(dispatches) != 2 {
					t.Fatalf("dispatch count = %d, want 2", len(dispatches))
				}
			case "N":
				if len(dispatches) < 3 {
					t.Fatalf("dispatch count = %d, want at least 3", len(dispatches))
				}
			}
		})
	}
}

func TestDurableSessionMapperRoundTrip_FakeServiceProjections(t *testing.T) {
	scenarios := []string{
		"petri-succeeded-one-dispatch",
		"javascript-running-n-dispatch",
		"javascript-paused-two-dispatch",
		"petri-canceled",
	}
	for _, scenarioID := range scenarios {
		t.Run(scenarioID, func(t *testing.T) {
			rawScenario := findScenario(t, loadDurableFixtureCatalog(t), scenarioID)
			session, ok := rawScenario["session"].(map[string]any)
			if !ok {
				t.Fatal("missing session")
			}
			assertAsyncStartMapperRoundTrip(t, scenarioID, mustFixtureMap(t,
				factorysession.AsyncStartResponseToAPI(asyncStartFromSessionFixture(session))))
			assertSessionReadMapperRoundTrip(t, scenarioID, session)

			resultFixture, ok := rawScenario["result"].(map[string]any)
			if !ok {
				t.Fatal("missing result fixture")
			}
			assertResultMapperRoundTrip(t, scenarioID, resultFixture)

			dispatches, ok := rawScenario["dispatches"].([]any)
			if !ok {
				t.Fatal("missing dispatches")
			}
			assertDispatchListMapperRoundTrip(t, scenarioID, rawScenario, dispatches)
		})
	}
}

func TestArtifactDetailCaptureMetadataRoundTrip(t *testing.T) {
	capturedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	dispatchID := "disp-petri-success-001"
	mimeType := "text/plain"
	apiValue := factoryapi.FactorySessionArtifactDetail{
		SessionId:  "dur-sess-petri-success-001",
		Id:         "art-petri-final-001",
		Kind:       factoryapi.FactoryArtifactKindFINALRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
		CaptureMetadata: &factoryapi.FactoryArtifactCaptureMetadata{
			CapturedAt:       &capturedAt,
			SourceDispatchId: &dispatchID,
			MimeType:         &mimeType,
		},
	}

	domain, err := factorysession.ArtifactDetailFromAPI(apiValue)
	if err != nil {
		t.Fatalf("ArtifactDetailFromAPI: %v", err)
	}
	if domain.CaptureMetadata == nil {
		t.Fatal("domain captureMetadata = nil, want populated metadata")
	}
	if got := domain.CaptureMetadata["sourceDispatchId"]; got != dispatchID {
		t.Fatalf("domain captureMetadata.sourceDispatchId = %v, want %q", got, dispatchID)
	}

	mapped := factorysession.ArtifactDetailResponseToAPI(domain)
	if mapped.CaptureMetadata == nil {
		t.Fatal("mapped captureMetadata = nil, want populated metadata")
	}
	if mapped.CaptureMetadata.SourceDispatchId == nil || *mapped.CaptureMetadata.SourceDispatchId != dispatchID {
		t.Fatalf("mapped captureMetadata.sourceDispatchId = %v, want %q", mapped.CaptureMetadata.SourceDispatchId, dispatchID)
	}
	if mapped.CaptureMetadata.MimeType == nil || *mapped.CaptureMetadata.MimeType != mimeType {
		t.Fatalf("mapped captureMetadata.mimeType = %v, want %q", mapped.CaptureMetadata.MimeType, mimeType)
	}
	if mapped.CaptureMetadata.CapturedAt == nil || !mapped.CaptureMetadata.CapturedAt.Equal(capturedAt) {
		t.Fatalf("mapped captureMetadata.capturedAt = %v, want %v", mapped.CaptureMetadata.CapturedAt, capturedAt)
	}
}

func TestDispatchAndArtifactMapperRoundTrip_MapsUsageWarningsAndRedactionCounts(t *testing.T) {
	inputTokens := int64(11)
	outputTokens := int64(7)
	totalTokens := int64(18)
	durationMillis := int64(250)
	costUSD := 1.25
	retryCount := int32(2)
	failureReason := "TEMPORARY"
	failureMessage := "provider unavailable"
	dispatch := factoryapi.FactoryDispatch{
		SessionId:        "dur-sess-1",
		Id:               "disp-1",
		Status:           factoryapi.FactoryDispatchStatusFAILED,
		DispatchKind:     factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		OrchestratorKind: factoryapi.FactoryOrchestratorKind("JAVASCRIPT"),
		Usage: &factoryapi.FactoryDispatchUsage{
			InputTokens:    &inputTokens,
			OutputTokens:   &outputTokens,
			TotalTokens:    &totalTokens,
			DurationMillis: &durationMillis,
			CostUsd:        &costUSD,
			RetryCount:     &retryCount,
		},
		Warnings: &[]factoryapi.FactoryDispatchWarning{
			{Code: "RATE_LIMIT", Message: " retried once "},
		},
		FailureDetail: &factoryapi.FailureDetail{
			Reason:  factoryapi.WorkFailureType(failureReason),
			Message: failureMessage,
		},
	}

	domain := factorysession.DispatchDetailFromAPI(dispatch)
	if domain.Usage == nil || domain.Usage.TotalTokens != 18 {
		t.Fatalf("domain usage = %#v, want populated usage", domain.Usage)
	}
	if len(domain.Warnings) != 1 || domain.Warnings[0].Message != "retried once" {
		t.Fatalf("domain warnings = %#v, want trimmed warning", domain.Warnings)
	}
	if domain.FailureDetail == nil || domain.FailureDetail.Reason != "TEMPORARY" {
		t.Fatalf("domain failure = %#v, want trimmed failure", domain.FailureDetail)
	}

	mapped := factorysession.DispatchDetailResponseToAPI(domain)
	if mapped.Usage == nil || mapped.Usage.TotalTokens == nil || *mapped.Usage.TotalTokens != 18 {
		t.Fatalf("mapped usage = %#v, want populated usage", mapped.Usage)
	}
}

func TestArtifactDetailMapperRoundTrip_MapsRedactionCountsAndSessionUsage(t *testing.T) {
	paths := int32(1)
	secrets := int32(2)
	tokens := int32(3)
	available := 4
	total := 8
	usageAPI := &factoryapi.FactorySessionUsage{
		Resources: []factoryapi.ResourceUsage{
			{Name: "tokens", Available: available, Total: total},
		},
	}
	capturedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	dispatchID := "disp-1"
	mimeType := "text/plain"
	artifact := factoryapi.FactorySessionArtifactDetail{
		SessionId:  "dur-sess-1",
		Id:         "art-1",
		Kind:       factoryapi.FactoryArtifactKindLOG,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
		RedactionCounts: &factoryapi.FactoryArtifactRedactionCounts{
			Paths:   &paths,
			Secrets: &secrets,
			Tokens:  &tokens,
		},
		CaptureMetadata: &factoryapi.FactoryArtifactCaptureMetadata{
			CapturedAt:       &capturedAt,
			SourceDispatchId: &dispatchID,
			MimeType:         &mimeType,
		},
	}
	domain, err := factorysession.ArtifactDetailFromAPI(artifact)
	if err != nil {
		t.Fatalf("ArtifactDetailFromAPI: %v", err)
	}
	if domain.RedactionCounts == nil || domain.RedactionCounts.Paths != 1 {
		t.Fatalf("domain redaction counts = %#v, want populated counts", domain.RedactionCounts)
	}
	mapped := factorysession.ArtifactDetailResponseToAPI(domain)
	if mapped.RedactionCounts == nil || mapped.RedactionCounts.Paths == nil || *mapped.RedactionCounts.Paths != 1 {
		t.Fatalf("mapped redaction counts = %#v, want populated counts", mapped.RedactionCounts)
	}

	read := factorysession.SessionReadResultFromAPI(factoryapi.FactorySessionDurableReadModel{
		SessionId: "dur-sess-usage-1",
		Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		Usage:     usageAPI,
	})
	if len(read.Usage.Resources) != 1 || read.Usage.Resources[0].Available != available {
		t.Fatalf("session usage = %#v, want mapped resources", read.Usage)
	}
}

func TestDurableSessionMapperBoundaryValidation(t *testing.T) {
	t.Run("unsupported source kind", func(t *testing.T) {
		_, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-invalid-source",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKind("HOST_PATH"),
			},
		})

		assertRequestValidationError(t, err)
	})

	t.Run("invalid result mode", func(t *testing.T) {
		mode := factoryapi.FactorySessionResultMode("invalid")
		raw, err := factorysession.ResultRequestFromAPI(factoryapi.GetFactorySessionResultsParams{Mode: &mode})
		if err != nil || raw.Mode != factorysessionexecution.ResultMode("invalid") {
			t.Fatalf("raw result request = %#v, error = %v", raw, err)
		}
	})

	t.Run("unsupported list scope", func(t *testing.T) {
		scope := factoryapi.FactorySessionListScope("workspace")
		raw, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &scope})
		if err != nil || raw.Scope != factorysessionexecution.SessionListScope("workspace") {
			t.Fatalf("raw list request = %#v, error = %v", raw, err)
		}
	})

	t.Run("negative event reconnect sequence", func(t *testing.T) {
		sequence := factoryapi.AfterSequence(-1)
		raw, err := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{AfterSequence: &sequence})
		if err != nil || raw.AfterSequence == nil || *raw.AfterSequence != -1 {
			t.Fatalf("raw reconnect request = %#v, error = %v", raw, err)
		}
	})

	t.Run("event reconnect cursor maps after event id and sequence", func(t *testing.T) {
		eventID := factoryapi.AfterEventId("evt-session-started-001")
		sequence := factoryapi.AfterSequence(2)
		mapped, err := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{
			AfterEventId:  &eventID,
			AfterSequence: &sequence,
		})

		if err != nil {
			t.Fatalf("EventReconnectRequestFromAPI: %v", err)
		}
		if mapped.AfterEventID != "evt-session-started-001" {
			t.Fatalf("AfterEventID = %q, want evt-session-started-001", mapped.AfterEventID)
		}
		if mapped.AfterSequence == nil || *mapped.AfterSequence != 2 {
			t.Fatalf("AfterSequence = %#v, want 2", mapped.AfterSequence)
		}
	})

	t.Run("malformed artifact retrieval ref", func(t *testing.T) {
		_, err := factorysession.ArtifactSummaryFromAPI(factoryapi.FactorySessionArtifactSummary{
			Id:         "art-001",
			Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			RetrievalRef: &factoryapi.FactorySessionArtifactRetrievalRef{
				Href: "file:///tmp/secret.json",
			},
		})
		assertRequestValidationError(t, err)
	})
}

func TestControlErrorToAPI_MapsTerminalSessionOutcome(t *testing.T) {
	mapped := factorysession.ControlErrorToAPI("dur-sess-js-success-002", &factorysessionexecution.ControlError{
		Operation: "RETRY_DISPATCH",
		Outcome:   factorysessionexecution.LifecycleControlOutcomeTerminalSession,
		Status:    factorysessionexecution.LifecycleStatusSucceeded,
		Message:   "session is terminal",
		Links: factorysessionexecution.LifecycleControlLinks{
			Session: "/factory-sessions/dur-sess-js-success-002",
			Results: "/factory-sessions/dur-sess-js-success-002/results",
		},
	})
	if mapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", mapped.Outcome)
	}
	if mapped.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", mapped.Status)
	}
	if mapped.Detail == nil || *mapped.Detail != "session is terminal" {
		t.Fatalf("detail = %#v", mapped.Detail)
	}
	if mapped.Links == nil || mapped.Links.Results == nil || *mapped.Links.Results == "" {
		t.Fatalf("links = %#v, want inspection links", mapped.Links)
	}
}

func TestControlErrorToAPI_MapsConflictOutcome(t *testing.T) {
	mapped := factorysession.ControlErrorToAPI("dur-sess-js-run-n-001", &factorysessionexecution.ControlError{
		Operation: factorysessionexecution.LifecycleControlResume,
		Outcome:   "CONFLICT",
		Status:    factorysessionexecution.LifecycleStatusPaused,
		Message:   "control requestId was reused with a different operation or target",
		Links: factorysessionexecution.LifecycleControlLinks{
			Session: "/factory-sessions/dur-sess-js-run-n-001",
		},
	})
	if mapped.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", mapped.Operation)
	}
	if mapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeConflict {
		t.Fatalf("outcome = %q, want CONFLICT", mapped.Outcome)
	}
	if mapped.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", mapped.Status)
	}
	if mapped.Links == nil || mapped.Links.Session == nil || *mapped.Links.Session == "" {
		t.Fatalf("links = %#v, want session inspection link", mapped.Links)
	}
}

func TestOrchestratorOverrideFromAPI_PreservesKindAndPayload(t *testing.T) {
	override, err := factorysession.OrchestratorOverrideFromAPI(factoryapi.FactoryOrchestrator{
		Kind: factoryapi.JAVASCRIPT,
	})
	if err != nil {
		t.Fatalf("OrchestratorOverrideFromAPI: %v", err)
	}
	if override.Kind != "JAVASCRIPT" {
		t.Fatalf("kind = %q, want JAVASCRIPT", override.Kind)
	}
	var decoded map[string]any
	if err := json.Unmarshal(override.Raw, &decoded); err != nil {
		t.Fatalf("decode orchestrator raw: %v", err)
	}
	if decoded["kind"] != "JAVASCRIPT" {
		t.Fatalf("raw kind = %#v", decoded["kind"])
	}
}

func assertAsyncStartMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionExecutionResponse
	decodeFixture(t, fixture, &apiValue, label+" async response")
	assertMapperRoundTrip(t, label+" async response", apiValue, func(value factoryapi.FactorySessionExecutionResponse) factoryapi.FactorySessionExecutionResponse {
		return factorysession.AsyncStartResponseToAPI(factorysession.AsyncStartResultFromAPI(value))
	}, assertAsyncStartFieldsPreserved)
}

func assertSessionReadMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionDurableReadModel
	decodeFixture(t, fixture, &apiValue, label+" session")
	assertMapperRoundTrip(t, label+" session", apiValue, func(value factoryapi.FactorySessionDurableReadModel) factoryapi.FactorySessionDurableReadModel {
		return factorysession.SessionReadResponseToAPI(factorysession.SessionReadResultFromAPI(value))
	}, assertSessionReadFieldsPreserved)
}

func assertListSummaryMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionDurableSummary
	decodeFixture(t, fixture, &apiValue, label+" list summary")
	assertMapperRoundTrip(t, label+" list summary", apiValue, func(value factoryapi.FactorySessionDurableSummary) factoryapi.FactorySessionDurableSummary {
		return factorysession.DurableSessionSummaryToAPI(factorysession.DurableSessionListSummaryFromAPI(value))
	}, assertListSummaryFieldsPreserved)
}

func assertResultMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionResult
	decodeFixture(t, fixture, &apiValue, label+" result")
	assertMapperRoundTrip(t, label+" result", apiValue, func(value factoryapi.FactorySessionResult) factoryapi.FactorySessionResult {
		return factorysession.ResultResponseToAPI(factorysession.ResultReadResultFromAPI(value))
	}, assertResultFieldsPreserved)
}

func assertDispatchListMapperRoundTrip(t *testing.T, label string, scenario map[string]any, dispatches []any) {
	t.Helper()
	sessionID := stringValue(scenario, "sessionId")
	if sessionID == "" {
		if session, ok := scenario["session"].(map[string]any); ok {
			sessionID = stringValue(session, "sessionId")
		}
	}
	listResponse := map[string]any{
		"sessionId":  sessionID,
		"dispatches": dispatches,
	}
	var apiValue factoryapi.ListFactorySessionDispatchesResponse
	decodeFixture(t, listResponse, &apiValue, label+" dispatch list")
	assertMapperRoundTrip(t, label+" dispatch list", apiValue, func(value factoryapi.ListFactorySessionDispatchesResponse) factoryapi.ListFactorySessionDispatchesResponse {
		domain := factorysessionexecution.ListDispatchesResult{SessionID: value.SessionId}
		for _, dispatch := range value.Dispatches {
			domain.Dispatches = append(domain.Dispatches, factorysession.DispatchSummaryFromAPI(dispatch))
		}
		return factorysession.ListDispatchesResponseToAPI(domain)
	}, assertDispatchListFieldsPreserved)
}

func assertDispatchDetailMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactoryDispatch
	decodeFixture(t, fixture, &apiValue, label+" dispatch detail")
	assertMapperRoundTrip(t, label+" dispatch detail", apiValue, func(value factoryapi.FactoryDispatch) factoryapi.FactoryDispatch {
		return factorysession.DispatchDetailResponseToAPI(factorysession.DispatchDetailFromAPI(value))
	}, assertDispatchDetailFieldsPreserved)
}

func assertArtifactListMapperRoundTrip(t *testing.T, label string, scenario map[string]any, artifacts []any) {
	t.Helper()
	sessionID := stringValue(scenario, "sessionId")
	if sessionID == "" {
		if session, ok := scenario["session"].(map[string]any); ok {
			sessionID = stringValue(session, "sessionId")
		}
	}
	listResponse := map[string]any{
		"sessionId": sessionID,
		"artifacts": artifacts,
	}
	var apiValue factoryapi.ListFactorySessionArtifactsResponse
	decodeFixture(t, listResponse, &apiValue, label+" artifact list")
	assertMapperRoundTrip(t, label+" artifact list", apiValue, func(value factoryapi.ListFactorySessionArtifactsResponse) factoryapi.ListFactorySessionArtifactsResponse {
		domain := factorysessionexecution.ListArtifactsResult{SessionID: value.SessionId}
		for _, artifact := range value.Artifacts {
			summary, err := factorysession.ArtifactSummaryFromAPI(artifact)
			if err != nil {
				t.Fatalf("%s artifact summary from API: %v", label, err)
			}
			domain.Artifacts = append(domain.Artifacts, summary)
		}
		return factorysession.ListArtifactsResponseToAPI(domain)
	}, assertArtifactListFieldsPreserved)
}

func assertArtifactDetailMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionArtifactDetail
	decodeFixture(t, fixture, &apiValue, label+" artifact detail")
	assertMapperRoundTrip(t, label+" artifact detail", apiValue, func(value factoryapi.FactorySessionArtifactDetail) factoryapi.FactorySessionArtifactDetail {
		domain, err := factorysession.ArtifactDetailFromAPI(value)
		if err != nil {
			t.Fatalf("%s artifact detail from API: %v", label, err)
		}
		return factorysession.ArtifactDetailResponseToAPI(domain)
	}, assertArtifactDetailFieldsPreserved)
}

func assertLifecycleControlMapperRoundTrip(t *testing.T, label string, fixture map[string]any) {
	t.Helper()
	var apiValue factoryapi.FactorySessionLifecycleControlResponse
	decodeFixture(t, fixture, &apiValue, label+" lifecycle control")
	assertMapperRoundTrip(t, label+" lifecycle control", apiValue, func(value factoryapi.FactorySessionLifecycleControlResponse) factoryapi.FactorySessionLifecycleControlResponse {
		return factorysession.LifecycleControlResponseToAPI(factorysession.LifecycleControlResultFromAPI(value))
	}, assertLifecycleControlFieldsPreserved)
}

func assertFixtureEventsMapperRoundTrip(t *testing.T, label string, events []any) {
	t.Helper()
	rawEvents := make([]json.RawMessage, 0, len(events))
	for index, item := range events {
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("%s event[%d] marshal: %v", label, index, err)
		}
		rawEvents = append(rawEvents, encoded)
	}
	mapped := factorysession.EventReadResponseToAPI(factorysessionexecution.EventReadResult{
		Events: rawEvents,
	})
	if len(mapped) != len(events) {
		t.Fatalf("%s mapped events = %d, want %d", label, len(mapped), len(events))
	}
	for index, event := range mapped {
		if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
			t.Fatalf("%s event[%d] schemaVersion = %q, want agent-factory.event.v1", label, index, event.SchemaVersion)
		}
		if event.Id == "" || event.Type == "" {
			t.Fatalf("%s event[%d] = %#v, want canonical id and type", label, index, event)
		}
	}
}

func assertMapperRoundTrip[T any](t *testing.T, label string, apiValue T, roundTrip func(T) T, assertFields func(*testing.T, map[string]any, T)) {
	t.Helper()
	first := roundTrip(apiValue)
	second := roundTrip(first)
	if !jsonValuesEqual(normalizeJSONValue(first), normalizeJSONValue(second)) {
		firstJSON, _ := json.Marshal(first)
		secondJSON, _ := json.Marshal(second)
		t.Fatalf("%s round-trip is not idempotent\nfirst:  %s\nsecond: %s", label, firstJSON, secondJSON)
	}
	fixture := mustFixtureMap(t, apiValue)
	assertFields(t, fixture, first)
}

func assertAsyncStartFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionExecutionResponse) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	if string(mapped.Status) != stringValue(fixture, "status") {
		t.Fatalf("status = %q, want %q", mapped.Status, stringValue(fixture, "status"))
	}
	if string(mapped.OrchestratorKind) != stringValue(fixture, "orchestrator") && string(mapped.OrchestratorKind) != stringValue(fixture, "orchestratorKind") {
		t.Fatalf("orchestratorKind = %q", mapped.OrchestratorKind)
	}
}

func assertSessionReadFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionDurableReadModel) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	if string(mapped.Status) != stringValue(fixture, "status") {
		t.Fatalf("status = %q, want %q", mapped.Status, stringValue(fixture, "status"))
	}
	if mapped.Phase != nil && *mapped.Phase != stringValue(fixture, "phase") {
		t.Fatalf("phase = %q, want %q", *mapped.Phase, stringValue(fixture, "phase"))
	}
	if mapped.Usage == nil || mapped.Usage.Resources == nil {
		t.Fatal("usage.resources missing from durable session read projection")
	}
	if budgets, ok := fixture["budgets"].(map[string]any); ok {
		if mapped.Budgets == nil || mapped.Budgets.MaxAgents == nil {
			t.Fatal("budgets.maxAgents missing from durable session read projection")
		}
		if int(*mapped.Budgets.MaxAgents) != intValue(budgets, "maxAgents") {
			t.Fatalf("budgets.maxAgents = %d, want %d", *mapped.Budgets.MaxAgents, intValue(budgets, "maxAgents"))
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this assertion keeps durable list summary fixture field checks together on one contract test seam.
func assertListSummaryFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionDurableSummary) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	if string(mapped.Status) != stringValue(fixture, "status") {
		t.Fatalf("status = %q, want %q", mapped.Status, stringValue(fixture, "status"))
	}
	if phase := stringValue(fixture, "phase"); phase != "" {
		if mapped.Phase == nil || *mapped.Phase != phase {
			t.Fatalf("phase = %#v, want %q", mapped.Phase, phase)
		}
	}
	if progress, ok := fixture["progress"].(map[string]any); ok {
		if mapped.Progress == nil {
			t.Fatal("progress lost during round trip")
		}
		if want := intValue(progress, "totalDispatches"); want > 0 {
			if mapped.Progress.TotalDispatches == nil || int(*mapped.Progress.TotalDispatches) != want {
				t.Fatalf("totalDispatches = %#v, want %d", mapped.Progress.TotalDispatches, want)
			}
		}
	}
	if resultSummary, ok := fixture["resultSummary"].(map[string]any); ok {
		if mapped.ResultSummary == nil {
			t.Fatal("resultSummary lost during round trip")
		}
		if string(mapped.ResultSummary.ResultStatus) != stringValue(resultSummary, "resultStatus") {
			t.Fatalf("resultStatus = %q, want %q", mapped.ResultSummary.ResultStatus, stringValue(resultSummary, "resultStatus"))
		}
	}
	if artifactCount, ok := fixture["artifactCount"].(float64); ok && int(artifactCount) > 0 {
		if mapped.ArtifactCount == nil || *mapped.ArtifactCount != int(artifactCount) {
			t.Fatalf("artifactCount = %#v, want %d", mapped.ArtifactCount, int(artifactCount))
		}
	}
	if recoverable, ok := fixture["recoverable"].(bool); ok && recoverable {
		if mapped.Recoverable == nil || !*mapped.Recoverable {
			t.Fatal("recoverable lost during round trip")
		}
	}
	if actions, ok := fixture["actions"].(map[string]any); ok {
		if mapped.Actions == nil {
			t.Fatal("actions lost during round trip")
		}
		if want, ok := actions["canPause"].(bool); ok && want {
			if mapped.Actions.CanPause == nil || !*mapped.Actions.CanPause {
				t.Fatal("canPause lost during round trip")
			}
		}
		if want, ok := actions["canRetryDispatch"].(bool); ok && !want {
			if mapped.Actions.CanRetryDispatch != nil && *mapped.Actions.CanRetryDispatch {
				t.Fatal("canRetryDispatch should remain false")
			}
		}
	}
}

func assertResultFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionResult) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	if string(mapped.ResultStatus) != stringValue(fixture, "resultStatus") {
		t.Fatalf("resultStatus = %q, want %q", mapped.ResultStatus, stringValue(fixture, "resultStatus"))
	}
	if mapped.PrimaryResult != nil && fixture["primaryResult"] == nil {
		t.Fatal("primaryResult lost during round trip")
	}
}

func assertDispatchListFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.ListFactorySessionDispatchesResponse) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	fixtureRows, ok := fixture["dispatches"].([]any)
	if !ok {
		return
	}
	if len(mapped.Dispatches) != len(fixtureRows) {
		t.Fatalf("dispatch count = %d, want %d", len(mapped.Dispatches), len(fixtureRows))
	}
	for index, row := range fixtureRows {
		fixtureRow, ok := row.(map[string]any)
		if !ok {
			continue
		}
		refs, ok := fixtureRow["providerSessionRefs"].([]any)
		if !ok || len(refs) == 0 {
			continue
		}
		if mapped.Dispatches[index].ProviderSessionRefs == nil || len(*mapped.Dispatches[index].ProviderSessionRefs) != len(refs) {
			t.Fatalf("dispatch[%d] providerSessionRefs = %#v, want %d refs", index, mapped.Dispatches[index].ProviderSessionRefs, len(refs))
		}
		if fixtureJavaScript, ok := fixtureRow["javascript"].(map[string]any); ok && fixtureJavaScript["executionMode"] != nil {
			if mapped.Dispatches[index].Javascript == nil || mapped.Dispatches[index].Javascript.ExecutionMode == nil || *mapped.Dispatches[index].Javascript.ExecutionMode != stringValue(fixtureJavaScript, "executionMode") {
				t.Fatalf("dispatch[%d] javascript.executionMode = %#v, want %q", index, mapped.Dispatches[index].Javascript, stringValue(fixtureJavaScript, "executionMode"))
			}
		}
	}
}

func assertDispatchDetailFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactoryDispatch) {
	t.Helper()
	if mapped.Id != stringValue(fixture, "id") {
		t.Fatalf("id = %q, want %q", mapped.Id, stringValue(fixture, "id"))
	}
	if string(mapped.Status) != stringValue(fixture, "status") {
		t.Fatalf("status = %q, want %q", mapped.Status, stringValue(fixture, "status"))
	}
	if fixtureJavaScript, ok := fixture["javascript"].(map[string]any); ok && fixtureJavaScript["executionMode"] != nil {
		if mapped.Javascript == nil || mapped.Javascript.ExecutionMode == nil || *mapped.Javascript.ExecutionMode != stringValue(fixtureJavaScript, "executionMode") {
			t.Fatalf("javascript.executionMode = %#v, want %q", mapped.Javascript, stringValue(fixtureJavaScript, "executionMode"))
		}
	}
	if fixtureTransitions, ok := fixture["statusTransitions"].([]any); ok {
		if mapped.StatusTransitions == nil || len(*mapped.StatusTransitions) != len(fixtureTransitions) {
			t.Fatalf("statusTransitions = %#v, want %d entries", mapped.StatusTransitions, len(fixtureTransitions))
		}
	}
}

func assertArtifactListFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.ListFactorySessionArtifactsResponse) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	fixtureRows, ok := fixture["artifacts"].([]any)
	if !ok {
		return
	}
	if len(mapped.Artifacts) != len(fixtureRows) {
		t.Fatalf("artifact count = %d, want %d", len(mapped.Artifacts), len(fixtureRows))
	}
}

func assertArtifactDetailFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionArtifactDetail) {
	t.Helper()
	if mapped.Id != stringValue(fixture, "id") {
		t.Fatalf("id = %q, want %q", mapped.Id, stringValue(fixture, "id"))
	}
	if mapped.Content != nil && fixture["content"] == nil {
		t.Fatal("content lost during round trip")
	}
	fixtureMetadata, hasFixtureMetadata := fixture["captureMetadata"].(map[string]any)
	if !hasFixtureMetadata {
		return
	}
	if mapped.CaptureMetadata == nil {
		t.Fatal("captureMetadata lost during round trip")
	}
	if fixtureMetadata["sourceDispatchId"] != nil {
		if mapped.CaptureMetadata.SourceDispatchId == nil || *mapped.CaptureMetadata.SourceDispatchId != stringValue(fixtureMetadata, "sourceDispatchId") {
			t.Fatalf("captureMetadata.sourceDispatchId = %v, want %q", mapped.CaptureMetadata.SourceDispatchId, stringValue(fixtureMetadata, "sourceDispatchId"))
		}
	}
	if fixtureMetadata["mimeType"] != nil {
		if mapped.CaptureMetadata.MimeType == nil || *mapped.CaptureMetadata.MimeType != stringValue(fixtureMetadata, "mimeType") {
			t.Fatalf("captureMetadata.mimeType = %v, want %q", mapped.CaptureMetadata.MimeType, stringValue(fixtureMetadata, "mimeType"))
		}
	}
	if fixtureMetadata["capturedAt"] != nil {
		if mapped.CaptureMetadata.CapturedAt == nil {
			t.Fatal("captureMetadata.capturedAt lost during round trip")
		}
	}
}

func assertLifecycleControlFieldsPreserved(t *testing.T, fixture map[string]any, mapped factoryapi.FactorySessionLifecycleControlResponse) {
	t.Helper()
	if mapped.SessionId != stringValue(fixture, "sessionId") {
		t.Fatalf("sessionId = %q, want %q", mapped.SessionId, stringValue(fixture, "sessionId"))
	}
	if string(mapped.Operation) != stringValue(fixture, "operation") {
		t.Fatalf("operation = %q, want %q", mapped.Operation, stringValue(fixture, "operation"))
	}
	if string(mapped.Outcome) != stringValue(fixture, "outcome") {
		t.Fatalf("outcome = %q, want %q", mapped.Outcome, stringValue(fixture, "outcome"))
	}
	if status := stringValue(fixture, "status"); status != "" && string(mapped.Status) != status {
		t.Fatalf("status = %q, want %q", mapped.Status, status)
	}
}

func decodeFixture(t *testing.T, fixture map[string]any, target any, label string) {
	t.Helper()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("%s marshal fixture: %v", label, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("%s decode fixture: %v", label, err)
	}
}

func mustFixtureMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal mapped value: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode mapped value: %v", err)
	}
	return out
}

func fixtureDispatchRows(dispatches []factoryapi.FactorySessionDispatchSummary) []any {
	rows := make([]any, 0, len(dispatches))
	for _, dispatch := range dispatches {
		raw, err := json.Marshal(dispatch)
		if err != nil {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err == nil {
			rows = append(rows, row)
		}
	}
	return rows
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeJSONValue(item))
		}
		return out
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed)
		}
		return typed
	default:
		return typed
	}
}

func jsonValuesEqual(left, right any) bool {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func TestRequestMappers_ForwardDecodedFieldsOnlyThroughInjectedPreparation(t *testing.T) {
	t.Parallel()

	requestID, reason := " control-1 ", " reason "
	previewID := " preview-1 "
	force := true
	scope := factoryapi.FactorySessionListScopePersisted
	mode := factoryapi.FactorySessionResultModePartial
	include := true
	afterEventID := factoryapi.AfterEventId(" event-1 ")
	afterSequence := factoryapi.AfterSequence(7)

	start, _ := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
		RequestId: " request-1 ",
		Source:    factoryapi.FactorySessionExecutionSource{Kind: factoryapi.FactorySessionExecutionSourceKindFactoryId, FactoryId: strPtr(" factory-1 ")},
	})
	control, _ := factorysession.ControlRequestFromAPI(factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID, Reason: &reason})
	approve, _ := factorysession.ApproveRequestFromAPI(factoryapi.FactorySessionApproveRequest{RequestId: &requestID, Reason: &reason, ApprovalPreviewId: &previewID})
	retry, _ := factorysession.RetryDispatchRequestFromAPI(factoryapi.FactorySessionRetryDispatchRequest{RequestId: &requestID, Reason: &reason, DispatchId: " dispatch-1 ", ForceNewAttempt: &force})
	interrupt, _ := factorysession.InterruptDispatchRequestFromAPI(factoryapi.FactorySessionInterruptDispatchRequest{RequestId: &requestID, Reason: &reason, DispatchId: " dispatch-2 "})
	list, _ := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &scope})
	result, _ := factorysession.ResultRequestFromAPI(factoryapi.GetFactorySessionResultsParams{Mode: &mode, IncludeArtifacts: &include})
	events, _ := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{AfterEventId: &afterEventID, AfterSequence: &afterSequence})

	if start.RequestID != " request-1 " || start.Source.FactoryID != " factory-1 " {
		t.Fatalf("start mapping = %#v", start)
	}
	if control.RequestID != requestID || control.Reason != reason || approve.ApprovalPreviewID != previewID {
		t.Fatalf("control/approve mapping = %#v %#v", control, approve)
	}
	if retry.DispatchID != " dispatch-1 " || !retry.ForceNewAttempt || interrupt.DispatchID != " dispatch-2 " {
		t.Fatalf("dispatch mapping = %#v %#v", retry, interrupt)
	}
	if list.Scope != factorysessionexecution.SessionListScopePersisted || result.Mode != factorysessionexecution.ResultModePartial || !result.IncludeArtifacts {
		t.Fatalf("list/result mapping = %#v %#v", list, result)
	}
	if events.AfterEventID != " event-1 " || events.AfterSequence == nil || *events.AfterSequence != 7 {
		t.Fatalf("event mapping = %#v", events)
	}
}

func assertRequestValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *apisurface.RequestValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want RequestValidationError", err)
	}
}
