package wire

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorydefaultscaffold "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/defaultscaffold"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
)

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

func provideFactoryInvocationPolicyPorts() (factorydefinitionswire.InvocationPolicyPorts, error) {
	return factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
}

func provideDecisionEnvelopeService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.DecisionEnvelopeService {
	return ports.DecisionEnvelope
}

func provideInvocationInterpolationService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.InvocationInterpolationService {
	return ports.InvocationInterpolation
}

func provideInvocationOutputShapingService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.InvocationOutputShapingService {
	return ports.InvocationOutput
}

func provideInvocationWorkTypeService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.InvocationWorkTypeService {
	return ports.InvocationWorkType
}

func provideQuorumPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.QuorumPolicyService {
	return ports.QuorumPolicy
}

func provideWorkPropagationPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.WorkPropagationPolicyService {
	return ports.WorkPropagation
}

func provideWorkstationExecutionPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.WorkstationExecutionPolicyService {
	return ports.WorkstationExecution
}

func provideTTSObservabilityService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitions.TTSObservabilityService {
	return ports.TTSObservability
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
	return platformclock.Real{}
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
) factorydefinitions.NamedPathFileSystem {
	if edges.FactoryDefinitionNamedPathFileSystem != nil {
		return edges.FactoryDefinitionNamedPathFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionNamedPathResolver(
	fileSystem factorydefinitions.NamedPathFileSystem,
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
	return portableconfig.NewPortableBundledFilesApplier(fileSystem)
}

func provideFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.FactoryStarterWorkApplier, error) {
	return portableconfig.NewFactoryStarterWorkApplier(fileSystem)
}

func providePortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledDocsPruner, error) {
	return portableconfig.NewPortableBundledDocsPruner(fileSystem)
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

func provideFactoryDefinitionLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factorydefinitions.LoadingFileSystem,
	namedPaths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
	sourceResolver factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
) *factorydefinitionswire.Loader {
	return wirefactorydefinitions.Loader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		loadingFileSystem,
		namedPaths,
		fileSystem,
		sourceResolver,
		inspectSource,
		requiredToolChecker,
	)
}

func provideAuthoredFactorySourceLoader(
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return wirefactorydefinitions.AuthoredFactorySourceLoader(fileSystem)
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
		wirefactorydefinitions.FactorySnapshotJSONDecoder(),
	)
}

func provideReplayRuntimeConfigDecoder() factorydefinitions.ReplayRuntimeConfigDecoder {
	return wirefactorydefinitions.ReplayRuntimeConfigDecoder()
}
