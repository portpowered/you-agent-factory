// Package climanifestcobra builds representative, session, work,
// factory/config/init, models/docs, and run/submit Cobra trees from generated
// manifest metadata and handwritten handler registries.
package climanifestcobra

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RepresentativeFamilyComponents holds detached representative-family commands
// before the session/show subtree is attached to the generated root.
type RepresentativeFamilyComponents struct {
	Root    *cobra.Command
	Session *cobra.Command
	Show    *cobra.Command
}

// NewRepresentativeFamilyCommand builds the representative you → session → show tree
// from generated metadata and attaches handwritten handlers by stable command ID.
// Only contracted representative-family commands are constructed.
func NewRepresentativeFamilyCommand(registry *commandregistry.Registry, bindings PersistentFlagBindings) (*cobra.Command, error) {
	components, err := NewRepresentativeFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Session.AddCommand(components.Show)
	components.Root.AddCommand(components.Session)
	return components.Root, nil
}

// NewRepresentativeFamilyComponents builds detached representative-family commands
// so production wiring can attach additional handwritten session siblings in order.
func NewRepresentativeFamilyComponents(registry *commandregistry.Registry, bindings PersistentFlagBindings) (RepresentativeFamilyComponents, error) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	return NewRepresentativeFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewRepresentativeFamilyCommandFromManifest builds the representative tree from one
// generated manifest snapshot. Manifest command IDs must stay within the representative family.
func NewRepresentativeFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings PersistentFlagBindings,
) (*cobra.Command, error) {
	components, err := NewRepresentativeFamilyComponentsFromManifest(manifest, registry, bindings)
	if err != nil {
		return nil, err
	}
	components.Session.AddCommand(components.Show)
	components.Root.AddCommand(components.Session)
	return components.Root, nil
}

// NewRepresentativeFamilyComponentsFromManifest builds detached representative-family
// commands from one generated manifest snapshot.
func NewRepresentativeFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
	bindings PersistentFlagBindings,
) (RepresentativeFamilyComponents, error) {
	if registry == nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: registry is required")
	}
	if err := validateBindings(bindings); err != nil {
		return RepresentativeFamilyComponents{}, err
	}
	if err := validateRepresentativeManifest(manifest); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	rootRecord, sessionRecord, showRecord, err := representativeManifestRecords(manifest)
	if err != nil {
		return RepresentativeFamilyComponents{}, err
	}

	root, err := buildCommandFromRecord(rootRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	root.SilenceUsage = true
	if err := registerPersistentFlags(root, rootRecord, bindings); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.AttachRunE(root, rootRecord.ID); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	session, err := buildCommandFromRecord(sessionRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if sessionRecord.Runnable {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %q must remain non-runnable", sessionRecord.ID)
	}

	show, err := buildCommandFromRecord(showRecord)
	if err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	show.Args = positionalArgsFromManifest(showRecord)
	show.PreRunE = rejectDeprecatedPortFlag
	if err := registerLocalFlags(show, showRecord, bindings); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}
	if err := registry.AttachRunE(show, showRecord.ID); err != nil {
		return RepresentativeFamilyComponents{}, fmt.Errorf("build representative family command: %w", err)
	}

	return RepresentativeFamilyComponents{
		Root:    root,
		Session: session,
		Show:    show,
	}, nil
}

func representativeManifestRecords(manifest climanifest.Manifest) (root, session, show climanifest.Command, err error) {
	root, err = manifest.CommandByID("you")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	session, err = manifest.CommandByID("you.session")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	show, err = manifest.CommandByID("you.session.show")
	if err != nil {
		return climanifest.Command{}, climanifest.Command{}, climanifest.Command{}, fmt.Errorf("build representative family command: %w", err)
	}
	return root, session, show, nil
}

func validateRepresentativeManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.RepresentativeFamilyCommandIDs) {
		return fmt.Errorf(
			"manifest command count = %d, want %d representative-family commands",
			len(manifest.Commands),
			len(climanifestgen.RepresentativeFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertRepresentativeFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.RepresentativeFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing representative-family command %q", commandID)
		}
	}
	return nil
}

func validateBindings(bindings PersistentFlagBindings) error {
	required := []struct {
		name string
		ok   bool
	}{
		{"Verbose", bindings.Verbose != nil},
		{"Debug", bindings.Debug != nil},
		{"Server", bindings.Server != nil},
		{"JSON", bindings.JSON != nil},
		{"DefaultWorkerModelProvider", bindings.DefaultWorkerModelProvider != nil},
		{"DefaultWorkerModel", bindings.DefaultWorkerModel != nil},
	}
	for _, field := range required {
		if !field.ok {
			return fmt.Errorf("build representative family command: bindings.%s is required", field.name)
		}
	}
	return nil
}

