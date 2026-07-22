package cli

import (
	"fmt"
	"strings"

	initcmd "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/spf13/cobra"
)

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) factoryConfigInitProductionCommands {
	registry, bindings, err := newFactoryConfigInitWiring(globals, diagnostics, options)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init handler registry: %v", err))
	}
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(registry, bindings)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}

func newFactoryConfigInitWiring(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	rootOptions CommandFactory,
) (*commandregistry.Registry, climanifestcobra.FactoryConfigInitFlagBindings, error) {
	state := factoryConfigInitBindingState{
		listDir:      defaultcmd.FactoryDir,
		createDir:    defaultcmd.FactoryDir,
		updateDir:    defaultcmd.FactoryDir,
		deleteDir:    defaultcmd.FactoryDir,
		initDir:      defaultcmd.FactoryDir,
		initType:     string(initcmd.DefaultScaffoldType),
		initExecutor: initcmd.DefaultStarterExecutor,
	}
	bindings := factoryConfigInitFlagBindingsFromState(&state)
	registry, err := newFactoryConfigInitHandlerRegistry(globals, diagnostics, rootOptions, &state)
	if err != nil {
		return nil, climanifestcobra.FactoryConfigInitFlagBindings{}, err
	}
	return registry, bindings, nil
}

type factoryConfigInitBindingState struct {
	listDir          string
	createDir        string
	updateDir        string
	deleteDir        string
	createFrom       string
	createSetCurrent bool
	updateFrom       string
	replaceSessionID string
	initDir          string
	initType         string
	initExecutor     string
}

func factoryConfigInitFlagBindingsFromState(state *factoryConfigInitBindingState) climanifestcobra.FactoryConfigInitFlagBindings {
	return climanifestcobra.FactoryConfigInitFlagBindings{
		FactoryListDir:          &state.listDir,
		FactoryCreateDir:        &state.createDir,
		FactoryUpdateDir:        &state.updateDir,
		FactoryDeleteDir:        &state.deleteDir,
		FactoryCreateFrom:       &state.createFrom,
		FactoryCreateSetCurrent: &state.createSetCurrent,
		FactoryUpdateFrom:       &state.updateFrom,
		FactoryReplaceSessionID: &state.replaceSessionID,
		InitDir:                 &state.initDir,
		InitType:                &state.initType,
		InitExecutor:            &state.initExecutor,
		FlagUsages:              factoryConfigInitFlagUsages(),
	}
}

func factoryConfigInitFlagUsages() map[string]string {
	return map[string]string{
		"dir":         "factory root directory containing named factories",
		"from":        "path to an existing factory.json payload (required)",
		"set-current": "update .current-factory to the created name",
		"session":     "target one live factory session; omit to use the default compatibility session",
		"type":        "scaffold type to generate (supported: default, ralph)",
		"executor": fmt.Sprintf(
			"starter scaffold to generate (%s)",
			strings.Join(initcmd.SupportedStarterExecutors(), ", "),
		),
	}
}

func newFactoryConfigInitHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	rootOptions CommandFactory,
	state *factoryConfigInitBindingState,
) (*commandregistry.Registry, error) {
	if state == nil {
		fallback := factoryConfigInitBindingState{
			listDir:      defaultcmd.FactoryDir,
			createDir:    defaultcmd.FactoryDir,
			updateDir:    defaultcmd.FactoryDir,
			deleteDir:    defaultcmd.FactoryDir,
			initDir:      defaultcmd.FactoryDir,
			initType:     string(initcmd.DefaultScaffoldType),
			initExecutor: initcmd.DefaultStarterExecutor,
		}
		state = &fallback
	}

	return commandregistry.NewFactoryConfigInitRegistry(commandregistry.FactoryConfigInitHandlers{
		FactoryQueryRunE: commandregistry.FactoryQueryRunE(commandregistry.FactoryQueryBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Query:             rootOptions.QueryFactory,
		}),
		FactoryListRunE: commandregistry.FactoryListRunE(commandregistry.FactoryListBinding{
			Dir:  &state.listDir,
			JSON: &globals.json,
			List: rootOptions.ListFactories,
		}),
		FactoryCreateRunE: commandregistry.FactoryCreateRunE(commandregistry.FactoryCreateBinding{
			Dir:        &state.createDir,
			From:       &state.createFrom,
			SetCurrent: &state.createSetCurrent,
			JSON:       &globals.json,
			Create:     rootOptions.CreateFactoryFromFile,
		}),
		FactoryUpdateRunE: commandregistry.FactoryUpdateRunE(commandregistry.FactoryUpdateBinding{
			Dir:    &state.updateDir,
			From:   &state.updateFrom,
			JSON:   &globals.json,
			Update: rootOptions.UpdateFactoryFromFile,
		}),
		FactoryDeleteRunE: commandregistry.FactoryDeleteRunE(commandregistry.FactoryDeleteBinding{
			Dir:    &state.deleteDir,
			JSON:   &globals.json,
			Delete: rootOptions.DeleteFactory,
		}),
		FactoryReplaceCurrentRunE: commandregistry.FactoryReplaceCurrentRunE(commandregistry.FactoryReplaceCurrentBinding{
			Server:            &globals.server,
			SessionID:         &state.replaceSessionID,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			DiagnosticsWriter: diagnostics.writer,
			ReplaceCurrent:    rootOptions.ReplaceFactoryCurrent,
		}),
		FactoryConfigValidateRunE: commandregistry.FactoryConfigValidateRunE(commandregistry.FactoryConfigValidateBinding{
			JSON:     &globals.json,
			Validate: rootOptions.ValidateFactory,
		}),
		FactoryConfigFlattenRunE: commandregistry.FactoryConfigFlattenRunE(commandregistry.FactoryConfigFlattenBinding{
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Flatten:           rootOptions.FlattenFactoryConfig,
		}),
		FactoryConfigExpandRunE: commandregistry.FactoryConfigExpandRunE(commandregistry.FactoryConfigExpandBinding{
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Expand:            rootOptions.ExpandFactoryConfig,
		}),
		ConfigInitRunE: commandregistry.ConfigInitRunE(commandregistry.ConfigInitBinding{
			HomeDir:           rootOptions.homeDir,
			JSON:              func() bool { return globals.json },
			DiagnosticsWriter: diagnostics.writer,
			Verbose:           diagnostics.verboseEnabled,
			Init:              rootOptions.InitSystemConfig,
		}),
		InitRunE: commandregistry.InitRunE(commandregistry.InitBinding{
			Dir:               &state.initDir,
			Type:              &state.initType,
			Executor:          &state.initExecutor,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Init:              rootOptions.InitFactory,
		}),
	})
}
