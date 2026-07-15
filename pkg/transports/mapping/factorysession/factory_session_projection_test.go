package factorysession_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestResultResponseToAPI_MapsProjectionFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)

	notReady := findScenario(t, catalog, "petri-running-one-dispatch")
	notReadyMapped := factorysession.ResultResponseToAPI(resultFromFixture(notReady["result"].(map[string]any)))
	if notReadyMapped.ResultStatus != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("not-ready status = %q", notReadyMapped.ResultStatus)
	}
	if notReadyMapped.Availability == nil || notReadyMapped.Availability.Reason == nil {
		t.Fatal("not-ready availability reason missing")
	}

	partial := findScenario(t, catalog, "javascript-running-n-dispatch")
	partialMapped := factorysession.ResultResponseToAPI(resultFromFixture(partial["result"].(map[string]any)))
	if partialMapped.ResultStatus != factoryapi.FactorySessionResultStatusPartial {
		t.Fatalf("partial status = %q", partialMapped.ResultStatus)
	}
	if partialMapped.PrimaryResult == nil || len(*partialMapped.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}

	final := findScenario(t, catalog, "petri-succeeded-one-dispatch")
	finalMapped := factorysession.ResultResponseToAPI(resultFromFixture(final["result"].(map[string]any)))
	if finalMapped.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("final status = %q", finalMapped.ResultStatus)
	}
	if finalMapped.ArtifactRefs == nil || len(*finalMapped.ArtifactRefs) != 1 {
		t.Fatalf("final artifactRefs = %#v", finalMapped.ArtifactRefs)
	}

	failedPartial := findScenario(t, catalog, "javascript-failed-with-partial")
	failedMapped := factorysession.ResultResponseToAPI(resultFromFixture(failedPartial["result"].(map[string]any)))
	if failedMapped.ResultStatus != factoryapi.FactorySessionResultStatusFailedWithPartial {
		t.Fatalf("failed-with-partial status = %q", failedMapped.ResultStatus)
	}
	if failedMapped.FailureDetail == nil || failedMapped.PartialResultAvailable == nil || !*failedMapped.PartialResultAvailable {
		t.Fatal("failed-with-partial failure detail missing")
	}

	unavailable := findScenario(t, catalog, "petri-canceled")
	unavailableMapped := factorysession.ResultResponseToAPI(resultFromFixture(unavailable["result"].(map[string]any)))
	if unavailableMapped.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("unavailable status = %q", unavailableMapped.ResultStatus)
	}
}

func TestListDispatchesResponseToAPI_EmitsEmptyTypedList(t *testing.T) {
	mapped := factorysession.ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: "dur-sess-js-awaiting-001",
	})
	if mapped.SessionId != "dur-sess-js-awaiting-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-awaiting-001", mapped.SessionId)
	}
	if mapped.Dispatches == nil {
		t.Fatal("dispatches = nil, want typed empty slice")
	}
	if len(mapped.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want empty slice", mapped.Dispatches)
	}
}

func TestListArtifactsResponseToAPI_EmitsEmptyTypedList(t *testing.T) {
	mapped := factorysession.ListArtifactsResponseToAPI(factorysessionexecution.ListArtifactsResult{
		SessionID: "dur-sess-js-awaiting-001",
	})
	if mapped.SessionId != "dur-sess-js-awaiting-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-awaiting-001", mapped.SessionId)
	}
	if mapped.Artifacts == nil {
		t.Fatal("artifacts = nil, want typed empty slice")
	}
	if len(mapped.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want empty slice", mapped.Artifacts)
	}
}

func TestArtifactDetailResponseToAPI_FallsBackToRetrievalRefAsContentRef(t *testing.T) {
	mapped := factorysession.ArtifactDetailResponseToAPI(factorysessionexecution.ArtifactDetail{
		ArtifactSummary: factorysessionexecution.ArtifactSummary{
			ID:         "artifact-1",
			Kind:       "LOG",
			Visibility: "PUBLIC",
			RetrievalRef: &factorysessionexecution.ArtifactRetrievalRef{
				Href:   "/factory-sessions/dur-sess-runtime-001/artifacts/artifact-1",
				Method: "GET",
			},
		},
		SessionID: "dur-sess-runtime-001",
	})
	if mapped.ContentRef == nil {
		t.Fatal("contentRef = nil, want summary retrievalRef fallback")
	}
	if mapped.ContentRef.Href != "/factory-sessions/dur-sess-runtime-001/artifacts/artifact-1" {
		t.Fatalf("contentRef.href = %q, want API-relative retrieval ref", mapped.ContentRef.Href)
	}
}

