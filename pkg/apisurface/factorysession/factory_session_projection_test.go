package factorysession_test

import (
	"encoding/json"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	if failedMapped.Failure == nil || failedMapped.Failure.PartialResultAvailable == nil || !*failedMapped.Failure.PartialResultAvailable {
		t.Fatal("failed-with-partial failure detail missing")
	}

	unavailable := findScenario(t, catalog, "petri-canceled")
	unavailableMapped := factorysession.ResultResponseToAPI(resultFromFixture(unavailable["result"].(map[string]any)))
	if unavailableMapped.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("unavailable status = %q", unavailableMapped.ResultStatus)
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
	if failure, ok := result["failure"].(map[string]any); ok {
		out.Failure = &factorysessionexecution.FailureSummary{
			Reason:                 stringValue(failure, "reason"),
			Message:                stringValue(failure, "message"),
			ErrorClass:             stringValue(failure, "errorClass"),
			PartialResultAvailable: boolValue(failure, "partialResultAvailable"),
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
		RunnerID:     dispatchRunnerIDFromFixtureMap(dispatch),
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
			Reason:     stringValue(failure, "reason"),
			Message:    stringValue(failure, "message"),
			ErrorClass: stringValue(failure, "errorClass"),
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
			TaskKind:  stringValue(javascript, "taskKind"),
			TaskLabel: stringValue(javascript, "taskLabel"),
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

func dispatchRunnerIDFromFixtureMap(dispatch map[string]any) string {
	if modelProvider := stringValue(dispatch, "modelProvider"); modelProvider != "" {
		if runnerID, ok := interfaces.InternalRunnerIDFromPublicWorkerModelProvider(factoryapi.WorkerModelProvider(modelProvider)); ok {
			return runnerID
		}
	}
	return stringValue(dispatch, "runnerId")
}
