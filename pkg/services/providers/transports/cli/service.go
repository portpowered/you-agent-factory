// Package cli defines the Providers service-owned CLI adapter.
package cli

import (
	"fmt"
	"io"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// Service exposes Providers CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Show(ShowConfig) error
}

type service struct {
	root providers.Service
}

// New constructs the Providers CLI service from the accepted Providers root
// contract. Construction is inert: it does not call ListProviders,
// GetProvider, or Execute on the injected root.
func New(root providers.Service) Service {
	if root == nil {
		return nil
	}
	return &service{root: root}
}

// CommandHandler owns Cobra-to-Providers request transformation for the
// public `you providers` family. The top-level CLI only attaches its methods
// by generated manifest handler ID.
type CommandHandler struct {
	providers         Service
	diagnosticsWriter func(*cobra.Command) io.Writer
}

// NewCommandHandler constructs the Providers-owned CLI handler from the
// already-injected Providers CLI service.
func NewCommandHandler(
	providersService Service,
	diagnosticsWriter func(*cobra.Command) io.Writer,
) *CommandHandler {
	return &CommandHandler{
		providers:         providersService,
		diagnosticsWriter: diagnosticsWriter,
	}
}

const (
	providersJSONInputID    = "you.flag.json"
	providersVerboseInputID = "you.flag.verbose"
	providersDebugInputID   = "you.flag.debug"
)

// List resolves the canonical global CLI inputs and delegates to the
// Providers-owned list operation. No provider execution or discovery effect
// is introduced at this command boundary.
func (handler *CommandHandler) List(
	cmd *cobra.Command,
	_ resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if handler == nil || handler.providers == nil {
		return fmt.Errorf("providers list service is required")
	}
	if cmd == nil {
		return fmt.Errorf("command is required")
	}
	jsonOutput, err := inherited.Bool(providersJSONInputID)
	if err != nil {
		return fmt.Errorf("resolve providers list JSON input: %w", err)
	}
	verbose, err := inherited.Bool(providersVerboseInputID)
	if err != nil {
		return fmt.Errorf("resolve providers list verbose input: %w", err)
	}
	debug, err := inherited.Bool(providersDebugInputID)
	if err != nil {
		return fmt.Errorf("resolve providers list debug input: %w", err)
	}
	var diagnostics io.Writer
	if handler.diagnosticsWriter != nil {
		diagnostics = handler.diagnosticsWriter(cmd)
	}
	return handler.providers.List(ListConfig{
		Context:     cmd.Context(),
		JSON:        jsonOutput,
		Verbose:     verbose || debug,
		Output:      cmd.OutOrStdout(),
		Diagnostics: diagnostics,
	})
}