func TestListDispatchesResponseToAPI_MapsQueuedRunningAndFailedFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)

	running := findScenario(t, catalog, "javascript-running-n-dispatch")
	runningMapped := factorysession.ListDispatchesResponseToAPI(listDispatchesFromFixture(running))
	if len(runningMapped.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 3", len(runningMapped.Dispatches))
	}
	if runningMapped.Dispatches[2].Status != factoryapi.FactoryDispatchStatusQUEUED {
		t.Fatalf("third dispatch status = %q, want QUEUED", runningMapped.Dispatches[2].Status)
	}

	failed := findScenario(t, catalog, "javascript-failed-with-partial")
	failedMapped := factorysession.ListDispatchesResponseToAPI(listDispatchesFromFixture(failed))
	if len(failedMapped.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(failedMapped.Dispatches))
	}
	if failedMapped.Dispatches[1].FailureDetail == nil {
		t.Fatal("failed dispatch failureDetail missing")
	}
	if failedMapped.Dispatches[1].Javascript == nil || failedMapped.Dispatches[1].Javascript.ExecutionMode == nil || *failedMapped.Dispatches[1].Javascript.ExecutionMode != "live" {
		t.Fatalf("failed dispatch javascript = %#v", failedMapped.Dispatches[1].Javascript)
	}
}

func TestDispatchDetailResponseToAPI_MapsPetriAndJavaScriptFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)

	petri := findScenario(t, catalog, "petri-succeeded-one-dispatch")
	petriMapped := factorysession.DispatchDetailResponseToAPI(dispatchDetailFromFixture(petri["dispatchDetail"].(map[string]any)))
	if petriMapped.Petri == nil || petriMapped.Petri.TransitionId != "transition-plan-task" {
		t.Fatalf("petri projection = %#v", petriMapped.Petri)
	}

	javascript := findScenario(t, catalog, "javascript-running-n-dispatch")
	javascriptMapped := factorysession.DispatchDetailResponseToAPI(dispatchDetailFromFixture(javascript["dispatchDetail"].(map[string]any)))
	if javascriptMapped.Javascript == nil || javascriptMapped.Javascript.TaskKind != factoryapi.FactoryDispatchJavaScriptTaskKindVERIFY {
		t.Fatalf("javascript projection = %#v", javascriptMapped.Javascript)
	}
	if javascriptMapped.Javascript.ExecutionMode == nil || *javascriptMapped.Javascript.ExecutionMode != "live" {
		t.Fatalf("javascript executionMode = %#v", javascriptMapped.Javascript)
	}
}

func TestArtifactProjectionToAPI_MapsSummaryAndDetailFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)

	succeeded := findScenario(t, catalog, "petri-succeeded-one-dispatch")
	listMapped := factorysession.ListArtifactsResponseToAPI(listArtifactsFromFixture(succeeded))
	if len(listMapped.Artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(listMapped.Artifacts))
	}
	if listMapped.Artifacts[0].RetrievalRef == nil || listMapped.Artifacts[0].RetrievalRef.Href == "" {
		t.Fatal("artifact retrievalRef missing")
	}

	detailMapped := factorysession.ArtifactDetailResponseToAPI(artifactDetailFromFixture(succeeded["artifactDetail"].(map[string]any)))
	if detailMapped.Content == nil || len(*detailMapped.Content) == 0 {
		t.Fatal("artifact detail content missing")
	}
}

func TestDispatchAndArtifactProjectionToAPI_MapsUsageWarningsAndRedactionCounts(t *testing.T) {
	t.Run("dispatch detail", testDispatchProjectionMapsUsageWarningsAndFailure)
	t.Run("artifact detail", testArtifactProjectionMapsRedactionCountsAndMetadata)
}

