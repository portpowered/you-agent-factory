package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	operatorsettingsmcp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingmcp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	mcpcontent "github.com/portpowered/infinite-you/pkg/transports/mcp/content"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
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
		profile, hasProfile := acp.InvocationProfileFromContext(ctx)
		homeDir := strings.TrimSpace(profile.HomeDir)
		if homeDir == "" {
			var err error
			homeDir, err = resolveHomeDir()
			if err != nil {
				return factorysessions.RuntimeOpeningRequest{}, err
			}
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
		environment := acpOperatorDefaultsEnvironment()
		if hasProfile {
			environment = operatorsettings.Defaults{
				WorkerModelProvider: strings.TrimSpace(profile.WorkerModelProvider),
				WorkerModel:         strings.TrimSpace(profile.WorkerModel),
			}
		}
		defaults, err := resolveOperatorDefaults(homeDir, environment, operatorsettings.FlagOverrides{})
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

type mcpServerBuilder func(
	factorysessionwire.DurableExecutionService,
	recordings.Service,
	factorysessionwire.RequestPreparation,
	factoryruntime.WorkflowPreviewOperation,
	factorysessions.TargetExecutionService,
) (*mcpserver.Server, error)

type mcpProviderConfigurer func(context.Context, string) error

func provideMCPProviderConfigurer(settings operatorsettings.Service, providerService providers.Service) mcpProviderConfigurer {
	return func(ctx context.Context, home string) error {
		return configureMCPProvidersAtHome(ctx, settings, providerService, home)
	}
}

// provideMCPServerBuilder composes owner adapters at the Wire boundary. The
// protocol stdio package receives only the resulting inert server and caller
// streams; it does not construct Factory Sessions, Recordings, or workflow
// services while an opening is being selected.
func provideMCPServerBuilder(
	workingDirectory platformfilesystem.WorkingDirectory,
	settings operatorsettings.Service,
	providerService providers.Service,
	settingsFiles operatorsettings.FileSystem,
	homeDirectory factorysessions.HomeDirectoryResolver,
) mcpServerBuilder {
	skills, resources, err := mcpSubagentContent(settings, providerService, settingsFiles, homeDirectory)
	if err != nil {
		return func(factorysessionwire.DurableExecutionService, recordings.Service, factorysessionwire.RequestPreparation, factoryruntime.WorkflowPreviewOperation, factorysessions.TargetExecutionService) (*mcpserver.Server, error) {
			return nil, err
		}
	}
	return func(
		execution factorysessionwire.DurableExecutionService,
		recordingsService recordings.Service,
		prepare factorysessionwire.RequestPreparation,
		workflowPreview factoryruntime.WorkflowPreviewOperation,
		target factorysessions.TargetExecutionService,
	) (*mcpserver.Server, error) {
		workingRoot, err := workingDirectory.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve MCP working directory: %w", err)
		}
		inspection := factorysessionmcp.RecordingsInspection(recordingsService)
		if inspection == nil {
			if bridge := factorysessionmapping.NewDurableInspectionBridge(execution); bridge != nil {
				inspection = recordingmcp.NewLegacyFactorySessionInspection(bridge)
			}
		}
		subagentOperation := mcpserver.ToolOperation(factorysessionmcp.BindToolOperation(
			execution, inspection, prepare, workflowPreview, target, workingRoot, factorysessions.SessionIDGenerator(uuid.NewString),
		))
		return mcpserver.New(mcpserver.Options{
			Skills:    skills,
			Resources: resources,
			ToolOperation: func(ctx context.Context, name string, raw json.RawMessage) (json.RawMessage, error) {
				if name == factorysessionmcp.ToolSubagent {
					if err := configureMCPProviders(ctx, settings, providerService, homeDirectory); err != nil {
						return nil, err
					}
				}
				return subagentOperation(ctx, name, raw)
			},
		})
	}
}

func configureMCPProviders(ctx context.Context, settings operatorsettings.Service, providerService providers.Service, homeDirectory factorysessions.HomeDirectoryResolver) error {
	home, err := resolveMCPHomeDirectory(ctx, homeDirectory)
	if err != nil {
		return err
	}
	return configureMCPProvidersAtHome(ctx, settings, providerService, home)
}

func configureMCPProvidersAtHome(ctx context.Context, settings operatorsettings.Service, providerService providers.Service, home string) error {
	if settings == nil || providerService == nil {
		return fmt.Errorf("operator settings and Providers services are required")
	}
	configurable, ok := providerService.(interface {
		ConfigureACPIntegrations(context.Context, []providers.ACPIntegration) error
	})
	if !ok {
		return fmt.Errorf("Providers ACP configuration is unavailable")
	}
	document, err := settings.LoadDocument(operatorsettings.LoadDocumentRequest{Path: settings.DefaultConfigPath(home)})
	if err != nil {
		return err
	}
	configured := document.Document.Workers.ACP.Integrations
	integrations := make([]providers.ACPIntegration, len(configured))
	for index, integration := range configured {
		integrations[index] = providers.ACPIntegration{ID: integration.ID, Name: providers.ID(integration.Name), Transport: integration.Transport, Command: integration.Command}
	}
	return configurable.ConfigureACPIntegrations(ctx, integrations)
}

func resolveMCPHomeDirectory(ctx context.Context, fallback factorysessions.HomeDirectoryResolver) (string, error) {
	if home := processcontract.HomeDirectory(ctx); home != "" {
		return home, nil
	}
	return fallback()
}

