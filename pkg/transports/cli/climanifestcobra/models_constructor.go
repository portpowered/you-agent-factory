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
func NewModelsCommand(registry *commandregistry.Registry) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	return NewModelsCommandFromManifest(manifest, registry)
}

// NewModelsCommandFromManifest builds `you models` entirely from manifest
// arguments and flags, attaching only the injected command handlers.
func NewModelsCommandFromManifest(manifest climanifest.Manifest, registry *commandregistry.Registry) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build models command: registry is required")
	}
	parentRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build models command: %q must remain non-runnable", parentRecord.ID)
	}
	parent := commandFromManifest(parentRecord, true)
	for _, id := range []string{"you.models.list", "you.models.inspect", "you.models.invoke", "you.models.pull"} {
		record, recordErr := manifest.CommandByID(id)
		if recordErr != nil {
			return nil, fmt.Errorf("build models command: %w", recordErr)
		}
		leaf, leafErr := buildRunnableModelsLeaf(record, registry)
		if leafErr != nil {
			return nil, leafErr
		}
		parent.AddCommand(leaf)
	}
	return parent, nil
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
