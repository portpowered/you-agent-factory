// Package initsetup adapts the public init command to atomic operator settings.
package initsetup

import (
	"context"
	"fmt"
	"io"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Config carries supplied provider/model setup inputs from the CLI boundary.
type Config struct {
	Context  context.Context
	HomeDir  string
	Provider string
	Model    *string
	Output   io.Writer
}

// NewConfigurer binds CLI setup rendering to the atomic operator-config service.
func NewConfigurer(service operatorsettings.ConfigDocumentService) func(Config) error {
	return func(cfg Config) error {
		return configure(cfg, service)
	}
}

func configure(cfg Config, service operatorsettings.ConfigDocumentService) error {
	if cfg.Context == nil {
		return fmt.Errorf("init context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("init output is required")
	}
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		return fmt.Errorf("init home directory is required")
	}
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		return fmt.Errorf("provider is required when interactive prompting is unavailable; use --provider")
	}
	var model *string
	if cfg.Model != nil {
		value := strings.TrimSpace(*cfg.Model)
		if value == "" {
			return fmt.Errorf("model must be non-empty when supplied")
		}
		model = &value
	}

	path := operatorsettings.DefaultConfigPath(homeDir)
	document, err := service.ConfigureProviderModel(
		cfg.Context,
		path,
		operatorsettings.ProviderModelUpdate{Provider: &provider, Model: model},
	)
	if err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	defaults := document.FileConfig().Defaults
	if defaults.WorkerModel == "" {
		_, err = fmt.Fprintf(cfg.Output, "Configured default provider %s in %s\n", defaults.WorkerModelProvider, path)
	} else {
		_, err = fmt.Fprintf(
			cfg.Output,
			"Configured default provider %s and model %s in %s\n",
			defaults.WorkerModelProvider,
			defaults.WorkerModel,
			path,
		)
	}
	return err
}
