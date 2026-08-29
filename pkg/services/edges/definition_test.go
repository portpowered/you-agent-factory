package edges

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	_ recordings.RecordingMakeDirectories     = Edges{}.RecordingMakeDirectories
	_ recordings.RecordingCreateTemporaryFile = Edges{}.RecordingCreateTempFile
	_ recordings.RecordingRemovePath          = Edges{}.RecordingRemovePath
	_ recordings.RecordingRenamePath          = Edges{}.RecordingRenamePath
	_ recordings.RecordingReadFile            = Edges{}.RecordingReadFile
	_ recordings.RecordingOpenFile            = Edges{}.RecordingOpenFile
)

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestMergeUsesExplicitReplacementsAndPreservesDefaults(t *testing.T) {
	t.Parallel()

	defaultStarter := platformhttpserver.Starter(func(context.Context, platformhttpserver.StartRequest) error {
		return nil
	})
	replacementStarter := platformhttpserver.Starter(func(context.Context, platformhttpserver.StartRequest) error {
		return nil
	})
	defaultBrowserOpener := platformbrowser.Opener(func(context.Context, string) error { return nil })
	replacementBrowserOpened := false
	replacementBrowserOpener := platformbrowser.Opener(func(context.Context, string) error {
		replacementBrowserOpened = true
		return nil
	})
	defaultProvider := &stubProvider{id: "default"}
	replacementProvider := &stubProvider{id: "replacement"}
	walked := false
	resolved := false
	workerDocsFileSystem := platformfilesystem.Local{}
	workerSymlinksResolved := false
	systemInitializationPathInspected := false
	cursorTemporaryFileRequested := false
	cursorTemporaryFileError := errors.New("cursor temporary file selected")
	requiredToolPathLookedUp := false
	requiredToolVersionProbed := false
	directoryReplacementStore := &edgeDirectoryReplacementStore{}
	workRequestIDGenerated := false
	workSubmittedFileRead := false
	workSubmittedFilePathInspected := false
	responseEventIDGenerated := false
	sessionIDGenerated := false
	homeDirectoryResolved := false
	worktreeGit := &edgeWorktreeGit{}
	scaffoldOutput := &bytes.Buffer{}
	contractFixtureRead := false
	invocationInputRead := false
	replayRecordingRead := false
	initialWorkRead := false
	retryRandom := platformrandom.SourceFunc(func(int64) (int64, error) { return 7, nil })
	providerTemporaryFileRequested := false
	providerTemporaryFileError := errors.New("provider temporary file selected")
	providerTemporaryFiles := &edgeProviderTemporaryFileSystem{
		create: func(string, string) (platformfilesystem.TemporaryFile, error) {
			providerTemporaryFileRequested = true
			return nil, providerTemporaryFileError
		},
	}
	directoryCreationRequested := false
	directoryCreator := factorydefinitions.PackagedInstallationDirectoryCreator(func(string, fs.FileMode) error {
		directoryCreationRequested = true
		return nil
	})
	invocationBackend := ModelInvocationBackend(func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return nil, nil, nil
	})
	asrBackend := ModelASRBackend(func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		return models.ASRBackendResponse{}, nil
	})
	embeddingBackend := ModelEmbeddingBackend(func(context.Context, models.EmbeddingBackendRequest) (models.EmbeddingBackendResponse, error) {
		return models.EmbeddingBackendResponse{}, nil
	})

	merged := Merge(Edges{
		APIServerStarter:     defaultStarter,
		BrowserOpener:        defaultBrowserOpener,
		ProviderOverride:     defaultProvider,
		HostedLinearEndpoint: "https://linear.example.test",
		ModelAssetHostPlatform: models.AssetHostPlatform{
			OperatingSystem: "default-os",
			Architecture:    "default-arch",
		},
		WorkContentHostPlatform: "default-os",
	}, Edges{
		APIServerStarter: replacementStarter,
		BrowserOpener:    replacementBrowserOpener,
		ProviderOverride: replacementProvider,
		FactoryRuntimeInputDirectoryWalker: func(string, fs.WalkDirFunc) error {
			walked = true
			return nil
		},
		FactoryRuntimeWorkflowSourceResolveSymlinks: func(path string) (string, error) {
			resolved = true
			return path, nil
		},
		ModelAssetHostPlatform: models.AssetHostPlatform{
			OperatingSystem: "replacement-os",
			Architecture:    "replacement-arch",
		},
		WorkContentHostPlatform:      "replacement-os",
		ModelInvocationBackend:       invocationBackend,
		ModelASRBackend:              asrBackend,
		ModelEmbeddingBackend:        embeddingBackend,
		WorkersFactoryDocsFileSystem: workerDocsFileSystem,
		WorkersResolveSymlinks: func(path string) (string, error) {
			workerSymlinksResolved = true
			return path, nil
		},
		WorkersExecutableLocator:           platformprocess.HostExecutableLocator{},
		ProvidersExecutableLocator:         platformprocess.HostExecutableLocator{},
		WorkersExecutableFileReader:        platformfilesystem.Local{},
		WorkersOperatingSystem:             "replacement-os",
		WorkersWorktreeFileSystem:          platformfilesystem.Local{},
		WorkersWorktreeGit:                 worktreeGit,
		WorkersAgentToolFileSystem:         platformfilesystem.Local{},
		WorkersMockWorkersConfigFileSystem: platformfilesystem.Local{},
		WorkersRetryRandomSource:           retryRandom,
		WorkersWorkstationFileSystem:       platformfilesystem.Local{},
		WorkersProviderTemporaryFileSystem: providerTemporaryFiles,
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			systemInitializationPathInspected = true
			return nil, fs.ErrNotExist
		},
		FactorySessionCursorPersistenceFileSystem:  platformfilesystem.Local{},
		FactorySessionRuntimePersistenceFileSystem: platformfilesystem.Local{},
		FactorySessionExecutionOpeningFileSystem:   platformfilesystem.Local{},
		FactorySessionDirectoryInspection:          platformfilesystem.Local{},
		FactorySessionContractFixtureReader: func(string) ([]byte, error) {
			contractFixtureRead = true
			return nil, nil
		},
		FactorySessionInvocationInputReader: func(string) ([]byte, error) {
			invocationInputRead = true
			return nil, nil
		},
		FactorySessionReplayRecordingReader: func(string) ([]byte, error) {
			replayRecordingRead = true
			return nil, nil
		},
		FactorySessionInitialWorkReader: func(string) ([]byte, error) {
			initialWorkRead = true
			return nil, nil
		},
		FactorySessionResolveHomeDirectory: func() (string, error) {
			homeDirectoryResolved = true
			return "/edge-home", nil
		},
		FactorySessionIDGenerator: func() string {
			sessionIDGenerated = true
			return "session-edge-id"
		},
		FactorySessionCursorCreateTemporaryFile: factorysessions.CursorPersistenceCreateTemporaryFile(func(string, string) (factorysessions.CursorPersistenceTemporaryFile, error) {
			cursorTemporaryFileRequested = true
			return nil, cursorTemporaryFileError
		}),
		FactoryDefinitionLoadingFileSystem:             platformfilesystem.Local{},
		FactoryDefinitionClock:                         platformclock.Real{},
		FactoryDefinitionVersionFileSystem:             platformfilesystem.Local{},
		FactoryDefinitionPackagedGoalPromptFileSystem:  platformfilesystem.Local{},
		FactoryDefinitionPortableBundledFileInspection: platformfilesystem.Local{},
		FactoryDefinitionRequiredToolPathLookup: func(command string) (string, error) {
			requiredToolPathLookedUp = true
			return "/tools/" + command, nil
		},
		FactoryDefinitionRequiredToolVersionProbe: func(string, ...string) ([]byte, error) {
			requiredToolVersionProbed = true
			return []byte("version"), nil
		},
		FactoryDefinitionNamedPathFileSystem:                  platformfilesystem.Local{},
		FactoryDefinitionNamedFactoryCatalogFileSystem:        platformfilesystem.Local{},
		FactoryDefinitionPackagedInstallationFileSystem:       platformfilesystem.Local{},
		FactoryDefinitionPackagedInstallationDirectoryCreator: directoryCreator,
		FactoryDefinitionPersistenceFileSystem:                platformfilesystem.Local{},
		FactoryDefinitionDirectoryReplacementStore:            directoryReplacementStore,
		FactoryDefinitionScaffoldFileSystem:                   platformfilesystem.Local{},
		FactoryDefinitionScaffoldOutput:                       scaffoldOutput,
		WorkRequestIDGenerator: func() string {
			workRequestIDGenerated = true
			return "work-id"
		},
		WorkSubmittedFileReader: func(string) ([]byte, error) {
			workSubmittedFileRead = true
			return []byte("work"), nil
		},
		WorkSubmittedFilePathInspector: func(string) (fs.FileInfo, error) {
			workSubmittedFilePathInspected = true
			return nil, nil
		},
		FactorySessionResponseEventIDGenerator: func() string {
			responseEventIDGenerated = true
			return "response-event-id"
		},
	})
	if err := merged.BrowserOpener(context.Background(), "https://factory.example"); err != nil || !replacementBrowserOpened {
		t.Fatalf("BrowserOpener replacement = (opened %t, error %v)", replacementBrowserOpened, err)
	}

	if merged.APIServerStarter == nil {
		t.Fatal("APIServerStarter = nil")
	}
	if merged.ProviderOverride != replacementProvider {
		t.Fatal("ProviderOverride did not use the explicit replacement")
	}
	if merged.HostedLinearEndpoint != "https://linear.example.test" {
		t.Fatal("Merge discarded an unreplaced default")
	}
	if merged.ModelAssetHostPlatform != (models.AssetHostPlatform{OperatingSystem: "replacement-os", Architecture: "replacement-arch"}) {
		t.Fatalf("ModelAssetHostPlatform = %#v, want explicit replacement", merged.ModelAssetHostPlatform)
	}
	if merged.WorkContentHostPlatform != "replacement-os" {
		t.Fatalf("WorkContentHostPlatform = %q, want explicit replacement", merged.WorkContentHostPlatform)
	}
	if merged.ModelInvocationBackend == nil || merged.ModelASRBackend == nil || merged.ModelEmbeddingBackend == nil {
		t.Fatal("typed model backends were not retained")
	}
	if _, _, err := merged.ModelInvocationBackend(context.Background(), models.InvokeModelRequest{}); err != nil {
		t.Fatalf("ModelInvocationBackend replacement error = %v", err)
	}
	if _, err := merged.ModelASRBackend(context.Background(), models.ASRBackendRequest{}); err != nil {
		t.Fatalf("ModelASRBackend replacement error = %v", err)
	}
	if err := merged.FactoryRuntimeInputDirectoryWalker("ignored", nil); err != nil || !walked {
		t.Fatalf("FactoryRuntimeInputDirectoryWalker replacement = (%v, %v), want injected call", err, walked)
	}
	if _, err := merged.FactoryRuntimeWorkflowSourceResolveSymlinks("ignored"); err != nil || !resolved {
		t.Fatalf("FactoryRuntimeWorkflowSourceResolveSymlinks replacement = (%v, %v), want injected call", err, resolved)
	}
	if got, ok := merged.WorkersFactoryDocsFileSystem.(platformfilesystem.Local); !ok || got != workerDocsFileSystem {
		t.Fatalf("WorkersFactoryDocsFileSystem replacement = %T, want exact injected filesystem", merged.WorkersFactoryDocsFileSystem)
	}
	if _, err := merged.WorkersResolveSymlinks("ignored"); err != nil || !workerSymlinksResolved {
		t.Fatalf("WorkersResolveSymlinks replacement = (%v, %v), want injected call", err, workerSymlinksResolved)
	}
	if _, ok := merged.WorkersExecutableLocator.(platformprocess.HostExecutableLocator); !ok {
		t.Fatalf("WorkersExecutableLocator = %T, want explicit replacement", merged.WorkersExecutableLocator)
	}
	if _, ok := merged.ProvidersExecutableLocator.(platformprocess.HostExecutableLocator); !ok {
		t.Fatalf("ProvidersExecutableLocator = %T, want explicit replacement", merged.ProvidersExecutableLocator)
	}
	if _, ok := merged.WorkersExecutableFileReader.(platformfilesystem.Local); !ok {
		t.Fatalf("WorkersExecutableFileReader = %T, want explicit replacement", merged.WorkersExecutableFileReader)
	}
	if merged.WorkersOperatingSystem != "replacement-os" {
		t.Fatalf("WorkersOperatingSystem = %q, want explicit replacement", merged.WorkersOperatingSystem)
	}
	if _, ok := merged.WorkersWorktreeFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("WorkersWorktreeFileSystem = %T, want explicit replacement", merged.WorkersWorktreeFileSystem)
	}
	if merged.WorkersWorktreeGit != worktreeGit {
		t.Fatal("WorkersWorktreeGit did not use the explicit replacement")
	}
	if _, ok := merged.WorkersAgentToolFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("WorkersAgentToolFileSystem = %T, want explicit replacement", merged.WorkersAgentToolFileSystem)
	}
	if _, ok := merged.WorkersMockWorkersConfigFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("WorkersMockWorkersConfigFileSystem = %T, want explicit replacement", merged.WorkersMockWorkersConfigFileSystem)
	}
	if _, err := merged.SystemInitializationInspectPath("ignored"); !errors.Is(err, fs.ErrNotExist) || !systemInitializationPathInspected {
		t.Fatalf("SystemInitializationInspectPath replacement = (%v, %v), want injected call", err, systemInitializationPathInspected)
	}
	if _, ok := merged.FactorySessionCursorPersistenceFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactorySessionCursorPersistenceFileSystem = %T, want explicit replacement", merged.FactorySessionCursorPersistenceFileSystem)
	}
	if _, ok := merged.FactorySessionDirectoryInspection.(platformfilesystem.Local); !ok {
		t.Fatalf("FactorySessionDirectoryInspection = %T, want explicit replacement", merged.FactorySessionDirectoryInspection)
	}
	if _, err := merged.FactorySessionContractFixtureReader.ReadFile("ignored"); err != nil || !contractFixtureRead {
		t.Fatalf("FactorySessionContractFixtureReader replacement = (%v, %v)", err, contractFixtureRead)
	}
	if _, err := merged.FactorySessionInvocationInputReader.ReadFile("ignored"); err != nil || !invocationInputRead {
		t.Fatalf("FactorySessionInvocationInputReader replacement = (%v, %v)", err, invocationInputRead)
	}
	if _, err := merged.FactorySessionReplayRecordingReader.ReadFile("ignored"); err != nil || !replayRecordingRead {
		t.Fatalf("FactorySessionReplayRecordingReader replacement = (%v, %v)", err, replayRecordingRead)
	}
	if _, err := merged.FactorySessionInitialWorkReader.ReadFile("ignored"); err != nil || !initialWorkRead {
		t.Fatalf("FactorySessionInitialWorkReader replacement = (%v, %v)", err, initialWorkRead)
	}
	if got, err := merged.FactorySessionResolveHomeDirectory(); err != nil || got != "/edge-home" || !homeDirectoryResolved {
		t.Fatalf("FactorySessionResolveHomeDirectory replacement = (%q, %v, %v)", got, err, homeDirectoryResolved)
	}
	if got := merged.FactorySessionIDGenerator(); got != "session-edge-id" || !sessionIDGenerated {
		t.Fatalf("FactorySessionIDGenerator replacement = (%q, %v)", got, sessionIDGenerated)
	}
	if _, ok := merged.FactorySessionExecutionOpeningFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactorySessionExecutionOpeningFileSystem = %T, want explicit replacement", merged.FactorySessionExecutionOpeningFileSystem)
	}
	if _, err := merged.FactorySessionCursorCreateTemporaryFile("ignored", "ignored"); !errors.Is(err, cursorTemporaryFileError) || !cursorTemporaryFileRequested {
		t.Fatalf("FactorySessionCursorCreateTemporaryFile replacement = (%v, %v), want injected call", err, cursorTemporaryFileRequested)
	}
	if _, ok := merged.FactorySessionRuntimePersistenceFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactorySessionRuntimePersistenceFileSystem = %T, want explicit replacement", merged.FactorySessionRuntimePersistenceFileSystem)
	}
	if _, ok := merged.FactoryDefinitionLoadingFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionLoadingFileSystem = %T, want explicit replacement", merged.FactoryDefinitionLoadingFileSystem)
	}
	if _, ok := merged.FactoryDefinitionClock.(platformclock.Real); !ok {
		t.Fatalf("FactoryDefinitionClock = %T, want explicit replacement", merged.FactoryDefinitionClock)
	}
	if _, ok := merged.FactoryDefinitionVersionFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionVersionFileSystem = %T, want explicit replacement", merged.FactoryDefinitionVersionFileSystem)
	}
	if _, ok := merged.FactoryDefinitionPackagedGoalPromptFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionPackagedGoalPromptFileSystem = %T, want explicit replacement", merged.FactoryDefinitionPackagedGoalPromptFileSystem)
	}
	if _, ok := merged.FactoryDefinitionPortableBundledFileInspection.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionPortableBundledFileInspection = %T, want explicit replacement", merged.FactoryDefinitionPortableBundledFileInspection)
	}
	if resolved, err := merged.FactoryDefinitionRequiredToolPathLookup("tool"); err != nil || resolved != "/tools/tool" || !requiredToolPathLookedUp {
		t.Fatalf("FactoryDefinitionRequiredToolPathLookup replacement = (%q, %v, %v)", resolved, err, requiredToolPathLookedUp)
	}
	if output, err := merged.FactoryDefinitionRequiredToolVersionProbe("/tools/tool", "version"); err != nil || string(output) != "version" || !requiredToolVersionProbed {
		t.Fatalf("FactoryDefinitionRequiredToolVersionProbe replacement = (%q, %v, %v)", output, err, requiredToolVersionProbed)
	}
	if got := merged.WorkRequestIDGenerator(); got != "work-id" || !workRequestIDGenerated {
		t.Fatalf("WorkRequestIDGenerator replacement = (%q, %v)", got, workRequestIDGenerated)
	}
	if output, err := merged.WorkSubmittedFileReader("work.json"); err != nil || string(output) != "work" || !workSubmittedFileRead {
		t.Fatalf("WorkSubmittedFileReader replacement = (%q, %v, %v)", output, err, workSubmittedFileRead)
	}
	if _, err := merged.WorkSubmittedFilePathInspector("prompt.txt"); err != nil || !workSubmittedFilePathInspected {
		t.Fatalf("WorkSubmittedFilePathInspector replacement = (%v, %v)", err, workSubmittedFilePathInspected)
	}
	if got := merged.FactorySessionResponseEventIDGenerator(); got != "response-event-id" || !responseEventIDGenerated {
		t.Fatalf("FactorySessionResponseEventIDGenerator replacement = (%q, %v)", got, responseEventIDGenerated)
	}
	if got, err := merged.WorkersRetryRandomSource.Int63n(10); err != nil || got != 7 {
		t.Fatalf("WorkersRetryRandomSource replacement = (%d, %v), want (7, nil)", got, err)
	}
	if _, err := merged.WorkersProviderTemporaryFileSystem.CreateTemp("ignored", "ignored"); !errors.Is(err, providerTemporaryFileError) || !providerTemporaryFileRequested {
		t.Fatalf("WorkersProviderTemporaryFileSystem replacement = (%v, %v), want exact injected filesystem", err, providerTemporaryFileRequested)
	}
	if _, ok := merged.WorkersWorkstationFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("WorkersWorkstationFileSystem = %T, want explicit replacement", merged.WorkersWorkstationFileSystem)
	}
	var _ work.RequestIDGenerator = merged.WorkRequestIDGenerator
	var _ work.SubmittedFileReader = merged.WorkSubmittedFileReader
	var _ work.SubmittedFilePathInspector = merged.WorkSubmittedFilePathInspector
	var _ factorydefinitions.LoadingFileSystem = merged.FactoryDefinitionLoadingFileSystem
	if _, ok := merged.FactoryDefinitionNamedPathFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionNamedPathFileSystem = %T, want explicit replacement", merged.FactoryDefinitionNamedPathFileSystem)
	}
	if _, ok := merged.FactoryDefinitionNamedFactoryCatalogFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionNamedFactoryCatalogFileSystem = %T, want explicit replacement", merged.FactoryDefinitionNamedFactoryCatalogFileSystem)
	}
	if _, ok := merged.FactoryDefinitionPackagedInstallationFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionPackagedInstallationFileSystem = %T, want explicit replacement", merged.FactoryDefinitionPackagedInstallationFileSystem)
	}
	if merged.FactoryDefinitionPackagedInstallationDirectoryCreator == nil {
		t.Fatal("FactoryDefinitionPackagedInstallationDirectoryCreator = nil, want explicit replacement")
	}
	if err := merged.FactoryDefinitionPackagedInstallationDirectoryCreator("ignored", 0o755); err != nil || !directoryCreationRequested {
		t.Fatalf("FactoryDefinitionPackagedInstallationDirectoryCreator replacement = (%v, %v), want injected call", err, directoryCreationRequested)
	}
	if _, ok := merged.FactoryDefinitionPersistenceFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionPersistenceFileSystem = %T, want explicit replacement", merged.FactoryDefinitionPersistenceFileSystem)
	}
	if merged.FactoryDefinitionDirectoryReplacementStore != directoryReplacementStore {
		t.Fatal("FactoryDefinitionDirectoryReplacementStore did not use explicit replacement")
	}
	if _, ok := merged.FactoryDefinitionScaffoldFileSystem.(platformfilesystem.Local); !ok {
		t.Fatalf("FactoryDefinitionScaffoldFileSystem = %T, want explicit replacement", merged.FactoryDefinitionScaffoldFileSystem)
	}
	if merged.FactoryDefinitionScaffoldOutput != scaffoldOutput {
		t.Fatal("FactoryDefinitionScaffoldOutput did not use explicit replacement")
	}
}

