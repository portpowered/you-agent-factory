package cli

import (
	"context"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
)

func newSessionHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) (*commandregistry.Registry, climanifestcobra.SessionFamilyBindings, error) {
	configs := newSessionFamilyBindings()
	diagnosticsBinding := commandregistry.SessionDiagnosticsBinding{
		Verbose: diagnostics.verboseEnabled, Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer,
	}
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: commandregistry.SessionCreateRunE(commandregistry.SessionCreateBinding{
			Config: configs.Create, Server: &globals.server, JSON: &globals.json,
			SessionDiagnosticsBinding: diagnosticsBinding, CreateSession: options.CreateSession,
		}),
		ListRunE: commandregistry.SessionListRunE(commandregistry.SessionListBinding{
			Config: configs.List, SessionDiagnosticsBinding: diagnosticsBinding,
			Server: &globals.server, JSON: &globals.json,
			Prepare: sessionListPrepare(options), ListSessions: options.ListSessions,
		}),
		ShowRunE: commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
			Server: &globals.server, JSON: &globals.json, Verbose: diagnostics.verboseEnabled,
			Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer, ShowSession: options.ShowSession,
		}),
		DeleteRunE: commandregistry.SessionDeleteRunE(commandregistry.SessionDeleteBinding{
			Config: configs.Delete, JSON: &globals.json,
			SessionDiagnosticsBinding: diagnosticsBinding, DeleteSession: options.DeleteSession,
		}),
		DispatchesRunE: commandregistry.SessionDispatchesRunE(commandregistry.SessionDispatchesBinding{
			Config: configs.Dispatches, Server: &globals.server, JSON: &globals.json,
			SessionDiagnosticsBinding: diagnosticsBinding, ListDispatches: options.ListSessionDispatches,
		}),
		PauseRunE:  sessionLifecycleRunE(configs.Pause, globals, diagnosticsBinding, options.PauseSession),
		ResumeRunE: sessionLifecycleRunE(configs.Resume, globals, diagnosticsBinding, options.ResumeSession),
	})
	return registry, configs, err
}

func sessionLifecycleRunE(
	config *sessioncli.LifecycleControlConfig,
	globals *cliGlobalOptions,
	diagnostics commandregistry.SessionDiagnosticsBinding,
	control func(sessioncli.LifecycleControlConfig) error,
) commandregistry.RunE {
	return commandregistry.SessionLifecycleRunE(commandregistry.SessionLifecycleBinding{
		Config: config, Server: &globals.server, JSON: &globals.json,
		SessionDiagnosticsBinding: diagnostics, Control: control,
	})
}

func newSessionFamilyBindings() climanifestcobra.SessionFamilyBindings {
	return climanifestcobra.SessionFamilyBindings{
		Create:     &sessioncli.CreateConfig{Port: defaultcmd.FactoryPort},
		List:       &sessioncli.ListConfig{Port: defaultcmd.FactoryPort, Scope: "live"},
		Delete:     &sessioncli.DeleteConfig{Port: defaultcmd.FactoryPort},
		Dispatches: &sessioncli.DispatchesConfig{},
		Pause:      &sessioncli.LifecycleControlConfig{},
		Resume:     &sessioncli.LifecycleControlConfig{},
	}
}

func sessionListPrepare(options CommandFactory) func(context.Context, *sessioncli.ListConfig) error {
	return func(ctx context.Context, cfg *sessioncli.ListConfig) error {
		scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
		if scope != fse.SessionListScopePersisted && scope != fse.SessionListScopeAll {
			return nil
		}
		service, err := buildWorkflowExecutionService(ctx, options, string(fse.ExecutionProviderFake), "", "", "")
		if err != nil {
			return err
		}
		cfg.DurableLister = service.ListSessions
		cfg.DurableCloser = service
		return nil
	}
}
