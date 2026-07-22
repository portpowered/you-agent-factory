package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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

func previewRequestFromSourceConfig(
	cfg SourceConfig,
) (workflowsource.WorkflowPreviewInput, error) {
	projectRoot := strings.TrimSpace(cfg.Dir)
	if projectRoot == "" {
		return workflowsource.WorkflowPreviewInput{}, fmt.Errorf("project root is required")
	}

	sourceKind, err := workflowSourceKindFromCLI(cfg.SourceKind)
	if err != nil {
		return workflowsource.WorkflowPreviewInput{}, err
	}
	if err := validateSourceConfig(cfg, sourceKind); err != nil {
		return workflowsource.WorkflowPreviewInput{}, err
	}

	var argsSchema []byte
	if trimmed := strings.TrimSpace(cfg.ArgsSchema); trimmed != "" {
		argsSchema = []byte(trimmed)
	}

	var requestedPolicy map[string]any
	if trimmed := strings.TrimSpace(cfg.RequestedPolicyJSON); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &requestedPolicy); err != nil {
			return workflowsource.WorkflowPreviewInput{}, fmt.Errorf("requested policy must be valid JSON: %w", err)
		}
	}

	return workflowsource.WorkflowPreviewInput{
		ProjectRoot: projectRoot,
		Source: workflowsource.WorkflowSourceRequest{
			Kind:         sourceKind,
			Value:        strings.TrimSpace(cfg.SourceValue),
			InlineSource: strings.TrimSpace(cfg.InlineSource),
			ArtifactRoot: strings.TrimSpace(cfg.ArtifactRoot),
		},
		ArgsSchema:      argsSchema,
		RequestedPolicy: requestedPolicy,
	}, nil
}

func workflowSourceKindFromCLI(kind string) (workflowsource.WorkflowSourceKind, error) {
	switch workflowsource.WorkflowSourceKind(strings.TrimSpace(kind)) {
	case workflowsource.WorkflowSourceKindFactoryID,
		workflowsource.WorkflowSourceKindFactoryInline,
		workflowsource.WorkflowSourceKindWorkflowFile,
		workflowsource.WorkflowSourceKindWorkflowName,
		workflowsource.WorkflowSourceKindInlineWorkflow:
		return workflowsource.WorkflowSourceKind(strings.TrimSpace(kind)), nil
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

func validateSourceConfig(cfg SourceConfig, kind workflowsource.WorkflowSourceKind) error {
	value := strings.TrimSpace(cfg.SourceValue)
	inline := strings.TrimSpace(cfg.InlineSource)

	switch kind {
	case workflowsource.WorkflowSourceKindFactoryID:
		return validateRefSourceConfig("FACTORY_ID", value, inline)
	case workflowsource.WorkflowSourceKindWorkflowFile:
		return validateRefSourceConfig("WORKFLOW_FILE", value, inline)
	case workflowsource.WorkflowSourceKindWorkflowName:
		return validateRefSourceConfig("WORKFLOW_NAME", value, inline)
	case workflowsource.WorkflowSourceKindFactoryInline:
		return validateInlineOnlySourceConfig("FACTORY_INLINE", value, inline)
	case workflowsource.WorkflowSourceKindInlineWorkflow:
		return validateInlineOnlySourceConfig("INLINE_WORKFLOW", value, inline)
	}
	return nil
}
