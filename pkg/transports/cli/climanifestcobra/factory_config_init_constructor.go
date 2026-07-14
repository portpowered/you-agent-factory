package climanifestcobra

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// FactoryConfigInitFamilyComponents holds detached factory/config/init commands
// before production root wiring attaches them as siblings.
type FactoryConfigInitFamilyComponents struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

// NewFactoryConfigInitFamilyComponents builds detached factory, system config, and
// init commands from generated metadata and attaches handwritten handlers by ID.
func NewFactoryConfigInitFamilyComponents(
	registry *commandregistry.Registry,
	bindings FactoryConfigInitFlagBindings,
) (FactoryConfigInitFamilyComponents, error) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	return NewFactoryConfigInitFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewFactoryConfigInitFamilyComponentsFromManifest builds detached factory/config/init
// commands from one generated manifest snapshot.
func NewFactoryConfigInitFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings FactoryConfigInitFlagBindings,
) (FactoryConfigInitFamilyComponents, error) {
	if registry == nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: registry is required")
	}
	if err := validateFactoryConfigInitManifest(manifest); err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}

	built, err := buildFactoryConfigInitCommandMap(manifest, registry, bindings)
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, err
	}

	factory := built["you.factory"]
	config := built["you.config"]
	initCmd := built["you.init"]
	if factory == nil || config == nil || initCmd == nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: missing top-level command")
	}
	return FactoryConfigInitFamilyComponents{
		Factory: factory,
		Config:  config,
		Init:    initCmd,
	}, nil
}

func validateFactoryConfigInitManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.FactoryConfigInitFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d factory/config/init commands",
			len(manifest.Commands),
			len(climanifestgen.FactoryConfigInitFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing factory/config/init command %q", commandID)
		}
	}
	return nil
}

func buildFactoryConfigInitCommandMap(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings FactoryConfigInitFlagBindings,
) (map[string]*cobra.Command, error) {
	built := make(map[string]*cobra.Command, len(manifest.Commands))
	for commandID, record := range manifest.Commands {
		cmd, err := buildFactoryConfigInitCommandFromRecord(record)
		if err != nil {
			return nil, fmt.Errorf("build factory/config/init family commands: %w", err)
		}
		if err := finalizeFactoryConfigInitCommand(cmd, record, registry, bindings); err != nil {
			return nil, fmt.Errorf("build factory/config/init family commands: %w", err)
		}
		built[commandID] = cmd
	}

	for commandID, cmd := range built {
		parentID := factoryConfigInitParentID(commandID)
		if parentID == "" {
			continue
		}
		parent, ok := built[parentID]
		if !ok {
			return nil, fmt.Errorf("build factory/config/init family commands: missing parent %q for %q", parentID, commandID)
		}
		parent.AddCommand(cmd)
	}
	return built, nil
}

func buildFactoryConfigInitCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	cmd := &cobra.Command{
		Use:     record.Usage.Line,
		Short:   record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if record.Visibility == "hidden" {
		cmd.Hidden = true
	}
	return cmd, nil
}

func finalizeFactoryConfigInitCommand(
	cmd *cobra.Command,
	record climanifest.Command,
	registry *commandregistry.Registry,
	bindings FactoryConfigInitFlagBindings,
) error {
	if !record.Runnable {
		return configureFactoryConfigInitGroupParent(cmd, record.ID)
	}

	if factoryConfigInitSilenceUsage(record.ID) {
		cmd.SilenceUsage = true
	}
	cmd.Args = factoryConfigInitPositionalArgs(record)
	if usesDeprecatedPortPreRun(record.ID) {
		cmd.PreRunE = rejectDeprecatedPortFlag
	}
	if err := registerFactoryConfigInitLocalFlags(cmd, record, bindings); err != nil {
		return err
	}
	return registry.AttachRunE(cmd, record.ID)
}

func configureFactoryConfigInitGroupParent(cmd *cobra.Command, commandID string) error {
	switch commandID {
	case "you.factory", "you.factory.config":
		configureGroupCommandUnknownSubcommandGuard(cmd)
		return nil
	case "you.config":
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		}
		return nil
	default:
		return fmt.Errorf("non-runnable command %q must be a group parent", commandID)
	}
}

func factoryConfigInitParentID(commandID string) string {
	switch commandID {
	case "you.factory", "you.config", "you.init":
		return ""
	default:
		if idx := strings.LastIndex(commandID, "."); idx > 0 {
			return commandID[:idx]
		}
		return ""
	}
}

