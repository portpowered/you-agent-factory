package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
)

// LoadFakeScenariosFromContractFixtures loads deterministic fake scenarios from the
// durable session contract fixture catalog.
func LoadFakeScenariosFromContractFixtures(
	path string,
	files fileeffects.ContractFixtureReader,
) ([]FakeScenario, error) {
	if files == nil {
		return nil, fmt.Errorf("Factory Session contract fixture file reader is required")
	}
	raw, err := files.ReadFile(path)
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

// pkgmaintcheck:ignore-cyclomatic-complexity this fixture parser keeps durable fake scenario projection fields together for contract-backed tests.
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
		AsyncStartResult:         asyncStart,
		SyncOutcome:              SyncOutcome(fixtureStringValue(document, "syncOutcome")),
		TimedOut:                 fixtureBoolValue(document, "timedOut"),
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

// pkgmaintcheck:ignore-cyclomatic-complexity this fixture mapper keeps durable session read projections together for fake-service seeding.
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
	if budgets, ok := session["budgets"].(map[string]any); ok {
		result.Budgets = sessionBudgetsFromFixtureMap(budgets)
	}
	if usage, ok := session["usage"].(map[string]any); ok {
		result.Usage = sessionUsageFromFixtureMap(usage)
	} else {
		result.Usage = EmptySessionUsage()
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
	if failure, ok := session["failureDetail"].(map[string]any); ok {
		result.Failure = &FailureSummary{
			Reason:                 fixtureStringValue(failure, "reason"),
			Message:                fixtureStringValue(failure, "message"),
			PartialResultAvailable: fixtureBoolValue(session, "partialResultAvailable"),
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
	if failure, ok := result["failureDetail"].(map[string]any); ok {
		out.Failure = &FailureSummary{
			Reason:                 fixtureStringValue(failure, "reason"),
			Message:                fixtureStringValue(failure, "message"),
			PartialResultAvailable: fixtureBoolValue(result, "partialResultAvailable"),
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
	if artifactCount, ok := summary["artifactCount"].(float64); ok {
		row.ArtifactCount = int(artifactCount)
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
	if refs, ok := dispatch["providerSessionRefs"].([]any); ok {
		for _, item := range refs {
			if row, ok := item.(map[string]any); ok {
				summary.ProviderSessionRefs = append(summary.ProviderSessionRefs, providerSessionRefFromFixtureMap(row))
			}
		}
	}
	if usage, ok := dispatch["usage"].(map[string]any); ok {
		summary.Usage = dispatchUsageFromFixtureMap(usage)
	}
	if failure, ok := dispatch["failureDetail"].(map[string]any); ok {
		summary.FailureDetail = &DispatchFailureDetail{
			Reason:  fixtureStringValue(failure, "reason"),
			Message: fixtureStringValue(failure, "message"),
		}
	}
	if javascript, ok := dispatch["javascript"].(map[string]any); ok {
		summary.JavaScript = &DispatchJavaScriptProjection{
			TaskKind:      fixtureStringValue(javascript, "taskKind"),
			TaskLabel:     fixtureStringValue(javascript, "taskLabel"),
			ExecutionMode: fixtureStringValue(javascript, "executionMode"),
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
			TaskKind:      fixtureStringValue(javascript, "taskKind"),
			TaskLabel:     fixtureStringValue(javascript, "taskLabel"),
			ExecutionMode: fixtureStringValue(javascript, "executionMode"),
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
		Kind:       workflowsource.WorkflowSourceKind(fixtureStringValue(source, "kind")),
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
		TotalDispatches:       fixtureIntValue(progress, "totalDispatches"),
		CompletedDispatches:   fixtureIntValue(progress, "completedDispatches"),
		FailedDispatches:      fixtureIntValue(progress, "failedDispatches"),
		InFlightDispatches:    fixtureIntValue(progress, "inFlightDispatches"),
		QueuedDispatches:      fixtureIntValue(progress, "queuedDispatches"),
		RunningDispatches:     fixtureIntValue(progress, "runningDispatches"),
		CanceledDispatches:    fixtureIntValue(progress, "canceledDispatches"),
		TimedOutDispatches:    fixtureIntValue(progress, "timedOutDispatches"),
		SkippedDispatches:     fixtureIntValue(progress, "skippedDispatches"),
		InterruptedDispatches: fixtureIntValue(progress, "interruptedDispatches"),
		PhaseCount:            fixtureIntValue(progress, "phaseCount"),
	}
}

func sessionBudgetsFromFixtureMap(budgets map[string]any) *SessionBudgets {
	if maxAgents := fixtureIntValue(budgets, "maxAgents"); maxAgents > 0 {
		return &SessionBudgets{MaxAgents: maxAgents}
	}
	return nil
}

func sessionUsageFromFixtureMap(usage map[string]any) SessionUsage {
	out := EmptySessionUsage()
	rows, ok := usage["resources"].([]any)
	if !ok {
		return out
	}
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out.Resources = append(out.Resources, ResourceUsage{
			Name:      fixtureStringValue(row, "name"),
			Available: fixtureIntValue(row, "available"),
			Total:     fixtureIntValue(row, "total"),
		})
	}
	return out
}

func providerSessionRefFromFixtureMap(ref map[string]any) ProviderSessionRef {
	return ProviderSessionRef{
		Provider: fixtureStringValue(ref, "provider"),
		Kind:     fixtureStringValue(ref, "kind"),
		ID:       fixtureStringValue(ref, "id"),
	}
}

func dispatchUsageFromFixtureMap(usage map[string]any) *DispatchUsage {
	out := &DispatchUsage{}
	if value := fixtureInt64Value(usage, "inputTokens"); value > 0 {
		out.InputTokens = value
	}
	if value := fixtureInt64Value(usage, "outputTokens"); value > 0 {
		out.OutputTokens = value
	}
	if value := fixtureInt64Value(usage, "totalTokens"); value > 0 {
		out.TotalTokens = value
	}
	if value := fixtureInt64Value(usage, "durationMillis"); value > 0 {
		out.DurationMillis = value
	}
	if value, ok := usage["costUsd"].(float64); ok && value > 0 {
		out.CostUSD = value
	}
	if value := fixtureIntValue(usage, "retryCount"); value > 0 {
		out.RetryCount = int32(value)
	}
	if out.InputTokens == 0 && out.OutputTokens == 0 && out.TotalTokens == 0 &&
		out.DurationMillis == 0 && out.CostUSD == 0 && out.RetryCount == 0 {
		return nil
	}
	return out
}

func cloneSessionUsage(usage SessionUsage) SessionUsage {
	cloned := SessionUsage{Resources: make([]ResourceUsage, 0, len(usage.Resources))}
	cloned.Resources = append(cloned.Resources, usage.Resources...)
	return cloned
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
	if value := fixtureTimeValue(lifecycle, "pausedAt"); value != nil {
		out.PausedAt = value
	}
	if value := fixtureTimeValue(lifecycle, "resumedAt"); value != nil {
		out.ResumedAt = value
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

// FakeScenario is one deterministic durable-session projection bundle used by
// FakeService. Scenarios are keyed by execution requestId for start routing.
type FakeScenario struct {
	ID              string
	RequestID       string
	Session         SessionReadResult
	Dispatches      []DispatchSummary
	DispatchDetails map[string]DispatchDetail
	Artifacts       []ArtifactSummary
	ArtifactDetails map[string]ArtifactDetail
	Result          ResultReadResult
	Events          []json.RawMessage
	AsyncStart      *AsyncStartResult
	SyncStart       *SyncStartResult
	ListSummary     *DurableSessionListSummary
}

type fakeSessionState struct {
	scenarioID      string
	session         SessionReadResult
	dispatches      []DispatchSummary
	dispatchDetails map[string]DispatchDetail
	artifacts       []ArtifactSummary
	artifactDetails map[string]ArtifactDetail
	result          ResultReadResult
	events          []json.RawMessage
}

func fakeSessionStateFromScenario(scenario FakeScenario) *fakeSessionState {
	state := &fakeSessionState{
		scenarioID:      scenario.ID,
		session:         cloneSessionRead(scenario.Session),
		dispatches:      cloneDispatchSummaries(scenario.Dispatches),
		dispatchDetails: cloneDispatchDetails(scenario.DispatchDetails),
		artifacts:       cloneArtifactSummaries(scenario.Artifacts),
		artifactDetails: cloneArtifactDetails(scenario.ArtifactDetails),
		result:          cloneResultRead(scenario.Result),
		events:          append([]json.RawMessage(nil), scenario.Events...),
	}
	if len(state.events) == 0 {
		state.events = deriveProjectionEvents(state.session, state.result)
	}
	return state
}

func cloneSessionRead(session SessionReadResult) SessionReadResult {
	cloned := session
	cloned.ResolvedSource = cloneResolvedSource(session.ResolvedSource)
	cloned.Policy = clonePolicyProjection(session.Policy)
	if session.Progress != nil {
		progress := *session.Progress
		cloned.Progress = &progress
	}
	if session.ResultSummary != nil {
		summary := *session.ResultSummary
		cloned.ResultSummary = &summary
	}
	if session.Failure != nil {
		failure := *session.Failure
		cloned.Failure = &failure
	}
	if session.Lifecycle != nil {
		lifecycle := *session.Lifecycle
		cloned.Lifecycle = &lifecycle
	}
	if session.Budgets != nil {
		budgets := *session.Budgets
		cloned.Budgets = &budgets
	}
	cloned.Usage = cloneSessionUsage(session.Usage)
	cloned.PhaseSummaries = append([]PhaseSummary(nil), session.PhaseSummaries...)
	if session.LatestCheckpoint != nil {
		checkpoint := *session.LatestCheckpoint
		cloned.LatestCheckpoint = &checkpoint
	}
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), session.ArtifactRefs...)
	cloned.Links = session.Links
	return cloned
}

func cloneResolvedSource(source ResolvedSource) ResolvedSource {
	cloned := source
	cloned.ResolutionOrder = append([]string(nil), source.ResolutionOrder...)
	if len(source.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(source.Metadata))
		for key, value := range source.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func clonePolicyProjection(policy PolicyProjection) PolicyProjection {
	cloned := PolicyProjection{
		EffectiveHash: policy.EffectiveHash,
	}
	if len(policy.Requested) > 0 {
		cloned.Requested = cloneArgs(policy.Requested)
	}
	if len(policy.Effective) > 0 {
		cloned.Effective = cloneArgs(policy.Effective)
	}
	return cloned
}

func cloneDispatchSummaries(dispatches []DispatchSummary) []DispatchSummary {
	if len(dispatches) == 0 {
		return nil
	}
	cloned := make([]DispatchSummary, len(dispatches))
	copy(cloned, dispatches)
	return cloned
}

func cloneDispatchDetails(details map[string]DispatchDetail) map[string]DispatchDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]DispatchDetail, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}

func cloneArtifactSummaries(artifacts []ArtifactSummary) []ArtifactSummary {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]ArtifactSummary, len(artifacts))
	copy(cloned, artifacts)
	return cloned
}

func cloneArtifactDetails(details map[string]ArtifactDetail) map[string]ArtifactDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]ArtifactDetail, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}

func cloneResultRead(result ResultReadResult) ResultReadResult {
	cloned := result
	if len(result.PrimaryResult) > 0 {
		cloned.PrimaryResult = append(json.RawMessage(nil), result.PrimaryResult...)
	}
	cloned.ArtifactIDs = append([]string(nil), result.ArtifactIDs...)
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), result.ArtifactRefs...)
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	if result.Availability != nil {
		availability := *result.Availability
		cloned.Availability = &availability
	}
	return cloned
}