func mcpSubagentContent(
	settings operatorsettings.Service,
	providerService providers.Service,
	files operatorsettings.FileSystem,
	homeDirectory factorysessions.HomeDirectoryResolver,
) ([]mcpserver.SkillEntry, []mcpserver.ResourceRegistration, error) {
	skill := mcpcontent.SkillBytes()
	schema := mcpcontent.SchemaBytes()
	digest := sha256.Sum256(skill)
	frontmatter, err := mcpcontent.SkillFrontmatter()
	if err != nil {
		return nil, nil, err
	}
	entries, err := buildMCPSubagentSkills(frontmatter, digest, skill)
	if err != nil {
		return nil, nil, err
	}
	resources, err := buildMCPResourceRegistrations(settings, providerService, files, homeDirectory, skill, schema)
	if err != nil {
		return nil, nil, err
	}
	return entries, resources, nil
}

func buildMCPSubagentSkills(frontmatter map[string]any, digest [32]byte, skill []byte) ([]mcpserver.SkillEntry, error) {
	entries := make([]mcpserver.SkillEntry, 0, len(mcpgenerated.PrimarySkills()))
	for _, definition := range mcpgenerated.PrimarySkills() {
		if !reflect.DeepEqual(frontmatter, definition.Frontmatter) {
			return nil, fmt.Errorf("MCP skill frontmatter differs from generated manifest")
		}
		entry := mcpserver.SkillEntry{URI: definition.URI, Frontmatter: definition.Frontmatter}
		linkedResources := make([]mcpserver.SkillResource, 0, len(definition.ResourceURIs))
		for _, uri := range definition.ResourceURIs {
			if uri != definition.URI {
				return nil, fmt.Errorf("unsupported MCP skill resource %q", uri)
			}
			linkedResources = append(linkedResources, mcpserver.SkillResource{
				URI: uri, Digest: "sha256:" + hex.EncodeToString(digest[:]), Size: int64(len(skill)),
			})
		}
		entry.Resources = linkedResources
		entries = append(entries, entry)
	}
	return entries, nil
}

func buildMCPResourceRegistrations(settings operatorsettings.Service, providerService providers.Service, files operatorsettings.FileSystem, homeDirectory factorysessions.HomeDirectoryResolver, skill, schema []byte) ([]mcpserver.ResourceRegistration, error) {
	resources := make([]mcpserver.ResourceRegistration, 0, len(mcpgenerated.PrimaryResources()))
	for _, definition := range mcpgenerated.PrimaryResources() {
		resource := &mcp.Resource{URI: definition.URI, Name: definition.Name, Description: definition.Description, MIMEType: definition.MIMEType}
		var read func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
		switch definition.Handler {
		case "mcp.handler.resource.subagent_skill":
			resource.Size = int64(len(skill))
			read = staticMCPResourceReader(definition.URI, definition.MIMEType, skill)
		case "mcp.handler.resource.operator_config_schema":
			resource.Size = int64(len(schema))
			read = staticMCPResourceReader(definition.URI, definition.MIMEType, schema)
		case "mcp.handler.resource.operator_current_config":
			uri, mimeType := definition.URI, definition.MIMEType
			read = func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				resourceHome, err := resolveMCPHomeDirectory(ctx, homeDirectory)
				if err != nil {
					return nil, err
				}
				data, err := operatorsettingsmcp.ReadCurrentConfig(ctx, settings.DefaultConfigPath, files, resourceHome)
				if err != nil {
					return nil, err
				}
				return mcpResourceResult(uri, mimeType, data), nil
			}
		case "mcp.handler.resource.providers_catalog":
			uri, mimeType := definition.URI, definition.MIMEType
			read = func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				resourceHome, err := resolveMCPHomeDirectory(ctx, homeDirectory)
				if err != nil {
					return nil, err
				}
				if err := configureMCPProvidersAtHome(ctx, settings, providerService, resourceHome); err != nil {
					return nil, err
				}
				catalog, err := providerService.ListProviders(ctx, providers.ListProvidersRequest{})
				if err != nil {
					return nil, err
				}
				data, err := json.Marshal(mcpProviderCatalog(catalog))
				if err != nil {
					return nil, err
				}
				return mcpResourceResult(uri, mimeType, data), nil
			}
		default:
			return nil, fmt.Errorf("unsupported MCP resource handler %q", definition.Handler)
		}
		resources = append(resources, mcpserver.ResourceRegistration{Resource: resource, Read: read})
	}
	return resources, nil
}

func staticMCPResourceReader(uri, mimeType string, data []byte) func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return mcpResourceResult(uri, mimeType, data), nil
	}
}

func mcpResourceResult(uri, mimeType string, data []byte) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mimeType, Text: string(data)}}}
}

func mcpProviderCatalog(catalog providers.ListProvidersResult) map[string]any {
	entries := make([]map[string]any, 0, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		models := make([]map[string]any, 0, len(provider.Models))
		for _, model := range provider.Models {
			models = append(models, map[string]any{"id": model.ID, "efforts": model.Efforts})
		}
		entries = append(entries, map[string]any{
			"id": provider.ID, "aliases": provider.Aliases, "displayName": provider.DisplayName,
			"availability": provider.Availability, "readiness": provider.Readiness,
			"models": models, "capabilities": provider.Capabilities,
		})
	}
	return map[string]any{"providers": entries}
}
