package wire

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
	"go.uber.org/zap"
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

// provideACPServerFactoryTargetRuntimeResolver constructs the closure that
// turns one ACP-selected Factory target identity (the same "factory:<name>"
// reference session/set_config_option's changeTarget already validates and
// binds) and the requesting Chat Session's exact editor working root into
// the concrete Runtime Opening request a dynamically-selected Factory
// Session activation needs -- the same named-Factory cross-root resolution
// and operator defaults resolution the rest of this graph already composes,
// not a second independently constructed lookup.
func provideACPServerFactoryTargetRuntimeResolver(
	resolveHomeDir acpServerResolveHomeDir,
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog,
	resolveOperatorDefaults operatorsettings.DefaultsResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
) factorysessionwire.FactoryTargetRuntimeResolver {
	return func(ctx context.Context, factoryTargetID, workingRoot string) (factorysessions.RuntimeOpeningRequest, error) {
		if err := ctx.Err(); err != nil {
			return factorysessions.RuntimeOpeningRequest{}, err
		}
		homeDir, err := resolveHomeDir()
		if err != nil {
			return factorysessions.RuntimeOpeningRequest{}, err
		}
		roots, err := factorydefinitions.ResolveNamedFactoryRoots(homeDir, workingRoot)
		if err != nil {
			return factorysessions.RuntimeOpeningRequest{}, err
		}
		bareName := strings.TrimPrefix(factoryTargetID, operatorsettings.ACPFactoryTargetNamespace)
		resolved, err := namedFactoryCatalog.ResolveNamedFactoryAcrossRoots(roots.Project, roots.Global, bareName)
		if err != nil {
			return factorysessions.RuntimeOpeningRequest{}, err
		}
		if resolved == nil {
			return factorysessions.RuntimeOpeningRequest{}, factorydefinitions.ErrNamedFactoryNotFound
		}
		defaults, err := resolveOperatorDefaults(homeDir, operatorsettings.Defaults{}, operatorsettings.FlagOverrides{})
		if err != nil {
			return factorysessions.RuntimeOpeningRequest{}, err
		}
		artifacts := artifactRoots(homeDir)
		return factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
				Directory: resolved.FactoryDir,
			},
			FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
				Mode:             factorydefinitions.RuntimeModeService,
				LogDirectory:     artifacts.Logs,
				MetricsDirectory: artifacts.Metrics,
			},
			FactorySession: factorysessions.SessionRuntimeOpeningRequest{
				SystemConfigHome: homeDir,
			},
			OperatorDefaults: defaults,
		}, nil
	}
}

// provideACPServerFactoryTarget constructs the consumer-owned, on-demand
// Factory Sessions activation the production ACP prompt-delegation consumer
// starts or invokes a Factory Session through. Unlike the CLI daemon's
// single fixed-project bootstrap, ACP episodes select their Factory target
// dynamically per session, so this activates one live runtime per target the
// first time it is needed (through the same invocation-mode Runtime Opening
// path the CLI's one-shot named invocation already uses) instead of relying
// on the process-scoped factorysessions.Service, which stays permanently
// inert outside the CLI daemon bootstrap. Construction alone performs no
// I/O and opens no runtime.
//
// This returns the concrete *factorysessionwire.OnDemandFactoryTargetService
// (not the narrower acp.FactoryTargetService interface) precisely so a
// second consumer -- provideApplicationProcessLifecycle -- can reach its
// io.Closer-satisfying Close method and compose it into the process's own
// reachable shutdown path; see provideACPServerFactoryTargetService for the
// interface-narrowing adapter the ACP transport itself consumes. Wire's own
// provider memoization guarantees both consumers observe this exact same
// singleton, not two independently constructed activations.
func provideACPServerFactoryTarget(
	openRuntime *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	resolveTarget factorysessionwire.FactoryTargetRuntimeResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*factorysessionwire.OnDemandFactoryTargetService, error) {
	return factorysessionwire.NewOnDemandFactoryTargetService(
		openRuntime,
		projectRuntimeOpeningExternalEffects(edges),
		resolveTarget,
		generateSessionID,
		logger,
	)
}

// provideACPServerFactoryTargetService wraps the on-demand Factory Sessions
// activation singleton through the existing, consumer-owned Factory Sessions
// shim (chat_sessions/wire.NewFactoryTargetService, a thin construction
// wrapper over chat_sessions/internal/factorysessionsshim.New) rather than
// exposing the activation singleton's own methods directly -- so the
// production ACP prompt-delegation consumer's every Factory Session call
// actually flows through that existing shim, not a second, independently
// constructed peer contract. Wire's own provider memoization guarantees this
// still shares the exact same activation singleton
// provideApplicationProcessLifecycle reaches for shutdown (see
// provideACPServerFactoryTarget), since both depend on the identical
// *factorysessionwire.OnDemandFactoryTargetService type.
//
// pkg-boundary forbids pkg/transports/acp from importing another service's
// wire subpackage, so it declares its own narrow acp.FactoryTargetService
// interface (an exact structural match for
// chatsessionswire.FactoryTargetService); only pkg/wire, exempt from that
// rule, may compose the two.
func provideACPServerFactoryTargetService(
	target *factorysessionwire.OnDemandFactoryTargetService,
) acp.FactoryTargetService {
	return chatsessionswire.NewFactoryTargetService(target)
}

// provideACPServer constructs the production ACP stdio Server from the same
// canonical chatsessions.Service, chatsessions.FactoryTargetCatalogService,
// events.Service, and Factory Sessions shim instances the rest of this graph
// composes, so the real "session/new", "session/set_config_option",
// "/factory", and ordinary prompt-delegation consumer observes the one
// process-scoped Chat Sessions, Events, and Factory Sessions authority
// instead of a second, independently constructed instance. Construction
// alone performs no I/O; it starts no goroutine, process, listener, session,
// or persistence.
func provideACPServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factoryTarget acp.FactoryTargetService,
	eventsService events.Service,
	resolveHomeDir acpServerResolveHomeDir,
	responseBridge acp.ResponseBridge,
) acp.Server {
	return acpwire.NewServer(logger, chatSessions, catalog, factoryTarget, eventsService, resolveHomeDir, responseBridge)
}

// provideACPServerResponseBridge constructs the production response-bridge
// collaborator the ACP prompt-delegation consumer calls around its
// synchronous Factory dispatch call (see dispatchFactoryTurn's two Factory
// dispatch branches in pkg/transports/acp/internal/stdio/session_prompt.go):
// a thin closure with exactly acp.ResponseBridge's signature that forwards
// to chat_sessions/wire.RunWithResponseBridge, the owning service's own
// translation/drain-loop/concurrency implementation. pkg/wire composes this
// closure (construction only, no I/O, no goroutine); this transport never
// implements Factory response-event translation, and the ACP transport
// package that calls the constructed closure never holds a raw concurrency
// primitive of its own either.
func provideACPServerResponseBridge() acp.ResponseBridge {
	return func(
		ctx context.Context,
		chatSessions chatsessions.Service,
		subscriber acp.FactoryTargetService,
		chatSessionID string,
		sessionVersion uint64,
		factorySessionID string,
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		return chatsessionswire.RunWithResponseBridge(ctx, chatSessions, subscriber, chatSessionID, sessionVersion, factorySessionID, invoke)
	}
}
