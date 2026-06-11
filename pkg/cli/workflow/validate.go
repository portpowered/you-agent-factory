package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// ValidateConfig holds parameters for workflow source validation output.
type ValidateConfig struct {
	SourceConfig
	JSON   bool
	Output io.Writer
}

// Validate resolves and validates workflow source using the shared validation contract.
func Validate(cfg ValidateConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	input, err := previewRequestFromSourceConfig(cfg.SourceConfig)
	if err != nil {
		return err
	}

	result := apisurface.FactoryWorkflowValidationResultFromPreview(
		apisurface.BuildFactoryWorkflowValidation(input),
	)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
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
	for _, diagnostic := range result.BlockingDiagnostics {
		fmt.Fprintf(output, "%s\n", formatWorkflowDiagnostic(diagnostic))
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
