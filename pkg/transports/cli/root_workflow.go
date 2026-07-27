package cli

import (
	"context"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func newSessionHandlerRegistry(
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) (*commandregistry.Registry, error) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		return nil, err
	}
	return commandregistry.NewSessionResolvedRegistry(manifest, commandregistry.SessionResolvedServices{
		Sessions:    options.SessionsCLI,
		PrepareList: sessionListPrepare(options),
		Diagnostics: diagnostics.writer,
	})
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
