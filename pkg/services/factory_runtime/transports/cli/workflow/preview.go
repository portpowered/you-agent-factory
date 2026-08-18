// Package workflow implements workflow validation and preview CLI behavior.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/spf13/cobra"
)

// PreviewConfig holds parameters for workflow preview output.
type PreviewConfig struct {
	SourceConfig
	Context context.Context
	JSON    bool
	Output  io.Writer
}

// PreviewRunE returns the handwritten workflow preview handler used by
// compatibility and generated command metadata wiring.
func PreviewRunE(
	preview factoryruntime.WorkflowPreviewOperation,
	cfg *PreviewConfig,
	jsonOutput *bool,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if jsonOutput != nil {
			cfg.JSON = *jsonOutput
		}
		cfg.Output = cmd.OutOrStdout()
		cfg.Context = cmd.Context()
		return Preview(preview, *cfg)
	}
}

// Preview resolves and validates workflow source, then prints shared preview diagnostics.
func Preview(
	preview factoryruntime.WorkflowPreviewOperation,
	cfg PreviewConfig,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.Context == nil {
		return fmt.Errorf("workflow preview context is required")
	}

	if preview == nil {
		return fmt.Errorf("workflow preview operation is required")
	}
	input, err := previewRequestFromSourceConfig(cfg.SourceConfig)
	if err != nil {
		return err
	}

	workflowPreview, err := preview.PreviewWorkflow(cfg.Context, input)
	if err != nil {
		return err
	}
	result := apisurface.FactoryPreviewResultFromPreview(workflowPreview)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return renderPreviewHuman(result, cfg.Output)
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
