package cli

import (
	"context"
	"fmt"
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
		RemoteSessions: options.SessionsCLI,
		LocalSessions:  options.LocalSessionsCLI,
		PrepareList:    sessionListPrepare(options),
		Diagnostics:    diagnostics.writer,
	})
}

func sessionListPrepare(options CommandFactory) func(context.Context, *sessioncli.ListConfig) error {
	return func(ctx context.Context, cfg *sessioncli.ListConfig) error {
		if cfg.LiveOnly || cfg.HistoryOnly {
			return nil
		}
		scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
		if scope != fse.SessionListScopePersisted && scope != fse.SessionListScopeAll {
			return nil
		}
		if options.FactorySessions == nil {
			return fmt.Errorf("list durable Factory Sessions: Factory Sessions service is required")
		}
		cfg.DurableLister = func(ctx context.Context, request fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
			result, err := options.FactorySessions.List(ctx, fse.SessionListRequest{
				Mode:    fse.SessionOperationModeDurable,
				Filters: request.Filters,
			})
			if err != nil {
				return fse.ListSessionsResult{}, err
			}
			return fse.ListSessionsResult{Scope: request.Scope, DurableSessions: result.DurableSessions}, nil
		}
		return nil
	}
}
