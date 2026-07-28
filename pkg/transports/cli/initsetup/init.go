// Package initsetup adapts the public init command to atomic operator settings.
package initsetup

import (
	"context"
	"fmt"
	"io"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingsservicewire "github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
)

// ContextLineReader is the exact cancellation-aware input role used by setup.
type ContextLineReader interface {
	ReadLine(context.Context) (string, error)
}

// ContextLineReaderFactory binds an invocation-local input stream to the
// policy-free line reader selected at composition.
type ContextLineReaderFactory func(io.Reader, int) (ContextLineReader, error)

// Config carries supplied provider/model setup inputs from the CLI boundary.
type Config struct {
	Context  context.Context
	HomeDir  string
	Provider string
	Model    *string
	Input    io.Reader
	Output   io.Writer
	// Interactive is true only when the process classified both stdin and
	// stdout as terminals. Supplied provider input never requires it.
	Interactive bool
}

// NewConfigurer binds CLI setup rendering to the Settings-owned adapter Service
// constructed from the injected operator-config service ports.
func NewConfigurer(
	service operatorsettings.ConfigDocumentService,
	newLineReader ContextLineReaderFactory,
) func(Config) error {
	return func(cfg Config) error {
		adapterCfg := operatorsettingscli.ConfigureConfig{
			Context:       cfg.Context,
			HomeDir:       cfg.HomeDir,
			Provider:      cfg.Provider,
			Model:         cfg.Model,
			Input:         cfg.Input,
			Output:        cfg.Output,
			Interactive:   cfg.Interactive,
			NewLineReader: adaptLineReaderFactory(newLineReader),
		}
		if err := operatorsettingscli.ValidateConfigureBoundary(adapterCfg); err != nil {
			return err
		}
		root, err := operatorsettingsservicewire.NewServiceFromConfigDocument(service)
		if err != nil {
			return fmt.Errorf("configure operator settings: %w", err)
		}
		return operatorsettingscli.Configure(adapterCfg, root)
	}
}

func adaptLineReaderFactory(
	factory ContextLineReaderFactory,
) operatorsettingscli.ContextLineReaderFactory {
	if factory == nil {
		return nil
	}
	return func(input io.Reader, maxLines int) (operatorsettingscli.ContextLineReader, error) {
		return factory(input, maxLines)
	}
}