func buildCommandFromRecord(record climanifest.Command) (*cobra.Command, error) {
	if err := climanifestgen.AssertRepresentativeFamilyCommandID(record.ID); err != nil {
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

func registerPersistentFlags(root *cobra.Command, record climanifest.Command, bindings PersistentFlagBindings) error {
	flags := sortedFlags(record.Flags)
	for _, flag := range flags {
		if flag.Scope != "persistent" {
			continue
		}
		target, err := persistentBindingTarget(flag.Long, bindings)
		if err != nil {
			return err
		}
		usage := flagUsage(bindings, flag.Long)
		if err := registerFlag(root.PersistentFlags(), flag, target, usage); err != nil {
			return fmt.Errorf("register root persistent flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(root.PersistentFlags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply root persistent flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func registerLocalFlags(cmd *cobra.Command, record climanifest.Command, bindings PersistentFlagBindings) error {
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
		target, err := localBindingTarget(flag)
		if err != nil {
			return err
		}
		usage := flagUsage(bindings, flag.Long)
		if err := registerFlag(cmd.Flags(), flag, target, usage); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func flagUsage(bindings PersistentFlagBindings, longName string) string {
	if bindings.FlagUsages == nil {
		return ""
	}
	return bindings.FlagUsages[longName]
}

type flagTarget struct {
	boolValue   *bool
	stringValue *string
	intValue    *int
}

func persistentBindingTarget(longName string, bindings PersistentFlagBindings) (flagTarget, error) {
	switch longName {
	case "verbose":
		return flagTarget{boolValue: bindings.Verbose}, nil
	case "debug":
		return flagTarget{boolValue: bindings.Debug}, nil
	case "server":
		return flagTarget{stringValue: bindings.Server}, nil
	case "json":
		return flagTarget{boolValue: bindings.JSON}, nil
	case "default-worker-model-provider":
		return flagTarget{stringValue: bindings.DefaultWorkerModelProvider}, nil
	case "default-worker-model":
		return flagTarget{stringValue: bindings.DefaultWorkerModel}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported root persistent flag %q", longName)
	}
}

func localBindingTarget(flag climanifest.Flag) (flagTarget, error) {
	switch flag.ValueType {
	case "int":
		value, err := strconv.Atoi(flag.Default)
		if err != nil {
			return flagTarget{}, fmt.Errorf("parse default for local flag %q: %w", flag.Long, err)
		}
		heap := value
		return flagTarget{intValue: &heap}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported local flag %q with value type %q", flag.Long, flag.ValueType)
	}
}

func registerFlag(flagSet *pflag.FlagSet, contract climanifest.Flag, target flagTarget, usage string) error {
	switch contract.ValueType {
	case "bool":
		if target.boolValue == nil {
			return fmt.Errorf("missing bool binding for flag %q", contract.Long)
		}
		defaultValue, err := strconv.ParseBool(contract.Default)
		if err != nil {
			return fmt.Errorf("parse default for flag %q: %w", contract.Long, err)
		}
		if contract.Shorthand != "" {
			flagSet.BoolVarP(target.boolValue, contract.Long, contract.Shorthand, defaultValue, usage)
		} else {
			flagSet.BoolVar(target.boolValue, contract.Long, defaultValue, usage)
		}
	case "string":
		if target.stringValue == nil {
			return fmt.Errorf("missing string binding for flag %q", contract.Long)
		}
		if contract.Shorthand != "" {
			return fmt.Errorf("string flag %q does not support shorthand in generated constructor", contract.Long)
		}
		flagSet.StringVar(target.stringValue, contract.Long, contract.Default, usage)
	case "int":
		if target.intValue == nil {
			return fmt.Errorf("missing int binding for flag %q", contract.Long)
		}
		defaultValue, err := strconv.Atoi(contract.Default)
		if err != nil {
			return fmt.Errorf("parse default for flag %q: %w", contract.Long, err)
		}
		flagSet.IntVar(target.intValue, contract.Long, defaultValue, usage)
	default:
		return fmt.Errorf("unsupported flag value type %q for %q", contract.ValueType, contract.Long)
	}
	return nil
}

func applyFlagContract(flag *pflag.Flag, contract climanifest.Flag) error {
	if flag == nil {
		return fmt.Errorf("flag %q was not registered", contract.Long)
	}
	if contract.Visibility == "hidden" {
		flag.Hidden = true
	}
	if contract.NoOptionDefault != "" {
		flag.NoOptDefVal = contract.NoOptionDefault
	}
	return nil
}

func sortedFlags(flags map[string]climanifest.Flag) []climanifest.Flag {
	if len(flags) == 0 {
		return nil
	}
	ordered := make([]climanifest.Flag, 0, len(flags))
	for _, flag := range flags {
		ordered = append(ordered, flag)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Long == ordered[j].Long {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Long < ordered[j].Long
	})
	return ordered
}

func positionalArgsFromManifest(record climanifest.Command) cobra.PositionalArgs {
	if len(record.Arguments) == 0 {
		return nil
	}
	args := make([]climanifest.Argument, 0, len(record.Arguments))
	for _, arg := range record.Arguments {
		args = append(args, arg)
	}
	sort.Slice(args, func(i, j int) bool { return args[i].Position < args[j].Position })

	variadic := false
	totalMin := 0
	totalMax := 0
	unboundedMax := false
	for _, arg := range args {
		if arg.Variadic {
			variadic = true
		}
		totalMin += arg.MinCardinality
		if arg.MaxCardinality < 0 {
			unboundedMax = true
			continue
		}
		if arg.MaxCardinality == 0 {
			continue
		}
		totalMax += arg.MaxCardinality
	}
	if variadic {
		if totalMin > 0 {
			return cobra.MinimumNArgs(totalMin)
		}
		return cobra.ArbitraryArgs
	}
	if unboundedMax {
		if totalMin > 0 {
			return cobra.MinimumNArgs(totalMin)
		}
		return cobra.ArbitraryArgs
	}
	if totalMax > 0 && totalMin == totalMax {
		return cobra.ExactArgs(totalMin)
	}
	if totalMax > 0 {
		return cobra.MaximumNArgs(totalMax)
	}
	if totalMin > 0 {
		return cobra.MinimumNArgs(totalMin)
	}
	return nil
}