func testDispatchProjectionMapsUsageWarningsAndFailure(t *testing.T) {
	t.Helper()

	dispatchMapped := factorysession.DispatchDetailResponseToAPI(factorysessionexecution.DispatchDetail{
		DispatchSummary: factorysessionexecution.DispatchSummary{
			ID:           "disp-1",
			Status:       factorysessionexecution.DispatchStatusFailed,
			DispatchKind: "JAVASCRIPT_AGENT",
			Usage: &factorysessionexecution.DispatchUsage{
				InputTokens:    11,
				OutputTokens:   7,
				TotalTokens:    18,
				DurationMillis: 250,
				CostUSD:        1.25,
				RetryCount:     2,
			},
			Warnings: []factorysessionexecution.DispatchWarning{
				{Code: "RATE_LIMIT", Message: " retried once "},
			},
			FailureDetail: &factorysessionexecution.DispatchFailureDetail{
				Reason:  " TEMPORARY ",
				Message: " provider unavailable ",
			},
		},
		SessionID:         "dur-sess-1",
		OrchestratorKind:  "JAVASCRIPT",
		StatusTransitions: []factorysessionexecution.DispatchStatus{factorysessionexecution.DispatchStatusQueued, factorysessionexecution.DispatchStatusFailed},
	})
	if dispatchMapped.Usage == nil || dispatchMapped.Usage.TotalTokens == nil || *dispatchMapped.Usage.TotalTokens != 18 {
		t.Fatalf("dispatch usage = %#v, want populated usage fields", dispatchMapped.Usage)
	}
	if dispatchMapped.Warnings == nil || len(*dispatchMapped.Warnings) != 1 || (*dispatchMapped.Warnings)[0].Message != "retried once" {
		t.Fatalf("dispatch warnings = %#v, want trimmed warning", dispatchMapped.Warnings)
	}
	if dispatchMapped.FailureDetail == nil || dispatchMapped.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown {
		t.Fatalf("dispatch failure detail = %#v, want trimmed failure", dispatchMapped.FailureDetail)
	}
	if dispatchMapped.StatusTransitions == nil || len(*dispatchMapped.StatusTransitions) != 2 {
		t.Fatalf("dispatch statusTransitions = %#v, want queued/failed history", dispatchMapped.StatusTransitions)
	}
}

func testArtifactProjectionMapsRedactionCountsAndMetadata(t *testing.T) {
	t.Helper()

	createdAt := mustParseTime(t, "2026-06-08T10:05:00Z")
	artifactMapped := factorysession.ArtifactDetailResponseToAPI(factorysessionexecution.ArtifactDetail{
		ArtifactSummary: factorysessionexecution.ArtifactSummary{
			ID:         "art-1",
			Kind:       "LOG",
			Visibility: "PUBLIC",
			CreatedAt:  &createdAt,
			RedactionCounts: &factorysessionexecution.ArtifactRedactionCounts{
				Paths:   1,
				Secrets: 2,
				Tokens:  3,
			},
		},
		SessionID: "dur-sess-1",
		CaptureMetadata: map[string]any{
			"capturedAt":       createdAt.Format(time.RFC3339),
			"sourceDispatchId": []byte("disp-1"),
			"mimeType":         " text/plain ",
		},
	})
	if artifactMapped.RedactionCounts == nil || artifactMapped.RedactionCounts.Paths == nil || *artifactMapped.RedactionCounts.Paths != 1 {
		t.Fatalf("artifact redaction counts = %#v, want populated counts", artifactMapped.RedactionCounts)
	}
	if artifactMapped.CaptureMetadata == nil || artifactMapped.CaptureMetadata.SourceDispatchId == nil || *artifactMapped.CaptureMetadata.SourceDispatchId != "disp-1" {
		t.Fatalf("artifact capture metadata = %#v, want projected sourceDispatchId", artifactMapped.CaptureMetadata)
	}
}

