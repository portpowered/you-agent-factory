// Package cli defines the Factory Definitions service-owned CLI adapter for
// packaged Factory installation.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// InstallPackagedFactoryOperation installs one built-in packaged Factory
// through the Definitions-owned distribute contract.
type InstallPackagedFactoryOperation = factorydefinitions.InstallPackagedFactoryOperation

// InstallPackagedFactoryConfig carries resolved CLI inputs for packaged
// Factory installation.
type InstallPackagedFactoryConfig struct {
	Context     context.Context
	HomeDir     string
	Package     string
	Dir         string
	DirChanged  bool
	Format      string
	FormatChanged bool
	Replace     bool
	JSON        bool
	Output      io.Writer
	Diagnostics io.Writer
	Verbose     bool
}

// InstallPackagedFactoryResult is the structured success payload for packaged
// initialization.
type InstallPackagedFactoryResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
	Outcome    string `json:"outcome"`
	Format     string `json:"format"`
}

// InstallPackagedFactory delegates packaged selection and materialization to
// Factory Definitions and renders the accepted CLI success or failure surfaces.
func InstallPackagedFactory(
	cfg InstallPackagedFactoryConfig,
	install InstallPackagedFactoryOperation,
) error {
	adapter := New(install)
	if adapter == nil {
		return fmt.Errorf("packaged factory installation service is required")
	}
	return adapter.InstallPackagedFactory(cfg)
}

func resolveInstallRootDir(cfg InstallPackagedFactoryConfig) (string, error) {
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		return "", fmt.Errorf("home directory is required")
	}
	if cfg.DirChanged {
		dir := strings.TrimSpace(cfg.Dir)
		if dir == "" {
			return "", fmt.Errorf("destination directory must be non-empty")
		}
		if filepath.IsAbs(dir) {
			return dir, nil
		}
		workingDirectory := ""
		if cfg.Context != nil {
			workingDirectory = strings.TrimSpace(workingDirectoryFromContext(cfg.Context))
		}
		if workingDirectory == "" {
			return "", fmt.Errorf("process working directory is required for relative destination %q", dir)
		}
		return filepath.Join(workingDirectory, dir), nil
	}
	return factorydefinitions.NamedFactoriesRoot(homeDir), nil
}

func parseInstallFormat(
	raw string,
	changed bool,
) (factorydefinitions.PackagedFactoryFormat, error) {
	trimmed := strings.TrimSpace(raw)
	if !changed || trimmed == "" {
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

func renderInstallPackagedFactoryError(err error) error {
	if err == nil {
		return nil
	}
	var unknown *factorydefinitions.UnknownPackagedFactoryError
	if errors.As(err, &unknown) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrIncompatibleFactoryDistributeOptions) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		return err
	}
	return fmt.Errorf("install packaged factory: %w", err)
}

func renderInstallPackagedFactorySuccess(
	cfg InstallPackagedFactoryConfig,
	result factorydefinitions.InstallPackagedFactoryResult,
) error {
	payload := InstallPackagedFactoryResult{
		Name:       result.Definition.Name,
		FactoryDir: result.Definition.FactoryDir,
		Outcome:    string(result.Outcome),
		Format:     string(result.Format),
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(payload)
	}
	switch result.Outcome {
	case factorydefinitions.PackagedFactoryInstallSkipped:
		_, err := fmt.Fprintf(
			cfg.Output,
			"Packaged factory %s is already installed at %s\n",
			payload.Name,
			payload.FactoryDir,
		)
		return err
	case factorydefinitions.PackagedFactoryInstallReplaced:
		_, err := fmt.Fprintf(
			cfg.Output,
			"Replaced packaged factory %s\nDirectory: %s\n",
			payload.Name,
			payload.FactoryDir,
		)
		return err
	default:
		_, err := fmt.Fprintf(
			cfg.Output,
			"Installed packaged factory %s\nDirectory: %s\n",
			payload.Name,
			payload.FactoryDir,
		)
		return err
	}
}

func diagnosticPrintf(output io.Writer, enabled bool, format string, args ...any) {
	if !enabled || output == nil {
		return
	}
	_, _ = fmt.Fprintf(output, format+"\n", args...)
}
