package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/initializer"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	platformstdio "github.com/portpowered/infinite-you/pkg/platform/stdio"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

const (
	standardCLIHTTPTimeout = 10 * time.Second
	extendedCLIHTTPTimeout = 15 * time.Second
)

type standardCLIHTTPProtocol struct {
	clihttp.Protocol
	timeout time.Duration
}
type extendedCLIHTTPProtocol struct {
	clihttp.Protocol
	timeout time.Duration
}
type watchCLIHTTPProtocol struct {
	clihttp.Protocol
}

type streamingCLIHTTPProtocol struct {
	clihttp.Protocol
}

func provideStandardCLIHTTPProtocol() (standardCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{Timeout: standardCLIHTTPTimeout}, platformclock.Real{})
	if err != nil {
		return standardCLIHTTPProtocol{}, fmt.Errorf("build standard CLI HTTP protocol: %w", err)
	}
	return standardCLIHTTPProtocol{Protocol: protocol, timeout: standardCLIHTTPTimeout}, nil
}

func provideRemoteInvocationOperation(
	transport standardCLIHTTPProtocol,
) runcli.RemoteInvocationOperation {
	return runcli.NewRemoteInvocation(transport.Protocol)
}

func provideRunRuntimeRunnerBuilder(
	build initializer.RuntimeRunnerBuilder,
	open *factorysessionwire.ApplicationService,
) (runcli.RuntimeRunnerBuilder, error) {
	if build == nil || open == nil {
		return nil, errors.New("run application lifecycle builder and Factory Session opener are required")
	}
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		sinkID factorysessions.VisualizationSinkID,
	) (initializer.LocalRuntimeRunner, error) {
		var replay *factorysessions.HistoricalReplayInspection
		var hostedInvocation runcli.HostedInvocationOperation
		var cleanInvocation factoryruntime.Service
		runner, err := build(ctx, func(openCtx context.Context) (initializer.OpenedApplication, error) {
			opened, err := open.OpenApplication(openCtx, request, sinkID)
			if err != nil {
				return initializer.OpenedApplication{}, err
			}
			replay = opened.HistoricalReplay
			hostedInvocation = opened.HostedInvocation
			cleanInvocation = opened.CleanInvocation
			return initializer.OpenedApplication{
				Plan:        opened.Plan,
				Diagnostics: runtimeartifact.Diagnostics(opened.Diagnostics),
				Ready:       opened.Ready,
			}, nil
		})
		if err != nil {
			return nil, err
		}
		runner = runcli.WithHostedInvocation(runner, hostedInvocation)
		runner = runcli.WithCleanInvocationSnapshot(runner, cleanInvocation)
		return runcli.WithHistoricalReplay(runner, replay), nil
	}, nil
}

func provideExtendedCLIHTTPProtocol() (extendedCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{Timeout: extendedCLIHTTPTimeout}, platformclock.Real{})
	if err != nil {
		return extendedCLIHTTPProtocol{}, fmt.Errorf("build extended CLI HTTP protocol: %w", err)
	}
	return extendedCLIHTTPProtocol{Protocol: protocol, timeout: extendedCLIHTTPTimeout}, nil
}

func provideStreamingCLIHTTPProtocol() (streamingCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{}, platformclock.Real{})
	if err != nil {
		return streamingCLIHTTPProtocol{}, fmt.Errorf("build streaming CLI HTTP protocol: %w", err)
	}
	return streamingCLIHTTPProtocol{Protocol: protocol}, nil
}

func provideBatchInputFileSystem() submitcli.BatchInputFileSystem {
	return platformfilesystem.Local{}
}

func provideNamedFactoryRootsResolver() cli.NamedFactoryRootsResolver {
	return factorydefinitions.ResolveNamedFactoryRoots
}

func provideNamedFactoryCandidatePathsResolver(
	resolver factorydefinitions.NamedPathResolver,
) factorydefinitions.NamedFactoryCandidatePathsResolver {
	return resolver.ResolveCandidatePaths
}

