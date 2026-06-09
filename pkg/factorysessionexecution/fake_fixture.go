package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// LoadFakeScenariosFromContractFixtures loads deterministic fake scenarios from the
// durable session contract fixture catalog.
func LoadFakeScenariosFromContractFixtures(path string) ([]FakeScenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract fixtures: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode contract fixtures: %w", err)
	}
	scenariosValue, ok := document["scenarios"].([]any)
	if !ok {
		return nil, fmt.Errorf("contract fixtures missing scenarios array")
	}
	scenarios := make([]FakeScenario, 0, len(scenariosValue)+1)
	for _, item := range scenariosValue {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		scenario, err := parseFakeScenarioFromFixture(row)
		if err != nil {
			return nil, fmt.Errorf("parse scenario %q: %w", fixtureStringValue(row, "id"), err)
		}
		if scenario.RequestID == "" {
			continue
		}
		scenarios = append(scenarios, scenario)
	}
	if replay, ok := document["idempotentReplay"].(map[string]any); ok {
		scenario, err := parseIdempotentReplayScenario(replay)
		if err != nil {
			return nil, fmt.Errorf("parse idempotentReplay: %w", err)
		}
		scenarios = append(scenarios, scenario)
	}
	scenarios = append(scenarios, BuiltinInterruptedRecoverableScenario())
	return scenarios, nil
}

func parseIdempotentReplayScenario(document map[string]any) (FakeScenario, error) {
	executionRequest, ok := document["executionRequest"].(map[string]any)
	if !ok {
		return FakeScenario{}, fmt.Errorf("missing executionRequest")
	}
	asyncResponse, ok := document["asyncResponse"].(map[string]any)
	if !ok {
		return FakeScenario{}, fmt.Errorf("missing asyncResponse")
	}
	scenario := FakeScenario{
		ID:        "idempotent-replay",
		RequestID: fixtureStringValue(executionRequest, "requestId"),
	}
	asyncStart, err := asyncStartFromFixtureMap(asyncResponse)
	if err != nil {
		return FakeScenario{}, err
	}
	scenario.AsyncStart = &asyncStart
	scenario.Session = SessionReadResult{
		SessionID:        asyncStart.SessionID,
		Status:           LifecycleStatus(asyncStart.Status),
		OrchestratorKind: asyncStart.OrchestratorKind,
		Dialect:          asyncStart.Dialect,
		ResolvedSource:   asyncStart.ResolvedSource,
		SourceHash:       asyncStart.SourceHash,
		Policy:           asyncStart.Policy,
		Links:            asyncStart.Links,
	}
	scenario.Result = ResultReadResult{
		SessionID:     asyncStart.SessionID,
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: LifecycleStatus(asyncStart.Status),
		Mode:          ResultModeFinal,
	}
	summary := DurableListSummaryFromSessionRead(scenario.Session)
	scenario.ListSummary = &summary
	return scenario, nil
}

func parseFakeScenarioFromFixture(document map[string]any) (FakeScenario, error) {
	scenario := FakeScenario{
		ID: fixtureStringValue(document, "id"),
	}
	if executionRequest, ok := document["executionRequest"].(map[string]any); ok {
		scenario.RequestID = fixtureStringValue(executionRequest, "requestId")
	}
	sessionFixture, ok := document["session"].(map[string]any)
	if !ok {
		return FakeScenario{}, fmt.Errorf("missing session fixture")
	}
	scenario.Session = sessionReadFromFixtureMap(sessionFixture)
	if dispatches, ok := document["dispatches"].([]any); ok {
		for _, item := range dispatches {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			scenario.Dispatches = append(scenario.Dispatches, dispatchSummaryFromFixtureMap(row))
		}
	}
	if detail, ok := document["dispatchDetail"].(map[string]any); ok {
		if scenario.DispatchDetails == nil {
			scenario.DispatchDetails = make(map[string]DispatchDetail)
		}
		parsed := dispatchDetailFromFixtureMap(detail)
		scenario.DispatchDetails[parsed.ID] = parsed
	}
	if artifacts, ok := document["artifacts"].([]any); ok {
		for _, item := range artifacts {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			scenario.Artifacts = append(scenario.Artifacts, artifactSummaryFromFixtureMap(row))
		}
	}
	if detail, ok := document["artifactDetail"].(map[string]any); ok {
		if scenario.ArtifactDetails == nil {
			scenario.ArtifactDetails = make(map[string]ArtifactDetail)
		}
		parsed := artifactDetailFromFixtureMap(detail)
		scenario.ArtifactDetails[parsed.ID] = parsed
	}
	if result, ok := document["result"].(map[string]any); ok {
		scenario.Result = resultReadFromFixtureMap(result)
	}
	if events, ok := document["events"].([]any); ok {
		for _, item := range events {
			encoded, err := json.Marshal(item)
			if err != nil {
				continue
			}
			scenario.Events = append(scenario.Events, encoded)
		}
	}
	if asyncResponse, ok := document["asyncResponse"].(map[string]any); ok {
		start, err := asyncStartFromFixtureMap(asyncResponse)
		if err != nil {
			return FakeScenario{}, err
		}
		scenario.AsyncStart = &start
	}
	if syncResponse, ok := document["syncResponse"].(map[string]any); ok {
		start, err := syncStartFromFixtureMap(syncResponse)
		if err != nil {
			return FakeScenario{}, err
		}
		scenario.SyncStart = &start
	}
	if listSummary, ok := document["listSummary"].(map[string]any); ok {
		summary := durableListSummaryFromFixtureMap(listSummary)
		scenario.ListSummary = &summary
	} else {
		summary := DurableListSummaryFromSessionRead(scenario.Session)
		scenario.ListSummary = &summary
	}
	return scenario, nil
}

