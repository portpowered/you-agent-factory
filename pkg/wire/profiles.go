package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/initializer"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformcontentstaging "github.com/portpowered/infinite-you/pkg/platform/contentstaging"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationwire "github.com/portpowered/infinite-you/pkg/services/system_initialization/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionmcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
	workersessionwire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	httpclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"go.uber.org/zap"
)

func provideCurrentFactoryDirectoryResolver(
	namedPaths factorydefinitions.NamedPathResolver,
) factorydefinitions.CurrentFactoryDirectoryResolver {
	return func(rootDir string) (string, error) {
		return factorydefinitionswire.ResolveCurrent(namedPaths, rootDir)
	}
}

func provideTerminalLoggerBuilder() terminalpolicy.LoggerBuilder {
	return func(mode terminalpolicy.Mode, debug bool) (*zap.Logger, error) {
		return logging.BuildTerminalLogger(string(mode), debug)
	}
}

func provideLiveRecordingTargetPlanner(
	reserver runtimeartifact.Reserver,
	source platformclock.Source,
) recordings.LiveRecordingTargetPlanner {
	return recordingswire.NewLiveRecordingTargetPlanner(
		source,
		reserver,
		filepath.Join,
	)
}

func provideCLIRunDefaults(
	recordingTargets recordings.LiveRecordingTargetPlanner,
	recordingsCLI recordingscli.Adapter,
	source platformclock.Source,
) runcli.RunConfig {
	return runcli.RunConfig{
		RuntimeLogConfig:            logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig:        platformmetrics.DefaultRuntimeMetricsConfig(),
		RecordingTargetPlanner:      recordingTargets,
		CanonicalSessionIDGenerator: uuid.NewString,
		RecordingsCLI:               recordingsCLI,
		Clock:                       source,
	}
}

func provideRunDirectoryCreator() platformfilesystem.DirectoryCreator {
	return platformfilesystem.Local{}
}

func provideBrowserOpener(edges serviceedges.Edges) platformbrowser.Opener {
	return provideBrowserOpenerWith(
		edges,
		os.LookupEnv,
		testing.Testing(),
		hostBrowserOpener,
	)
}

var errBrowserLaunchInTestBinary = errors.New(
	"refusing to launch a real browser from a test binary: inject Edges.BrowserOpener",
)

// hostBrowserOpener selects the real host browser launcher. A test binary can
// never reach it: tests that build the process without injecting a
// BrowserOpener edge get a refusing opener instead of a tab on the desktop.
func hostBrowserOpener() platformbrowser.Opener {
	return hostBrowserOpenerFor(testing.Testing(), runtime.GOOS)
}

func hostBrowserOpenerFor(inTestBinary bool, goos string) platformbrowser.Opener {
	if inTestBinary {
		return func(context.Context, string) error { return errBrowserLaunchInTestBinary }
	}
	return platformbrowser.NewHost(goos).Open
}

const browserOpenOptOutEnvironment = "YOU_NO_BROWSER_OPEN"

func provideBrowserOpenerWith(
	edges serviceedges.Edges,
	lookupEnv func(string) (string, bool),
	inTestBinary bool,
	hostFactory func() platformbrowser.Opener,
) platformbrowser.Opener {
	if edges.BrowserOpener != nil {
		return edges.BrowserOpener
	}
	if inTestBinary {
		return func(context.Context, string) error { return errBrowserLaunchInTestBinary }
	}
	if value, ok := lookupEnv(browserOpenOptOutEnvironment); ok && value == "1" {
		return func(context.Context, string) error { return nil }
	}
	return hostFactory()
}

