package commandregistry

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
)

// RunnableRepresentativeCommandIDs returns contracted runnable command IDs for
// the representative family in stable sorted order.
func RunnableRepresentativeCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.RepresentativeFamilyCommandIDs))
	for _, commandID := range climanifestgen.RepresentativeFamilyCommandIDs {
		if err := climanifestgen.AssertRepresentativeFamilyCommandID(commandID); err != nil {
			return nil, err
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyRepresentativeRunnableCoverage fails when any contracted runnable
// representative-family command ID lacks a registered handwritten handler.
func (r *Registry) VerifyRepresentativeRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableRepresentativeCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("representative runnable command handlers missing for: %v", missing)
	}
	return nil
}

// RunnableSessionCommandIDs returns contracted runnable session command IDs in
// stable sorted order and validates their stable handler identities.
func RunnableSessionCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.SessionFamilyCommandIDs)-1)
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if !record.Runnable {
			continue
		}
		if record.Handler == nil || record.Handler.ID != commandID+".handler" {
			return nil, fmt.Errorf("session runnable command %q has invalid handler id", commandID)
		}
		ids = append(ids, commandID)
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifySessionRunnableCoverage rejects missing, extra, and cross-family
// registrations so each generated runnable leaf resolves exactly one handler.
func (r *Registry) VerifySessionRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableSessionCommandIDs(manifest)
	if err != nil {
		return err
	}
	want := make(map[string]bool, len(runnableIDs))
	for _, commandID := range runnableIDs {
		want[commandID] = true
	}
	var missing, extra []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if r != nil {
		for commandID := range r.handlers {
			if !want[commandID] {
				extra = append(extra, commandID)
			}
		}
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("session runnable handler coverage mismatch: missing=%v extra=%v", missing, extra)
	}
	return nil
}

// RepresentativeHandlers carries handwritten RunE handlers for contracted runnable
// representative-family command IDs.
type RepresentativeHandlers struct {
	RootRunE        RunE
	SessionShowRunE RunE
}

// SessionHandlers carries the existing handwritten RunE behavior for every
// canonical runnable Factory Session command.
type SessionHandlers struct {
	CreateRunE     RunE
	ListRunE       RunE
	ShowRunE       RunE
	DeleteRunE     RunE
	PauseRunE      RunE
	ResumeRunE     RunE
	DispatchesRunE RunE
}

// NewSessionRegistry registers exactly one handwritten handler for each
// canonical runnable Factory Session command.
func NewSessionRegistry(handlers SessionHandlers) (*Registry, error) {
	registrations := []struct {
		commandID string
		handler   RunE
	}{
		{"you.session.create", handlers.CreateRunE},
		{"you.session.list", handlers.ListRunE},
		{"you.session.show", handlers.ShowRunE},
		{"you.session.delete", handlers.DeleteRunE},
		{"you.session.pause", handlers.PauseRunE},
		{"you.session.resume", handlers.ResumeRunE},
		{"you.session.dispatches", handlers.DispatchesRunE},
	}
	registry := NewRegistry()
	for _, registration := range registrations {
		if registration.handler == nil {
			return nil, fmt.Errorf("build session handler registry: %s handler is required", registration.commandID)
		}
		if err := registry.Register(registration.commandID, registration.handler); err != nil {
			return nil, fmt.Errorf("build session handler registry: %w", err)
		}
	}
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build session handler registry: %w", err)
	}
	if err := registry.VerifySessionRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build session handler registry: %w", err)
	}
	return registry, nil
}

// SessionDiagnosticsBinding supplies the shared CLI diagnostic policy bindings.
type SessionDiagnosticsBinding struct {
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
}

func applySessionDiagnostics(cmd *cobra.Command, binding SessionDiagnosticsBinding, verbose, debug *bool, diagnostics *io.Writer) {
	if binding.DiagnosticsWriter != nil {
		*diagnostics = binding.DiagnosticsWriter(cmd)
	}
	if binding.Verbose != nil {
		*verbose = binding.Verbose()
	}
	if binding.Debug != nil {
		*debug = *binding.Debug
	}
}

// SessionCreateBinding supplies handwritten session create dependencies.
type SessionCreateBinding struct {
	Config *sessioncli.CreateConfig
	JSON   *bool
	SessionDiagnosticsBinding
	CreateSession func(sessioncli.CreateConfig) error
}

// SessionCreateRunE preserves the handwritten session create execution path.
func SessionCreateRunE(binding SessionCreateBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.CreateSession == nil {
			return fmt.Errorf("session create service is required")
		}
		if binding.Config == nil {
			return fmt.Errorf("session create config is required")
		}
		cfg := *binding.Config
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		applySessionDiagnostics(cmd, binding.SessionDiagnosticsBinding, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
		return binding.CreateSession(cfg)
	}
}

