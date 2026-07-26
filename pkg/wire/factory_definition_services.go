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
	factorydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/decisionenvelope"
	factoryinvocationinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationinterpolation"
	factoryinvocationoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationoutput"
	factoryinvocationworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationworktype"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryquorumpolicy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy"
	factoryttsobservability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/ttsobservability"
	factorydefaultscaffold "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/defaultscaffold"
	factoryworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workpropagation"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingreplay "github.com/portpowered/infinite-you/pkg/services/recordings/replay"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workmaterialize "github.com/portpowered/infinite-you/pkg/services/work/materialize"
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
			Timeout:       workmaterialize.DefaultHTTPTimeout,
			CheckRedirect: workmaterialize.RedirectPolicy(0, false),
		}
	}
	return workmaterialize.New(
		hostPlatform, 0, 0, 0, false, httpDoer, "",
		inspectPath, createTempFile, removePath, writeFile, openFile,
	)
}

func provideDecisionEnvelopeService() factorydefinitions.DecisionEnvelopeService {
	return factorydecisionenvelope.NewService()
}

func provideInvocationInterpolationService() factorydefinitions.InvocationInterpolationService {
	return factoryinvocationinterpolation.NewService()
}

func provideInvocationOutputShapingService() factorydefinitions.InvocationOutputShapingService {
	return factoryinvocationoutput.NewService()
}

func provideInvocationWorkTypeService() factorydefinitions.InvocationWorkTypeService {
	return factoryinvocationworktype.NewService()
}

func provideQuorumPolicyService() factorydefinitions.QuorumPolicyService {
	return factoryquorumpolicy.NewService()
}

func provideWorkPropagationPolicyService() factorydefinitions.WorkPropagationPolicyService {
	return factoryworkpropagation.NewService()
}

func provideTTSObservabilityService() factorydefinitions.TTSObservabilityService {
	return factoryttsobservability.NewService()
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
	return factoryloading.NewPathRequiredToolChecker(lookPath, versionProbe)
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
	return factorynamedpaths.New(fileSystem)
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

func providePortableBundledFilesMaterializer(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesMaterializer {
	return portableconfig.NewMaterializer(fileSystem)
}

func providePortableBundledFileWritesValidator(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFileWritesValidator {
	return portableconfig.NewWritesValidator(fileSystem)
}

func providePortableBundledFilesCopier(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFilesCopier {
	return portableconfig.NewFilesCopier(fileSystem)
}

func providePortableBundledFileSourceResolver(fileSystem portablefiles.FileSystem) (factorydefinitions.PortableBundledFileSourceResolver, error) {
	return portableconfig.NewSupportedSourceResolver(fileSystem)
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
) *factoryloading.Loader {
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
	loader *factoryloading.Loader,
) factorydefinitions.LoadedFactoryLoader {
	return func(factoryDir string, workstationLoader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return loader.LoadSourceFromFactoryDir(factoryDir, workstationLoader)
	}
}

func provideReplayArtifactStorage() platformreplay.Storage {
	return platformreplay.NewLocal(runtime.GOOS)
}

func provideReplayArtifactLoader(storage platformreplay.Storage) recordings.ReplayArtifactLoader {
	return func(path string) (*factorydefinitions.ReplayArtifact, error) {
		return recordingreplay.Load(storage, path, wirefactorydefinitions.FactorySnapshotJSONDecoder())
	}
}

func provideReplayRuntimeConfigDecoder() factorydefinitions.ReplayRuntimeConfigDecoder {
	return wirefactorydefinitions.ReplayRuntimeConfigDecoder()
}
