package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	platformstdio "github.com/portpowered/infinite-you/pkg/platform/stdio"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
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
		request factorysessionwire.ApplicationOpeningRequest,
	) (initializer.LocalRuntimeRunner, error) {
		var replay *factorysessions.HistoricalReplayInspection
		var hostedInvocation runcli.HostedInvocationOperation
		runner, err := build(ctx, func(openCtx context.Context) (initializer.OpenedApplication, error) {
			opened, err := open.OpenApplication(openCtx, request)
			if err != nil {
				return initializer.OpenedApplication{}, err
			}
			replay = opened.HistoricalReplay
			hostedInvocation = opened.HostedInvocation
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
	return sessioncli.New(transport.Protocol, prepare)
}

func provideLocalSessionsCLIService(
	service factorysessions.Service,
) cli.LocalSessionsCLIService {
	return sessioncli.NewLocalLifecycleControls(service)
}
func provideModelsCLIService(
	transport standardCLIHTTPProtocol,
	invocation modelscli.InvocationOperation,
) modelscli.Service {
	return modelscli.New(transport.Protocol, invocation)
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
