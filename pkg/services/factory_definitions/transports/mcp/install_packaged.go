package factorydefinition

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// InstallPackagedInput is the MCP request shape for you.factory_definition.install_packaged.
type InstallPackagedInput struct {
	Package string `json:"package"`
	Dir     string `json:"dir,omitempty"`
	Format  string `json:"format,omitempty"`
	Replace *bool  `json:"replace,omitempty"`
}

// InstallPackagedResult is the structured success payload for packaged installation.
type InstallPackagedResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
	Outcome    string `json:"outcome"`
	Format     string `json:"format"`
}

// InstallPackaged installs one built-in packaged Factory through the
// you.factory_definition.install_packaged MCP tool.
func InstallPackaged(
	ctx context.Context,
	install factorydefinitions.InstallPackagedFactoryOperation,
	input InstallPackagedInput,
) ToolResponse[InstallPackagedResult] {
	if ctx == nil {
		envelope := decodeInputErrorEnvelope("install packaged factory", errMissingRequestContext)
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[InstallPackagedResult](ctx); done {
		return response
	}
	if install == nil {
		envelope := unavailableInstallErrorEnvelope()
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Package) == "" {
		envelope := missingPackageIdentityErrorEnvelope()
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}

	rootDir, err := resolveInstallRootDir(ctx, input)
	if err != nil {
		envelope := decodeInputErrorEnvelope("resolve install destination", err)
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}
	format, err := parseInstallFormat(input.Format)
	if err != nil {
		envelope := decodeInputErrorEnvelope("parse install format", err)
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}
	replace := false
	if input.Replace != nil {
		replace = *input.Replace
	}

	result, err := install(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: rootDir,
			Name:    strings.TrimSpace(input.Package),
			Format:  format,
			Replace: replace,
		},
	)
	if err != nil {
		envelope := installPackagedErrorEnvelope(err)
		return ToolResponse[InstallPackagedResult]{Error: &envelope}
	}

	payload := InstallPackagedResult{
		Name:       result.Definition.Name,
		FactoryDir: result.Definition.FactoryDir,
		Outcome:    string(result.Outcome),
		Format:     string(result.Format),
	}
	return ToolResponse[InstallPackagedResult]{Result: &payload}
}

func resolveInstallRootDir(ctx context.Context, input InstallPackagedInput) (string, error) {
	trimmedDir := strings.TrimSpace(input.Dir)
	if trimmedDir != "" {
		if filepath.IsAbs(trimmedDir) {
			return trimmedDir, nil
		}
		workingDirectory := ""
		if ctx != nil {
			workingDirectory = strings.TrimSpace(startupcli.WorkingDirectory(ctx))
		}
		if workingDirectory == "" {
			return "", fmt.Errorf("process working directory is required for relative destination %q", trimmedDir)
		}
		return filepath.Join(workingDirectory, trimmedDir), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home directory is required")
	}
	return factorydefinitions.NamedFactoriesRoot(homeDir), nil
}

func parseInstallFormat(raw string) (factorydefinitions.PackagedFactoryFormat, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	switch strings.ToLower(trimmed) {
	case "json":
		return factorydefinitions.PackagedFactoryFormatJSON, nil
	case "yaml":
		return factorydefinitions.PackagedFactoryFormatYAML, nil
	case "yml":
		return factorydefinitions.PackagedFactoryFormatYML, nil
	default:
		return "", fmt.Errorf(
			"unsupported format %q; accepted values are json, yaml, and yml",
			raw,
		)
	}
}
