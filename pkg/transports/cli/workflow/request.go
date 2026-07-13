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

	sourceKind, err := workflowSourceKindFromCLI(cfg.SourceKind)
	if err != nil {
		return workflowpreview.Request{}, err
	}
	if err := validateSourceConfig(cfg, sourceKind); err != nil {
		return workflowpreview.Request{}, err
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
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

func validateRefSourceConfig(kindLabel, value, inline string) error {
	if value == "" {
		return fmt.Errorf("value is required when kind is %s", kindLabel)
	}
	if inline != "" {
		return fmt.Errorf("--inline cannot be used when kind is %s", kindLabel)
	}
	return nil
}

func validateInlineOnlySourceConfig(kindLabel, value, inline string) error {
	if inline == "" {
		return fmt.Errorf("inline is required when kind is %s", kindLabel)
	}
	if value != "" {
		return fmt.Errorf("--value cannot be used when kind is %s", kindLabel)
	}
	return nil
}

func validateSourceConfig(cfg SourceConfig, kind workflowsource.Kind) error {
	value := strings.TrimSpace(cfg.SourceValue)
	inline := strings.TrimSpace(cfg.InlineSource)

	switch kind {
	case workflowsource.KindFactoryID:
		return validateRefSourceConfig("FACTORY_ID", value, inline)
	case workflowsource.KindWorkflowFile:
		return validateRefSourceConfig("WORKFLOW_FILE", value, inline)
	case workflowsource.KindWorkflowName:
		return validateRefSourceConfig("WORKFLOW_NAME", value, inline)
	case workflowsource.KindFactoryInline:
		return validateInlineOnlySourceConfig("FACTORY_INLINE", value, inline)
	case workflowsource.KindInlineWorkflow:
		return validateInlineOnlySourceConfig("INLINE_WORKFLOW", value, inline)
	}
	return nil
}
