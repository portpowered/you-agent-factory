package wire

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
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

// acpServerResolveHomeDir is the ACP stdio server's own home-directory
// resolver type, distinct from every other func() (string, error) provider
// this graph registers, so Wire's generated bundle can bind it uniquely.
type acpServerResolveHomeDir func() (string, error)

// provideACPServerResolveHomeDir constructs the operator home directory
// resolver the production ACP stdio server uses to derive the Operator
// Settings document path and Factory discovery roots for "session/new". ACP
// and Factory Sessions share this exact external-effect edge so one process
// cannot resolve two different operator homes.
func provideACPServerResolveHomeDir(edges serviceedges.Edges) acpServerResolveHomeDir {
	if edges.FactorySessionResolveHomeDirectory != nil {
		return acpServerResolveHomeDir(edges.FactorySessionResolveHomeDirectory)
	}
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
		defaults, err := resolveOperatorDefaults(homeDir, acpOperatorDefaultsEnvironment(), operatorsettings.FlagOverrides{})
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

// acpOperatorDefaultsEnvironment reads the operator-default environment layer
// for an ACP-selected Factory target runtime.
//
// The CLI has always supplied this layer (see resolveOperatorDefaults in
// pkg/transports/cli), so `YOU_DEFAULT_WORKER_MODEL_PROVIDER` selects the
// Worker provider for `you run`. The ACP resolver passed an empty layer, which
// silently dropped both variables and left the runtime with whatever the
// persisted Operator Settings document alone supplied.
//
// That gap is invisible for a Factory whose workers name their own provider,
// and fatal for one whose workers do not -- a JavaScript Factory's agent.run
// children carry no provider, so with no operator default their dispatch is
// rejected before any provider runs. An ACP client cannot pass `--provider`,
// so the environment is the only layer it has.
//
// Reading the process environment directly matches how this file already
// resolves its own wire-log configuration.
func acpOperatorDefaultsEnvironment() operatorsettings.Defaults {
	return operatorsettings.Defaults{
		WorkerModelProvider: strings.TrimSpace(os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider)),
		WorkerModel:         strings.TrimSpace(os.Getenv(operatorsettings.EnvDefaultWorkerModel)),
	}
}

// provideACPServerFactoryTarget constructs Factory Sessions' own on-demand
// activation the production ACP prompt-delegation consumer starts or invokes
// a Factory Session through. Unlike the CLI daemon's
// single fixed-project bootstrap, ACP episodes select their Factory target
// dynamically per session, so this activates one live runtime per target the
// first time it is needed (through the same invocation-mode Runtime Opening
// path the CLI's one-shot named invocation already uses) instead of relying
// on the process-scoped factorysessions.Service, which stays permanently
// inert outside the CLI daemon bootstrap. Construction alone performs no
// I/O and opens no runtime.
//
// This returns the concrete *factorysessionwire.OnDemandFactoryTargetService
// (not the narrower factorysessions.TargetExecutionService capability)
// precisely so a second consumer -- provideApplicationProcessLifecycle --
// can reach its io.Closer-satisfying Close method and compose it into the
// process's own reachable shutdown path; see
// provideACPServerFactoryTargetService for the interface-narrowing provider
// the ACP transport itself consumes. Wire's own provider memoization
// guarantees both consumers observe this exact same singleton, not two
// independently constructed activations.
func provideACPServerFactoryTarget(
	openRuntime factorysessionwire.InvocationRuntimeOpening,
	resolveTarget factorysessionwire.FactoryTargetRuntimeResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*factorysessionwire.OnDemandFactoryTargetService, error) {
	return factorysessionwire.NewOnDemandFactoryTargetService(
		openRuntime,
		resolveTarget,
		generateSessionID,
		logger,
	)
}

// provideACPServerFactoryTargetService exposes the on-demand Factory
// Sessions activation singleton directly as the production ACP
// prompt-delegation consumer's Factory Sessions-owned
// factorysessions.TargetExecutionService dependency -- no adapter changes
// contexts, identifiers, requests, results, or errors. Wire's own provider
// memoization guarantees this shares the exact same activation singleton
// provideApplicationProcessLifecycle reaches for shutdown (see
// provideACPServerFactoryTarget), since both depend on the identical
// *factorysessionwire.OnDemandFactoryTargetService type, which satisfies
// factorysessions.TargetExecutionService structurally (see
// pkg/services/factory_sessions/wire/on_demand_factory_target.go).
func provideACPServerFactoryTargetService(
	target *factorysessionwire.OnDemandFactoryTargetService,
) factorysessions.TargetExecutionService {
	return target
}