func TestMergeUsesCallerOwnedAgyPTYClock(t *testing.T) {
	t.Parallel()

	replacement := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
	merged := Merge(
		Edges{AgyPTYClock: platformclock.Real{}},
		Edges{AgyPTYClock: replacement},
	)
	if merged.AgyPTYClock != replacement {
		t.Fatal("Merge did not preserve the caller-owned Agy PTY clock edge")
	}
}

func TestMergeAppendsAndDetachesProviderRegistrations(t *testing.T) {
	t.Parallel()

	defaultRegistration := inference.Registration{Manifest: inference.Manifest{ID: "customer.alpha"}}
	addedRegistration := inference.Registration{Manifest: inference.Manifest{ID: "customer.beta"}}
	defaults := []inference.Registration{defaultRegistration}
	additions := []inference.Registration{addedRegistration}

	merged := Merge(
		Edges{ProviderRegistrations: defaults},
		Edges{ProviderRegistrations: additions},
	)
	defaults[0] = addedRegistration
	additions[0] = defaultRegistration

	want := inference.ProviderRegistrations{defaultRegistration, addedRegistration}
	if !reflect.DeepEqual(merged.ProviderRegistrations, want) {
		t.Fatalf("ProviderRegistrations = %#v, want detached append %#v", merged.ProviderRegistrations, want)
	}
}

