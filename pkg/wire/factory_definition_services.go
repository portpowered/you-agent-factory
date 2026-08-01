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
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorydefaultscaffold "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/defaultscaffold"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
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
) factorydefinitionswire.DecisionEnvelopeService {
	return ports.DecisionEnvelope
}

func provideInvocationInterpolationService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.InvocationInterpolationService {
	return ports.InvocationInterpolation
}

func provideInvocationOutputShapingService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.InvocationOutputShapingService {
	return ports.InvocationOutput
}

func provideInvocationWorkTypeService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.InvocationWorkTypeService {
	return ports.InvocationWorkType
}

func provideQuorumPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.QuorumPolicyService {
	return ports.QuorumPolicy
}

func provideWorkPropagationPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.WorkPropagationPolicyService {
	return ports.WorkPropagation
}

func provideWorkstationExecutionPolicyService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.WorkstationExecutionPolicyService {
	return ports.WorkstationExecution
}

func provideTTSObservabilityService(
	ports factorydefinitionswire.InvocationPolicyPorts,
) factorydefinitionswire.TTSObservabilityService {
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
) factorydefinitionswire.LoadingFileSystem {
	if edges.FactoryDefinitionLoadingFileSystem != nil {
		return edges.FactoryDefinitionLoadingFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionClock(edges serviceedges.Edges) factorydefinitionswire.Clock {
	if edges.FactoryDefinitionClock != nil {
		return edges.FactoryDefinitionClock
	}
	return platformclock.Real{}
}

func provideFactoryDefinitionVersionFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.VersionFileSystem {
	if edges.FactoryDefinitionVersionFileSystem != nil {
		return edges.FactoryDefinitionVersionFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPackagedGoalPromptFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.PackagedGoalPromptFileSystem {
	if edges.FactoryDefinitionPackagedGoalPromptFileSystem != nil {
		return edges.FactoryDefinitionPackagedGoalPromptFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPortableBundledFileInspection(
	edges serviceedges.Edges,
) factorydefinitionswire.PortableBundledFileInspection {
	if edges.FactoryDefinitionPortableBundledFileInspection != nil {
		return edges.FactoryDefinitionPortableBundledFileInspection
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionRequiredToolPathLookup(
	edges serviceedges.Edges,
) factorydefinitionswire.RequiredToolPathLookup {
	if edges.FactoryDefinitionRequiredToolPathLookup != nil {
		return edges.FactoryDefinitionRequiredToolPathLookup
	}
	return exec.LookPath
}

func provideFactoryDefinitionRequiredToolVersionProbe(
	edges serviceedges.Edges,
) factorydefinitionswire.RequiredToolVersionProbe {
	if edges.FactoryDefinitionRequiredToolVersionProbe != nil {
		return edges.FactoryDefinitionRequiredToolVersionProbe
	}
	return func(path string, args ...string) ([]byte, error) {
		return exec.Command(path, args...).CombinedOutput()
	}
}

func provideFactoryDefinitionRequiredToolChecker(
	lookPath factorydefinitionswire.RequiredToolPathLookup,
	versionProbe factorydefinitionswire.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return factorydefinitionswire.NewPathRequiredToolChecker(lookPath, versionProbe)
}

func provideFactoryDefinitionPersistenceFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.PersistenceFileSystem {
	if edges.FactoryDefinitionPersistenceFileSystem != nil {
		return edges.FactoryDefinitionPersistenceFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionDirectoryReplacementStore(
	edges serviceedges.Edges,
) factorydefinitionswire.DirectoryReplacementStore {
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
) (factorydefinitionswire.NamedPathResolver, error) {
	return factorydefinitionswire.NewPathResolver(fileSystem)
}

func provideFactoryDefinitionNamedFactoryCatalogFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.NamedFactoryCatalogFileSystem {
	if edges.FactoryDefinitionNamedFactoryCatalogFileSystem != nil {
		return edges.FactoryDefinitionNamedFactoryCatalogFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionPackagedInstallationFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.PackagedInstallationFileSystem {
	if edges.FactoryDefinitionPackagedInstallationFileSystem != nil {
		return edges.FactoryDefinitionPackagedInstallationFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionAuthoredReaderFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.AuthoredLayoutReaderFileSystem {
	if edges.FactoryDefinitionAuthoredReaderFileSystem != nil {
		return edges.FactoryDefinitionAuthoredReaderFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionAuthoredWriterFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.AuthoredLayoutWriterFileSystem {
	if edges.FactoryDefinitionAuthoredWriterFileSystem != nil {
		return edges.FactoryDefinitionAuthoredWriterFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionScaffoldFileSystem(
	edges serviceedges.Edges,
) factorydefinitionswire.ScaffoldFileSystem {
	if edges.FactoryDefinitionScaffoldFileSystem != nil {
		return edges.FactoryDefinitionScaffoldFileSystem
	}
	return platformfilesystem.Local{}
}

func provideFactoryDefinitionScaffoldOutput(
	edges serviceedges.Edges,
) factorydefinitionswire.ScaffoldOutput {
	if edges.FactoryDefinitionScaffoldOutput != nil {
		return edges.FactoryDefinitionScaffoldOutput
	}
	return os.Stdout
}

func provideFactoryScaffoldCommandInitializer(
	files factorydefinitionswire.ScaffoldFileSystem,
	output factorydefinitionswire.ScaffoldOutput,
) (factorydefinitions.ScaffoldInitializer, error) {
	return factorydefaultscaffold.NewScaffoldInitializer(files, output)
}

func provideFactoryDefinitionInputInboxSentinelEnsurer(
	fileSystem factorydefinitionswire.AuthoredLayoutWriterFileSystem,
) factorydefinitionswire.InputInboxSentinelEnsurer {
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

func provideFactoryDefinitionLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factorydefinitionswire.LoadingFileSystem,
	namedPaths factorydefinitionswire.NamedPathResolver,
	fileSystem factorydefinitionswire.AuthoredLayoutReaderFileSystem,
	sourceResolver factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitionswire.PortableBundledFileInspection,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
) *factorydefinitionswire.Loader {
	return factorydefinitionswire.NewLoader(
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
	fileSystem factorydefinitionswire.AuthoredLayoutReaderFileSystem,
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
		factorydefinitionswire.NewFactorySnapshotJSONDecoder(),
	)
}

func provideReplayRuntimeConfigDecoder() factorydefinitionswire.ReplayRuntimeConfigDecoder {
	return factorydefinitionswire.NewReplayRuntimeConfigDecoder()
}