func factoryConfigInitSilenceUsage(commandID string) bool {
	switch commandID {
	case "you.factory.query",
		"you.factory.list",
		"you.factory.create",
		"you.factory.update",
		"you.factory.delete",
		"you.factory.replace-current",
		"you.factory.config.validate":
		return true
	default:
		return false
	}
}

func usesDeprecatedPortPreRun(commandID string) bool {
	switch commandID {
	case "you.factory.query", "you.factory.replace-current":
		return true
	default:
		return false
	}
}

func registerFactoryConfigInitLocalFlags(
	cmd *cobra.Command,
	record climanifest.Command,
	bindings FactoryConfigInitFlagBindings,
) error {
	var deprecatedPort int
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup("port"), flag); err != nil {
				return fmt.Errorf("apply port flag contract: %w", err)
			}
			continue
		}
		target, err := factoryConfigInitLocalBindingTarget(record.ID, flag, bindings)
		if err != nil {
			return err
		}
		usage := factoryConfigInitFlagUsage(record.ID, flag.Long, bindings)
		if err := registerFlag(cmd.Flags(), flag, target, usage); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
		if flag.Required {
			_ = cmd.MarkFlagRequired(flag.Long)
		}
	}
	return nil
}

func factoryConfigInitPositionalArgs(record climanifest.Command) cobra.PositionalArgs {
	if record.ID == "you.factory.replace-current" {
		return cobra.NoArgs
	}
	return positionalArgsFromManifest(record)
}

func factoryConfigInitFlagUsage(commandID, longName string, bindings FactoryConfigInitFlagBindings) string {
	switch commandID {
	case "you.init":
		switch longName {
		case "dir":
			return "base directory to create"
		case "type":
			return "scaffold type to generate (supported: default, ralph)"
		}
	}
	if bindings.FlagUsages == nil {
		return ""
	}
	return bindings.FlagUsages[longName]
}

func factoryConfigInitLocalBindingTarget(
	commandID string,
	flag climanifest.Flag,
	bindings FactoryConfigInitFlagBindings,
) (flagTarget, error) {
	switch commandID {
	case "you.factory.list":
		if flag.Long == "dir" {
			return requireStringBinding(flag.Long, bindings.FactoryListDir)
		}
	case "you.factory.create":
		switch flag.Long {
		case "dir":
			return requireStringBinding(flag.Long, bindings.FactoryCreateDir)
		case "from":
			return requireStringBinding(flag.Long, bindings.FactoryCreateFrom)
		case "set-current":
			return requireBoolBinding(flag.Long, bindings.FactoryCreateSetCurrent)
		}
	case "you.factory.update":
		switch flag.Long {
		case "dir":
			return requireStringBinding(flag.Long, bindings.FactoryUpdateDir)
		case "from":
			return requireStringBinding(flag.Long, bindings.FactoryUpdateFrom)
		}
	case "you.factory.delete":
		if flag.Long == "dir" {
			return requireStringBinding(flag.Long, bindings.FactoryDeleteDir)
		}
	case "you.factory.replace-current":
		if flag.Long == "session" {
			return requireStringBinding(flag.Long, bindings.FactoryReplaceSessionID)
		}
	case "you.init":
		switch flag.Long {
		case "dir":
			return requireStringBinding(flag.Long, bindings.InitDir)
		case "type":
			return requireStringBinding(flag.Long, bindings.InitType)
		case "executor":
			return requireStringBinding(flag.Long, bindings.InitExecutor)
		}
	}
	return flagTarget{}, fmt.Errorf("unsupported local flag %q on %q", flag.Long, commandID)
}

func requireStringBinding(name string, target *string) (flagTarget, error) {
	if target == nil {
		return flagTarget{}, fmt.Errorf("bindings for local flag %q are required", name)
	}
	return flagTarget{stringValue: target}, nil
}

func requireBoolBinding(name string, target *bool) (flagTarget, error) {
	if target == nil {
		return flagTarget{}, fmt.Errorf("bindings for local flag %q are required", name)
	}
	return flagTarget{boolValue: target}, nil
}

func configureGroupCommandUnknownSubcommandGuard(cmd *cobra.Command) {
	cmd.DisableFlagParsing = true
	cmd.Args = rejectUnknownSubcommandArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rejectUnknownSubcommandArgs(cmd, args)
	}
}

func rejectUnknownSubcommandArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cmd.Help()
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}
