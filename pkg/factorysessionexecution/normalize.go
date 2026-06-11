package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	jssource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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

func normalizeSource(src Source) (Source, error) {
	switch src.Kind {
	case jssource.KindFactoryID:
		factoryID := strings.TrimSpace(src.FactoryID)
		if factoryID == "" {
			return Source{}, NewValidationError("source.factoryId", "factoryId is required when source.kind is FACTORY_ID")
		}
		return Source{Kind: src.Kind, FactoryID: factoryID}, nil
	case jssource.KindFactoryInline:
		if len(src.FactoryInline) == 0 {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline is required when source.kind is FACTORY_INLINE")
		}
		canonical, err := canonicalizeRawJSON(src.FactoryInline)
		if err != nil {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline must be a JSON object")
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return Source{}, fmt.Errorf("marshal factoryInline: %w", err)
		}
		return Source{Kind: src.Kind, FactoryInline: encoded}, nil
	case jssource.KindWorkflowFile:
		workflowFile := strings.TrimSpace(src.WorkflowFile)
		if workflowFile == "" {
			return Source{}, NewValidationError("source.workflowFile", "workflowFile is required when source.kind is WORKFLOW_FILE")
		}
		return Source{Kind: src.Kind, WorkflowFile: workflowFile}, nil
	case jssource.KindWorkflowName:
		workflowName := strings.TrimSpace(src.WorkflowName)
		if workflowName == "" {
			return Source{}, NewValidationError("source.workflowName", "workflowName is required when source.kind is WORKFLOW_NAME")
		}
		return Source{Kind: src.Kind, WorkflowName: workflowName}, nil
	case jssource.KindInlineWorkflow:
		if src.InlineWorkflow == nil {
			return Source{}, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inlineSource := strings.TrimSpace(src.InlineWorkflow.InlineSource)
		if inlineSource == "" {
			return Source{}, NewValidationError("source.inlineWorkflow.inlineSource", "inlineSource is required when source.kind is INLINE_WORKFLOW")
		}
		return Source{
			Kind: src.Kind,
			InlineWorkflow: &InlineWorkflowSource{
				Dialect:      strings.TrimSpace(src.InlineWorkflow.Dialect),
				InlineSource: inlineSource,
				Entrypoint:   strings.TrimSpace(src.InlineWorkflow.Entrypoint),
				Metadata:     cloneStringMap(src.InlineWorkflow.Metadata),
			},
		}, nil
	default:
		return Source{}, NewValidationError("source.kind", "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW")
	}
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
