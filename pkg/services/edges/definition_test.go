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

	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
		FactoryDefinitionNamedPathFileSystem:            platformfilesystem.Local{},
		FactoryDefinitionNamedFactoryCatalogFileSystem:  platformfilesystem.Local{},
		FactoryDefinitionPackagedInstallationFileSystem: platformfilesystem.Local{},
		FactoryDefinitionPersistenceFileSystem:          platformfilesystem.Local{},
		FactoryDefinitionDirectoryReplacementStore:      directoryReplacementStore,
		FactoryDefinitionScaffoldFileSystem:             platformfilesystem.Local{},
		FactoryDefinitionScaffoldOutput:                 scaffoldOutput,
		WorkRequestIDGenerator: func() string {
			workRequestIDGenerated = true
			return "work-id"
		},
		WorkSubmittedFileReader: func(string) ([]byte, error) {
			workSubmittedFileRead = true
			return []byte("work"), nil
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

type stubProvider struct{ id string }

type edgeDirectoryReplacementStore struct{}

type edgeWorktreeGit struct{}

func (*edgeWorktreeGit) Run(context.Context, string, ...string) (string, string, int, error) {
	return "", "", 0, nil
}

func (*edgeDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}

func (*edgeDirectoryReplacementStore) Restore(string, string) {}

func (*stubProvider) Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
	return workers.RunnerExecutionResult{}, nil
}
