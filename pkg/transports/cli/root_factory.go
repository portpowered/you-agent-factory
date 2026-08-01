package cli

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

func newRootCommandWithFactory(options CommandFactory) *cobra.Command {
	root := newRootCommandWithGeneratedRepresentativeFamily(options)
	if root == nil {
		return nil
	}
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if requiresSystemInitialization(cmd.CommandPath(), args) {
			if options.initializer == nil {
				return fmt.Errorf("system initializer is required")
			}
			homeDir, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			if err := options.initializer.InitializeSystem(cmd.Context(), homeDir); err != nil {
				return fmt.Errorf("initialize system: %w", err)
			}
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return root
}

func requiresSystemInitialization(commandPath string, args []string) bool {
	switch commandPath {
	case "you":
		return len(args) > 0
	case "you mcp serve", "you run":
		return true
	default:
		return false
	}
}

func executeServerCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions CommandFactory,
) error {
	cfg := defaultcmd.ServerRunConfig(rootOptions.runDefaults)
	if err := selectCurrentFactoryFromWorkingDirectory(cmd, &cfg); err != nil {
		mapped := runcli.MapCurrentFactoryFailure(err)
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
		return mapped
	}
	policy := diagnostics.resolvePolicy(false)
	err := runFactoryWithOptions(
		cmd, cfg, nil, globals, operatorDefaults, policy, rootOptions, true,
	)
	if err == nil {
		return nil
	}
	mapped := runcli.MapServerFailure(err)
	mapped = runcli.MapCurrentFactoryFailure(factoryload.MaybeFormatOperatorError(mapped, cfg.Dir))
	_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
	return mapped
}

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) factoryConfigInitProductionCommands {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			QueryFactory:           options.QueryFactory,
			ListFactories:          options.ListFactories,
			CreateFactoryFromFile:  options.CreateFactoryFromFile,
			UpdateFactoryFromFile:  options.UpdateFactoryFromFile,
			DeleteFactory:          options.DeleteFactory,
			ReplaceFactoryCurrent:  options.ReplaceFactoryCurrent,
			ValidateFactory:        options.ValidateFactory,
			FlattenFactoryConfig:   options.FlattenFactoryConfig,
			ExpandFactoryConfig:    options.ExpandFactoryConfig,
			ConfigureInit:          options.ConfigureInit,
			InstallPackagedFactory: options.InstallPackagedFactory,
			HomeDir:                options.homeDir,
			ResolveFactoryRoots:    options.resolveNamedFactoryRoots,
			DiagnosticsWriter:      diagnostics.writer,
		},
	)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(handler)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	if options.completePackagedFactoryNames != nil {
		if err := cobracompletion.RegisterPackagedFactoryNames(
			components.Init,
			options.completePackagedFactoryNames,
		); err != nil {
			panic(fmt.Sprintf("register packaged factory init completion: %v", err))
		}
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}

// FamilyHandler is the raw CLI forwarding contract. The top-level CLI keeps
// the original Cobra command and argument slice intact; an owner adapter owns
// decoding, validation, presentation, and root invocation.
type FamilyHandler func(*cobra.Command, []string) error

// CommandFamily is one owner-published command family in the process-scoped
// CLI registry.
type CommandFamily struct {
	Name    string
	Handler FamilyHandler
}

// FamilyRegistry is an immutable command-family registry composed once by
// Wire. It has no product roots or input-resolution policy.
type FamilyRegistry struct {
	families []CommandFamily
	byName   map[string]FamilyHandler
}

// NewFamilyRegistry validates and snapshots the owner-published families.
func NewFamilyRegistry(families []CommandFamily) (*FamilyRegistry, error) {
	if len(families) == 0 {
		return nil, fmt.Errorf("CLI family registry requires at least one family")
	}
	registry := &FamilyRegistry{
		families: make([]CommandFamily, 0, len(families)),
		byName:   make(map[string]FamilyHandler, len(families)),
	}
	for index, family := range families {
		name := strings.TrimSpace(family.Name)
		if name == "" {
			return nil, fmt.Errorf("CLI family registry family %d has no name", index)
		}
		if family.Handler == nil {
			return nil, fmt.Errorf("CLI family registry family %q has no handler", name)
		}
		if _, exists := registry.byName[name]; exists {
			return nil, fmt.Errorf("CLI family registry contains duplicate family %q", name)
		}
		registry.families = append(registry.families, CommandFamily{Name: name, Handler: family.Handler})
		registry.byName[name] = family.Handler
	}
	return registry, nil
}

// Families returns a detached catalog in composition order.
func (registry *FamilyRegistry) Families() []CommandFamily {
	if registry == nil {
		return nil
	}
	return append([]CommandFamily(nil), registry.families...)
}

// Lookup returns the owner handler registered for name.
func (registry *FamilyRegistry) Lookup(name string) (FamilyHandler, bool) {
	if registry == nil {
		return nil, false
	}
	handler, ok := registry.byName[strings.TrimSpace(name)]
	return handler, ok
}

// Dispatch forwards the original command and arguments to one registered
// owner handler. No flag parsing or argument copying occurs here.
func (registry *FamilyRegistry) Dispatch(name string, command *cobra.Command, arguments []string) error {
	handler, ok := registry.Lookup(name)
	if !ok {
		return fmt.Errorf("CLI family %q is not registered", strings.TrimSpace(name))
	}
	if command == nil {
		return fmt.Errorf("CLI family %q dispatch requires a command", strings.TrimSpace(name))
	}
	return handler(command, arguments)
}
