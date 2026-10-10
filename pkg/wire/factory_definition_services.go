package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorydefaultscaffold "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/defaultscaffold"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// newDefaultWorkTypeResolver binds Work admission's omitted-type policy to the
// native configuration in the selected Factory Session. The session projection
// avoids serializing an editable Factory to resolve its default Work Type.
func newDefaultWorkTypeResolver(
	sessions factorysessions.LiveControlService,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
) func(context.Context, string) (string, error) {
	return func(ctx context.Context, sessionID string) (string, error) {
		if sessions == nil || invocationWorkType == nil {
			return "", nil
		}
		projection, err := sessions.GetFactorySession(ctx, sessionID)
		if err != nil {
			if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorydefinitions.ErrCurrentFactoryNotFound) {
				return "", nil
			}
			return "", err
		}
		defaultID, err := invocationWorkType.DefaultWorkType(projection.Context.FactoryCfg)
		if err != nil {
			return "", nil
		}
		return defaultID, nil
	}
}

func provideWorkContentHostPlatform(edges serviceedges.Edges) work.ContentHostPlatform {
	hostPlatform := edges.WorkContentHostPlatform
	if hostPlatform == "" {
		hostPlatform = work.ContentHostPlatform(runtime.GOOS)
	}
	return hostPlatform
}

func provideContentMaterializer(
	hostPlatform work.ContentHostPlatform,
	edges serviceedges.Edges,
) (work.ContentMaterializer, error) {
	inspectPath := edges.WorkContentInspectPath
	if inspectPath == nil {
		inspectPath = os.Stat
	}
	createTempFile := edges.WorkContentCreateTempFile
	if createTempFile == nil {
		createTempFile = func(dir, pattern string) (work.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		}
	}
	removePath := edges.WorkContentRemovePath
	if removePath == nil {
		removePath = os.Remove
	}
	writeFile := edges.WorkContentWriteFile
	if writeFile == nil {
		writeFile = os.WriteFile
	}
	openFile := edges.WorkContentOpenFile
	if openFile == nil {
		openFile = func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		}
	}
	httpDoer := edges.WorkContentHTTPDoer
	if httpDoer == nil {
		httpDoer = &http.Client{
			Timeout:       workwire.DefaultContentMaterializationHTTPTimeout,
			CheckRedirect: workwire.ContentMaterializationRedirectPolicy(0, false),
		}
	}
	return workwire.NewContentMaterializationService(
		hostPlatform, httpDoer,
		inspectPath, createTempFile, removePath, writeFile, openFile,
	)
}

