package wire

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformstdio "github.com/portpowered/infinite-you/pkg/platform/stdio"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
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

func provideStandardCLIHTTPProtocol() (standardCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{Timeout: standardCLIHTTPTimeout}, platformclock.Real{})
	if err != nil {
		return standardCLIHTTPProtocol{}, fmt.Errorf("build standard CLI HTTP protocol: %w", err)
	}
	return standardCLIHTTPProtocol{Protocol: protocol, timeout: standardCLIHTTPTimeout}, nil
}

func provideExtendedCLIHTTPProtocol() (extendedCLIHTTPProtocol, error) {
	protocol, err := clihttp.NewProtocol(&http.Client{Timeout: extendedCLIHTTPTimeout}, platformclock.Real{})
	if err != nil {
		return extendedCLIHTTPProtocol{}, fmt.Errorf("build extended CLI HTTP protocol: %w", err)
	}
	return extendedCLIHTTPProtocol{Protocol: protocol, timeout: extendedCLIHTTPTimeout}, nil
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

func provideOperatorDefaultsResolver(files operatorsettings.FileSystem, decode operatorsettings.ConfigDecoder) operatorsettings.DefaultsResolver {
	return func(home string, environment operatorsettings.Defaults, flags operatorsettings.FlagOverrides) (operatorsettings.ResolvedDefaults, error) {
		return operatorsettings.ResolveFromHomeWithEnvironment(files, decode, home, environment, flags)
	}
}

func provideSubmitWorkOperation(read work.PayloadFileReader, transport extendedCLIHTTPProtocol) cli.SubmitWorkOperation {
	return submitcli.NewSubmit(read, transport.Protocol)
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
func provideModelsCLIService(
	transport standardCLIHTTPProtocol,
	invocation modelscli.InvocationOperation,
) modelscli.Service {
	return modelscli.New(transport.Protocol, invocation)
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
	service operatorsettings.ConfigDocumentService,
) cli.ConfigureInitOperation {
	return initsetup.NewConfigurer(
		service,
		func(input io.Reader, maxLines int) (initsetup.ContextLineReader, error) {
			return platformstdio.NewContextLineReader(input, maxLines)
		},
	)
}

func provideACPCLIService(
	settings operatorsettings.ConfigDocumentService,
	providersFactory providers.Factory,
	generateID operatorsettings.IDGenerator,
) acpcli.Service {
	return acpcli.Service{Settings: settings, ProvidersFactory: providersFactory, GenerateID: generateID}
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
