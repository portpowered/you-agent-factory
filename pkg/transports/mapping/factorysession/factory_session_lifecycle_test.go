package factorysession_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestSessionReadResponseToAPI_MapsRunningFixture(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	scenario := findScenario(t, catalog, "petri-running-one-dispatch")
	sessionFixture, ok := scenario["session"].(map[string]any)
	if !ok {
		t.Fatal("missing session fixture")
	}

	mapped := factorysession.SessionReadResponseToAPI(sessionReadFromFixture(sessionFixture))
	if mapped.SessionId != "dur-sess-petri-run-001" {
		t.Fatalf("sessionId = %q", mapped.SessionId)
	}
	if mapped.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", mapped.Status)
	}
	if mapped.Phase == nil || *mapped.Phase != "triage" {
		t.Fatalf("phase = %#v, want triage", mapped.Phase)
	}
	if mapped.Progress == nil || mapped.Progress.InFlightDispatches == nil || *mapped.Progress.InFlightDispatches != 1 {
		t.Fatalf("progress = %#v, want one in-flight dispatch", mapped.Progress)
	}
}

func TestLifecycleControlResponseToAPI_MapsPauseCancelAndRetryFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)

	pauseScenario := findScenario(t, catalog, "javascript-paused-two-dispatch")
	pauseControl, ok := pauseScenario["lifecycleControl"].(map[string]any)
	if !ok {
		t.Fatal("missing pause lifecycleControl fixture")
	}
	pauseMapped := factorysession.LifecycleControlResponseToAPI(lifecycleControlFromFixture(pauseControl))
	if pauseMapped.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", pauseMapped.Operation)
	}
	if pauseMapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", pauseMapped.Outcome)
	}

	cancelScenario := findScenario(t, catalog, "petri-canceled")
	cancelControl, ok := cancelScenario["lifecycleControl"].(map[string]any)
	if !ok {
		t.Fatal("missing cancel lifecycleControl fixture")
	}
	cancelMapped := factorysession.LifecycleControlResponseToAPI(lifecycleControlFromFixture(cancelControl))
	if cancelMapped.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("status = %q, want CANCELING", cancelMapped.Status)
	}

	retryScenario := findScenario(t, catalog, "javascript-succeeded-two-dispatch")
	retryControl, ok := retryScenario["lifecycleControl"].(map[string]any)
	if !ok {
		t.Fatal("missing retry lifecycleControl fixture")
	}
	retryMapped := factorysession.LifecycleControlResponseToAPI(lifecycleControlFromFixture(retryControl))
	if retryMapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", retryMapped.Outcome)
	}
	if retryMapped.DispatchId == nil || *retryMapped.DispatchId != "disp-js-success-002" {
		t.Fatalf("dispatchId = %#v", retryMapped.DispatchId)
	}
}

func TestRetryDispatchRequestFromAPI_RequiresDispatchID(t *testing.T) {
	raw, err := factorysession.RetryDispatchRequestFromAPI(factoryapi.FactorySessionRetryDispatchRequest{})
	if err != nil {
		t.Fatalf("RetryDispatchRequestFromAPI: %v", err)
	}
	if raw.DispatchID != "" {
		t.Fatalf("dispatchId = %q, want raw empty value", raw.DispatchID)
	}
}

func TestControlRequestFromAPI_NormalizesOptionalMetadata(t *testing.T) {
	requestID := "req-control-001"
	reason := "operator pause"
	mapped, err := factorysession.ControlRequestFromAPI(factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: &requestID,
		Reason:    &reason,
	})
	if err != nil {
		t.Fatalf("ControlRequestFromAPI: %v", err)
	}
	if mapped.RequestID != requestID || mapped.Reason != reason {
		t.Fatalf("mapped = %#v, want requestId=%q reason=%q", mapped, requestID, reason)
	}
}