func provideFactoryDefinitionPortableFileSystem(
	edges serviceedges.Edges,
) portablefiles.FileSystem {
	if edges.FactoryDefinitionPortableFileSystem != nil {
		return edges.FactoryDefinitionPortableFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionLoadingFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.LoadingFileSystem {
	if edges.FactoryDefinitionLoadingFileSystem != nil {
		return edges.FactoryDefinitionLoadingFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionClock(edges serviceedges.Edges) factorydefinitions.Clock {
	if edges.FactoryDefinitionClock != nil {
		return edges.FactoryDefinitionClock
	}
	return edges.Clock
}

func provideFactoryDefinitionVersionFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.VersionFileSystem {
	if edges.FactoryDefinitionVersionFileSystem != nil {
		return edges.FactoryDefinitionVersionFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPackagedGoalPromptFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.PackagedGoalPromptFileSystem {
	if edges.FactoryDefinitionPackagedGoalPromptFileSystem != nil {
		return edges.FactoryDefinitionPackagedGoalPromptFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPortableBundledFileInspection(
	edges serviceedges.Edges,
) factorydefinitions.PortableBundledFileInspection {
	if edges.FactoryDefinitionPortableBundledFileInspection != nil {
		return edges.FactoryDefinitionPortableBundledFileInspection
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionRequiredToolPathLookup(
	edges serviceedges.Edges,
) factorydefinitions.RequiredToolPathLookup {
	if edges.FactoryDefinitionRequiredToolPathLookup != nil {
		return edges.FactoryDefinitionRequiredToolPathLookup
	}
	return exec.LookPath
}

func provideFactoryDefinitionRequiredToolVersionProbe(
	edges serviceedges.Edges,
) factorydefinitions.RequiredToolVersionProbe {
	if edges.FactoryDefinitionRequiredToolVersionProbe != nil {
		return edges.FactoryDefinitionRequiredToolVersionProbe
	}
	return func(path string, args ...string) ([]byte, error) {
		return exec.Command(path, args...).CombinedOutput()
	}
}

func provideFactoryDefinitionRequiredToolChecker(
	lookPath factorydefinitions.RequiredToolPathLookup,
	versionProbe factorydefinitions.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return factorydefinitionswire.NewPathRequiredToolChecker(lookPath, versionProbe)
}

func provideFactoryDefinitionPersistenceFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.PersistenceFileSystem {
	if edges.FactoryDefinitionPersistenceFileSystem != nil {
		return edges.FactoryDefinitionPersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionDirectoryReplacementStore(
	edges serviceedges.Edges,
) factorydefinitions.DirectoryReplacementStore {
	if edges.FactoryDefinitionDirectoryReplacementStore != nil {
		return edges.FactoryDefinitionDirectoryReplacementStore
	}
	return directoryreplace.NewLocal(runtime.GOOS)
}

func provideFactoryDefinitionNamedPathFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.NamedPathFileSystem {
	if edges.FactoryDefinitionNamedPathFileSystem != nil {
		return edges.FactoryDefinitionNamedPathFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionNamedPathResolver(
	fileSystem factorydefinitionswire.NamedPathFileSystem,
) (factorydefinitions.NamedPathResolver, error) {
	return factorydefinitionswire.NewPathResolver(fileSystem)
}

func provideFactoryDefinitionNamedFactoryCatalogFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.NamedFactoryCatalogFileSystem {
	if edges.FactoryDefinitionNamedFactoryCatalogFileSystem != nil {
		return edges.FactoryDefinitionNamedFactoryCatalogFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPackagedInstallationFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.PackagedInstallationFileSystem {
	if edges.FactoryDefinitionPackagedInstallationFileSystem != nil {
		return edges.FactoryDefinitionPackagedInstallationFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPackagedInstallationDirectoryCreator(
	edges serviceedges.Edges,
) factorydefinitions.PackagedInstallationDirectoryCreator {
	if edges.FactoryDefinitionPackagedInstallationDirectoryCreator != nil {
		return edges.FactoryDefinitionPackagedInstallationDirectoryCreator
	}
	return os.Mkdir
}

func provideFactoryDefinitionAuthoredReaderFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.AuthoredLayoutReaderFileSystem {
	if edges.FactoryDefinitionAuthoredReaderFileSystem != nil {
		return edges.FactoryDefinitionAuthoredReaderFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionAuthoredWriterFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.AuthoredLayoutWriterFileSystem {
	if edges.FactoryDefinitionAuthoredWriterFileSystem != nil {
		return edges.FactoryDefinitionAuthoredWriterFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionScaffoldFileSystem(
	edges serviceedges.Edges,
) factorydefinitions.ScaffoldFileSystem {
	if edges.FactoryDefinitionScaffoldFileSystem != nil {
		return edges.FactoryDefinitionScaffoldFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionScaffoldOutput(
	edges serviceedges.Edges,
) factorydefinitions.ScaffoldOutput {
	if edges.FactoryDefinitionScaffoldOutput != nil {
		return edges.FactoryDefinitionScaffoldOutput
	}
	return os.Stdout
}

func provideFactoryScaffoldCommandInitializer(
	files factorydefinitions.ScaffoldFileSystem,
	output factorydefinitions.ScaffoldOutput,
) (factorydefinitions.ScaffoldInitializer, error) {
	return factorydefaultscaffold.NewScaffoldInitializer(files, output)
}

func provideFactoryDefinitionInputInboxSentinelEnsurer(
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
) factorydefinitions.InputInboxSentinelEnsurer {
	return inboxgitkeep.NewLocal(fileSystem)
}

func providePortableBundledFilesApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledFilesApplier, error) {
	return factorydefinitionswire.NewPortableBundledFilesApplier(fileSystem)
}

func provideFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.FactoryStarterWorkApplier, error) {
	return factorydefinitionswire.NewFactoryStarterWorkApplier(fileSystem)
}

func providePortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledDocsPruner, error) {
	return factorydefinitionswire.NewPortableBundledDocsPruner(fileSystem)
}

func providePortableBundledFilesMaterializer(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesMaterializer {
	return factorydefinitionswire.NewPortableBundledFilesMaterializer(fileSystem)
}

func providePortableBundledFileWritesValidator(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFileWritesValidator {
	return factorydefinitionswire.NewPortableBundledFileWritesValidator(fileSystem)
}

func providePortableBundledFilesCopier(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesCopier {
	return factorydefinitionswire.NewPortableBundledFilesCopier(fileSystem)
}

func providePortableBundledFileSourceResolver(fileSystem portablefiles.FileSystem) (factorydefinitions.PortableBundledFileSourceResolver, error) {
	return factorydefinitionswire.NewPortableBundledFileSourceResolver(fileSystem)
}

func provideFactoryDefinitionAuthoredReader(files factorydefinitions.AuthoredLayoutReaderFileSystem) *factorydefinitionswire.AuthoredLayoutReader {
	return factorydefinitionswire.NewAuthoredLayoutReader(files)
}

func provideFactoryDefinitionConfigDecoder(source factorydefinitions.SerializedFactoryConfigReader) factorydefinitions.FactoryConfigJSONDecoder {
	return factorydefinitionswire.FactoryConfigDecoder(source)
}

func provideFactoryDefinitionCanonicalNormalizer() factorydefinitionswire.CanonicalFactoryNormalizer {
	return factorydefinitionswire.CanonicalFactoryNormalizerFromMapper()
}

func provideFactoryDefinitionLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factorydefinitions.LoadingFileSystem,
	namedPaths factorydefinitions.NamedPathResolver,
	reader *factorydefinitionswire.AuthoredLayoutReader,
	loadAuthoredSource factorydefinitions.AuthoredFactorySourceLoader,
	newSource factorydefinitions.LoadedFactorySourceFactory,
	sourceResolver factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	decode factorydefinitions.FactoryConfigJSONDecoder,
	normalize factorydefinitionswire.CanonicalFactoryNormalizer,
) *factorydefinitionswire.Loader {
	return factorydefinitionswire.NewLoader(applySupportedFiles, applyStarterWork,
		materializeFiles, loadingFileSystem, namedPaths, reader, loadAuthoredSource,
		newSource, sourceResolver, inspectSource, requiredToolChecker, decode, normalize)
}

func provideAuthoredFactorySourceLoader(
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return factorydefinitionswire.AuthoredFactorySourceLoader(fileSystem)
}

func provideLoadedFactoryLoader(
	loader *factorydefinitionswire.Loader,
) factorydefinitions.LoadedFactoryLoader {
	return func(factoryDir string, workstationLoader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return loader.LoadSourceFromFactoryDir(factoryDir, workstationLoader)
	}
}

func provideReplayArtifactStorage() platformreplay.Storage {
	return platformreplay.NewLocal(runtime.GOOS)
}

func provideReplayArtifactLoader(storage platformreplay.Storage) recordings.ReplayArtifactLoader {
	return recordingswire.NewReplayArtifactLoader(
		storage,
		factorydefinitionswire.FactorySnapshotJSONDecoder(),
	)
}

func provideReplayRuntimeConfigDecoder() factorydefinitions.ReplayRuntimeConfigDecoder {
	return factorydefinitionswire.ReplayRuntimeConfigDecoder()
}

func provideFactoryDefinitionsRuntimeRouter() *factorysessions.DefinitionRuntimeRouter {
	return factorysessionwire.NewDefinitionRuntimeRouter()
}

func provideFactoryDefinitionCompilation(
	loader *factorydefinitionswire.Loader,
) factorydefinitionswire.Compilation {
	return factorydefinitionswire.NewCompilationService(
		loader.LoadSourceFromCanonicalJSON,
		loader.LoadSourceFromFactoryDir,
		factorydefinitionswire.FactoryConfigJSONEncoder(),
	)
}

func provideFactoryDefinitionsHost(
	router *factorysessions.DefinitionRuntimeRouter,
	persistence factorydefinitions.Persistence,
	loader *factorydefinitionswire.Loader,
	paths factorydefinitions.NamedPathResolver,
	prepare factorydefinitions.PortableFactoryConfigPreparer,
	capture factorydefinitions.FactorySnapshotCapturer,
) (factorydefinitionswire.LifecycleHost, error) {
	if router == nil {
		return nil, fmt.Errorf("construct Factory Definitions: runtime router is required")
	}
	return factorydefinitionswire.NewLifecycleHost(router.Host(), persistence, loader, paths, prepare, capture)
}

func provideFactoryDefinitionsRoot(
	router *factorysessions.DefinitionRuntimeRouter,
	host factorydefinitionswire.LifecycleHost,
	catalog factorydefinitionswire.Catalog,
	validation factorydefinitionswire.Validation,
	authoring factorydefinitionswire.AuthoringLayout,
	distribution factorydefinitionswire.Distribution,
	snapshot factorydefinitionswire.RuntimeSnapshot,
	compilation factorydefinitionswire.Compilation,
	files factorydefinitions.VersionFileSystem,
	list factorydefinitions.EffectiveFactoryCatalogOperation,
	portability factorydefinitionswire.SnapshotsPortability,
) (factorydefinitions.Service, error) {
	return factorydefinitionswire.NewService(host, router.ActivationGateway(), catalog, validation,
		authoring, distribution, snapshot, compilation, files, list, portability)
}

func provideFactoryDefinitionsDistribution(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
) (factorydefinitionswire.Distribution, error) {
	// Root scaffolding remains explicitly unavailable; CLI scaffold owns its separate command boundary.
	return factorydefinitionswire.NewDistribution(catalog, installer, nil, nil)
}

func provideFactoryDefinitionsSnapshotsPortability(
	loader *factorydefinitionswire.Loader,
	capture factorydefinitions.LoadedFactorySnapshotCapturer,
	prepare factorydefinitions.PortableFactoryConfigPreparer,
	decode factorydefinitions.FactorySnapshotJSONDecoder,
	materialize factorydefinitions.PortableBundledFilesMaterializer,
	validate factorydefinitions.PortableBundledFileWritesValidator,
) (factorydefinitionswire.SnapshotsPortability, error) {
	return factorydefinitionswire.NewSnapshotsPortability(loader.LoadSourceFromCanonicalJSON,
		capture, prepare, decode, materialize, validate)
}

// definitionsAuthoringFileSystem preserves the portable filesystem selected for root authoring.
type definitionsAuthoringFileSystem interface {
	portablefiles.FileSystem
	factorydefinitions.AuthoredLayoutWriterFileSystem
	factorydefinitions.PersistenceFileSystem
}

func provideFactoryDefinitionsAuthoringFileSystem(files portablefiles.FileSystem) (definitionsAuthoringFileSystem, error) {
	authored, ok := files.(definitionsAuthoringFileSystem)
	if !ok {
		return nil, fmt.Errorf("construct Factory Definitions: portable filesystem must support authoring_layout persistence")
	}
	return authored, nil
}

type definitionsAuthoringInbox interface {
	factorydefinitions.InputInboxSentinelEnsurer
}

func provideFactoryDefinitionsAuthoringInbox(files definitionsAuthoringFileSystem) definitionsAuthoringInbox {
	return inboxgitkeep.NewLocal(files)
}

func provideFactoryDefinitionsAuthoredWriter(
	files definitionsAuthoringFileSystem,
	inbox definitionsAuthoringInbox,
) *factorydefinitionswire.AuthoredLayoutWriter {
	return factorydefinitionswire.NewAuthoredLayoutWriter(files, inbox, factorydefinitionswire.AuthoredAgentsFileWriter(files))
}

func provideFactoryDefinitionsAuthoringLayout(
	validator factorydefinitions.Validator,
	loader *factorydefinitionswire.Loader,
	writer *factorydefinitionswire.AuthoredLayoutWriter,
	materialize factorydefinitions.PortableBundledFilesMaterializer,
	prune factorydefinitions.PortableBundledDocsPruner,
	validate factorydefinitions.PortableBundledFileWritesValidator,
	copyFiles factorydefinitions.PortableBundledFilesCopier,
	files definitionsAuthoringFileSystem,
	paths factorydefinitions.NamedPathResolver,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitionswire.AuthoringLayout, error) {
	mapper := factorymapping.NewFactoryConfigMapper()
	return factorydefinitionswire.NewAuthoringLayout(validator, validationentry.MapFactoryJSONForPersistence,
		mapper.Expand, authoredmapping.AuthoredFactoryConfigForExpandedLayout, mapper.Flatten,
		factorydefinitionswire.PreparedAuthoredLayoutWriter(writer, materialize, prune),
		factorydefinitionswire.AuthoredLayoutValidator(loader, validate), loader.FlattenFactoryConfig,
		factorydefinitionswire.AuthoredLayoutExpander(loader, writer, validate, materialize, copyFiles),
		files, paths.RequireDefinitionDir, directories)
}

// provideFactoryRuntimeRoot composes the singular process-scoped Runtime root.
// Factory Sessions supplies the activation operation with each request.
func provideFactoryRuntimeRoot(
	orchestration factoryruntimewire.Orchestration,
	instanceHost factoryruntimewire.InstanceHost,
	dispatchPlan factoryruntimewire.DispatchPlanning,
) (factorysessionwire.FactoryRuntimeRoot, error) {
	return factoryruntimewire.NewService(orchestration, instanceHost, dispatchPlan)
}

// provideRuntimeOrchestration completes the process owner with both workflow ports.
func provideRuntimeOrchestration(mapper factoryruntimewire.DefinitionMapper, workflows factoryruntime.JavaScriptWorkflows) factoryruntimewire.Orchestration {
	return factoryruntimewire.NewOrchestration(mapper, workflows, workflows)
}

// provideRuntimeDispatchPlanning supplies explicit dormant edges until activation
// owns per-runtime planning (T15). No missing effect selects a fallback.
func provideRuntimeDispatchPlanning() factoryruntimewire.DispatchPlanning {
	return factoryruntimewire.NewDispatchPlanning(
		func(context.Context, workers.WorkstationDispatchRequest) error { return factoryruntime.ErrNotRunning },
		func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, factoryruntime.ErrNotRunning
		},
	)
}

func provideFactoryDefinitionValidationOwner(
	operations factorydefinitions.ValidationOperations,
	compilation factorydefinitionswire.Compilation,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) factorydefinitionswire.Validation {
	return factorydefinitionswire.NewValidationService(operations, operations, compilation.LoadCanonicalFactorySource, requiredToolChecker, orchestratorValidator)
}

// provideFactoryDefinitionRuntimeSnapshot binds the session query without executing it.
func provideFactoryDefinitionRuntimeSnapshot(
	loader *factorydefinitionswire.Loader,
	router *factorysessions.DefinitionRuntimeRouter,
) factorydefinitionswire.RuntimeSnapshot {
	return factorydefinitionswire.NewRuntimeSnapshot(
		loader.LoadSourceFromCanonicalJSON,
		loader.LoadSourceFromFactoryDir,
		router.Host().WorkstationLoader,
		factorydefinitions.FileReader(loader.ReadFile),
	)
}

// definitionsPersistenceWriter retains persistence's separately selected filesystem.
type definitionsPersistenceWriter factorydefinitionswire.AuthoredLayoutWriter

func provideFactoryDefinitionPersistenceWriter(
	files factorydefinitions.AuthoredLayoutWriterFileSystem,
	inbox factorydefinitions.InputInboxSentinelEnsurer,
) *definitionsPersistenceWriter {
	return (*definitionsPersistenceWriter)(factorydefinitionswire.NewAuthoredLayoutWriter(files, inbox,
		factorydefinitionswire.AuthoredAgentsFileWriter(files)))
}

func provideFactoryDefinitionPersistenceEncoder(source factorydefinitions.CanonicalFactoryConfigReader) factorydefinitions.FactoryConfigJSONEncoder {
	return factorydefinitionswire.CanonicalFactoryConfigEncoder(source)
}