func provideSubmitPayloadReader() work.PayloadFileReader {
	return work.NewPayloadFileReader(platformfilesystem.Local{})
}

func provideOperatorConfigDecoder() operatorsettings.ConfigDecoder {
	return globalconfigmapping.Decode
}

func provideOperatorConfigEncoder() operatorsettings.ConfigEncoder {
	return globalconfigmapping.Encode
}

func provideOperatorDefaultsResolver(settings operatorsettings.Service) operatorsettings.DefaultsResolver {
	return func(home string, environment operatorsettings.Defaults, flags operatorsettings.FlagOverrides) (operatorsettings.ResolvedDefaults, error) {
		return settings.ResolveFromHomeWithEnvironment(home, environment, flags)
	}
}

func provideSubmitWorkOperation(read work.PayloadFileReader, transport extendedCLIHTTPProtocol) cli.SubmitWorkOperation {
	return submitcli.NewSubmit(read, transport.Protocol)
}

func provideListWorkerSessionsOperation(transport standardCLIHTTPProtocol) cli.ListWorkerSessionsOperation {
	return workersessionscli.BindList(transport.Protocol)
}

func provideShowWorkerSessionOperation(transport standardCLIHTTPProtocol) cli.ShowWorkerSessionsOperation {
	return workersessionscli.BindShow(transport.Protocol)
}

func provideReadWorkerSessionOperation(transport standardCLIHTTPProtocol) cli.ReadWorkerSessionOperation {
	return workersessionscli.BindRead(transport.Protocol)
}

func provideStreamWorkerSessionOperation(transport streamingCLIHTTPProtocol) cli.StreamWorkerSessionOperation {
	return workersessionscli.BindStream(transport.Protocol)
}

func provideWorkerSessionsCLIIdentityGenerator() workersessionscli.IDGenerator {
	return uuid.NewString
}

func provideWorkerSessionsCLIExecutionFileReader() workersessionscli.ExecutionFileReader {
	return os.ReadFile
}

func provideContinueWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
	generateID workersessionscli.IDGenerator,
) cli.ContinueWorkerSessionOperation {
	return workersessionscli.BindContinue(transport.Protocol, local, workersessionscli.Effects{GenerateID: generateID})
}

func provideInterruptWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
	generateID workersessionscli.IDGenerator,
) cli.InterruptWorkerSessionOperation {
	return workersessionscli.BindInterrupt(transport.Protocol, local, workersessionscli.Effects{GenerateID: generateID})
}

func providePauseWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
) cli.PauseWorkerSessionOperation {
	return cli.PauseWorkerSessionOperation(bindWorkerSessionControlOperation(transport.Protocol, local, workersessions.ControlActionPause))
}

func provideResumeWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
) cli.ResumeWorkerSessionOperation {
	return cli.ResumeWorkerSessionOperation(bindWorkerSessionControlOperation(transport.Protocol, local, workersessions.ControlActionResume))
}

func provideCancelWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
) cli.CancelWorkerSessionOperation {
	return cli.CancelWorkerSessionOperation(bindWorkerSessionControlOperation(transport.Protocol, local, workersessions.ControlActionCancel))
}

func provideTerminateWorkerSessionOperation(
	transport standardCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
) cli.TerminateWorkerSessionOperation {
	return cli.TerminateWorkerSessionOperation(bindWorkerSessionControlOperation(transport.Protocol, local, workersessions.ControlActionTerminate))
}

func bindWorkerSessionControlOperation(
	transport clihttp.Protocol,
	local *localWorkerSessionsBoundary,
	action workersessions.ControlAction,
) workersessionscli.ControlOperation {
	operation := workersessionscli.BindControl(transport, local)
	return func(config workersessionscli.ControlConfig) error {
		config.Action = action
		return operation(config)
	}
}

// localWorkerSessionsBoundary owns the process-scoped direct Worker route used
// by the CLI. Direct invoke has already resolved the provider-facing execution
// fields, so its user-facing workstation name is not an authored route. The
// boundary rewrites only the pool route while preserving the execution payload
// that the provider-invocation executor consumes.
type localWorkerSessionsBoundary struct {
	service workersessions.Service
}

