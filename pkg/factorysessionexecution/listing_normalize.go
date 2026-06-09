package factorysessionexecution

import (
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// NormalizeListSessionsRequest validates and normalizes one scoped session list request.
func NormalizeListSessionsRequest(req ListSessionsRequest) (ListSessionsRequest, error) {
	scope := req.Scope
	if scope == "" {
		scope = DefaultSessionListScope
	}
	switch scope {
	case SessionListScopeLive, SessionListScopePersisted, SessionListScopeAll:
	default:
		return ListSessionsRequest{}, NewValidationError("scope", "scope must be live, persisted, or all")
	}

	filters, err := normalizeSessionListFilters(req.Filters)
	if err != nil {
		return ListSessionsRequest{}, err
	}
	return ListSessionsRequest{
		Scope:   scope,
		Filters: filters,
	}, nil
}

func normalizeSessionListFilters(filters SessionListFilters) (SessionListFilters, error) {
	normalized := SessionListFilters{
		SourceKind:      filters.SourceKind,
		SourceRef:       strings.TrimSpace(filters.SourceRef),
		ProjectBoundary: strings.TrimSpace(filters.ProjectBoundary),
		Recoverable:     filters.Recoverable,
		StaleLease:      filters.StaleLease,
		CreatedAfter:    filters.CreatedAfter,
		CreatedBefore:   filters.CreatedBefore,
		UpdatedAfter:    filters.UpdatedAfter,
		UpdatedBefore:   filters.UpdatedBefore,
	}
	if len(filters.Statuses) > 0 {
		normalized.Statuses = append([]LifecycleStatus(nil), filters.Statuses...)
	}
	if len(filters.OrchestratorKinds) > 0 {
		normalized.OrchestratorKinds = make([]string, 0, len(filters.OrchestratorKinds))
		for _, kind := range filters.OrchestratorKinds {
			trimmed := strings.TrimSpace(kind)
			if trimmed != "" {
				normalized.OrchestratorKinds = append(normalized.OrchestratorKinds, trimmed)
			}
		}
	}
	if filters.SourceKind != "" && !isKnownWorkflowSourceKind(filters.SourceKind) {
		return SessionListFilters{}, NewValidationError("filters.sourceKind", "unsupported source kind")
	}
	if err := validateTimeRange("filters.created", normalized.CreatedAfter, normalized.CreatedBefore); err != nil {
		return SessionListFilters{}, err
	}
	if err := validateTimeRange("filters.updated", normalized.UpdatedAfter, normalized.UpdatedBefore); err != nil {
		return SessionListFilters{}, err
	}
	return normalized, nil
}

func isKnownWorkflowSourceKind(kind workflowsource.Kind) bool {
	switch kind {
	case workflowsource.KindFactoryID,
		workflowsource.KindFactoryInline,
		workflowsource.KindWorkflowFile,
		workflowsource.KindWorkflowName,
		workflowsource.KindInlineWorkflow:
		return true
	default:
		return false
	}
}

func validateTimeRange(field string, after, before *time.Time) error {
	if after != nil && before != nil && after.After(*before) {
		return NewValidationError(field, "after must be before or equal to before")
	}
	return nil
}
