// Package workflow implements workflow validation and preview CLI behavior.
package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/workflowpreview"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// PreviewConfig holds parameters for workflow preview output.
type PreviewConfig struct {
	Dir         string
	SourceKind  string
	SourceValue string
	InlineSource string
	ArtifactRoot string
	JSON        bool
	Output      io.Writer
}

// Preview resolves and validates workflow source, then prints shared preview diagnostics.
func Preview(cfg PreviewConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	input, err := previewRequestFromConfig(cfg)
	if err != nil {
		return err
	}

	result := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(input))
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return renderPreviewHuman(result, cfg.Output)
}

func previewRequestFromConfig(cfg PreviewConfig) (workflowpreview.Request, error) {
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

func renderPreviewHuman(result factoryapi.FactoryPreviewResult, output io.Writer) error {
	if result.Valid {
		fmt.Fprintf(output, "Factory preview passed.\n")
	} else {
		fmt.Fprintf(output, "Factory preview failed.\n")
	}
	renderPreviewSourceMetadata(result, output)
	renderPreviewResolutionDiagnostics(result, output)
	renderPreviewValidationIssues(result, output)
	renderPreviewPolicyDiagnostics(result, output)
	fmt.Fprintf(output, "Result constraints: structured JSON required; artifact scheme %s; max embedded bytes %d\n",
		result.ResultConstraints.ArtifactUriScheme,
		result.ResultConstraints.MaxEmbeddedBytes,
	)
	if !result.Valid {
		return fmt.Errorf("factory preview found blocking issues")
	}
	return nil
}

func renderPreviewSourceMetadata(result factoryapi.FactoryPreviewResult, output io.Writer) {
	if ref := result.SourceResolution.SourceRef; ref != nil && strings.TrimSpace(*ref) != "" {
		fmt.Fprintf(output, "Source ref: %s\n", strings.TrimSpace(*ref))
	}
	if hash := result.SourceResolution.SourceHash; hash != nil && strings.TrimSpace(*hash) != "" {
		fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*hash))
	}
	fmt.Fprintf(output, "Policy hash: %s\n", strings.TrimSpace(result.PolicyPreview.PolicyHash))
}

func renderPreviewResolutionDiagnostics(result factoryapi.FactoryPreviewResult, output io.Writer) {
	if result.SourceResolution.Diagnostics != nil {
		for _, diagnostic := range *result.SourceResolution.Diagnostics {
			if diagnostic.Code != "" || diagnostic.Message != "" {
				fmt.Fprintf(output, "Source resolution: %s: %s\n", diagnostic.Code, diagnostic.Message)
			}
		}
	}
	if result.SourceResolution.ArtifactRoot != nil && result.SourceResolution.ArtifactRoot.Diagnostic != nil {
		diagnostic := result.SourceResolution.ArtifactRoot.Diagnostic
		fmt.Fprintf(output, "Artifact root: %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
}

func renderPreviewValidationIssues(result factoryapi.FactoryPreviewResult, output io.Writer) {
	for _, issue := range result.SourceValidationIssues {
		fmt.Fprintf(output, "%s\n", formatWorkflowDiagnostic(issue))
	}
}

func renderPreviewPolicyDiagnostics(result factoryapi.FactoryPreviewResult, output io.Writer) {
	for _, issue := range result.PolicyPreview.ValidationIssues {
		fmt.Fprintf(output, "%s\n", formatWorkflowDiagnostic(issue))
	}
	for _, diagnostic := range result.PolicyPreview.DeniedCapabilities {
		fmt.Fprintf(output, "Denied capability: %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
}

func formatWorkflowDiagnostic(issue factoryapi.WorkflowDiagnostic) string {
	location := ""
	if issue.Line != nil && *issue.Line > 0 {
		if issue.Column != nil && *issue.Column > 0 {
			location = fmt.Sprintf(" (line %d, column %d)", *issue.Line, *issue.Column)
		} else {
			location = fmt.Sprintf(" (line %d)", *issue.Line)
		}
	}
	path := ""
	if issue.Path != nil && strings.TrimSpace(*issue.Path) != "" {
		path = strings.TrimSpace(*issue.Path) + ": "
	}
	return fmt.Sprintf("%s%s%s: %s", path, issue.Code, location, issue.Message)
}