func TestApproveRequestFromAPI_NormalizesApprovalFields(t *testing.T) {
	requestID := "req-approve-001"
	previewID := "preview-001"
	mapped, err := factorysession.ApproveRequestFromAPI(factoryapi.FactorySessionApproveRequest{
		RequestId:         &requestID,
		ApprovalPreviewId: &previewID,
	})
	if err != nil {
		t.Fatalf("ApproveRequestFromAPI: %v", err)
	}
	if mapped.RequestID != requestID || mapped.ApprovalPreviewID != previewID {
		t.Fatalf("mapped = %#v", mapped)
	}
}

func TestRetryDispatchRequestFromAPI_NormalizesDispatchAndFlags(t *testing.T) {
	requestID := "req-retry-001"
	force := true
	reset := true
	mapped, err := factorysession.RetryDispatchRequestFromAPI(factoryapi.FactorySessionRetryDispatchRequest{
		RequestId:         &requestID,
		DispatchId:        "disp-001",
		ForceNewAttempt:   &force,
		ResetAttemptCount: &reset,
	})
	if err != nil {
		t.Fatalf("RetryDispatchRequestFromAPI: %v", err)
	}
	if mapped.DispatchID != "disp-001" || !mapped.ForceNewAttempt || !mapped.ResetAttemptCount {
		t.Fatalf("mapped = %#v", mapped)
	}
}

func TestInterruptDispatchRequestFromAPI_RequiresDispatchID(t *testing.T) {
	raw, err := factorysession.InterruptDispatchRequestFromAPI(factoryapi.FactorySessionInterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("InterruptDispatchRequestFromAPI: %v", err)
	}
	if raw.DispatchID != "" {
		t.Fatalf("dispatchId = %q, want raw empty value", raw.DispatchID)
	}
}

func TestInterruptDispatchRequestFromAPI_NormalizesDispatchAndMetadata(t *testing.T) {
	requestID := "req-interrupt-001"
	reason := "operator stop"
	mapped, err := factorysession.InterruptDispatchRequestFromAPI(factoryapi.FactorySessionInterruptDispatchRequest{
		RequestId:  &requestID,
		DispatchId: "disp-001",
		Reason:     &reason,
	})
	if err != nil {
		t.Fatalf("InterruptDispatchRequestFromAPI: %v", err)
	}
	if mapped.DispatchID != "disp-001" || mapped.RequestID != requestID || mapped.Reason != reason {
		t.Fatalf("mapped = %#v", mapped)
	}
}

func TestEvaluateInterruptDispatchControlFromServiceSpec(t *testing.T) {
	for _, outcome := range []factorysessionexecution.LifecycleControlOutcome{
		"ACCEPTED", "INVALID_STATE", "NO_OP", "TERMINAL_SESSION",
	} {
		mapped := factorysession.LifecycleControlResponseToAPI(factorysessionexecution.LifecycleControlResult{
			SessionID: "session-1",
			Operation: "INTERRUPT_DISPATCH",
			Outcome:   outcome,
			Status:    factorysessionexecution.LifecycleStatusRunning,
		})
		if string(mapped.Outcome) != string(outcome) {
			t.Fatalf("mapped outcome = %q, want %q", mapped.Outcome, outcome)
		}
	}
}

func TestEvaluateLifecycleControlFromServiceSpec_MatchesRetryTerminalFixture(t *testing.T) {
	mapped := factorysession.LifecycleControlResponseToAPI(factorysessionexecution.LifecycleControlResult{
		SessionID: "dur-sess-js-success-002",
		Operation: "RETRY_DISPATCH",
		Outcome:   "TERMINAL_SESSION",
		Status:    factorysessionexecution.LifecycleStatusSucceeded,
	})
	if mapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", mapped.Outcome)
	}
}

func findScenario(t *testing.T, catalog durableFixtureCatalog, id string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "http", "testdata", "durable-session-contract-fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	scenarios, ok := document["scenarios"].([]any)
	if !ok {
		t.Fatal("missing scenarios array")
	}
	for _, item := range scenarios {
		scenario, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(scenario, "id") == id {
			return scenario
		}
	}
	t.Fatalf("missing scenario %q", id)
	return nil
}