// SessionListBinding supplies handwritten session list dependencies.
type SessionListBinding struct {
	Config *sessioncli.ListConfig
	Server *string
	JSON   *bool
	SessionDiagnosticsBinding
	Prepare      func(context.Context, *sessioncli.ListConfig) error
	ListSessions func(sessioncli.ListConfig) error
}

// SessionListRunE preserves the handwritten session list execution path.
func SessionListRunE(binding SessionListBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.ListSessions == nil {
			return fmt.Errorf("session list service is required")
		}
		if binding.Config == nil {
			return fmt.Errorf("session list config is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		if binding.Server != nil && cmd.Root().PersistentFlags().Changed("server") {
			cfg.Server = *binding.Server
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		if binding.Prepare != nil {
			if err := binding.Prepare(cmd.Context(), &cfg); err != nil {
				return err
			}
		}
		cfg.Output = cmd.OutOrStdout()
		applySessionDiagnostics(cmd, binding.SessionDiagnosticsBinding, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
		return binding.ListSessions(cfg)
	}
}

// SessionDeleteBinding supplies handwritten session delete dependencies.
type SessionDeleteBinding struct {
	Config *sessioncli.DeleteConfig
	JSON   *bool
	SessionDiagnosticsBinding
	DeleteSession func(sessioncli.DeleteConfig) error
}

// SessionDeleteRunE preserves the handwritten session delete execution path.
func SessionDeleteRunE(binding SessionDeleteBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.DeleteSession == nil {
			return fmt.Errorf("session delete service is required")
		}
		if binding.Config == nil {
			return fmt.Errorf("session delete config is required")
		}
		cfg := *binding.Config
		cfg.SessionID = args[0]
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		applySessionDiagnostics(cmd, binding.SessionDiagnosticsBinding, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
		return binding.DeleteSession(cfg)
	}
}

// SessionDispatchesBinding supplies handwritten session dispatch inspection dependencies.
type SessionDispatchesBinding struct {
	Config *sessioncli.DispatchesConfig
	Server *string
	JSON   *bool
	SessionDiagnosticsBinding
	ListDispatches func(sessioncli.DispatchesConfig) error
}

// SessionDispatchesRunE preserves the handwritten dispatch inspection path.
func SessionDispatchesRunE(binding SessionDispatchesBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.ListDispatches == nil {
			return fmt.Errorf("session dispatches service is required")
		}
		if binding.Config == nil {
			return fmt.Errorf("session dispatches config is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		cfg.SessionID = args[0]
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		applySessionDiagnostics(cmd, binding.SessionDiagnosticsBinding, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
		return binding.ListDispatches(cfg)
	}
}

// SessionLifecycleBinding supplies handwritten pause or resume dependencies.
type SessionLifecycleBinding struct {
	Config *sessioncli.LifecycleControlConfig
	Server *string
	JSON   *bool
	SessionDiagnosticsBinding
	Control func(sessioncli.LifecycleControlConfig) error
}

// SessionLifecycleRunE preserves a handwritten pause or resume execution path.
func SessionLifecycleRunE(binding SessionLifecycleBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Config == nil {
			return fmt.Errorf("session lifecycle control config is required")
		}
		if binding.Control == nil {
			return fmt.Errorf("session lifecycle control handler is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		if len(args) == 1 {
			cfg.SessionID = args[0]
		}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		applySessionDiagnostics(cmd, binding.SessionDiagnosticsBinding, &cfg.Verbose, &cfg.Debug, &cfg.Diagnostics)
		return binding.Control(cfg)
	}
}

// NewRepresentativeRegistry registers handwritten handlers for the representative
// family and verifies contracted runnable command coverage.
func NewRepresentativeRegistry(handlers RepresentativeHandlers) (*Registry, error) {
	if handlers.RootRunE == nil {
		return nil, fmt.Errorf("build representative handler registry: root handler is required")
	}
	if handlers.SessionShowRunE == nil {
		return nil, fmt.Errorf("build representative handler registry: session show handler is required")
	}

	registry := NewRegistry()
	if err := registry.Register("you", handlers.RootRunE); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	if err := registry.Register("you.session.show", handlers.SessionShowRunE); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}

	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	return registry, nil
}

// SessionShowBinding supplies handwritten session show execution dependencies.
type SessionShowBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ShowSession       func(sessioncli.ShowConfig) error
}

// SessionShowRunE returns the handwritten session show RunE used by production wiring.
func SessionShowRunE(binding SessionShowBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.ShowSession == nil {
			return fmt.Errorf("session show service is required")
		}
		cfg := sessioncli.ShowConfig{}
		cfg.Context = cmd.Context()
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.SessionID = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return binding.ShowSession(cfg)
	}
}