func asyncStartFromFixtureMap(document map[string]any) (AsyncStartResult, error) {
	sessionID := fixtureStringValue(document, "sessionId")
	result := AsyncStartResult{
		SessionID:        sessionID,
		Status:           fixtureStringValue(document, "status"),
		OrchestratorKind: fixtureStringValue(document, "orchestratorKind"),
		Dialect:          fixtureStringValue(document, "dialect"),
		SourceHash:       fixtureStringValue(document, "sourceHash"),
	}
	if resolved, ok := document["resolvedSource"].(map[string]any); ok {
		result.ResolvedSource = resolvedSourceFromFixtureMap(resolved)
	}
	if requested, ok := document["requestedPolicy"].(map[string]any); ok {
		result.Policy.Requested = cloneFixtureMap(requested)
	}
	if effective, ok := document["effectivePolicy"].(map[string]any); ok {
		result.Policy.Effective = cloneFixtureMap(effective)
	}
	result.Policy.EffectiveHash = fixtureStringValue(document, "effectivePolicyHash")
	if links, ok := document["links"].(map[string]any); ok {
		result.Links = inspectionLinksFromFixtureMap(links)
	} else if sessionID != "" {
		result.Links = InspectionLinksForSession(sessionID, true)
	}
	return result, nil
}

func syncStartFromFixtureMap(document map[string]any) (SyncStartResult, error) {
	asyncStart, err := asyncStartFromFixtureMap(document)
	if err != nil {
		return SyncStartResult{}, err
	}
	result := SyncStartResult{
		AsyncStartResult: asyncStart,
		SyncOutcome:    SyncOutcome(fixtureStringValue(document, "syncOutcome")),
		TimedOut:       fixtureBoolValue(document, "timedOut"),
		SessionCanceledByTimeout: fixtureBoolValue(document, "sessionCanceledByTimeout"),
	}
	if raw, ok := document["result"]; ok {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return SyncStartResult{}, fmt.Errorf("marshal sync result: %w", err)
		}
		result.Result = encoded
	}
	return result, nil
}