func TestMergeAppendsAndDetachesCatalogCapabilityOverrides(t *testing.T) {
	t.Parallel()

	defaultOverride := inference.CatalogCapabilityOverride{
		Provider:     providers.IDCodex,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	addedOverride := inference.CatalogCapabilityOverride{
		Provider:     providers.IDClaude,
		Capabilities: []providers.Capability{providers.CapabilitySessionResume},
	}
	defaults := []inference.CatalogCapabilityOverride{defaultOverride}
	additions := []inference.CatalogCapabilityOverride{addedOverride}

	merged := Merge(
		Edges{ProviderCatalogCapabilityOverrides: defaults},
		Edges{ProviderCatalogCapabilityOverrides: additions},
	)
	defaults[0].Capabilities[0] = providers.CapabilityUsage
	additions[0].Capabilities[0] = providers.CapabilityUsage

	want := []inference.CatalogCapabilityOverride{
		{
			Provider:     providers.IDCodex,
			Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
		},
		{
			Provider:     providers.IDClaude,
			Capabilities: []providers.Capability{providers.CapabilitySessionResume},
		},
	}
	if !reflect.DeepEqual(merged.ProviderCatalogCapabilityOverrides, want) {
		t.Fatalf("ProviderCatalogCapabilityOverrides = %#v, want detached append %#v", merged.ProviderCatalogCapabilityOverrides, want)
	}
}

type edgeAgyPTYHost struct{}

func (*edgeAgyPTYHost) Allocate(context.Context) (platformpty.Allocation, error) { return nil, nil }
func (*edgeAgyPTYHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, nil
}

func TestMergeUsesCallerOwnedAgyPTYHost(t *testing.T) {
	t.Parallel()

	replacement := &edgeAgyPTYHost{}
	merged := Merge(Edges{}, Edges{AgyPTYHost: replacement})
	if merged.AgyPTYHost != replacement {
		t.Fatal("Merge did not preserve the caller-owned Agy PTY host edge")
	}
}

type edgeProviderTemporaryFileSystem struct {
	create func(string, string) (platformfilesystem.TemporaryFile, error)
}

func (f *edgeProviderTemporaryFileSystem) CreateTemp(dir, pattern string) (platformfilesystem.TemporaryFile, error) {
	return f.create(dir, pattern)
}

func (*edgeProviderTemporaryFileSystem) Remove(string) error { return nil }

func TestMergeWithEmptyReplacementsPreservesProductionDefaults(t *testing.T) {
	t.Parallel()

	starter := platformhttpserver.Starter(func(context.Context, platformhttpserver.StartRequest) error {
		return nil
	})
	defaults := Edges{APIServerStarter: starter}

	merged := Merge(defaults, Edges{})

	if merged.APIServerStarter == nil {
		t.Fatal("APIServerStarter = nil")
	}
}

func TestMergeAppliesAssetAndHostedEndpointReplacements(t *testing.T) {
	t.Parallel()

	merged := Merge(
		Edges{HostedLinearEndpoint: "https://default.example"},
		Edges{
			HostedLinearEndpoint: "https://replacement.example",
			ModelAssetEndpoints: models.RuntimeAssetEndpoints{
				BaseURL:    "https://assets.example",
				APIBaseURL: "https://api-assets.example",
			},
			ModelAssetHostPlatform: models.AssetHostPlatform{
				OperatingSystem: "replacement-os",
			},
		},
	)
	if merged.HostedLinearEndpoint != "https://replacement.example" {
		t.Fatalf("HostedLinearEndpoint = %q, want replacement", merged.HostedLinearEndpoint)
	}
	if merged.ModelAssetEndpoints.BaseURL != "https://assets.example" {
		t.Fatalf("ModelAssetEndpoints.BaseURL = %q, want replacement", merged.ModelAssetEndpoints.BaseURL)
	}
	if merged.ModelAssetEndpoints.APIBaseURL != "https://api-assets.example" {
		t.Fatalf("ModelAssetEndpoints.APIBaseURL = %q, want replacement", merged.ModelAssetEndpoints.APIBaseURL)
	}
	if merged.ModelAssetHostPlatform.OperatingSystem != "replacement-os" {
		t.Fatalf("ModelAssetHostPlatform.OperatingSystem = %q, want replacement", merged.ModelAssetHostPlatform.OperatingSystem)
	}
}

func TestMergeAppliesModelAssetAndHostEffectReplacements(t *testing.T) {
	t.Parallel()

	environment := func(name string) string { return "value:" + name }
	embedding := ModelEmbeddingBackend(func(context.Context, models.EmbeddingBackendRequest) (models.EmbeddingBackendResponse, error) {
		return models.EmbeddingBackendResponse{Embeddings: []float64{0.1}}, nil
	})
	backendArtifactResolver := ModelResolveBackendArtifact(func(
		context.Context,
		ModelBackendArtifactSelectionRequest,
	) (ModelBackendArtifactSelection, error) {
		return ModelBackendArtifactSelection{Name: "fixture-backend"}, nil
	})
	protocol := &edgeModelHostProtocol{}
	dialer := &edgeModelHostGRPCDialer{}
	invocationDialer := platformgrpc.NetworkDialer{}
	compatibility := &edgeModelHostCompatibilityChecker{}
	coordinationFactory := AssetStagingCoordinationFactory(func() (AssetStagingCoordination, error) {
		return nil, nil
	})
	merged := Merge(Edges{}, Edges{
		ModelAssetResolveEnvironment:         environment,
		ModelEmbeddingBackend:                embedding,
		ModelResolveBackendArtifact:          backendArtifactResolver,
		ModelAssetStagingCoordinationFactory: coordinationFactory,
		ModelHostProtocolNegotiator:          protocol,
		ModelHostGRPCDialer:                  dialer,
		ModelInvocationGRPCDialer:            invocationDialer,
		ModelHostCompatibilityChecker:        compatibility,
	})
	if merged.ModelAssetResolveEnvironment("CACHE") != "value:CACHE" {
		t.Fatalf("asset environment edge was not replaced")
	}
	if merged.ModelEmbeddingBackend == nil {
		t.Fatal("embedding backend edge was not replaced")
	}
	if response, err := merged.ModelEmbeddingBackend(context.Background(), models.EmbeddingBackendRequest{Text: "hello"}); err != nil || len(response.Embeddings) != 1 {
		t.Fatalf("embedding backend edge = (%#v, %v), want fixture response", response, err)
	}
	artifact, err := merged.ModelResolveBackendArtifact(context.Background(), ModelBackendArtifactSelectionRequest{})
	if err != nil || artifact.Name != "fixture-backend" {
		t.Fatalf("backend artifact resolver = (%#v, %v), want fixture-backend", artifact, err)
	}
	if merged.ModelHostProtocolNegotiator != protocol {
		t.Fatal("protocol negotiator edge was not replaced")
	}
	if merged.ModelHostGRPCDialer != dialer {
		t.Fatal("gRPC dialer edge was not replaced")
	}
	if merged.ModelInvocationGRPCDialer != invocationDialer {
		t.Fatal("invocation gRPC dialer edge was not replaced")
	}
	if merged.ModelHostCompatibilityChecker != compatibility {
		t.Fatal("compatibility checker edge was not replaced")
	}
	if merged.ModelAssetStagingCoordinationFactory == nil {
		t.Fatal("asset staging coordination factory edge was not replaced")
	}
	coordination, err := merged.ModelAssetStagingCoordinationFactory()
	if err != nil || coordination != nil {
		t.Fatalf("asset staging coordination factory = (%#v, %v), want nil fixture result", coordination, err)
	}
}

func TestMergeAppliesAndPreservesModelInvocationBackend(t *testing.T) {
	t.Parallel()

	defaultBackend := ModelInvocationBackend(func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return []models.InferenceContent{{Content: "default"}}, nil, nil
	})
	replacementBackend := ModelInvocationBackend(func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return []models.InferenceContent{{Content: "replacement"}}, nil, nil
	})

	merged := Merge(Edges{ModelInvocationBackend: defaultBackend}, Edges{ModelInvocationBackend: replacementBackend})
	content, _, err := merged.ModelInvocationBackend(context.Background(), models.InvokeModelRequest{})
	if err != nil || len(content) != 1 || content[0].Content != "replacement" {
		t.Fatalf("replaced ModelInvocationBackend = (%#v, %v), want replacement content", content, err)
	}

	preserved := Merge(Edges{ModelInvocationBackend: defaultBackend}, Edges{})
	content, _, err = preserved.ModelInvocationBackend(context.Background(), models.InvokeModelRequest{})
	if err != nil || len(content) != 1 || content[0].Content != "default" {
		t.Fatalf("preserved ModelInvocationBackend = (%#v, %v), want default content", content, err)
	}

	defaultASR := ModelASRBackend(func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		return models.ASRBackendResponse{Text: "default"}, nil
	})
	replacementASR := ModelASRBackend(func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		return models.ASRBackendResponse{Text: "replacement"}, nil
	})
	merged = Merge(Edges{ModelASRBackend: defaultASR}, Edges{ModelASRBackend: replacementASR})
	response, err := merged.ModelASRBackend(context.Background(), models.ASRBackendRequest{})
	if err != nil || response.Text != "replacement" {
		t.Fatalf("replaced ModelASRBackend = (%#v, %v), want replacement response", response, err)
	}
}