// provideACPServer constructs the production ACP stdio Server from the same
// canonical chatsessions.Service, Events service, and Factory Sessions-owned
// target-execution capability instances the rest of this graph composes.
func provideACPServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factoryTarget factorysessions.TargetExecutionService,
	eventsService events.Service,
	resolveHomeDir acpServerResolveHomeDir,
	responseBridge acp.ResponseBridge,
	wireRecorder acp.WireRecorder,
) acp.Server {
	return acpwire.NewServer(
		logger, chatSessions, catalog, factoryTarget, eventsService,
		resolveHomeDir, responseBridge, wireRecorder,
	)
}

// ACP wire recording is on by default. The point of the artifact is that a
// customer who hits a problem already has the evidence; a recorder they must
// know to enable before reproducing is one they will not have running when it
// matters.
const (
	acpWireLogEnvironment    = "YOU_ACP_WIRE_LOG"
	acpWireLogDirEnvironment = "YOU_ACP_WIRE_LOG_DIR"
	acpWireLogDisabledValue  = "off"
)

// provideACPWireRecorder constructs the per-connection ACP wire transcript
// opener.
//
// The environment is read here, in the composition root, rather than inside
// the transport or any service. The recorder writes only to its own file
// handle, so it structurally cannot reach the protocol stream that `you server
// acp` reserves on stdout.
func provideACPWireRecorder(
	edges serviceedges.Edges,
	paths platformruntimeartifact.Reserver,
	clock runtimeArtifactClock,
	resolveHomeDir acpServerResolveHomeDir,
) (acp.WireRecorder, error) {
	if edges.ACPWireRecorder != nil {
		return edges.ACPWireRecorder, nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(acpWireLogEnvironment)), acpWireLogDisabledValue) {
		return nil, nil
	}
	opener, err := wiretranscript.NewOpener(paths, wireTranscriptClock(clock))
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(os.Getenv(acpWireLogDirEnvironment))
	return func(connectionID string) (acp.WireTranscript, error) {
		directory := root
		if directory == "" {
			home, homeErr := resolveHomeDir()
			if homeErr != nil {
				return nil, homeErr
			}
			directory = wiretranscript.Root(home)
		}
		return opener.Open(wiretranscript.OpeningRequest{
			RootDirectory: directory,
			ConnectionID:  connectionID,
			StartTimeUTC:  clock(),
		})
	}, nil
}

// provideACPServerResponseBridge constructs the production response-bridge
// collaborator the ACP prompt-delegation consumer calls around its
// synchronous Factory dispatch call (see dispatchFactoryTurn's two Factory
// dispatch branches in pkg/transports/acp/internal/stdio/session_prompt.go):
// a thin closure with exactly acp.ResponseBridge's signature that forwards
// to the Chat Sessions-owned response bridge, the owning service's own
// translation/drain-loop/concurrency implementation. pkg/wire composes this
// closure (construction only, no I/O, no goroutine); the ACP transport never
// implements Factory response-event translation, and the ACP transport
// package that calls the constructed closure never holds a raw concurrency
// primitive of its own either.
func provideACPServerResponseBridge(bridge *chatsessionswire.ResponseBridge) acp.ResponseBridge {
	return func(
		ctx context.Context,
		chatSessionID string,
		sessionVersion uint64,
		factorySessionID string,
		liveDrain func(context.Context),
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		return bridge.Run(ctx, chatSessionID, sessionVersion, factorySessionID, liveDrain, invoke)
	}
}

// provideChatSessionsResponseBridge constructs the Chat Sessions-owned
// response-event bridge over the singular production Chat Sessions and
// Factory Sessions services, plus the canonical logging abstraction.
func provideChatSessionsResponseBridge(
	chatSessions chatsessions.Service,
	factoryTarget factorysessions.TargetExecutionService,
	eventsService events.Service,
	logger logging.Logger,
) *chatsessionswire.ResponseBridge {
	return chatsessionswire.NewResponseBridge(chatSessions, factoryTarget, eventsService, logger)
}

// wireTranscriptClock adapts the injected runtime-artifact clock to the
// transcript package's Clock, so the transcript never reaches for time.Now
// itself.
type wireTranscriptClock runtimeArtifactClock

func (c wireTranscriptClock) Now() time.Time { return c() }