func deriveProjectionEvents(session SessionReadResult, result ResultReadResult) []json.RawMessage {
	return BuildCanonicalSessionEvents(session, result)
}

// BuiltinInterruptedRecoverableScenario is a deterministic JavaScript session that
// was interrupted with a stale lease and remains recoverable for persisted listing.
func BuiltinInterruptedRecoverableScenario() FakeScenario {
	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 8, 10, 5, 0, 0, time.UTC)
	sessionID := "dur-sess-js-interrupted-001"
	links := InspectionLinksForSession(sessionID, true)
	session := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusInterrupted,
		OrchestratorKind: "JAVASCRIPT",
		Dialect:          "you-workflow-v1",
		ResolvedSource: ResolvedSource{
			Kind:       workflowsource.WorkflowSourceKindWorkflowName,
			SourceRef:  "workflow/recoverable-audit",
			SourceHash: "sha256:js-workflow-recoverable-audit",
			Dialect:    "you-workflow-v1",
		},
		SourceHash: "sha256:js-workflow-recoverable-audit",
		Phase:      "audit",
		Progress: &ProgressCounts{
			TotalDispatches:     2,
			CompletedDispatches: 1,
			FailedDispatches:    0,
			InFlightDispatches:  0,
		},
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusPartial),
			Summary:      "Interrupted after partial audit progress.",
		},
		StaleLease: true,
		Lifecycle: &LifecycleTimestamps{
			StartedAt:     &startedAt,
			InterruptedAt: &interruptedAt,
			UpdatedAt:     &interruptedAt,
		},
		Links: links,
	}
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusPartial,
		SessionStatus: LifecycleStatusInterrupted,
		Mode:          ResultModePartial,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"Partial audit notes before interruption."}]`),
	}
	dispatches := []DispatchSummary{
		{
			ID:           "disp-js-interrupted-001",
			Status:       DispatchStatusCompleted,
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "plan",
			Label:        "plan-audit",
			Attempt:      1,
		},
		{
			ID:           "disp-js-interrupted-002",
			Status:       DispatchStatusCanceled,
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        "audit",
			Label:        "audit",
			Attempt:      1,
		},
	}
	listSummary := DurableListSummaryFromSessionRead(session)
	return FakeScenario{
		ID:         "javascript-interrupted-recoverable",
		RequestID:  "req-js-interrupted-001",
		Session:    session,
		Dispatches: dispatches,
		DispatchDetails: map[string]DispatchDetail{
			"disp-js-interrupted-002": {
				DispatchSummary:  dispatches[1],
				SessionID:        sessionID,
				OrchestratorKind: "JAVASCRIPT",
				JavaScript: &DispatchJavaScriptProjection{
					TaskKind:  "AGENT",
					TaskLabel: "audit",
				},
			},
		},
		Result:      result,
		ListSummary: &listSummary,
		AsyncStart: &AsyncStartResult{
			SessionID:        sessionID,
			Status:           string(LifecycleStatusInterrupted),
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			ResolvedSource:   session.ResolvedSource,
			SourceHash:       session.SourceHash,
			Links:            links,
		},
	}
}
