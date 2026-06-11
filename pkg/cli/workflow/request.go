package workflow

import (
	"fmt"
	"strings"

	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// SourceConfig holds shared workflow source CLI inputs.
type SourceConfig struct {
	Dir          string
	SourceKind   string
	SourceValue  string
	InlineSource string
	ArtifactRoot string
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

	return workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:         sourceKind,
			Value:        strings.TrimSpace(cfg.SourceValue),
			InlineSource: strings.TrimSpace(cfg.InlineSource),
			ArtifactRoot: strings.TrimSpace(cfg.ArtifactRoot),
		},
		Context: ctx,
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