func TestMergeAppliesCLIOutputInspectionReplacement(t *testing.T) {
	t.Parallel()

	defaultErr := errors.New("default inspect")
	replacementErr := errors.New("replacement inspect")
	defaultInspect := func(string) (fs.FileInfo, error) { return nil, defaultErr }
	replacementInspect := func(string) (fs.FileInfo, error) { return nil, replacementErr }
	merged := Merge(
		Edges{ModelCLIOutputInspectPath: defaultInspect},
		Edges{ModelCLIOutputInspectPath: replacementInspect},
	)
	if _, err := merged.ModelCLIOutputInspectPath("mapped.out"); !errors.Is(err, replacementErr) {
		t.Fatalf("merged ModelCLIOutputInspectPath error = %v, want replacement error", err)
	}
}

func TestMergeAppliesCLIInputReadReplacement(t *testing.T) {
	t.Parallel()

	defaultRead := ModelCLIInputReadFile(func(context.Context, string, int64) ([]byte, error) {
		return []byte("default"), nil
	})
	replacementRead := ModelCLIInputReadFile(func(context.Context, string, int64) ([]byte, error) {
		return []byte("replacement"), nil
	})
	merged := Merge(
		Edges{ModelCLIInputReadFile: defaultRead},
		Edges{ModelCLIInputReadFile: replacementRead},
	)
	got, err := merged.ModelCLIInputReadFile(context.Background(), "prompt.txt", 1024)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("merged ModelCLIInputReadFile = (%q, %v), want replacement", got, err)
	}
}