func TestResultRequestFromAPI_MapsModeAndIncludeArtifacts(t *testing.T) {
	mode := factoryapi.FactorySessionResultModePartial
	include := true
	req, err := factorysession.ResultRequestFromAPI(factoryapi.GetFactorySessionResultsParams{
		Mode:             &mode,
		IncludeArtifacts: &include,
	})
	if err != nil {
		t.Fatalf("ResultRequestFromAPI: %v", err)
	}
	if req.Mode != factorysessionexecution.ResultModePartial || !req.IncludeArtifacts {
		t.Fatalf("request = %#v", req)
	}
}

func TestResultRequestFromAPI_RejectsInvalidMode(t *testing.T) {
	mode := factoryapi.FactorySessionResultMode("invalid")
	_, err := factorysession.ResultRequestFromAPI(factoryapi.GetFactorySessionResultsParams{Mode: &mode})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestEventAndProjectionResponses_HandleEmptyAndInvalidBranches(t *testing.T) {
	t.Run("reconnect request", testEventReconnectRequestBranches)
	t.Run("event read response", testEventReadResponseBranches)
	t.Run("factory event stream", testFactoryEventStreamBranches)
}

func testEventReconnectRequestBranches(t *testing.T) {
	t.Helper()

	reconnect, err := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{})
	if err != nil {
		t.Fatalf("EventReconnectRequestFromAPI empty: %v", err)
	}
	if reconnect.AfterEventID != "" || reconnect.AfterSequence != nil {
		t.Fatalf("reconnect = %#v", reconnect)
	}

	afterEventID := factoryapi.AfterEventId("event-2")
	afterSequence := factoryapi.AfterSequence(3)
	reconnect, err = factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{
		AfterEventId:  &afterEventID,
		AfterSequence: &afterSequence,
	})
	if err != nil {
		t.Fatalf("EventReconnectRequestFromAPI populated: %v", err)
	}
	if reconnect.AfterEventID != "event-2" || reconnect.AfterSequence == nil || *reconnect.AfterSequence != 3 {
		t.Fatalf("reconnect populated = %#v", reconnect)
	}
}

func testEventReadResponseBranches(t *testing.T) {
	t.Helper()

	if events := factorysession.EventReadResponseToAPI(factorysessionexecution.EventReadResult{}); events != nil {
		t.Fatalf("empty events = %#v, want nil", events)
	}

	validEvent := []byte(`{"id":"event-1","type":"factory.session.started","sequence":1}`)
	events := factorysession.EventReadResponseToAPI(factorysessionexecution.EventReadResult{
		Events: []json.RawMessage{
			validEvent,
			json.RawMessage(`{not-json}`),
		},
	})
	if len(events) != 1 || events[0].Id != "event-1" {
		t.Fatalf("events = %#v", events)
	}
}

