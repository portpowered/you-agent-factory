package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeStartRequest validates and normalizes one durable execution start request.
func NormalizeStartRequest(req StartRequest) (StartRequest, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return StartRequest{}, NewValidationError("requestId", "requestId is required")
	}

	source, err := normalizeSource(req.Source)
	if err != nil {
		return StartRequest{}, err
	}

	normalized := StartRequest{
		RequestID:       requestID,
		Source:          source,
		Args:            cloneArgs(req.Args),
		RequestedPolicy: cloneArgs(req.RequestedPolicy),
	}
	if req.Runtime != nil {
		runtime := *req.Runtime
		runtime.ChildExecutorMode = normalizeChildExecutorMode(runtime.ChildExecutorMode)
		normalized.Runtime = &runtime
	}
	if req.Orchestrator != nil {
		override := *req.Orchestrator
		override.Kind = strings.TrimSpace(override.Kind)
		if len(override.Raw) > 0 {
			canonical, err := canonicalizeRawJSON(override.Raw)
			if err != nil {
				return StartRequest{}, NewValidationError("orchestrator", "orchestrator must be a JSON object")
			}
			encoded, err := json.Marshal(canonical)
			if err != nil {
				return StartRequest{}, fmt.Errorf("marshal orchestrator: %w", err)
			}
			override.Raw = encoded
		}
		normalized.Orchestrator = &override
	}
	if req.Wait != nil {
		wait := *req.Wait
		normalized.Wait = &wait
	}
	return normalized, nil
}

func normalizeSource(source Source) (Source, error) {
	switch source.Kind {
	case workflowsource.KindFactoryID:
		factoryID := strings.TrimSpace(source.FactoryID)
		if factoryID == "" {
			return Source{}, NewValidationError("source.factoryId", "factoryId is required when source.kind is FACTORY_ID")
		}
		return Source{Kind: source.Kind, FactoryID: factoryID}, nil
	case workflowsource.KindFactoryInline:
		if len(source.FactoryInline) == 0 {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline is required when source.kind is FACTORY_INLINE")
		}
		canonical, err := canonicalizeRawJSON(source.FactoryInline)
		if err != nil {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline must be a JSON object")
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return Source{}, fmt.Errorf("marshal factoryInline: %w", err)
		}
		return Source{Kind: source.Kind, FactoryInline: encoded}, nil
	case workflowsource.KindWorkflowFile:
		workflowFile := strings.TrimSpace(source.WorkflowFile)
		if workflowFile == "" {
			return Source{}, NewValidationError("source.workflowFile", "workflowFile is required when source.kind is WORKFLOW_FILE")
		}
		return Source{Kind: source.Kind, WorkflowFile: workflowFile}, nil
	case workflowsource.KindWorkflowName:
		workflowName := strings.TrimSpace(source.WorkflowName)
		if workflowName == "" {
			return Source{}, NewValidationError("source.workflowName", "workflowName is required when source.kind is WORKFLOW_NAME")
		}
		return Source{Kind: source.Kind, WorkflowName: workflowName}, nil
	case workflowsource.KindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return Source{}, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inlineSource := strings.TrimSpace(source.InlineWorkflow.InlineSource)
		if inlineSource == "" {
			return Source{}, NewValidationError("source.inlineWorkflow.inlineSource", "inlineSource is required when source.kind is INLINE_WORKFLOW")
		}
		return Source{
			Kind: source.Kind,
			InlineWorkflow: &InlineWorkflowSource{
				Dialect:       strings.TrimSpace(source.InlineWorkflow.Dialect),
				InlineSource:  inlineSource,
				Entrypoint:    strings.TrimSpace(source.InlineWorkflow.Entrypoint),
				Metadata:      cloneStringMap(source.InlineWorkflow.Metadata),
				Agents:        cloneJavaScriptAgents(source.InlineWorkflow.Agents),
				ArgsSchema:    append(json.RawMessage(nil), source.InlineWorkflow.ArgsSchema...),
				DefaultPolicy: append(json.RawMessage(nil), source.InlineWorkflow.DefaultPolicy...),
			},
		}, nil
	default:
		return Source{}, NewValidationError("source.kind", "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW")
	}
}

func cloneJavaScriptAgents(agents map[string]interfaces.FactoryOrchestratorJavaScriptAgent) map[string]interfaces.FactoryOrchestratorJavaScriptAgent {
	if len(agents) == 0 {
		return nil
	}
	cloned := make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(agents))
	for name, agent := range agents {
		cloned[name] = agent
	}
	return cloned
}

func cloneArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func normalizeChildExecutorMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", ChildExecutorModeFake:
		return ChildExecutorModeFake
	case ChildExecutorModeLive:
		return ChildExecutorModeLive
	default:
		return strings.TrimSpace(mode)
	}
}

func resolveChildExecutorMode(configMode string, req StartRequest) string {
	if req.Runtime != nil && strings.TrimSpace(req.Runtime.ChildExecutorMode) != "" {
		return normalizeChildExecutorMode(req.Runtime.ChildExecutorMode)
	}
	return normalizeChildExecutorMode(configMode)
}

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