func sessionReadFromFixtureMap(session map[string]any) SessionReadResult {
	result := SessionReadResult{
		SessionID:        fixtureStringValue(session, "sessionId"),
		Status:           LifecycleStatus(fixtureStringValue(session, "status")),
		OrchestratorKind: fixtureStringValue(session, "orchestratorKind"),
		Dialect:          fixtureStringValue(session, "dialect"),
		SourceHash:       fixtureStringValue(session, "sourceHash"),
		Phase:            fixtureStringValue(session, "phase"),
		StaleLease:       fixtureBoolValue(session, "staleLease"),
	}
	if resolved, ok := session["resolvedSource"].(map[string]any); ok {
		result.ResolvedSource = resolvedSourceFromFixtureMap(resolved)
	}
	if requested, ok := session["requestedPolicy"].(map[string]any); ok {
		result.Policy.Requested = cloneFixtureMap(requested)
	}
	if effective, ok := session["effectivePolicy"].(map[string]any); ok {
		result.Policy.Effective = cloneFixtureMap(effective)
	}
	result.Policy.EffectiveHash = fixtureStringValue(session, "effectivePolicyHash")
	if progress, ok := session["progress"].(map[string]any); ok {
		result.Progress = progressCountsFromFixtureMap(progress)
	}
	if summaries, ok := session["phaseSummaries"].([]any); ok {
		for _, item := range summaries {
			if row, ok := item.(map[string]any); ok {
				result.PhaseSummaries = append(result.PhaseSummaries, phaseSummaryFromFixtureMap(row))
			}
		}
	}
	if summary, ok := session["resultSummary"].(map[string]any); ok {
		result.ResultSummary = &ResultSummary{
			ResultStatus: fixtureStringValue(summary, "resultStatus"),
			Summary:      fixtureStringValue(summary, "summary"),
		}
	}
	if failure, ok := session["failure"].(map[string]any); ok {
		result.Failure = &FailureSummary{
			Reason:                 fixtureStringValue(failure, "reason"),
			Message:                fixtureStringValue(failure, "message"),
			ErrorClass:             fixtureStringValue(failure, "errorClass"),
			PartialResultAvailable: fixtureBoolValue(failure, "partialResultAvailable"),
		}
	}
	if lifecycle, ok := session["lifecycle"].(map[string]any); ok {
		result.Lifecycle = lifecycleTimestampsFromFixtureMap(lifecycle)
	}
	if refs, ok := session["artifactRefs"].([]any); ok {
		for _, item := range refs {
			if row, ok := item.(map[string]any); ok {
				result.ArtifactRefs = append(result.ArtifactRefs, artifactRefFromFixtureMap(row))
			}
		}
	}
	result.ArtifactCount = len(result.ArtifactRefs)
	if links, ok := session["links"].(map[string]any); ok {
		result.Links = inspectionLinksFromFixtureMap(links)
	} else if result.SessionID != "" {
		result.Links = InspectionLinksForSession(result.SessionID, true)
	}
	return result
}

func resultReadFromFixtureMap(result map[string]any) ResultReadResult {
	out := ResultReadResult{
		SessionID:     fixtureStringValue(result, "sessionId"),
		ResultStatus:  ResultStatus(fixtureStringValue(result, "resultStatus")),
		SessionStatus: LifecycleStatus(fixtureStringValue(result, "sessionStatus")),
		Mode:          ResultMode(fixtureStringValue(result, "mode")),
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
				out.ArtifactRefs = append(out.ArtifactRefs, artifactRefFromFixtureMap(row))
			}
		}
	}
	if failure, ok := result["failure"].(map[string]any); ok {
		out.Failure = &FailureSummary{
			Reason:                 fixtureStringValue(failure, "reason"),
			Message:                fixtureStringValue(failure, "message"),
			ErrorClass:             fixtureStringValue(failure, "errorClass"),
			PartialResultAvailable: fixtureBoolValue(failure, "partialResultAvailable"),
		}
	}
	if availability, ok := result["availability"].(map[string]any); ok {
		out.Availability = &ResultAvailabilityDetail{
			Reason:    fixtureStringValue(availability, "reason"),
			Message:   fixtureStringValue(availability, "message"),
			Retryable: fixtureBoolValue(availability, "retryable"),
		}
	}
	return out
}

func durableListSummaryFromFixtureMap(summary map[string]any) DurableSessionListSummary {
	row := DurableSessionListSummary{
		SessionID:        fixtureStringValue(summary, "sessionId"),
		Status:           LifecycleStatus(fixtureStringValue(summary, "status")),
		OrchestratorKind: fixtureStringValue(summary, "orchestratorKind"),
		Dialect:          fixtureStringValue(summary, "dialect"),
		SourceHash:       fixtureStringValue(summary, "sourceHash"),
		Phase:            fixtureStringValue(summary, "phase"),
		StaleLease:       fixtureBoolValue(summary, "staleLease"),
	}
	if resolved, ok := summary["resolvedSource"].(map[string]any); ok {
		row.ResolvedSource = resolvedSourceFromFixtureMap(resolved)
	}
	if requested, ok := summary["requestedPolicy"].(map[string]any); ok {
		row.Policy.Requested = cloneFixtureMap(requested)
	}
	if effective, ok := summary["effectivePolicy"].(map[string]any); ok {
		row.Policy.Effective = cloneFixtureMap(effective)
	}
	row.Policy.EffectiveHash = fixtureStringValue(summary, "effectivePolicyHash")
	if progress, ok := summary["progress"].(map[string]any); ok {
		row.Progress = progressCountsFromFixtureMap(progress)
	}
	if resultSummary, ok := summary["resultSummary"].(map[string]any); ok {
		row.ResultSummary = &ResultSummary{
			ResultStatus: fixtureStringValue(resultSummary, "resultStatus"),
			Summary:      fixtureStringValue(resultSummary, "summary"),
		}
	}
	row.Recoverable = IsRecoverableSession(row.Status, row.StaleLease)
	row.Actions = DeriveSessionActionAvailability(row.Status)
	if row.SessionID != "" {
		row.Links = InspectionLinksForSession(row.SessionID, true)
	}
	return row
}