func sessionReadFromFixture(session map[string]any) factorysessionexecution.SessionReadResult {
	result := factorysessionexecution.SessionReadResult{
		SessionID:        stringValue(session, "sessionId"),
		Status:           factorysessionexecution.LifecycleStatus(stringValue(session, "status")),
		OrchestratorKind: stringValue(session, "orchestratorKind"),
		Dialect:          stringValue(session, "dialect"),
		SourceHash:       stringValue(session, "sourceHash"),
		Phase:            stringValue(session, "phase"),
	}
	applySessionReadFixtureFields(&result, session)
	return result
}

func applySessionReadFixtureFields(result *factorysessionexecution.SessionReadResult, session map[string]any) {
	applySessionReadFixtureCoreFields(result, session)
	applySessionReadFixtureOutcomeFields(result, session)
}

func applySessionReadFixtureCoreFields(result *factorysessionexecution.SessionReadResult, session map[string]any) {
	if resolved, ok := session["resolvedSource"].(map[string]any); ok {
		result.ResolvedSource = resolvedSourceFromFixture(resolved)
	}
	if requested, ok := session["requestedPolicy"].(map[string]any); ok {
		result.Policy.Requested = cloneFixtureMap(requested)
	}
	if effective, ok := session["effectivePolicy"].(map[string]any); ok {
		result.Policy.Effective = cloneFixtureMap(effective)
	}
	result.Policy.EffectiveHash = stringValue(session, "effectivePolicyHash")
	if progress, ok := session["progress"].(map[string]any); ok {
		result.Progress = progressCountsFromFixture(progress)
	}
	if budgets, ok := session["budgets"].(map[string]any); ok {
		if maxAgents := intValue(budgets, "maxAgents"); maxAgents > 0 {
			result.Budgets = &factorysessionexecution.SessionBudgets{MaxAgents: maxAgents}
		}
	}
	result.Usage = sessionUsageFromFixture(session)
}

func applySessionReadFixtureOutcomeFields(result *factorysessionexecution.SessionReadResult, session map[string]any) {
	if summaries, ok := session["phaseSummaries"].([]any); ok {
		for _, item := range summaries {
			if row, ok := item.(map[string]any); ok {
				result.PhaseSummaries = append(result.PhaseSummaries, phaseSummaryFromFixture(row))
			}
		}
	}
	if summary, ok := session["resultSummary"].(map[string]any); ok {
		result.ResultSummary = &factorysessionexecution.ResultSummary{
			ResultStatus: stringValue(summary, "resultStatus"),
			Summary:      stringValue(summary, "summary"),
		}
	}
	if failure, ok := session["failureDetail"].(map[string]any); ok {
		result.Failure = &factorysessionexecution.FailureSummary{
			Reason:                 stringValue(failure, "reason"),
			Message:                stringValue(failure, "message"),
			PartialResultAvailable: boolValue(session, "partialResultAvailable"),
		}
	}
	if lifecycle, ok := session["lifecycle"].(map[string]any); ok {
		result.Lifecycle = lifecycleTimestampsFromFixture(lifecycle)
	}
	if refs, ok := session["artifactRefs"].([]any); ok {
		for _, item := range refs {
			if row, ok := item.(map[string]any); ok {
				result.ArtifactRefs = append(result.ArtifactRefs, artifactRefFromFixture(row))
			}
		}
	}
	if links, ok := session["links"].(map[string]any); ok {
		result.Links = inspectionLinksFromFixture(links)
	}
}

func sessionUsageFromFixture(session map[string]any) factorysessionexecution.SessionUsage {
	usage := factorysessionexecution.SessionUsage{Resources: []factorysessionexecution.ResourceUsage{}}
	rawUsage, ok := session["usage"].(map[string]any)
	if !ok {
		return usage
	}
	rows, ok := rawUsage["resources"].([]any)
	if !ok {
		return usage
	}
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		usage.Resources = append(usage.Resources, factorysessionexecution.ResourceUsage{
			Name:      stringValue(row, "name"),
			Available: intValue(row, "available"),
			Total:     intValue(row, "total"),
		})
	}
	return usage
}

