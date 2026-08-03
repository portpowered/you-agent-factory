package wire

import (
	"context"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
)

// acpServerFactoryDefinitions adapts the two Factory Definitions operations
// that are actually constructible at process scope -- effective-catalog
// listing and named-Factory cross-root resolution, both session-free -- to
// the full factorydefinitions.Service interface the Chat Sessions Factory
// target-catalog root depends on. Every other Service method is
// unimplemented at this scope: it requires a live Factory Session's
// activation and validation ports, which do not exist before one is opened.
type acpServerFactoryDefinitions struct {
	factorydefinitions.UnimplementedService
	listEffective       factorydefinitions.EffectiveFactoryCatalogOperation
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog
}

func (a acpServerFactoryDefinitions) ListEffectiveFactories(
	ctx context.Context,
	req factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return a.listEffective(ctx, req)
}

func (a acpServerFactoryDefinitions) ResolveNamedFactory(
	ctx context.Context,
	req factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	resolution, err := a.namedFactoryCatalog.ResolveNamedFactoryAcrossRoots(req.ProjectRoot, req.GlobalRoot, req.Name)
	if err != nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	if resolution == nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
	}
	return factorydefinitions.ResolveNamedFactoryResult{Resolution: *resolution}, nil
}

// The five methods below complete factorydefinitions.Service beyond what
// factorydefinitions.UnimplementedService already stubs. None is reachable
// through the production ACP Factory target-catalog path (session/new,
// session/set_config_option, and /factory call only ListEffectiveFactories
// and ResolveNamedFactory), so each returns a collaborator-required failure
// rather than session-scoped behavior.
func (a acpServerFactoryDefinitions) ActivateNamedFactory(context.Context, string) error {
	return fmt.Errorf("activate named factory: live Factory Session collaborator is required")
}

func (a acpServerFactoryDefinitions) Save(
	context.Context, string, factorydefinitions.SaveMode, factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, fmt.Errorf("save factory: live Factory Session collaborator is required")
}

func (a acpServerFactoryDefinitions) GetCurrentNamedFactory(context.Context) (*factorydefinitions.FactorySnapshot, error) {
	return nil, fmt.Errorf("get current named factory: live Factory Session collaborator is required")
}

func (a acpServerFactoryDefinitions) GetCurrentFactoryForSession(
	context.Context, string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, fmt.Errorf("get current factory for session: live Factory Session collaborator is required")
}

func (a acpServerFactoryDefinitions) CurrentFactoryDefinitionVersionAtRoot(
	string, string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, fmt.Errorf("current factory definition version at root: live Factory Session collaborator is required")
}

// provideACPServerFactoryDefinitions constructs the process-scoped Factory
// Definitions slice the production ACP consumer's Factory target catalog
// resolves against, from the same named-path and effective-catalog ports
// the rest of this graph composes.
func provideACPServerFactoryDefinitions(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog,
) factorydefinitions.Service {
	return acpServerFactoryDefinitions{
		listEffective:       listEffective,
		namedFactoryCatalog: namedFactoryCatalog,
	}
}

// acpServerResolveHomeDir is the ACP stdio server's own home-directory
// resolver type, distinct from every other func() (string, error) provider
// this graph registers, so Wire's generated bundle can bind it uniquely.
type acpServerResolveHomeDir func() (string, error)

// provideACPServerResolveHomeDir constructs the operator home directory
// resolver the production ACP stdio server uses to derive the Operator
// Settings document path and Factory discovery roots for "session/new". It
// is os.UserHomeDir directly, with no dependency bag or lookup indirection.
func provideACPServerResolveHomeDir() acpServerResolveHomeDir {
	return os.UserHomeDir
}

// provideACPServer constructs the production ACP stdio Server from the same
// canonical chatsessions.Service and chatsessions.FactoryTargetCatalogService
// instances the rest of this graph composes, so the real "session/new",
// "session/set_config_option", and "/factory" consumer observes the one
// process-scoped Chat Sessions authority instead of a second, independently
// constructed instance. Construction alone performs no I/O; it starts no
// goroutine, process, listener, session, or persistence.
func provideACPServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	resolveHomeDir acpServerResolveHomeDir,
) acp.Server {
	return acpwire.NewServer(logger, chatSessions, catalog, resolveHomeDir)
}
