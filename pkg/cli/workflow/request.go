package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// SourceConfig holds shared workflow source CLI inputs.
type SourceConfig struct {
	Dir                 string
	SourceKind          string
	SourceValue         string
	InlineSource        string
	ArtifactRoot        string
	ArgsSchema          string
	RequestedPolicyJSON string
}

func previewRequestFromSourceConfig(cfg SourceConfig) (workflowpreview.Request, error) {
	projectRoot := strings.TrimSpace(cfg.Dir)
	if projectRoot == "" {
		return workflowpreview.Request{}, fmt.Errorf("project root is required")
	}
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return workflowpreview.Request{}, err
	}

	sourceKind, err := workflowSourceKindFromCLI(cfg.SourceKind)
	if err != nil {
		return workflowpreview.Request{}, err
	}

	var argsSchema []byte
	if trimmed := strings.TrimSpace(cfg.ArgsSchema); trimmed != "" {
		argsSchema = []byte(trimmed)
	}

	var requestedPolicy map[string]any
	if trimmed := strings.TrimSpace(cfg.RequestedPolicyJSON); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &requestedPolicy); err != nil {
			return workflowpreview.Request{}, fmt.Errorf("requested policy must be valid JSON: %w", err)
		}
	}

	return workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:         sourceKind,
			Value:        strings.TrimSpace(cfg.SourceValue),
			InlineSource: strings.TrimSpace(cfg.InlineSource),
			ArtifactRoot: strings.TrimSpace(cfg.ArtifactRoot),
		},
		Context:         ctx,
		ArgsSchema:      argsSchema,
		RequestedPolicy: requestedPolicy,
	}, nil
}

func workflowSourceKindFromCLI(kind string) (workflowsource.Kind, error) {
	switch workflowsource.Kind(strings.TrimSpace(kind)) {
	case workflowsource.KindFactoryID,
		workflowsource.KindFactoryInline,
		workflowsource.KindWorkflowFile,
		workflowsource.KindWorkflowName,
		workflowsource.KindInlineWorkflow:
		return workflowsource.Kind(strings.TrimSpace(kind)), nil
	default:
		return "", fmt.Errorf("source kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW")
	}
}