func TestMergeReplacesAndPreservesRecordingArtifactReadEffect(t *testing.T) {
	t.Parallel()

	defaultRead := recordings.RecordingReadFile(func(string) ([]byte, error) {
		return []byte("default"), nil
	})
	replacementRead := recordings.RecordingReadFile(func(string) ([]byte, error) {
		return []byte("replacement"), nil
	})

	merged := Merge(
		Edges{RecordingReadFile: defaultRead},
		Edges{RecordingReadFile: replacementRead},
	)
	if got, err := merged.RecordingReadFile("artifact"); err != nil || string(got) != "replacement" {
		t.Fatalf("merged RecordingReadFile = (%q, %v), want replacement", got, err)
	}

	preserved := Merge(Edges{RecordingReadFile: defaultRead}, Edges{})
	if got, err := preserved.RecordingReadFile("artifact"); err != nil || string(got) != "default" {
		t.Fatalf("preserved RecordingReadFile = (%q, %v), want default", got, err)
	}

	defaultOpen := recordings.RecordingOpenFile(func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("default"))), nil
	})
	replacementOpen := recordings.RecordingOpenFile(func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("replacement"))), nil
	})
	merged = Merge(
		Edges{RecordingOpenFile: defaultOpen},
		Edges{RecordingOpenFile: replacementOpen},
	)
	opened, err := merged.RecordingOpenFile("artifact")
	if err != nil {
		t.Fatalf("merged RecordingOpenFile: %v", err)
	}
	got, err := io.ReadAll(opened)
	_ = opened.Close()
	if err != nil || string(got) != "replacement" {
		t.Fatalf("merged RecordingOpenFile = (%q, %v), want replacement", got, err)
	}

	preserved = Merge(Edges{RecordingOpenFile: defaultOpen}, Edges{})
	opened, err = preserved.RecordingOpenFile("artifact")
	if err != nil {
		t.Fatalf("preserved RecordingOpenFile: %v", err)
	}
	got, err = io.ReadAll(opened)
	_ = opened.Close()
	if err != nil || string(got) != "default" {
		t.Fatalf("preserved RecordingOpenFile = (%q, %v), want default", got, err)
	}
}