type localWorkerSessionsExecution struct {
	workers.Service
	publisher workers.ProgressPublisher
}

func (execution localWorkerSessionsExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	request.Input.ProgressPublisher = execution.publisher
	return execution.Service.Execute(ctx, request)
}

var _ workersessionscli.LocalInvokeBoundary = (*localWorkerSessionsBoundary)(nil)
var _ workersessionscli.LocalControlBoundary = (*localWorkerSessionsBoundary)(nil)

func (b *localWorkerSessionsBoundary) Start(
	ctx context.Context,
	req workersessions.StartRequest,
) (workersessions.StartResult, error) {
	if b == nil || b.service == nil {
		return workersessions.StartResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	req.Execution.WorkstationName = workers.ProviderInvocationRoute
	req.Execution.Execution.Dispatch.WorkstationName = workers.ProviderInvocationRoute
	return b.service.Start(ctx, req)
}

func (b *localWorkerSessionsBoundary) Continue(
	ctx context.Context,
	req workersessions.ContinueRequest,
) (workersessions.ContinueResult, error) {
	if b == nil || b.service == nil {
		return workersessions.ContinueResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Continue(ctx, req)
}

func (b *localWorkerSessionsBoundary) Interrupt(
	ctx context.Context,
	req workersessions.InterruptRequest,
) (workersessions.InterruptResult, error) {
	if b == nil || b.service == nil {
		return workersessions.InterruptResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Interrupt(ctx, req)
}

func (b *localWorkerSessionsBoundary) Pause(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	if b == nil || b.service == nil {
		return workersessions.ControlResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Pause(ctx, req)
}

func (b *localWorkerSessionsBoundary) Resume(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	if b == nil || b.service == nil {
		return workersessions.ControlResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Resume(ctx, req)
}

func (b *localWorkerSessionsBoundary) Cancel(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	if b == nil || b.service == nil {
		return workersessions.ControlResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Cancel(ctx, req)
}

func (b *localWorkerSessionsBoundary) Terminate(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	if b == nil || b.service == nil {
		return workersessions.ControlResult{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.Terminate(ctx, req)
}

func (b *localWorkerSessionsBoundary) StreamObservationsByWorkerSessionID(
	ctx context.Context,
	req workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, error) {
	if b == nil || b.service == nil {
		return workersessions.ObservationSubscription{}, fmt.Errorf("local Worker Sessions service is unavailable")
	}
	return b.service.StreamObservationsByWorkerSessionID(ctx, req)
}

func (b *localWorkerSessionsBoundary) Close(ctx context.Context) error {
	return nil
}

func provideLocalWorkerSessionsBoundary(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	workerService workers.Service,
	recording recordings.WorkerSessionRecordingService,
) (*localWorkerSessionsBoundary, error) {
	if workerService == nil {
		return nil, fmt.Errorf("construct local Worker Sessions boundary: Workers service is required")
	}
	// The local direct route has no Factory Runtime publisher to supply. Bind
	// the same Worker Sessions-owned observation bridge used by Factory Runtime
	// before the first dispatch so provider-session association and source-native
	// Worker drafts reach the local session topic as well.
	observationPublisher := workersessions.NewProviderSessionObservationPublisher(nil)
	execution := localWorkerSessionsExecution{Service: workerService, publisher: observationPublisher.Publish}
	service, err := workersessionswire.NewService(
		execution,
		eventsService,
		logger,
		platformclock.Real{},
		providerSessions,
		recording,
	)
	if err != nil {
		return nil, fmt.Errorf("construct local Worker Sessions service: %w", err)
	}
	observationPublisher.Bind(service)
	return &localWorkerSessionsBoundary{service: service}, nil
}

func provideInvokeWorkerSessionOperation(
	transport streamingCLIHTTPProtocol,
	local *localWorkerSessionsBoundary,
	generateID workersessionscli.IDGenerator,
	readFile workersessionscli.ExecutionFileReader,
) cli.InvokeWorkerSessionOperation {
	return workersessionscli.BindInvoke(transport.Protocol, local, workersessionscli.Effects{
		GenerateID: generateID,
		ReadFile:   readFile,
	})
}
func provideSubmitBatchOperation(
	transport extendedCLIHTTPProtocol,
	prepare work.FactoryRequestBatchPreparation,
) cli.SubmitBatchOperation {
	return submitcli.NewSubmitBatch(transport.Protocol, prepare)
}
func provideSessionsCLIService(
	transport standardCLIHTTPProtocol,
	prepare factorysessionwire.RequestPreparation,
) sessioncli.Service {
	return sessioncli.NewWithRequestIDGenerator(transport.Protocol, prepare, uuid.NewString)
}

func provideLocalSessionsCLIService(
	service factorysessions.Service,
) cli.LocalSessionsCLIService {
	return sessioncli.NewLocalLifecycleControls(service)
}
func provideModelsCLIService(
	transport standardCLIHTTPProtocol,
	invocation modelscli.InvocationOperation,
	composition modelscli.CompositionScopeProvider,
) modelscli.Service {
	return modelscli.New(transport.Protocol, invocation, composition)
}

// modelsCLIScopeSource is the narrow application-composition capability used
// to preserve the existing local Factory-derived Models scope behavior. It is
// intentionally private to Wire; Models transport receives the explicit
// CompositionScopeProvider below rather than discovering a Sessions
// collaborator from an invocation value.
type modelsCLIScopeSource interface {
	OpenModelsCatalogScope(context.Context) (modelservice.PresentationScope, error)
	OpenModelsPresentationScope(context.Context, modelservice.PresentationScopeRequest) (modelservice.PresentationScope, error)
}

type modelsCLIComposition struct {
	root   modelservice.Service
	source modelsCLIScopeSource
}

func provideModelsCLIComposition(
	root modelservice.Service,
	invocation factorysessionwire.InvocationOperation,
) (modelscli.CompositionScopeProvider, error) {
	if root == nil {
		return nil, errors.New("Models CLI composition requires the Models root")
	}
	source, ok := invocation.(modelsCLIScopeSource)
	if !ok || source == nil {
		return nil, errors.New("Models CLI composition requires a Models scope source")
	}
	return modelsCLIComposition{root: root, source: source}, nil
}

func (composition modelsCLIComposition) CompositionModelsRoot() modelservice.Service {
	return composition.root
}

func (composition modelsCLIComposition) CompositionOpenCatalogScope(
	ctx context.Context,
) (modelscli.InvokeRuntimeScope, error) {
	opened, err := composition.source.OpenModelsCatalogScope(ctx)
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
}

func (composition modelsCLIComposition) CompositionOpenInvokeScope(
	ctx context.Context,
	cfg modelscli.InvokeConfig,
) (modelscli.InvokeRuntimeScope, error) {
	opened, err := composition.source.OpenModelsPresentationScope(ctx, modelservice.PresentationScopeRequest{
		FactoryDir: cfg.FactoryDir,
		HomeDir:    cfg.HomeDir,
		OperatorDefaults: modelservice.PresentationOperatorDefaults{
			WorkerModelProvider: cfg.OperatorDefaults.WorkerModelProvider,
			WorkerModel:         cfg.OperatorDefaults.WorkerModel,
		},
		Logger:        cfg.Logger,
		Verbose:       cfg.Verbose,
		ModelCacheDir: "",
	})
	if err != nil {
		return modelscli.InvokeRuntimeScope{}, err
	}
	return modelscli.InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
}

func provideProvidersCLIService(service providers.Service) providerscli.Service {
	return providerscli.New(service)
}

func provideFlattenFactoryConfigOperation(
	persistence factorydefinitions.Persistence,
) cli.FlattenFactoryConfigOperation {
	return configcli.NewFlattenFactoryConfig(persistence)
}

func provideExpandFactoryConfigOperation(
	persistence factorydefinitions.Persistence,
) cli.ExpandFactoryConfigOperation {
	return configcli.NewExpandFactoryConfig(persistence)
}

func provideConfigureInitOperation(
	service operatorsettings.Service,

) (cli.ConfigureInitOperation, error) {
	packaged, err := providerswire.PackagedACPIntegrations()
	if err != nil {
		return nil, fmt.Errorf("load packaged ACP integrations for init: %w", err)
	}
	defaults := make([]operatorsettings.ACPIntegration, 0, len(packaged))
	for _, integration := range packaged {
		defaults = append(defaults, operatorsettings.ACPIntegration{
			ID: integration.ID, Name: integration.Name.String(),
			Transport: integration.Transport, Command: integration.Command,
		})
	}
	return initsetup.NewConfigurer(
		service,
		func(input io.Reader, maxLines int) (initsetup.ContextLineReader, error) {
			return platformstdio.NewContextLineReader(input, maxLines)
		},
		defaults,
	), nil
}

func provideACPCLIService(
	settings operatorsettings.Service,
	providersService providers.Service,
	generateID operatorsettings.IDGenerator,
) acpcli.Service {
	return acpcli.Service{Settings: settings, Providers: providersService, GenerateID: generateID}
}

func provideQueryFactoryOperation(transport standardCLIHTTPProtocol) cli.QueryFactoryOperation {
	return factorycli.NewQuery(transport.Protocol)
}

func provideEffectiveFactoryCatalogDiscovery(
	catalog factorydefinitions.NamedFactoryCatalog,
	files factorydefinitions.AuthoredLayoutReaderFileSystem,
	packaged []factorydefinitions.PackagedDefinition,
) (factorydefinitions.EffectiveFactoryCatalogDiscovery, error) {
	return factorydefinitionswire.NewEffectiveCatalogDiscovery(
		catalog.ListNamedFactories,
		files.ReadFile,
		packaged,
	)
}

func provideEffectiveFactoryDefinitionNormalizer() factorydefinitions.EffectiveFactoryDefinitionNormalizer {
	mapper := factorymapping.NewFactoryConfigMapper()
	return func(
		ctx context.Context,
		candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	) (*factorydefinitions.FactoryConfig, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		definition, err := mapper.Expand(candidate.Canonical)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return definition, err
	}
}

func provideEffectiveFactoryCatalogOperation(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	return factorydefinitionswire.NewEffectiveCatalog(discovery, normalize)
}

func provideEffectiveFactoryDefinitionsService(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
) (*factorydefinitionswire.EffectiveCatalogService, error) {
	return factorydefinitionswire.NewEffectiveCatalogService(catalog)
}

func provideCurrentFactoryPointerReader(
	namedPaths factorydefinitions.NamedPathResolver,
) factorydefinitions.CurrentFactoryPointerReader {
	return namedPaths.ReadCurrentPointer
}

func provideListFactoriesOperation(
	definitions *factorydefinitionswire.EffectiveCatalogService,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
) cli.ListFactoriesOperation {
	return factorycli.NewList(definitions.ListEffectiveFactories, readCurrent)
}

func provideFactoryNameCompletionOperation(
	definitions *factorydefinitionswire.EffectiveCatalogService,
) cobracompletion.FactoryNamesOperation {
	return cobracompletion.NewFactoryNames(definitions.ListEffectiveFactories)
}

func provideSelectedFactorySignatureCompletionOperation(
	definitions *factorydefinitionswire.EffectiveCatalogService,
) (cobracompletion.SelectedFactorySignatureOperation, error) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return nil, err
	}
	return cobracompletion.NewSelectedFactorySignature(
		definitions.ListEffectiveFactories,
		manifest,
	), nil
}

func provideValidateFactoryOperation(
	validator factorydefinitions.SubmittedDefinitionValidationOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) cli.ValidateFactoryOperation {
	return func(config factorycli.ValidateConfig) error {
		return factorycli.ValidateWithServices(config, validator, loadSource)
	}
}

func provideCreateFactoryFromFileOperation(
	persist factorydefinitions.NamedFactoryPersistenceOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) cli.CreateFactoryFromFileOperation {
	return func(config factorycli.CreateFromFileConfig) error {
		return factorycli.CreateFromFileWithServices(config, persist, loadSource)
	}
}

func provideReplaceFactoryCurrentOperation(transport standardCLIHTTPProtocol) cli.ReplaceFactoryCurrentOperation {
	return factorycli.NewReplaceCurrent(transport.Protocol)
}

func provideUpdateFactoryFromFileOperation(
	persist factorydefinitions.NamedFactoryPersistenceOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) cli.UpdateFactoryFromFileOperation {
	return func(config factorycli.UpdateFromFileConfig) error {
		return factorycli.UpdateFromFileWithServices(config, persist, loadSource)
	}
}

func provideFactoryConfigRootResolver(
	source factorydefinitions.AuthoredLayoutReaderFileSystem,
) factorydefinitions.FactoryConfigRootResolver {
	return factorydefinitions.NewFactoryConfigRootResolver(source)
}

func provideFactoryConfigFileLoader(
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) factorydefinitions.FactoryConfigFileLoader {
	mapper := factorymapping.NewFactoryConfigMapper()
	return factorydefinitions.NewFactoryConfigFileLoader(loadSource, mapper.Expand)
}

func provideWorkRequestFileLoader() work.RequestFileLoader {
	return work.NewRequestFileLoader(platformfilesystem.Local{})
}

func provideDeleteFactoryOperation(
	catalog factorydefinitions.NamedFactoryCatalog,
) cli.DeleteFactoryOperation {
	return factorycli.NewDelete(catalog)
}

func provideListWorkOperation(
	transport standardCLIHTTPProtocol,
	prepare work.ListRequestPreparation,
) cli.ListWorkOperation {
	return workcli.NewList(transport.Protocol, prepare)
}

func provideListHumanApprovalsOperation(
	transport standardCLIHTTPProtocol,
) cli.ListHumanApprovalsOperation {
	return workcli.NewListHumanApprovals(transport.Protocol)
}

func provideShowHumanApprovalOperation(
	transport standardCLIHTTPProtocol,
) cli.ShowHumanApprovalOperation {
	return workcli.NewShowHumanApproval(transport.Protocol)
}

func provideWatchCLIHTTPProtocol() (watchCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{}, platformclock.Real{})
	if err != nil {
		return watchCLIHTTPProtocol{}, fmt.Errorf("build watch CLI HTTP protocol: %w", err)
	}
	return watchCLIHTTPProtocol{Protocol: protocol}, nil
}

func provideWatchWorkOperation(
	transport watchCLIHTTPProtocol,
) cli.WatchWorkOperation {
	return workcli.NewWatch(transport.Protocol)
}

func provideShowWorkOperation(transport standardCLIHTTPProtocol) cli.ShowWorkOperation {
	return workcli.NewShow(transport.Protocol)
}
func provideMoveWorkOperation(transport extendedCLIHTTPProtocol) cli.MoveWorkOperation {
	return workcli.NewMove(transport.Protocol)
}
func provideWorkVisualizationOperation() work.VisualizationOperation {
	return work.NewVisualizationOperation(platformfilesystem.Local{})
}

func provideVisualizeWorkOperation(
	visualize work.VisualizationOperation,
) cli.VisualizeWorkOperation {
	return workcli.NewVisualize(visualize)
}

func provideCLIExecutionServiceBuilder(
	build factorysessionwire.ExecutionServiceBuilder,
) cli.ExecutionServiceBuilder {
	return func(ctx context.Context, provider, projectRoot, fixtureCatalogPath, childExecutorMode string) (cli.OwnedExecutionService, error) {
		return build(ctx, provider, projectRoot, fixtureCatalogPath, childExecutorMode)
	}
}
