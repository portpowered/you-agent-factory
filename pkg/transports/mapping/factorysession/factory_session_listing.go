package factorysession

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/logicaltarget"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// SessionSummaryToAPI maps one live Factory Session to its public summary.
func SessionSummaryToAPI(session *factorysessions.LiveSession) factoryapi.FactorySessionSummary {
	if session == nil {
		return factoryapi.FactorySessionSummary{}
	}
	return factoryapi.FactorySessionSummary{
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Id:         factorysessions.CanonicalFactorySessionID(session),
		IsDefault:  session.IsDefault,
		Project:    session.Project,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(session.Target.Kind),
			Name: optionalTrimmedString(session.Target.Name),
		},
	}
}

// SortSessionSummaries orders public summaries with default sessions first, then by id.
func SortSessionSummaries(summaries []factoryapi.FactorySessionSummary) {
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IsDefault != summaries[j].IsDefault {
			return summaries[i].IsDefault
		}
		return summaries[i].Id < summaries[j].Id
	})
}

// TargetToAPI maps one discovered Factory Session target to the public shape.
func TargetToAPI(target factorysessions.Target) factoryapi.FactorySessionTarget {
	return factoryapi.FactorySessionTarget{
		FactoryDir: target.FactoryDir,
		FolderPath: target.FolderPath,
		Label:      target.Label,
		Project:    target.Project,
		Ref: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(target.Ref.Kind),
			Name: optionalTrimmedString(target.Ref.Name),
		},
	}
}

// TargetsToAPI maps discovered Factory Session targets to the public shape.
func TargetsToAPI(targets []factorysessions.Target) []factoryapi.FactorySessionTarget {
	if len(targets) == 0 {
		return nil
	}
	response := make([]factoryapi.FactorySessionTarget, 0, len(targets))
	for _, target := range targets {
		response = append(response, TargetToAPI(target))
	}
	return response
}

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

// LogicalTargetToAPI maps one normalized Factory Session logical target to the
// public client-safe API shape.
func LogicalTargetToAPI(ref logicaltarget.CanonicalReference) factoryapi.FactorySessionLogicalTarget {
	target := factoryapi.FactorySessionLogicalTarget{
		Kind:       logicalTargetKindToAPI(ref.Kind),
		FolderPath: ref.FolderPath,
	}
	if ref.Kind == logicaltarget.KindNamed {
		namedTarget := ref.NamedTarget
		target.NamedTarget = &namedTarget
	}
	if ref.Kind == logicaltarget.KindProvider && ref.Provider != nil {
		target.ProviderBoundary = &factoryapi.FactorySessionLogicalProviderBoundary{
			Provider: ref.Provider.Provider,
			Kind:     ref.Provider.Kind,
			Boundary: ref.Provider.Boundary,
		}
	}
	return target
}

// LogicalTargetFromSession derives public normalized target metadata for one
// live Factory Session within backendScopeID.
func LogicalTargetFromSession(
	backendScopeID string,
	session *factorysessions.LiveSession,
) (*factoryapi.FactorySessionLogicalTarget, error) {
	if session == nil {
		return nil, nil
	}
	ref, err := logicaltarget.NormalizeTargetRef(backendScopeID, session.FolderPath, session.Target)
	if err != nil {
		return nil, err
	}
	target := LogicalTargetToAPI(ref)
	return &target, nil
}

func logicalTargetKindToAPI(kind logicaltarget.Kind) factoryapi.FactorySessionLogicalTargetKind {
	switch kind {
	case logicaltarget.KindNamed:
		return factoryapi.FactorySessionLogicalTargetKindNamed
	case logicaltarget.KindProvider:
		return factoryapi.FactorySessionLogicalTargetKindProvider
	default:
		return factoryapi.FactorySessionLogicalTargetKindDefault
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
