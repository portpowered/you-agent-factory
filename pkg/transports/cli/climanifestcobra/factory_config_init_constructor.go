package climanifestcobra

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// NewSessionFamilyCommandFromManifest constructs an independently executable
// `you session` family from the generated Session manifest snapshot. Registry
// entries are addressed exclusively by stable handler IDs from that manifest.
func NewSessionFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build session family command: registry is required")
	}
	if err := registry.VerifySessionHandlerIDCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build session family command: %w", err)
	}

	handlers := make(CobraHandlerRegistry)
	resolvedHandlers := make(ResolvedCobraHandlerRegistry)
	for _, record := range manifest.Commands {
		if !record.Runnable {
			continue
		}
		registered, err := registry.LookupHandlers(record.Handler.ID)
		if err != nil {
			return nil, fmt.Errorf("build session family command: %w", err)
		}
		handler := registered
		if handler.ResolvedRunE != nil {
			resolvedHandlers[record.Handler.ID] = handler.ResolvedRunE
			continue
		}
		handlers[record.Handler.ID] = func(
			cmd *cobra.Command,
			args []string,
			_ map[string]any,
			_ resolvedinput.Inputs,
		) error {
			if handler.PreRunE != nil {
				if err := handler.PreRunE(cmd, args); err != nil {
					return err
				}
			}
			return handler.RunE(cmd, args)
		}
	}

	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build session family command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build session family command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord

	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		CobraHandlers:           handlers,
		ResolvedCobraHandlers:   resolvedHandlers,
		GuardUnknownSubcommands: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build session family command: %w", err)
	}
	root.SilenceUsage = true
	return root, nil
}

// FactoryConfigInitFamilyComponents holds detached factory/config/init commands
// before production root wiring attaches them as siblings.
type FactoryConfigInitFamilyComponents struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

// NewFactoryConfigInitFamilyComponents builds the complete family through the
// generic manifest constructor and attaches transport handlers by stable ID.
func NewFactoryConfigInitFamilyComponents(
	handler commandregistry.FactoryConfigInitHandler,
) (FactoryConfigInitFamilyComponents, error) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewFactoryConfigInitFamilyComponentsFromManifest(manifest, handler)
}

// NewFactoryConfigInitFamilyComponentsFromManifest projects and detaches the
// complete factory/config/init family from one canonical manifest snapshot.
func NewFactoryConfigInitFamilyComponentsFromManifest(
	manifest climanifest.Manifest,
	handler commandregistry.FactoryConfigInitHandler,
) (FactoryConfigInitFamilyComponents, error) {
	if handler == nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	resolvedHandlers, err := factoryConfigInitResolvedHandlers(manifest, handler)
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, err
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers:   resolvedHandlers,
		GuardUnknownSubcommands: true,
	})
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: %w", err)
	}
	components, err := detachFactoryConfigInitComponents(root)
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, err
	}
	if err := preserveFactoryConfigInitArgumentDiagnostics(components, manifest); err != nil {
		return FactoryConfigInitFamilyComponents{}, err
	}
	return components, nil
}

func factoryConfigInitResolvedHandlers(
	manifest climanifest.Manifest,
	handler commandregistry.FactoryConfigInitHandler,
) (ResolvedCobraHandlerRegistry, error) {
	bindings := map[string]ResolvedCobraHandler{
		"you.factory.query":           handler.FactoryQuery,
		"you.factory.list":            handler.FactoryList,
		"you.factory.create":          handler.FactoryCreate,
		"you.factory.update":          handler.FactoryUpdate,
		"you.factory.delete":          handler.FactoryDelete,
		"you.factory.replace-current": handler.FactoryReplaceCurrent,
		"you.factory.config.validate": handler.FactoryConfigValidate,
		"you.factory.config.flatten":  handler.FactoryConfigFlatten,
		"you.factory.config.expand":   handler.FactoryConfigExpand,
		"you.init":                    handler.Init,
	}
	resolved := make(ResolvedCobraHandlerRegistry, len(bindings))
	for commandID, binding := range bindings {
		if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID(commandID); err != nil {
			return nil, fmt.Errorf("build factory/config/init family commands: %w", err)
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, fmt.Errorf("build factory/config/init family commands: %w", err)
		}
		if !record.Runnable || record.Handler.ID == "" {
			return nil, fmt.Errorf("build factory/config/init family commands: command %q must declare a handler", commandID)
		}
		resolved[record.Handler.ID] = binding
	}
	return resolved, nil
}

func detachFactoryConfigInitComponents(root *cobra.Command) (FactoryConfigInitFamilyComponents, error) {
	findAndDetach := func(name string) (*cobra.Command, error) {
		command, _, err := root.Find([]string{name})
		if err != nil {
			return nil, err
		}
		root.RemoveCommand(command)
		return command, nil
	}
	factory, err := findAndDetach("factory")
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: find factory: %w", err)
	}
	config, err := findAndDetach("config")
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: find config: %w", err)
	}
	initCommand, err := findAndDetach("init")
	if err != nil {
		return FactoryConfigInitFamilyComponents{}, fmt.Errorf("build factory/config/init family commands: find init: %w", err)
	}
	return FactoryConfigInitFamilyComponents{Factory: factory, Config: config, Init: initCommand}, nil
}

func preserveFactoryConfigInitArgumentDiagnostics(
	components FactoryConfigInitFamilyComponents,
	manifest climanifest.Manifest,
) error {
	commands := []struct {
		id   string
		root *cobra.Command
		path []string
	}{
		{id: "you.factory.create", root: components.Factory, path: []string{"create"}},
		{id: "you.factory.update", root: components.Factory, path: []string{"update"}},
		{id: "you.factory.delete", root: components.Factory, path: []string{"delete"}},
		{id: "you.factory.config.validate", root: components.Factory, path: []string{"config", "validate"}},
		{id: "you.factory.config.flatten", root: components.Factory, path: []string{"config", "flatten"}},
		{id: "you.factory.config.expand", root: components.Factory, path: []string{"config", "expand"}},
	}
	for _, item := range commands {
		command, _, err := item.root.Find(item.path)
		if err != nil {
			return fmt.Errorf("build factory/config/init family commands: find %q: %w", item.id, err)
		}
		record, err := manifest.CommandByID(item.id)
		if err != nil {
			return fmt.Errorf("build factory/config/init family commands: %w", err)
		}
		preserveExactArgumentDiagnostic(command, record)
	}
	return nil
}
