package factorysession_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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
	_, err := factorysession.RetryDispatchRequestFromAPI(factoryapi.FactorySessionRetryDispatchRequest{})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestEvaluateLifecycleControlFromServiceSpec_MatchesRetryTerminalFixture(t *testing.T) {
	outcome := factorysessionexecution.EvaluateLifecycleControl(
		factorysessionexecution.LifecycleControlRetryDispatch,
		factorysessionexecution.LifecycleStatusSucceeded,
	)
	if outcome != factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", outcome)
	}
}

func findScenario(t *testing.T, catalog durableFixtureCatalog, id string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
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
	if usage, ok := session["usage"].(map[string]any); ok {
		result.Usage = factorysessionexecution.EmptySessionUsage()
		if rows, ok := usage["resources"].([]any); ok {
			for _, item := range rows {
				if row, ok := item.(map[string]any); ok {
					result.Usage.Resources = append(result.Usage.Resources, factorysessionexecution.ResourceUsage{
						Name:      stringValue(row, "name"),
						Available: intValue(row, "available"),
						Total:     intValue(row, "total"),
					})
				}
			}
		}
	} else {
		result.Usage = factorysessionexecution.EmptySessionUsage()
	}
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
	if failure, ok := session["failure"].(map[string]any); ok {
		result.Failure = &factorysessionexecution.FailureSummary{
			Reason:                 stringValue(failure, "reason"),
			Message:                stringValue(failure, "message"),
			ErrorClass:             stringValue(failure, "errorClass"),
			PartialResultAvailable: boolValue(failure, "partialResultAvailable"),
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
	return result
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
		Kind:       workflowsource.Kind(stringValue(source, "kind")),
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
		TotalDispatches:     intValue(progress, "totalDispatches"),
		CompletedDispatches: intValue(progress, "completedDispatches"),
		FailedDispatches:    intValue(progress, "failedDispatches"),
		InFlightDispatches:  intValue(progress, "inFlightDispatches"),
		PhaseCount:          intValue(progress, "phaseCount"),
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