func provideFactorySessionsWorkingDirectory(
	edges serviceedges.Edges,
) platformfilesystem.WorkingDirectory {
	if edges.FactorySessionsWorkingDirectory != nil {
		return edges.FactorySessionsWorkingDirectory
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionDirectoryInspection(
	edges serviceedges.Edges,
) factorysessionwire.DirectoryInspection {
	if edges.FactorySessionDirectoryInspection != nil {
		return edges.FactorySessionDirectoryInspection
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionContractFixtureReader(edges serviceedges.Edges) factorysessionwire.ContractFixtureReader {
	if edges.FactorySessionContractFixtureReader != nil {
		return edges.FactorySessionContractFixtureReader
	}
	return factorysessionwire.ContractFixtureReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInvocationInputReader(edges serviceedges.Edges) factorysessionwire.InvocationInputReader {
	if edges.FactorySessionInvocationInputReader != nil {
		return edges.FactorySessionInvocationInputReader
	}
	return factorysessionwire.InvocationInputReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionReplayRecordingReader(edges serviceedges.Edges) factorysessionwire.ReplayRecordingReader {
	if edges.FactorySessionReplayRecordingReader != nil {
		return edges.FactorySessionReplayRecordingReader
	}
	return factorysessionwire.ReplayRecordingReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionInitialWorkReader(edges serviceedges.Edges) factorysessionwire.InitialWorkReader {
	if edges.FactorySessionInitialWorkReader != nil {
		return edges.FactorySessionInitialWorkReader
	}
	return factorysessionwire.InitialWorkReader(platformfilesystem.Local{}.ReadFile)
}

func provideFactorySessionResolveHomeDirectory(
	edges serviceedges.Edges,
) factorysessions.HomeDirectoryResolver {
	if edges.FactorySessionResolveHomeDirectory != nil {
		return edges.FactorySessionResolveHomeDirectory
	}
	return os.UserHomeDir
}

func provideFactorySessionResolveLogicalTargetSymlinks(
	edges serviceedges.Edges,
) factorysessions.LogicalTargetResolveSymlinks {
	if edges.FactorySessionResolveLogicalTargetSymlinks != nil {
		return edges.FactorySessionResolveLogicalTargetSymlinks
	}
	return filepath.EvalSymlinks
}

func provideFactorySessionCursorPersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessionwire.CursorPersistenceFileSystem {
	if edges.FactorySessionCursorPersistenceFileSystem != nil {
		return edges.FactorySessionCursorPersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactorySessionCursorCreateTemporaryFile(
	edges serviceedges.Edges,
) factorysessionwire.CursorPersistenceCreateTemporaryFile {
	if edges.FactorySessionCursorCreateTemporaryFile != nil {
		return edges.FactorySessionCursorCreateTemporaryFile
	}
	return func(dir, pattern string) (factorysessionwire.CursorPersistenceTemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
}

func provideFactorySessionCursorStoreFactory(
	files factorysessionwire.CursorPersistenceFileSystem,
	createTemporaryFile factorysessionwire.CursorPersistenceCreateTemporaryFile,
) factorysessionwire.CursorStoreFactory {
	return func(dir string) (factorysessions.CursorStore, error) {
		return factorysessionwire.NewCursorFileStore(dir, files, createTemporaryFile)
	}
}

func provideFactorySessionRuntimePersistenceFileSystem(
	edges serviceedges.Edges,
) factorysessionwire.RuntimePersistenceFileSystem {
	if edges.FactorySessionRuntimePersistenceFileSystem != nil {
		return edges.FactorySessionRuntimePersistenceFileSystem
	}
	return runtimePersistenceFileSystem{
		directories: platformfilesystem.Local{},
		recovery:    platformfilesystem.NewRecovery(platformfilesystem.Local{}, platformfilesystem.Local{}),
		storage:     platformreplay.NewLocal(runtime.GOOS),
	}
}

// runtimePersistenceFileSystem preserves the established Sessions directory
// lifecycle effect while routing snapshot bytes through replay.Storage's
// temp-write, sync, close, and replace implementation.
type runtimePersistenceFileSystem struct {
	directories platformfilesystem.Local
	recovery    platformfilesystem.Recovery
	storage     platformreplay.Storage
}

func (files runtimePersistenceFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return files.directories.MkdirAll(path, mode)
}

func (files runtimePersistenceFileSystem) ReadFile(path string) ([]byte, error) {
	return files.storage.ReadFile(path)
}

func (files runtimePersistenceFileSystem) ReadFileBounded(path string, limit int64) ([]byte, error) {
	return files.recovery.ReadFileBounded(path, limit)
}

func (files runtimePersistenceFileSystem) RenameNoReplace(source, destination string) error {
	return files.recovery.RenameNoReplace(source, destination)
}

func (files runtimePersistenceFileSystem) WriteFile(path string, data []byte, _ fs.FileMode) error {
	return files.storage.WriteFile(path, data)
}

func provideFactorySessionRuntimePersistenceStoreFactory(
	files factorysessionwire.RuntimePersistenceFileSystem,
) factorysessionwire.RuntimePersistenceStoreFactory {
	return func(projectRoot string) (factorysessionwire.RuntimePersistenceStore, error) {
		return factorysessionwire.NewRuntimeProjectStore(projectRoot, files)
	}
}

func provideOperatorSettingsFileSystem(edges serviceedges.Edges) operatorsettings.FileSystem {
	if edges.OperatorSettingsFileSystem != nil {
		return edges.OperatorSettingsFileSystem
	}
	return platformfilesystem.Local{}
}

func provideOperatorSettingsCreateTemporaryFile(edges serviceedges.Edges) operatorsettings.CreateTemporaryFile {
	if edges.OperatorSettingsCreateTemporaryFile != nil {
		return edges.OperatorSettingsCreateTemporaryFile
	}
	return func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	}
}

func provideOperatorSettingsProviderCatalog(
	providersService providers.Service,
) operatorsettings.ProviderCatalog {
	return func(value string) (string, bool) {
		if providersService == nil {
			return "", false
		}
		resolved, err := providersService.ResolveIdentity(
			context.Background(),
			providers.ResolveIdentityRequest{Identity: value},
		)
		if err != nil {
			return "", false
		}
		return resolved.ID.String(), true
	}
}

// provideOperatorSettingsLogger converts the canonical process-wide zap
// logger into the logging.Logger abstraction accepted by the Operator
// Settings service, so ResolveACPAgentProfile/UpdateACPAgentProfile emit
// operation logs through the same logger the rest of the process uses.
func provideOperatorSettingsLogger(logger *zap.Logger) logging.Logger {
	return logging.NewZapLogger(logger, false)
}

func provideOperatorSettingsDocumentPreserver() operatorsettings.ConfigDocumentPreserver {
	return globalconfigmapping.PreserveUnknownFields
}

func provideOperatorSettingsIDGenerator(edges serviceedges.Edges) operatorsettings.IDGenerator {
	if edges.OperatorSettingsIDGenerator != nil {
		return edges.OperatorSettingsIDGenerator
	}
	return uuid.NewString
}

func provideSystemInitializationInspectPath(
	edges serviceedges.Edges,
) systeminitializationwire.InspectPath {
	if edges.SystemInitializationInspectPath != nil {
		return edges.SystemInitializationInspectPath
	}
	return os.Stat
}

func provideOperatorConfigLoader(settings operatorsettings.Service) operatorsettings.ConfigLoader {
	return func(path string) (operatorsettings.Config, error) {
		return settings.LoadFileConfig(path)
	}
}

func provideOperatorBackendScopeEnsurer(settings operatorsettings.Service) operatorsettings.BackendScopeEnsurer {
	return func(path string) (operatorsettings.ResolvedBackendScope, error) {
		return settings.EnsureLocalBackendScope(path)
	}
}

func provideModelInvocationArtifactExporter(
	edges serviceedges.Edges,
) (modelswire.InvocationArtifactExporter, error) {
	filesystem := edges.ModelInvocationArtifactFileSystem
	if filesystem == nil {
		filesystem = platformfilesystem.Local{}
	}
	return modelswire.NewInvocationArtifactExporter(filesystem)
}

func provideModelInvocationTimeout() factorysessions.ModelInvocationTimeout {
	return factorysessions.DefaultModelInvocationTimeout
}

func provideModelInvocationOperation(
	operation factorysessionwire.InvocationOperation,
) factorysessionwire.ModelInvocationOperation {
	return operation
}

func provideSystemInitializationService(
	installer factorydefinitions.PackagedFactoryInstaller,
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	settings operatorsettings.Service,
	inspectPath systeminitializationwire.InspectPath,
) (systeminitialization.Service, error) {
	return systeminitializationwire.NewService(settings, catalog, installer, inspectPath)
}

func provideSystemInitializationOperation(
	service systeminitialization.Service,
) initializerapplication.SystemInitializationOperation {
	return func(ctx context.Context, homeDir string) error {
		_, err := service.Initialize(ctx, systeminitialization.Request{HomeDir: homeDir})
		return err
	}
}

func providePackagedFactoryDefinitions() ([]factorydefinitions.PackagedDefinition, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.All(), nil
}

func provideSerializedFactoryConfigReader() (factorydefinitions.SerializedFactoryConfigReader, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.ReadSerializedFactoryConfig, nil
}

func provideCanonicalFactoryConfigReader() (factorydefinitions.CanonicalFactoryConfigReader, error) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.ReadCanonicalFactoryConfig, nil
}

func providePackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionswire.NewPackagedFactoryCatalog(definitions)
}

func providePackagedFactoryInstallation(installer *factorydefinitionswire.PackagedFactoryInstallationService) factorydefinitions.PackagedFactoryInstallationOperations {
	return factorydefinitions.PackagedFactoryInstallationOperations{Install: installer.InstallPackagedFactory}
}

// provideDurableOpening binds the execution opener and provider identity
// policy once; live and replay requests cannot substitute these collaborators.
func provideDurableOpening(
	loadOperatorConfig operatorsettings.ConfigLoader,
	execution *factorysessionwire.RuntimeBackedExecution,
	allocator providerswire.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	providerOverride providerOverrideService,
	providerIdentities factorysessions.ProviderIdentityResolver,
) *factorysessionwire.DurableOpening {
	return factorysessionwire.NewDurableOpening(loadOperatorConfig, execution.Acquire, providerIdentities,
		providerOverride == nil && adaptRunner != nil && allocator != nil)
}

func provideFactoryRuntimeClockResolver(processClock factoryruntime.Clock) factoryruntime.ClockResolver {
	return func(clock factoryruntime.Clock) factoryruntime.Clock {
		if clock != nil {
			return clock
		}
		return processClock
	}
}

func provideFactoryRuntimeMetricsClock(edges serviceedges.Edges) platformclock.TimerSource {
	return edges.ProcessScheduler
}

func providePprofCommandLineReader() platformhttpserver.CommandLineReader {
	return func() []string {
		return append([]string(nil), os.Args...)
	}
}

func provideFactoryRuntimeSessionLoggerFactory() factoryruntime.SessionLoggerFactory {
	return factoryruntime.NewSessionLogger
}

func provideAPIServerStarter(
	edges serviceedges.Edges,
	commandLineReader platformhttpserver.CommandLineReader,
) (platformhttpserver.Starter, error) {
	starter := edges.APIServerStarter
	if starter == nil {
		var err error
		starter, err = platformhttpserver.NewStarter(net.Listen, platformprocessmemory.CurrentCommit, commandLineReader)
		if err != nil {
			return nil, err
		}
	}
	return platformhttpserver.NewObservedStarter(starter), nil
}

func provideRuntimeHostOperation(
	starter platformhttpserver.Starter,
	scheduler platformclock.TimerSource,
) factorysessionwire.RuntimeHostOperation {
	return factorysessionwire.NewRuntimeHostService(starter, scheduler)
}

func provideProcessRuntimeFactory(
	host factorysessionwire.RuntimeHostOperation,
) (factorysessionwire.ProcessRuntimeFactory, error) {
	return factorysessionwire.NewProcessLifecycleFactory(host)
}

func provideFactoryVisualizationOpening(
	reader factoryvisualization.RuntimeReader,
	openSource func(string) factoryvisualization.Source,
	openScope factoryvisualizationwire.ScopeOpening,
	sinks factoryvisualization.RuntimeSinkOwner,
	edges serviceedges.Edges,
) factoryvisualization.RuntimeOpening {
	return factoryvisualizationwire.NewRuntimeOpening(reader, openSource, openScope, sinks,
		edges.FactoryVisualizationSink, edges.FactoryVisualizationRootObserver)
}

func provideWorkContentStagingService(
	edges serviceedges.Edges,
	source platformclock.Source,
) (work.ContentStagingService, error) {
	filesystem := edges.WorkContentStagingFileSystem
	if filesystem == nil {
		filesystem = platformcontentstaging.FileSystem{}
	}
	random := edges.WorkContentStagingRandom
	if random == nil {
		random = platformcontentstaging.Random{}
	}
	clock := edges.WorkContentStagingClock
	if clock == nil {
		clock = source
	}
	return workwire.NewContentStagingService(filesystem, random, clock, 0)
}

func provideManagedRunnerFactory() runtimeapplication.ManagedRunnerFactory {
	return runtimeapplication.NewManagedRunner
}

// MCP selection contains invocation data only; collaborators remain injected.
type mcpIntentKey struct{}

func selectedMCPIntent(ctx context.Context) (processcontract.MCPIntent, error) {
	intent, ok := ctx.Value(mcpIntentKey{}).(processcontract.MCPIntent)
	if !ok {
		return processcontract.MCPIntent{}, errors.New("MCP invocation selection is required")
	}
	return intent, nil
}

func provideMCPWorkerHostClient() (*httpclient.Client, error) {
	return httpclient.NewClient("http://mcp.invalid", httpclient.WithHTTPClient(&http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}), httpclient.WithRequestEditorFn(func(ctx context.Context, request *http.Request) error {
		intent, err := selectedMCPIntent(ctx)
		if err != nil {
			return err
		}
		endpoint, err := url.Parse(strings.TrimRight(intent.ServerURL, "/") + request.URL.RequestURI())
		if err != nil {
			return err
		}
		request.URL = endpoint
		if intent.WorkerSessionID != "" || intent.WorkerSessionToken != "" {
			request.Header.Set("X-You-Worker-Session-Id", intent.WorkerSessionID)
			request.Header.Set("Authorization", "Bearer "+intent.WorkerSessionToken)
		}
		return nil
	}))
}

func provideMCPWorkerAdapter(client *httpclient.Client) (*workersessionmcp.Adapter, error) {
	return workersessionwire.NewMCPAdapter(client)
}

func provideMCPInspection(recordings recordings.Service) factorysessionmcp.RecordingsInspection {
	return factorysessionmcp.RecordingsInspection(recordings)
}

func provideMCPToolOperation(inspection factorysessionmcp.RecordingsInspection, prepare factorysessionwire.RequestPreparation, preview factoryruntime.WorkflowPreviewOperation, sessions factorysessions.Service, generateID factorysessions.SessionIDGenerator, resolve factorysessionmcp.ProviderIdentityResolver) factorysessionmcp.ToolOperation {
	return func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		intent, err := selectedMCPIntent(ctx)
		if err != nil {
			return nil, err
		}
		return factorysessionmcp.CallTool(ctx, prepare, preview, name, input, inspection, sessions, intent.ProjectRoot, generateID, resolve)
	}
}

func provideMCPDispatch(worker *workersessionmcp.Adapter, tools factorysessionmcp.ToolOperation, generateID factorysessions.SessionIDGenerator) mcpserver.ToolOperation {
	return func(ctx context.Context, name string, raw json.RawMessage) (json.RawMessage, error) {
		if name != factorysessionmcp.ToolSubagent {
			return tools(ctx, name, raw)
		}
		intent, err := selectedMCPIntent(ctx)
		if err != nil {
			return nil, err
		}
		var caller *workersessions.CallerIdentity
		if intent.WorkerSessionID != "" || intent.WorkerSessionToken != "" {
			caller = &workersessions.CallerIdentity{WorkerSessionID: intent.WorkerSessionID, Token: intent.WorkerSessionToken}
		}
		return worker.Subagent(ctx, raw, caller, func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return worker.Start(ctx, input, intent.ProjectRoot, generateID)
		})
	}
}

func provideMCPServerOptions(settings operatorsettings.Service, providers providers.Service, files operatorsettings.FileSystem, home factorysessions.HomeDirectoryResolver) (mcpserver.Options, error) {
	skills, resources, err := mcpSubagentContent(settings, providers, files, home)
	return mcpserver.Options{Skills: skills, Resources: resources}, err
}

func provideMCPServer(operation mcpserver.ToolOperation, options mcpserver.Options) (*mcpserver.Server, error) {
	return mcpserver.New(operation, options)
}

// provideMCPProviderIdentityResolver projects one explicit you.subagent provider
// selection through the authoritative Providers catalog. Production composition
// binds it before the packaged subagent Factory Session starts, so an unknown
// identifier is a validation failure instead of a runtime provider failure. The
// caller context is preserved so a rejected selection cannot outlive its request.
func provideMCPProviderIdentityResolver(providerService providers.Service) factorysessionmcp.ProviderIdentityResolver {
	return func(ctx context.Context, identity string) (string, error) {
		resolved, err := providerService.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: identity})
		if err != nil {
			return "", err
		}
		return resolved.ID.String(), nil
	}
}

func stdioLifecyclePlan(transport lifecycle.Component) lifecycle.Plan {
	return lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "stdio transport", Component: transport, Primary: true,
	}}}
}