func lifecycleControlFromFixture(control map[string]any) factorysessionexecution.LifecycleControlResult {
	result := factorysessionexecution.LifecycleControlResult{
		SessionID:       stringValue(control, "sessionId"),
		Operation:       factorysessionexecution.LifecycleControlKind(stringValue(control, "operation")),
		Outcome:         factorysessionexecution.LifecycleControlOutcome(stringValue(control, "outcome")),
		Status:          factorysessionexecution.LifecycleStatus(stringValue(control, "status")),
		DispatchID:      stringValue(control, "dispatchId"),
		RetryDispatchID: stringValue(control, "retryDispatchId"),
		Detail:          stringValue(control, "detail"),
	}
	if links, ok := control["links"].(map[string]any); ok {
		result.Links = lifecycleControlLinksFromFixture(links)
	}
	return result
}

func resolvedSourceFromFixture(source map[string]any) factorysessionexecution.ResolvedSource {
	result := factorysessionexecution.ResolvedSource{
		Kind:       factory.WorkflowSourceKind(stringValue(source, "kind")),
		SourceRef:  stringValue(source, "sourceRef"),
		SourceHash: stringValue(source, "sourceHash"),
		Dialect:    stringValue(source, "dialect"),
	}
	if order, ok := source["resolutionOrder"].([]any); ok {
		for _, item := range order {
			if value, ok := item.(string); ok {
				result.ResolutionOrder = append(result.ResolutionOrder, value)
			}
		}
	}
	return result
}

func progressCountsFromFixture(progress map[string]any) *factorysessionexecution.ProgressCounts {
	return &factorysessionexecution.ProgressCounts{
		TotalDispatches:       intValue(progress, "totalDispatches"),
		CompletedDispatches:   intValue(progress, "completedDispatches"),
		FailedDispatches:      intValue(progress, "failedDispatches"),
		InFlightDispatches:    intValue(progress, "inFlightDispatches"),
		QueuedDispatches:      intValue(progress, "queuedDispatches"),
		RunningDispatches:     intValue(progress, "runningDispatches"),
		CanceledDispatches:    intValue(progress, "canceledDispatches"),
		TimedOutDispatches:    intValue(progress, "timedOutDispatches"),
		SkippedDispatches:     intValue(progress, "skippedDispatches"),
		InterruptedDispatches: intValue(progress, "interruptedDispatches"),
		PhaseCount:            intValue(progress, "phaseCount"),
	}
}

func phaseSummaryFromFixture(summary map[string]any) factorysessionexecution.PhaseSummary {
	return factorysessionexecution.PhaseSummary{
		Phase:                  stringValue(summary, "phase"),
		Label:                  stringValue(summary, "label"),
		DispatchCount:          intValue(summary, "dispatchCount"),
		CompletedDispatchCount: intValue(summary, "completedDispatchCount"),
		FailedDispatchCount:    intValue(summary, "failedDispatchCount"),
	}
}

func lifecycleTimestampsFromFixture(lifecycle map[string]any) *factorysessionexecution.LifecycleTimestamps {
	out := &factorysessionexecution.LifecycleTimestamps{}
	if value := timeValue(lifecycle, "queuedAt"); value != nil {
		out.QueuedAt = value
	}
	if value := timeValue(lifecycle, "startedAt"); value != nil {
		out.StartedAt = value
	}
	if value := timeValue(lifecycle, "finishedAt"); value != nil {
		out.FinishedAt = value
	}
	return out
}

func artifactRefFromFixture(ref map[string]any) factorysessionexecution.ArtifactRefSummary {
	return factorysessionexecution.ArtifactRefSummary{
		ID:          stringValue(ref, "id"),
		Kind:        stringValue(ref, "kind"),
		Visibility:  stringValue(ref, "visibility"),
		ContentHash: stringValue(ref, "contentHash"),
		SizeBytes:   int64Value(ref, "sizeBytes"),
	}
}

func inspectionLinksFromFixture(links map[string]any) factorysessionexecution.InspectionLinks {
	return factorysessionexecution.InspectionLinks{
		Session:    stringValue(links, "session"),
		Status:     stringValue(links, "status"),
		Events:     stringValue(links, "events"),
		Results:    stringValue(links, "results"),
		Dispatches: stringValue(links, "dispatches"),
		Artifacts:  stringValue(links, "artifacts"),
	}
}

