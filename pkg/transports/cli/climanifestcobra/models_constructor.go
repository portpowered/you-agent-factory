package climanifestcobra

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// NewDocsCommand builds the independently injected `you docs` command through
// the accepted generic manifest constructor.
func NewDocsCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewDocsCommandFromManifest(manifest, handler)
}

// NewDocsCommandFromManifest projects and detaches `you docs` from the generic
// root/docs tree. Positional validation, help, and completion remain entirely
// manifest-owned.
func NewDocsCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build docs command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID: rootRecord,
		docsRecord.ID: docsRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			docsRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	cmd, _, err := root.Find([]string{docsRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build docs command: find projected command: %w", err)
	}
	root.RemoveCommand(cmd)
	cmd.SilenceUsage = true
	return cmd, nil
}

// NewModelsCommand builds the independently injected `you models` family.
func NewModelsCommand(handler commandregistry.ModelsHandler) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewModelsCommandFromManifest(manifest, handler)
}

// NewModelsCommandFromManifest projects list and inspect through the generic
// constructor while retaining narrow legacy dispatch only for the later
// invoke and pull migration slices.
func NewModelsCommandFromManifest(
	manifest climanifest.Manifest,
	handler commandregistry.ModelsHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build models command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build models command: %q must remain non-runnable", parentRecord.ID)
	}
	familyManifest := manifest
	listRecord, err := manifest.CommandByID("you.models.list")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	inspectRecord, err := manifest.CommandByID("you.models.inspect")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parent, err := buildResolvedModelsParent(
		manifest,
		rootRecord,
		parentRecord,
		listRecord,
		inspectRecord,
		handler,
	)
	if err != nil {
		return nil, err
	}
	if err := attachLegacyModelsLeaves(parent, familyManifest, handler); err != nil {
		return nil, err
	}
	return parent, nil
}

func buildResolvedModelsParent(
	manifest climanifest.Manifest,
	rootRecord climanifest.Command,
	parentRecord climanifest.Command,
	listRecord climanifest.Command,
	inspectRecord climanifest.Command,
	handler commandregistry.ModelsHandler,
) (*cobra.Command, error) {
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:    rootRecord,
		parentRecord.ID:  parentRecord,
		listRecord.ID:    listRecord,
		inspectRecord.ID: inspectRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			listRecord.Handler.ID:    handler.List,
			inspectRecord.Handler.ID: handler.Inspect,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build models command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	for _, id := range []string{listRecord.ID, inspectRecord.ID} {
		command, _, findErr := parent.Find([]string{strings.TrimPrefix(id, parentRecord.ID+".")})
		if findErr != nil {
			return nil, fmt.Errorf("build models command: find %q: %w", id, findErr)
		}
		command.PreRunE = rejectDeprecatedPortFlag
		if id == inspectRecord.ID {
			preserveModelsExactArgumentDiagnostic(command, inspectRecord)
		}
	}
	return parent, nil
}

func preserveModelsExactArgumentDiagnostic(
	command *cobra.Command,
	record climanifest.Command,
) {
	arguments := make([]climanifest.Argument, 0, len(record.Arguments))
	for _, argument := range record.Arguments {
		arguments = append(arguments, argument)
	}
	minimum, maximum := argumentCardinality(arguments)
	if minimum != maximum {
		return
	}
	validate := command.Args
	command.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) != minimum {
			return cobra.ExactArgs(minimum)(cmd, args)
		}
		return validate(cmd, args)
	}
}

func attachLegacyModelsLeaves(
	parent *cobra.Command,
	manifest climanifest.Manifest,
	handler commandregistry.ModelsHandler,
) error {
	registry, err := commandregistry.NewModelsRegistry(handler)
	if err != nil {
		return err
	}
	for _, id := range []string{"you.models.invoke", "you.models.pull"} {
		commandRecord, commandErr := manifest.CommandByID(id)
		if commandErr != nil {
			return fmt.Errorf("build models command: %w", commandErr)
		}
		leaf, leafErr := buildRunnableModelsLeaf(commandRecord, registry)
		if leafErr != nil {
			return leafErr
		}
		parent.AddCommand(leaf)
	}
	return nil
}

func buildRunnableModelsLeaf(record climanifest.Command, registry *commandregistry.Registry) (*cobra.Command, error) {
	if !strings.HasPrefix(record.ID, "you.models.") || !record.Runnable {
		return nil, fmt.Errorf("build models command: %q must be a runnable models command", record.ID)
	}
	cmd := commandFromManifest(record, false)
	cmd.Args = positionalArgsFromManifest(record)
	cmd.PreRunE = rejectDeprecatedPortFlag
	if err := registerManifestLocalFlags(cmd, record); err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	return cmd, nil
}

func commandFromManifest(record climanifest.Command, includeLong bool) *cobra.Command {
	cmd := &cobra.Command{
		Use: record.Usage.Line, Short: record.Documentation.Documentation.Title.CanonicalEnglish,
		Aliases: append([]string(nil), record.Aliases...),
	}
	if includeLong {
		cmd.Long = record.Documentation.Documentation.Description.CanonicalEnglish
	}
	cmd.Hidden = record.Visibility == "hidden"
	return cmd
}

func registerManifestLocalFlags(cmd *cobra.Command, record climanifest.Command) error {
	var deprecatedPort int
	for _, flag := range sortedFlags(record.Flags) {
		if flag.Scope != "local" {
			continue
		}
		if flag.Long == "port" {
			registerDeprecatedPortFlag(cmd, &deprecatedPort)
			if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
				return err
			}
			continue
		}
		target, err := manifestFlagTarget(flag)
		if err != nil {
			return err
		}
		if err := registerFlag(cmd.Flags(), flag, target, manifestFlagUsage(flag)); err != nil {
			return fmt.Errorf("register local flag %q: %w", flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply local flag %q contract: %w", flag.Long, err)
		}
	}
	return nil
}

func manifestFlagTarget(flag climanifest.Flag) (flagTarget, error) {
	switch flag.ValueType {
	case "string":
		return flagTarget{stringValue: new(string)}, nil
	case "bool":
		return flagTarget{boolValue: new(bool)}, nil
	case "int":
		return flagTarget{intValue: new(int)}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported manifest value type %q for flag %q", flag.ValueType, flag.Long)
	}
}

func manifestFlagUsage(flag climanifest.Flag) string {
	return "value for --" + flag.Long
}
