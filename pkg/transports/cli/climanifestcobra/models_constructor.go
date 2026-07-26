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

// NewModelsCommandFromManifest projects the complete Models family through the
// generic manifest constructor.
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
	listRecord, err := manifest.CommandByID("you.models.list")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	inspectRecord, err := manifest.CommandByID("you.models.inspect")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	pullRecord, err := manifest.CommandByID("you.models.pull")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	invokeRecord, err := manifest.CommandByID("you.models.invoke")
	if err != nil {
		return nil, fmt.Errorf("build models command: %w", err)
	}
	parent, err := buildResolvedModelsParent(
		manifest,
		rootRecord,
		parentRecord,
		listRecord,
		inspectRecord,
		pullRecord,
		invokeRecord,
		handler,
	)
	if err != nil {
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
	pullRecord climanifest.Command,
	invokeRecord climanifest.Command,
	handler commandregistry.ModelsHandler,
) (*cobra.Command, error) {
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:    rootRecord,
		parentRecord.ID:  parentRecord,
		listRecord.ID:    listRecord,
		inspectRecord.ID: inspectRecord,
		pullRecord.ID:    pullRecord,
		invokeRecord.ID:  invokeRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			listRecord.Handler.ID:    handler.List,
			inspectRecord.Handler.ID: handler.Inspect,
			pullRecord.Handler.ID:    handler.Pull,
			invokeRecord.Handler.ID:  handler.Invoke,
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
	for _, id := range []string{listRecord.ID, inspectRecord.ID, pullRecord.ID, invokeRecord.ID} {
		command, _, findErr := parent.Find([]string{strings.TrimPrefix(id, parentRecord.ID+".")})
		if findErr != nil {
			return nil, fmt.Errorf("build models command: find %q: %w", id, findErr)
		}
		command.PreRunE = rejectDeprecatedPortFlag
		if id != listRecord.ID {
			preserveModelsExactArgumentDiagnostic(command, manifest.Commands[id])
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

// NewMCPCommand builds the independently injected `you mcp` family through
// the accepted generic manifest constructor.
func NewMCPCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.MCPFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewMCPCommandFromManifest(manifest, handler)
}

// NewMCPCommandFromManifest projects and detaches the complete MCP family.
func NewMCPCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build MCP command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.mcp")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build MCP command: %q must remain non-runnable", parentRecord.ID)
	}
	serveRecord, err := manifest.CommandByID("you.mcp.serve")
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		parentRecord.ID: parentRecord,
		serveRecord.ID:  serveRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			serveRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	serve, _, err := parent.Find([]string{serveRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build MCP command: find projected serve command: %w", err)
	}
	preserveMCPSourceRelationshipDiagnostic(serve, serveRecord)
	return parent, nil
}

func preserveMCPSourceRelationshipDiagnostic(
	command *cobra.Command,
	record climanifest.Command,
) {
	const relationshipID = "you.mcp.serve.relationship.runtime-source"
	if _, declared := record.Relationships[relationshipID]; !declared {
		return
	}
	validate := command.PreRunE
	command.PreRunE = func(cmd *cobra.Command, args []string) error {
		err := validate(cmd, args)
		if err != nil && strings.Contains(err.Error(), `input relationship "`+relationshipID+`"`) {
			return fmt.Errorf("cannot combine --runtime with --fixture-catalog")
		}
		return err
	}
}