func testFactoryEventStreamBranches(t *testing.T) {
	t.Helper()

	validEvent := []byte(`{"id":"event-1","type":"factory.session.started","sequence":1}`)

	stream := factorysession.FactoryEventStreamFromReadResult(factorysessionexecution.EventReadResult{})
	if stream == nil || len(stream.History) != 0 {
		t.Fatalf("empty stream = %#v", stream)
	}
	if _, ok := <-stream.Events; ok {
		t.Fatal("empty stream channel should be closed")
	}

	stream = factorysession.FactoryEventStreamFromReadResult(factorysessionexecution.EventReadResult{
		Events: []json.RawMessage{validEvent},
	})
	if len(stream.History) != 1 || stream.History[0].Id != "event-1" {
		t.Fatalf("stream history = %#v", stream.History)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this boundary regression intentionally covers optional-field trimming in one table-like scenario.
func TestProjectionResponses_TrimAndOmitOptionalFields(t *testing.T) {
	mapped := factorysession.ResultResponseToAPI(factorysessionexecution.ResultReadResult{
		SessionID:        "dur-sess-1",
		ResultStatus:     factorysessionexecution.ResultStatusFinal,
		SessionStatus:    factorysessionexecution.LifecycleStatusSucceeded,
		Mode:             factorysessionexecution.ResultModeFinal,
		IncludeArtifacts: true,
		PrimaryResult:    json.RawMessage(`{"bad":true}`),
		ArtifactIDs:      []string{" one ", " ", "two"},
		Failure:          &factorysessionexecution.FailureSummary{},
		Availability:     &factorysessionexecution.ResultAvailabilityDetail{},
	})
	if mapped.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want omitted for invalid work content", mapped.PrimaryResult)
	}
	if mapped.ArtifactIds == nil || len(*mapped.ArtifactIds) != 2 || (*mapped.ArtifactIds)[0] != "one" {
		t.Fatalf("artifactIds = %#v", mapped.ArtifactIds)
	}
	if mapped.FailureDetail != nil {
		t.Fatalf("failure = %#v, want omitted", mapped.FailureDetail)
	}
	if mapped.Availability != nil {
		t.Fatalf("availability = %#v, want omitted", mapped.Availability)
	}

	retryable := true
	dispatches := factorysession.ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: "dur-sess-1",
		Dispatches: []factorysessionexecution.DispatchSummary{{
			ID:                    "disp-1",
			Status:                factorysessionexecution.DispatchStatusRunning,
			DispatchKind:          "MODEL",
			Phase:                 " plan ",
			Label:                 " summarize ",
			Attempt:               2,
			Retryable:             &retryable,
			FailureClassification: string(workerexecution.WorkFailureTypeTimeout),
			RunnerID:              "runner-1",
			Model:                 "gpt",
			Provider:              "openai",
			ProviderSessionRefs:   []factorysessionexecution.ProviderSessionRef{{Provider: "openai", Kind: "RESPONSES", ID: "prov-1"}},
			OutputArtifactIDs:     []string{" out-1 ", " "},
		}},
	})
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", dispatches.Dispatches)
	}
	if dispatches.Dispatches[0].OutputArtifactIds == nil || len(*dispatches.Dispatches[0].OutputArtifactIds) != 1 {
		t.Fatalf("outputArtifactIds = %#v", dispatches.Dispatches[0].OutputArtifactIds)
	}
	if dispatches.Dispatches[0].ProviderSessionRefs == nil || len(*dispatches.Dispatches[0].ProviderSessionRefs) != 1 {
		t.Fatalf("providerSessionRefs = %#v", dispatches.Dispatches[0].ProviderSessionRefs)
	}
	if dispatches.Dispatches[0].Retryable == nil || !*dispatches.Dispatches[0].Retryable ||
		dispatches.Dispatches[0].FailureClassification == nil || *dispatches.Dispatches[0].FailureClassification != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("retry diagnostics = retryable:%v classification:%v", dispatches.Dispatches[0].Retryable, dispatches.Dispatches[0].FailureClassification)
	}

	artifacts := factorysession.ListArtifactsResponseToAPI(factorysessionexecution.ListArtifactsResult{
		SessionID: "dur-sess-1",
		Artifacts: []factorysessionexecution.ArtifactSummary{{
			ID:          "art-1",
			Kind:        "LOG",
			Visibility:  "PUBLIC",
			Label:       " audit ",
			ContentHash: " hash ",
			DispatchID:  "disp-1",
			AuditMode:   " SUMMARY ",
		}},
	})
	if len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].Label == nil || *artifacts.Artifacts[0].Label != "audit" {
		t.Fatalf("artifacts = %#v", artifacts.Artifacts)
	}
}

func TestValidateProjectionConsistencyFromFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	scenario := findScenario(t, catalog, "javascript-running-n-dispatch")

	sessionFixture, ok := scenario["session"].(map[string]any)
	if !ok {
		t.Fatal("missing session fixture")
	}
	resultFixture, ok := scenario["result"].(map[string]any)
	if !ok {
		t.Fatal("missing result fixture")
	}

	session := sessionReadFromFixture(sessionFixture)
	result := resultFromFixture(resultFixture)
	dispatches := listDispatchesFromFixture(scenario).Dispatches

	if err := factorysessionexecution.ValidateResultMatchesSessionRead(session, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}
	if err := factorysessionexecution.ValidateDispatchListMatchesSessionProgress(session, dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func resultRequestFromFixture(result map[string]any) factorysessionexecution.ResultRequest {
	req := factorysessionexecution.ResultRequest{}
	if mode := stringValue(result, "mode"); mode != "" {
		req.Mode = factorysessionexecution.ResultMode(mode)
	}
	if includeArtifacts, ok := result["includeArtifacts"].(bool); ok {
		req.IncludeArtifacts = includeArtifacts
	}
	return req
}

func resultFromFixture(result map[string]any) factorysessionexecution.ResultReadResult {
	out := factorysessionexecution.ResultReadResult{
		SessionID:    stringValue(result, "sessionId"),
		ResultStatus: factorysessionexecution.ResultStatus(stringValue(result, "resultStatus")),
		SessionStatus: factorysessionexecution.LifecycleStatus(
			stringValue(result, "sessionStatus"),
		),
		Mode: factorysessionexecution.ResultMode(stringValue(result, "mode")),
	}
	if includeArtifacts, ok := result["includeArtifacts"].(bool); ok {
		out.IncludeArtifacts = includeArtifacts
	}
	if primary, ok := result["primaryResult"]; ok {
		encoded, err := json.Marshal(primary)
		if err == nil {
			out.PrimaryResult = encoded
		}
	}
	if ids, ok := result["artifactIds"].([]any); ok {
		for _, item := range ids {
			if value, ok := item.(string); ok {
				out.ArtifactIDs = append(out.ArtifactIDs, value)
			}
		}
	}
	if refs, ok := result["artifactRefs"].([]any); ok {
		for _, item := range refs {
			if row, ok := item.(map[string]any); ok {
				out.ArtifactRefs = append(out.ArtifactRefs, artifactRefFromFixture(row))
			}
		}
	}
	if failure, ok := result["failureDetail"].(map[string]any); ok {
		out.Failure = &factorysessionexecution.FailureSummary{
			Reason:                 stringValue(failure, "reason"),
			Message:                stringValue(failure, "message"),
			PartialResultAvailable: boolValue(result, "partialResultAvailable"),
		}
	}
	if availability, ok := result["availability"].(map[string]any); ok {
		out.Availability = &factorysessionexecution.ResultAvailabilityDetail{
			Reason:    stringValue(availability, "reason"),
			Message:   stringValue(availability, "message"),
			Retryable: boolValue(availability, "retryable"),
		}
	}
	return out
}

func listDispatchesFromFixture(scenario map[string]any) factorysessionexecution.ListDispatchesResult {
	sessionFixture, _ := scenario["session"].(map[string]any)
	result := factorysessionexecution.ListDispatchesResult{
		SessionID: stringValue(sessionFixture, "sessionId"),
	}
	rows, ok := scenario["dispatches"].([]any)
	if !ok {
		return result
	}
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result.Dispatches = append(result.Dispatches, dispatchSummaryFromFixture(row))
	}
	return result
}

func dispatchSummaryFromFixture(dispatch map[string]any) factorysessionexecution.DispatchSummary {
	summary := factorysessionexecution.DispatchSummary{
		ID:           stringValue(dispatch, "id"),
		Status:       factorysessionexecution.DispatchStatus(stringValue(dispatch, "status")),
		DispatchKind: stringValue(dispatch, "dispatchKind"),
		Phase:        stringValue(dispatch, "phase"),
		Label:        stringValue(dispatch, "label"),
		Attempt:      intValue(dispatch, "attempt"),
		RunnerID:     stringValue(dispatch, "runnerId"),
		Model:        stringValue(dispatch, "model"),
		Provider:     stringValue(dispatch, "provider"),
	}
	if ids, ok := dispatch["outputArtifactIds"].([]any); ok {
		for _, item := range ids {
			if value, ok := item.(string); ok {
				summary.OutputArtifactIDs = append(summary.OutputArtifactIDs, value)
			}
		}
	}
	if refs, ok := dispatch["providerSessionRefs"].([]any); ok {
		for _, item := range refs {
			if row, ok := item.(map[string]any); ok {
				summary.ProviderSessionRefs = append(summary.ProviderSessionRefs, factorysessionexecution.ProviderSessionRef{
					Provider: stringValue(row, "provider"),
					Kind:     stringValue(row, "kind"),
					ID:       stringValue(row, "id"),
				})
			}
		}
	}
	if failure, ok := dispatch["failureDetail"].(map[string]any); ok {
		summary.FailureDetail = &factorysessionexecution.DispatchFailureDetail{
			Reason:  stringValue(failure, "reason"),
			Message: stringValue(failure, "message"),
		}
	}
	if javascript, ok := dispatch["javascript"].(map[string]any); ok {
		summary.JavaScript = &factorysessionexecution.DispatchJavaScriptProjection{
			TaskKind:      stringValue(javascript, "taskKind"),
			TaskLabel:     stringValue(javascript, "taskLabel"),
			ExecutionMode: stringValue(javascript, "executionMode"),
		}
	}
	return summary
}

