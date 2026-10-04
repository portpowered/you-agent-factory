package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorsettingsmcp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	mcpcontent "github.com/portpowered/infinite-you/pkg/transports/mcp/content"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/contextscope"
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
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
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

type acpFactoryTargetSelection struct {
	factoryDir string
	homeDir    string
	artifacts  factoryruntime.RuntimeArtifactRoots
	defaults   operatorsettings.ResolvedDefaults
}

func resolveACPFactoryTargetSelection(
	ctx context.Context,
	factoryTargetID, workingRoot string,
	resolveHomeDir acpServerResolveHomeDir,
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog,
	resolveOperatorDefaults operatorsettings.DefaultsResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
) (acpFactoryTargetSelection, error) {
	if err := ctx.Err(); err != nil {
		return acpFactoryTargetSelection{}, err
	}
	profile, hasProfile := acp.InvocationProfileFromContext(ctx)
	homeDir := strings.TrimSpace(profile.HomeDir)
	if homeDir == "" {
		var err error
		homeDir, err = resolveHomeDir()
		if err != nil {
			return acpFactoryTargetSelection{}, err
		}
	}
	roots, err := factorydefinitions.ResolveNamedFactoryRoots(homeDir, workingRoot)
	if err != nil {
		return acpFactoryTargetSelection{}, err
	}
	bareName := strings.TrimPrefix(factoryTargetID, operatorsettings.ACPFactoryTargetNamespace)
	resolved, err := namedFactoryCatalog.ResolveNamedFactoryAcrossRoots(roots.Project, roots.Global, bareName)
	if err != nil {
		return acpFactoryTargetSelection{}, err
	}
	if resolved == nil {
		return acpFactoryTargetSelection{}, factorydefinitions.ErrNamedFactoryNotFound
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
		return acpFactoryTargetSelection{}, err
	}
	return acpFactoryTargetSelection{
		factoryDir: resolved.FactoryDir,
		homeDir:    homeDir,
		artifacts:  artifactRoots(homeDir),
		defaults:   defaults,
	}, nil
}

// provideACPServerFactorySessionStartResolver constructs the canonical
// Factory Session Start resolver the ACP transport consumes.
func provideACPServerFactorySessionStartResolver(
	resolveHomeDir acpServerResolveHomeDir,
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog,
	resolveOperatorDefaults operatorsettings.DefaultsResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
) acp.FactorySessionStartResolver {
	return func(ctx context.Context, factoryTargetID, workingRoot, requestID string) (factorysessions.SessionStartRequest, error) {
		sel, err := resolveACPFactoryTargetSelection(ctx, factoryTargetID, workingRoot, resolveHomeDir, namedFactoryCatalog, resolveOperatorDefaults, artifactRoots)
		if err != nil {
			return factorysessions.SessionStartRequest{}, err
		}
		return mapACPFactorySessionStart(requestID, factoryTargetID, workingRoot, sel.factoryDir, sel.homeDir, sel.artifacts, sel.defaults), nil
	}
}

// mapACPFactorySessionStart turns resolved ACP target values into a canonical
// Factory Sessions Start request without opening a runtime or storing state.
func mapACPFactorySessionStart(requestID, factoryTargetID, workingRoot, factoryDir, homeDir string, artifacts factoryruntime.RuntimeArtifactRoots, defaults operatorsettings.ResolvedDefaults) factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{
		Mode:           factorysessions.SessionOperationModeLive,
		ActivationOnly: true,
		Correlation:    factorysessions.SessionOperationCorrelation{RequestID: requestID},
		Definition:     factorysessions.SessionDefinitionSelection{FactoryID: factoryTargetID},
		Source: factorysessions.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factoryTargetID,
		},
		Args:       map[string]any{"workingRoot": workingRoot},
		FolderPath: factoryDir,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			SystemConfigHome: homeDir,
			LogDirectory:     artifacts.Logs,
			MetricsDirectory: artifacts.Metrics,
			OperatorDefaults: defaults,
			Mode:             factorysessions.SessionRuntimeModeService,
		},
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
// Operator-default selection remains separate from the wire-log configuration
// captured at the process construction boundary.
func acpOperatorDefaultsEnvironment() operatorsettings.Defaults {
	return operatorsettings.Defaults{
		WorkerModelProvider: strings.TrimSpace(os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider)),
		WorkerModel:         strings.TrimSpace(os.Getenv(operatorsettings.EnvDefaultWorkerModel)),
	}
}

// provideACPServer constructs the production ACP stdio Server from the same
// canonical chatsessions.Service, Events service, and Factory Sessions-owned
// target-execution capability instances the rest of this graph composes.
func provideACPServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factorySessions factorysessions.Service,
	eventsService events.Service,
	resolveHomeDir acpServerResolveHomeDir,
	responseBridge acp.ResponseBridge,
	wireRecorder acp.WireRecorder,
	startResolver acp.FactorySessionStartResolver,
) acp.Server {
	return acpwire.NewServer(
		logger, chatSessions, catalog, factorySessions, eventsService,
		resolveHomeDir, responseBridge, wireRecorder, startResolver,
		func(ctx context.Context) acp.InvocationScope { return contextscope.New(ctx) },
	)
}

// ACPWireLogSettings is configuration captured at the caller boundary before
// graph construction. It contains no effects or service collaborators.
type ACPWireLogSettings struct {
	Disabled  bool
	Directory string
}

// provideACPWireRecorder constructs the per-connection ACP wire transcript
// opener. Recording is enabled by default so diagnostic evidence is available
// when a customer encounters a problem.
//
// The caller boundary captures configuration once before graph construction.
// The recorder writes only to its own file
// handle, so it structurally cannot reach the protocol stream that `you server
// acp` reserves on stdout.
func provideACPWireRecorder(
	edges serviceedges.Edges,
	settings ACPWireLogSettings,
	paths platformruntimeartifact.Reserver,
	clock runtimeArtifactClock,
	resolveHomeDir acpServerResolveHomeDir,
) (acp.WireRecorder, error) {
	if edges.ACPWireRecorder != nil {
		return edges.ACPWireRecorder, nil
	}
	if settings.Disabled {
		return func(string) (acp.WireTranscript, error) { return nil, acp.ErrWireRecordingDisabled }, nil
	}
	opener, err := wiretranscript.NewOpener(paths, wireTranscriptClock(clock))
	if err != nil {
		return nil, err
	}
	root := settings.Directory
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
	factorySessions factorysessions.Service,
	eventsService events.Service,
	logger logging.Logger,
) *chatsessionswire.ResponseBridge {
	return chatsessionswire.NewResponseBridge(chatSessions, factorySessions, eventsService, logger)
}

// wireTranscriptClock adapts the injected runtime-artifact clock to the
// transcript package's Clock, so the transcript never reaches for time.Now
// itself.
type wireTranscriptClock runtimeArtifactClock

func (c wireTranscriptClock) Now() time.Time { return c() }

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