func dispatchSummaryFromFixtureMap(dispatch map[string]any) DispatchSummary {
	summary := DispatchSummary{
		ID:           fixtureStringValue(dispatch, "id"),
		Status:       DispatchStatus(fixtureStringValue(dispatch, "status")),
		DispatchKind: fixtureStringValue(dispatch, "dispatchKind"),
		Phase:        fixtureStringValue(dispatch, "phase"),
		Label:        fixtureStringValue(dispatch, "label"),
		Attempt:      fixtureIntValue(dispatch, "attempt"),
		RunnerID:     fixtureStringValue(dispatch, "runnerId"),
		Model:        fixtureStringValue(dispatch, "model"),
		Provider:     fixtureStringValue(dispatch, "provider"),
	}
	if ids, ok := dispatch["outputArtifactIds"].([]any); ok {
		for _, item := range ids {
			if value, ok := item.(string); ok {
				summary.OutputArtifactIDs = append(summary.OutputArtifactIDs, value)
			}
		}
	}
	if failure, ok := dispatch["failureDetail"].(map[string]any); ok {
		summary.FailureDetail = &DispatchFailureDetail{
			Reason:     fixtureStringValue(failure, "reason"),
			Message:    fixtureStringValue(failure, "message"),
			ErrorClass: fixtureStringValue(failure, "errorClass"),
		}
	}
	return summary
}

func dispatchDetailFromFixtureMap(dispatch map[string]any) DispatchDetail {
	summary := dispatchSummaryFromFixtureMap(dispatch)
	detail := DispatchDetail{
		DispatchSummary:  summary,
		SessionID:        fixtureStringValue(dispatch, "sessionId"),
		OrchestratorKind: fixtureStringValue(dispatch, "orchestratorKind"),
	}
	if ids, ok := dispatch["artifactIds"].([]any); ok {
		for _, item := range ids {
			if value, ok := item.(string); ok {
				detail.ArtifactIDs = append(detail.ArtifactIDs, value)
			}
		}
	}
	if petri, ok := dispatch["petri"].(map[string]any); ok {
		detail.Petri = &DispatchPetriProjection{
			TransitionID:    fixtureStringValue(petri, "transitionId"),
			WorkstationName: fixtureStringValue(petri, "workstationName"),
			WorkerType:      fixtureStringValue(petri, "workerType"),
		}
	}
	if javascript, ok := dispatch["javascript"].(map[string]any); ok {
		detail.JavaScript = &DispatchJavaScriptProjection{
			TaskKind:  fixtureStringValue(javascript, "taskKind"),
			TaskLabel: fixtureStringValue(javascript, "taskLabel"),
		}
	}
	return detail
}

func artifactSummaryFromFixtureMap(artifact map[string]any) ArtifactSummary {
	summary := ArtifactSummary{
		ID:          fixtureStringValue(artifact, "id"),
		Kind:        fixtureStringValue(artifact, "kind"),
		Visibility:  fixtureStringValue(artifact, "visibility"),
		Label:       fixtureStringValue(artifact, "label"),
		ContentHash: fixtureStringValue(artifact, "contentHash"),
		SizeBytes:   fixtureInt64Value(artifact, "sizeBytes"),
		DispatchID:  fixtureStringValue(artifact, "dispatchId"),
		AuditMode:   fixtureStringValue(artifact, "auditMode"),
	}
	if createdAt := fixtureTimeValue(artifact, "createdAt"); createdAt != nil {
		summary.CreatedAt = createdAt
	}
	if ref, ok := artifact["retrievalRef"].(map[string]any); ok {
		summary.RetrievalRef = &ArtifactRetrievalRef{
			Href:   fixtureStringValue(ref, "href"),
			Method: fixtureStringValue(ref, "method"),
		}
	}
	return summary
}