func provideStdioHandler(
	build initializer.LifecycleRunnerBuilder,
	newRunner lifecycle.RunnerFactory,
	open mcpstdio.Opener,
	server *mcpserver.Server,
	workingDirectory platformfilesystem.WorkingDirectory,
) (processcontract.StdioHandler, error) {
	return func(ctx context.Context, intent processcontract.MCPIntent) error {
		if ctx == nil {
			return errors.New("MCP stdio context is required")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if intent.Stdin == nil || intent.Stdout == nil {
			return errors.New("MCP stdio input and output are required")
		}
		endpoint, err := url.Parse(intent.ServerURL)
		if err != nil || endpoint == nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return errors.New("construct subagent MCP host operation: subagent host and HTTP transport are required")
		}
		intent.ProjectRoot = strings.TrimSpace(intent.ProjectRoot)
		if intent.ProjectRoot == "" {
			intent.ProjectRoot, err = workingDirectory.Getwd()
			if err != nil {
				return fmt.Errorf("resolve MCP working directory: %w", err)
			}
		}
		ctx = context.WithValue(ctx, mcpIntentKey{}, intent)
		transport, err := open(server, intent.Stdin, intent.Stdout)
		if err != nil {
			return err
		}
		runner, err := build(ctx, stdioLifecyclePlan(newRunner(transport.Run)), runtimeartifact.Diagnostics{}, nil)
		if err != nil {
			return err
		}
		if runner == nil {
			return errors.New("MCP stdio application is required")
		}
		return runner.Run(ctx)
	}, nil
}

