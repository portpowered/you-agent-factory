package factorysession

import (
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListSessionsRequestFromAPI maps one public session list request into the shared
// service contract. Scope defaults to live for backward-compatible live workspace
// session listing.
func ListSessionsRequestFromAPI(params factoryapi.ListFactorySessionsParams) (factorysessionexecution.ListSessionsRequest, error) {
	req := factorysessionexecution.ListSessionsRequest{}
	if params.Scope != nil {
		req.Scope = factorysessionexecution.SessionListScope(*params.Scope)
	}
	return factorysessionexecution.NormalizeListSessionsRequest(req)
}

// ListSessionsResponseToAPI maps one scoped session list result to the public response shape.
func ListSessionsResponseToAPI(result factorysessionexecution.ListSessionsResult) factoryapi.ListFactorySessionsResponse {
	scope := factoryapi.FactorySessionListScope(result.Scope)
	response := factoryapi.ListFactorySessionsResponse{
		Scope:    &scope,
		Sessions: make([]factoryapi.FactorySessionSummary, 0, len(result.LiveSessions)),
	}
	for _, session := range result.LiveSessions {
		response.Sessions = append(response.Sessions, LiveSessionSummaryToAPI(session))
	}
	if len(result.DurableSessions) > 0 {
		durable := make([]factoryapi.FactorySessionDurableSummary, 0, len(result.DurableSessions))
		for _, summary := range result.DurableSessions {
			durable = append(durable, DurableSessionSummaryToAPI(summary))
		}
		response.DurableSessions = &durable
	}
	return response
}

// LiveSessionSummaryToAPI maps one live workspace session row to the public summary shape.
func LiveSessionSummaryToAPI(session factorysessionexecution.LiveSessionSummary) factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		Id:         session.ID,
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Project:    session.Project,
		IsDefault:  session.IsDefault,
		Target:     factoryapi.FactorySessionTargetRef{},
	}
}

// DurableSessionSummaryToAPI maps one durable session list row to the public summary shape.
func DurableSessionSummaryToAPI(summary factorysessionexecution.DurableSessionListSummary) factoryapi.FactorySessionDurableSummary {
	response := factoryapi.FactorySessionDurableSummary{
		SessionId:        summary.SessionID,
		Status:           factoryapi.FactorySessionDurableLifecycleStatus(summary.Status),
		OrchestratorKind: interfaces.GeneratedPublicFactoryOrchestratorKind(summary.OrchestratorKind),
		ResolvedSource:   resolvedSourceToAPI(summary.ResolvedSource),
	}
	if dialect := strings.TrimSpace(summary.Dialect); dialect != "" {
		response.Dialect = &dialect
	}
	if sourceHash := strings.TrimSpace(summary.SourceHash); sourceHash != "" {
		response.SourceHash = &sourceHash
	}
	if requested := policyToAPI(summary.Policy.Requested); requested != nil {
		response.RequestedPolicy = requested
	}
	if effective := effectivePolicyToAPI(summary.Policy.Effective); effective != nil {
		response.EffectivePolicy = effective
	}
	if effectiveHash := strings.TrimSpace(summary.Policy.EffectiveHash); effectiveHash != "" {
		response.EffectivePolicyHash = &effectiveHash
	}
	if phase := strings.TrimSpace(summary.Phase); phase != "" {
		response.Phase = &phase
	}
	if progress := progressCountsToAPI(summary.Progress); progress != nil {
		response.Progress = progress
	}
	if resultSummary := resultSummaryToAPI(summary.ResultSummary); resultSummary != nil {
		response.ResultSummary = resultSummary
	}
	if summary.ArtifactCount > 0 {
		count := summary.ArtifactCount
		response.ArtifactCount = &count
	}
	if summary.Recoverable {
		recoverable := true
		response.Recoverable = &recoverable
	}
	response.Actions = sessionActionAvailabilityToAPI(summary.Actions)
	if summary.StaleLease {
		stale := true
		response.StaleLease = &stale
	}
	if lifecycle := lifecycleTimestampsToAPI(summary.Lifecycle); lifecycle != nil {
		response.Lifecycle = lifecycle
	}
	if links := executionLinksToAPI(summary.Links); links != nil {
		response.Links = links
	}
	return response
}