func lifecycleControlLinksFromFixture(links map[string]any) factorysessionexecution.LifecycleControlLinks {
	return factorysessionexecution.LifecycleControlLinks{
		Session:    stringValue(links, "session"),
		Status:     stringValue(links, "status"),
		Events:     stringValue(links, "events"),
		Results:    stringValue(links, "results"),
		Dispatches: stringValue(links, "dispatches"),
		Artifacts:  stringValue(links, "artifacts"),
	}
}

func stringValue(document map[string]any, key string) string {
	value, ok := document[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func boolValue(document map[string]any, key string) bool {
	value, ok := document[key].(bool)
	if !ok {
		return false
	}
	return value
}

func intValue(document map[string]any, key string) int {
	switch typed := document[key].(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func int64Value(document map[string]any, key string) int64 {
	switch typed := document[key].(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func timeValue(document map[string]any, key string) *time.Time {
	raw, ok := document[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func cloneFixtureMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
func TestLifecycleControlErrorResponse_MapsControlConflictAndNotFound(t *testing.T) {
	status, response, ok := factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-success-002",
		&factorysessionexecution.ControlError{
			Operation: "CANCEL",
			Outcome:   factorysessionexecution.LifecycleControlOutcomeTerminalSession,
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
			Message:   "terminal",
		},
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	mapped, ok := response.(factoryapi.FactorySessionLifecycleControlResponse)
	if !ok {
		t.Fatalf("response = %T, want FactorySessionLifecycleControlResponse", response)
	}
	if mapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", mapped.Outcome)
	}

	status, response, ok = factorysession.LifecycleControlErrorResponse(
		"dur-sess-missing-999",
		factorysessionexecution.ErrSessionNotFound,
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for not found")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("response = %#v, want NOT_FOUND ErrorResponse", response)
	}

	status, response, ok = factorysession.LifecycleControlErrorResponse(
		"live-session-missing-001",
		fmt.Errorf("%w: live-session-missing-001", apisurface.ErrFactorySessionNotFound),
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for live not found")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok = response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("response = %#v, want NOT_FOUND ErrorResponse", response)
	}
}

func TestLifecycleControlSuccessStatus_MapsAcceptedCancelAndTerminate(t *testing.T) {
	cancelStatus := factorysession.LifecycleControlSuccessStatus(factorysessionexecution.LifecycleControlResult{
		Outcome: factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:  factorysessionexecution.LifecycleStatusCanceling,
	})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202", cancelStatus)
	}

	terminateStatus := factorysession.LifecycleControlSuccessStatus(factorysessionexecution.LifecycleControlResult{
		Outcome: factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:  factorysessionexecution.LifecycleStatus("TERMINATED"),
	})
	if terminateStatus != http.StatusOK {
		t.Fatalf("terminate status = %d, want 200", terminateStatus)
	}

	noOpStatus := factorysession.LifecycleControlSuccessStatus(factorysessionexecution.LifecycleControlResult{
		Outcome: factorysessionexecution.LifecycleControlOutcomeNoOp,
		Status:  factorysessionexecution.LifecycleStatusRunning,
	})
	if noOpStatus != http.StatusOK {
		t.Fatalf("no-op status = %d, want 200", noOpStatus)
	}
}

func TestLifecycleControlErrorResponse_ReturnsFalseForUnknownErrors(t *testing.T) {
	if _, _, ok := factorysession.LifecycleControlErrorResponse("dur-sess-001", errors.New("other")); ok {
		t.Fatal("LifecycleControlErrorResponse = true, want false")
	}
}

func TestLifecycleControlErrorResponse_MapsValidationAndDispatchNotFound(t *testing.T) {
	status, response, ok := factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-run-n-001",
		&factorysessionexecution.ExecutionValidationError{Field: "dispatchId", Message: "dispatchId is required"},
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for validation")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("response = %#v, want BAD_REQUEST ErrorResponse", response)
	}

	status, response, ok = factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-run-n-001",
		factorysessionexecution.ErrDispatchNotFound,
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for dispatch not found")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok = response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("response = %#v, want NOT_FOUND ErrorResponse", response)
	}
}