func provideLifecycleRunnerFactory() lifecycle.RunnerFactory {
	return lifecycle.NewRunner
}

func provideRunInvocationOperation(
	invocation factorysessionwire.InvocationOperation,
) runcli.InvocationOperation {
	return invocation
}

func provideRunSelectionFactory(
	buildRunner runcli.RuntimeRunnerBuilder,
	invocation factorysessionwire.InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript runcli.DirectJavaScriptRunOperation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigDiagnosticsLoader,
	buildRuntimeRequest runcli.SessionStartRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (runcli.SelectionFactory, error) {
	return runcli.NewSelectionFactory(
		buildRunner, invocation, presentation, directJavaScript,
		prepareWorkTarget, loadMockWorkers, buildRuntimeRequest, presentations, visualizations,
	)
}

func provideFactorySessionHTTPRequestPreparation(
	prepare factorysessionwire.RequestPreparation,
) factorysessionshttp.RequestPreparation {
	return prepare
}

func provideWorkStopSummaryProjector() factorysessions.WorkStopSummaryProjector {
	return factorysessionwire.NewWorkStopSummaryProjector()
}

func provideResponsePresentation(scheduler platformclock.TimerSource) factoryvisualization.ResponsePresentation {
	return factoryvisualizationwire.NewResponsePresentation(scheduler)
}

// provideProcessLogger selects the process backend once, independently of CLI output policy.
func provideProcessLogger(edges serviceedges.Edges) (*zap.Logger, error) {
	if edges.ProcessLogger != nil {
		return edges.ProcessLogger, nil
	}
	return logging.NewDefaultLogger()
}