func dispatchDetailFromFixture(dispatch map[string]any) factorysessionexecution.DispatchDetail {
	summary := dispatchSummaryFromFixture(dispatch)
	detail := factorysessionexecution.DispatchDetail{
		DispatchSummary:  summary,
		SessionID:        stringValue(dispatch, "sessionId"),
		OrchestratorKind: stringValue(dispatch, "orchestratorKind"),
	}
	if ids, ok := dispatch["artifactIds"].([]any); ok {
		for _, item := range ids {
			if value, ok := item.(string); ok {
				detail.ArtifactIDs = append(detail.ArtifactIDs, value)
			}
		}
	}
	if petri, ok := dispatch["petri"].(map[string]any); ok {
		detail.Petri = &factorysessionexecution.DispatchPetriProjection{
			TransitionID:    stringValue(petri, "transitionId"),
			WorkstationName: stringValue(petri, "workstationName"),
			WorkerType:      stringValue(petri, "workerType"),
		}
	}
	if javascript, ok := dispatch["javascript"].(map[string]any); ok {
		detail.JavaScript = &factorysessionexecution.DispatchJavaScriptProjection{
			TaskKind:      stringValue(javascript, "taskKind"),
			TaskLabel:     stringValue(javascript, "taskLabel"),
			ExecutionMode: stringValue(javascript, "executionMode"),
		}
	}
	if transitions, ok := dispatch["statusTransitions"].([]any); ok {
		for _, item := range transitions {
			if value, ok := item.(string); ok {
				detail.StatusTransitions = append(detail.StatusTransitions, factorysessionexecution.DispatchStatus(value))
			}
		}
	}
	return detail
}

func listArtifactsFromFixture(scenario map[string]any) factorysessionexecution.ListArtifactsResult {
	sessionFixture, _ := scenario["session"].(map[string]any)
	result := factorysessionexecution.ListArtifactsResult{
		SessionID: stringValue(sessionFixture, "sessionId"),
	}
	rows, ok := scenario["artifacts"].([]any)
	if !ok {
		return result
	}
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result.Artifacts = append(result.Artifacts, artifactSummaryFromFixture(row))
	}
	return result
}

func artifactSummaryFromFixture(artifact map[string]any) factorysessionexecution.ArtifactSummary {
	summary := factorysessionexecution.ArtifactSummary{
		ID:          stringValue(artifact, "id"),
		Kind:        stringValue(artifact, "kind"),
		Visibility:  stringValue(artifact, "visibility"),
		Label:       stringValue(artifact, "label"),
		ContentHash: stringValue(artifact, "contentHash"),
		SizeBytes:   int64Value(artifact, "sizeBytes"),
		DispatchID:  stringValue(artifact, "dispatchId"),
		AuditMode:   stringValue(artifact, "auditMode"),
	}
	if createdAt := timeValue(artifact, "createdAt"); createdAt != nil {
		summary.CreatedAt = createdAt
	}
	if ref, ok := artifact["retrievalRef"].(map[string]any); ok {
		summary.RetrievalRef = &factorysessionexecution.ArtifactRetrievalRef{
			Href:   stringValue(ref, "href"),
			Method: stringValue(ref, "method"),
		}
	}
	return summary
}

func artifactDetailFromFixture(artifact map[string]any) factorysessionexecution.ArtifactDetail {
	return factorysessionexecution.ArtifactDetail{
		ArtifactSummary: artifactSummaryFromFixture(artifact),
		SessionID:       stringValue(artifact, "sessionId"),
		Summary:         stringValue(artifact, "summary"),
		Content:         marshalFixtureValue(artifact["content"]),
	}
}

func marshalFixtureValue(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func mustParseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", raw, err)
	}
	return value
}