type stubProvider struct {
	id string
	testutil.ProviderServiceAdapter
}

type edgeDirectoryReplacementStore struct{}

type edgeWorktreeGit struct{}

type edgeModelHostProtocol struct{}

func (edgeModelHostProtocol) Negotiate(
	context.Context,
	string,
	ModelHostProtocolNegotiationRequest,
) (ModelHostProtocolNegotiationResult, error) {
	return ModelHostProtocolNegotiationResult{}, nil
}

type edgeModelHostGRPCDialer struct{}

func (edgeModelHostGRPCDialer) Dial(context.Context, string) (interface {
	Negotiate(
		context.Context,
		ModelHostProtocolNegotiationRequest,
	) (ModelHostProtocolNegotiationResult, error)
	Close() error
}, error) {
	return nil, nil
}

type edgeModelHostCompatibilityChecker struct{}

func (edgeModelHostCompatibilityChecker) Check(context.Context, ModelHostCompatibilityRequest) error {
	return nil
}

func (*edgeWorktreeGit) Run(context.Context, string, ...string) (string, string, int, error) {
	return "", "", 0, nil
}

func (*edgeDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}

func (*edgeDirectoryReplacementStore) Restore(string, string) {}

func (*stubProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	return workers.InferenceResponse{}, nil
}