func artifactDetailFromFixtureMap(artifact map[string]any) ArtifactDetail {
	detail := ArtifactDetail{
		ArtifactSummary: artifactSummaryFromFixtureMap(artifact),
		SessionID:       fixtureStringValue(artifact, "sessionId"),
		Summary:         fixtureStringValue(artifact, "summary"),
	}
	if content, ok := artifact["content"]; ok {
		encoded, err := json.Marshal(content)
		if err == nil {
			detail.Content = encoded
		}
	}
	if ref, ok := artifact["contentRef"].(map[string]any); ok {
		detail.ContentRef = &ArtifactRetrievalRef{
			Href:   fixtureStringValue(ref, "href"),
			Method: fixtureStringValue(ref, "method"),
		}
	}
	return detail
}

func resolvedSourceFromFixtureMap(source map[string]any) ResolvedSource {
	result := ResolvedSource{
		Kind:       workflowsource.Kind(fixtureStringValue(source, "kind")),
		SourceRef:  fixtureStringValue(source, "sourceRef"),
		SourceHash: fixtureStringValue(source, "sourceHash"),
		Dialect:    fixtureStringValue(source, "dialect"),
	}
	if order, ok := source["resolutionOrder"].([]any); ok {
		for _, item := range order {
			if value, ok := item.(string); ok {
				result.ResolutionOrder = append(result.ResolutionOrder, value)
			}
		}
	}
	if metadata, ok := source["metadata"].(map[string]any); ok {
		result.Metadata = make(map[string]string, len(metadata))
		for key, value := range metadata {
			if typed, ok := value.(string); ok {
				result.Metadata[key] = typed
			}
		}
	}
	return result
}

func progressCountsFromFixtureMap(progress map[string]any) *ProgressCounts {
	return &ProgressCounts{
		TotalDispatches:     fixtureIntValue(progress, "totalDispatches"),
		CompletedDispatches: fixtureIntValue(progress, "completedDispatches"),
		FailedDispatches:    fixtureIntValue(progress, "failedDispatches"),
		InFlightDispatches:  fixtureIntValue(progress, "inFlightDispatches"),
		PhaseCount:          fixtureIntValue(progress, "phaseCount"),
	}
}

func phaseSummaryFromFixtureMap(summary map[string]any) PhaseSummary {
	return PhaseSummary{
		Phase:                  fixtureStringValue(summary, "phase"),
		Label:                  fixtureStringValue(summary, "label"),
		DispatchCount:          fixtureIntValue(summary, "dispatchCount"),
		CompletedDispatchCount: fixtureIntValue(summary, "completedDispatchCount"),
		FailedDispatchCount:    fixtureIntValue(summary, "failedDispatchCount"),
	}
}

func lifecycleTimestampsFromFixtureMap(lifecycle map[string]any) *LifecycleTimestamps {
	out := &LifecycleTimestamps{}
	if value := fixtureTimeValue(lifecycle, "queuedAt"); value != nil {
		out.QueuedAt = value
	}
	if value := fixtureTimeValue(lifecycle, "startedAt"); value != nil {
		out.StartedAt = value
	}
	if value := fixtureTimeValue(lifecycle, "finishedAt"); value != nil {
		out.FinishedAt = value
	}
	if value := fixtureTimeValue(lifecycle, "interruptedAt"); value != nil {
		out.InterruptedAt = value
	}
	if value := fixtureTimeValue(lifecycle, "updatedAt"); value != nil {
		out.UpdatedAt = value
	}
	return out
}

func artifactRefFromFixtureMap(ref map[string]any) ArtifactRefSummary {
	return ArtifactRefSummary{
		ID:          fixtureStringValue(ref, "id"),
		Kind:        fixtureStringValue(ref, "kind"),
		Visibility:  fixtureStringValue(ref, "visibility"),
		ContentHash: fixtureStringValue(ref, "contentHash"),
		SizeBytes:   fixtureInt64Value(ref, "sizeBytes"),
	}
}

func inspectionLinksFromFixtureMap(links map[string]any) InspectionLinks {
	return InspectionLinks{
		Session:    fixtureStringValue(links, "session"),
		Status:     fixtureStringValue(links, "status"),
		Events:     fixtureStringValue(links, "events"),
		Results:    fixtureStringValue(links, "results"),
		Dispatches: fixtureStringValue(links, "dispatches"),
		Artifacts:  fixtureStringValue(links, "artifacts"),
	}
}

func fixtureStringValue(document map[string]any, key string) string {
	value, ok := document[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func fixtureBoolValue(document map[string]any, key string) bool {
	value, ok := document[key].(bool)
	if !ok {
		return false
	}
	return value
}

func fixtureIntValue(document map[string]any, key string) int {
	switch typed := document[key].(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func fixtureInt64Value(document map[string]any, key string) int64 {
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

func fixtureTimeValue(document map[string]any, key string) *time.Time {
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
