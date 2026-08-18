package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/spf13/cobra"
)

// ValidateConfig holds parameters for workflow source validation output.
type ValidateConfig struct {
	SourceConfig
	Context context.Context
	JSON    bool
	Output  io.Writer
}

// ValidateRunE returns the handwritten workflow validation handler used by
// compatibility and generated command metadata wiring.
func ValidateRunE(
	preview factoryruntime.WorkflowPreviewOperation,
	cfg *ValidateConfig,
	jsonOutput *bool,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if jsonOutput != nil {
			cfg.JSON = *jsonOutput
		}
		cfg.Output = cmd.OutOrStdout()
		cfg.Context = cmd.Context()
		return Validate(preview, *cfg)
	}
}

// Validate resolves and validates workflow source using the shared validation contract.
func Validate(
	preview factoryruntime.WorkflowPreviewOperation,
	cfg ValidateConfig,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.Context == nil {
		return fmt.Errorf("workflow validation context is required")
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
	result := apisurface.FactoryWorkflowValidationResultFromPreview(workflowPreview)
	if cfg.JSON {
		if err := json.NewEncoder(cfg.Output).Encode(result); err != nil {
			return err
		}
		if !result.Valid {
			return fmt.Errorf("workflow validation found blocking issues")
		}
		return nil
	}
	return renderValidateHuman(result, cfg.Output)
}

func renderValidateHuman(result apisurface.FactoryWorkflowValidationResult, output io.Writer) error {
	if result.Valid {
		fmt.Fprintf(output, "Workflow validation passed.\n")
	} else {
		fmt.Fprintf(output, "Workflow validation failed.\n")
	}
	renderValidationSourceMetadata(result, output)
	if len(result.BlockingDiagnostics) > 0 {
		fmt.Fprintf(output, "Blocking diagnostics:\n")
		for _, diagnostic := range result.BlockingDiagnostics {
			fmt.Fprintf(output, "  %s\n", formatWorkflowDiagnostic(diagnostic))
		}
	}
	if !result.Valid {
		return fmt.Errorf("workflow validation found blocking issues")
	}
	return nil
}

func renderValidationSourceMetadata(result apisurface.FactoryWorkflowValidationResult, output io.Writer) {
	if ref := result.SourceResolution.SourceRef; ref != nil && strings.TrimSpace(*ref) != "" {
		fmt.Fprintf(output, "Source ref: %s\n", strings.TrimSpace(*ref))
	}
	if hash := result.SourceResolution.SourceHash; hash != nil && strings.TrimSpace(*hash) != "" {
		fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*hash))
	}
}
