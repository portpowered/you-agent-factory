package wire

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"go.uber.org/zap"
)

func TestNewServiceRejectsMissingConstructionPorts(t *testing.T) {
	t.Parallel()

	edges := validConstructionEdges()
	for _, test := range missingConstructionPortCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertMissingConstructionPort(t, edges, test)
		})
	}
}

type missingConstructionPortCase struct {
	name   string
	mutate func(*constructionEdges)
	want   string
}

func missingConstructionPortCases() []missingConstructionPortCase {
	cases := missingAssetConstructionPortCases()
	cases = append(cases, missingHostRuntimeConstructionPortCases()...)
	return cases
}

func missingAssetConstructionPortCases() []missingConstructionPortCase {
	return []missingConstructionPortCase{
		{
			name: "issuer entropy",
			mutate: func(edges *constructionEdges) {
				edges.issuerEntropy = nil
			},
			want: "issuer entropy is required",
		},
		{
			name: "asset host platform",
			mutate: func(edges *constructionEdges) {
				edges.assetPlatform = models.AssetHostPlatform{}
			},
			want: "asset host platform is required",
		},
		{
			name: "asset HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.assetHTTP = nil
			},
			want: "asset HTTP client is required",
		},
		{
			name: "asset make-directories effect",
			mutate: func(edges *constructionEdges) {
				edges.assetMkdirAll = nil
			},
			want: "asset make-directories effect is required",
		},
		{
			name: "asset inspect-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetStat = nil
			},
			want: "asset inspect-path effect is required",
		},
		{
			name: "asset resolve-home effect",
			mutate: func(edges *constructionEdges) {
				edges.assetHome = nil
			},
			want: "asset resolve-home effect is required",
		},
		{
			name: "asset write-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetWriteFile = nil
			},
			want: "asset write-file effect is required",
		},
		{
			name: "asset rename-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetRename = nil
			},
			want: "asset rename-path effect is required",
		},
		{
			name: "asset remove-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetRemove = nil
			},
			want: "asset remove-path effect is required",
		},
		{
			name: "asset read-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetReadFile = nil
			},
			want: "asset read-file effect is required",
		},
		{
			name: "asset read-directory effect",
			mutate: func(edges *constructionEdges) {
				edges.assetReadDir = nil
			},
			want: "asset read-directory effect is required",
		},
		{
			name: "asset create-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetCreate = nil
			},
			want: "asset create-file effect is required",
		},
		{
			name: "asset open-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetOpen = nil
			},
			want: "asset open-file effect is required",
		},
	}
}

func missingHostRuntimeConstructionPortCases() []missingConstructionPortCase {
	return []missingConstructionPortCase{
		{
			name: "process launcher",
			mutate: func(edges *constructionEdges) {
				edges.processLauncher = nil
			},
			want: "model host process launcher is required",
		},
		{
			name: "host HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.hostHTTP = nil
			},
			want: "model host HTTP client is required",
		},
		{
			name: "host clock",
			mutate: func(edges *constructionEdges) {
				edges.hostClock = nil
			},
			want: "model host clock is required",
		},
		{
			name: "runtime command runner",
			mutate: func(edges *constructionEdges) {
				edges.runtimeRunner = nil
			},
			want: "model runtime command runner is required",
		},
		{
			name: "runtime HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.runtimeHTTP = nil
			},
			want: "model runtime HTTP client is required",
		},
		{
			name: "runtime file inspector",
			mutate: func(edges *constructionEdges) {
				edges.runtimeInspect = nil
			},
			want: "model runtime file inspector is required",
		},
		{
			name: "runtime temporary directory resolver",
			mutate: func(edges *constructionEdges) {
				edges.runtimeTempDir = nil
			},
			want: "model runtime temporary directory resolver is required",
		},
		{
			name: "runtime temporary file creator",
			mutate: func(edges *constructionEdges) {
				edges.runtimeTempFile = nil
			},
			want: "model runtime temporary file creator is required",
		},
		{
			name: "process clock",
			mutate: func(edges *constructionEdges) {
				edges.now = nil
			},
			want: "process clock is required",
		},
	}
}

func assertMissingConstructionPort(
	t *testing.T,
	edges constructionEdges,
	test missingConstructionPortCase,
) {
	t.Helper()

	current := edges
	test.mutate(&current)
	service, err := current.newService()
	if service != nil {
		t.Fatal("NewService() returned non-nil service, want nil")
	}
	if err == nil || !strings.Contains(err.Error(), test.want) {
		t.Fatalf("NewService() error = %v, want %q", err, test.want)
	}
}

func TestNewServiceReturnsPublishedRootInterface(t *testing.T) {
	t.Parallel()

	var service models.Service = mustNewWireService(t)
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
}

func TestNewInvocationArtifactExporterConstructsPublishedExporter(t *testing.T) {
	t.Parallel()

	exporter, err := NewInvocationArtifactExporter(invocationArtifactFileSystemStub{})
	if err != nil {
		t.Fatalf("NewInvocationArtifactExporter() error = %v", err)
	}
	if exporter == nil {
		t.Fatal("NewInvocationArtifactExporter() returned nil exporter")
	}
}

func TestNewServiceHonorsRuntimeAssetEndpointOverrides(t *testing.T) {
	t.Parallel()

	edges := validConstructionEdges()
	edges.assetEndpoints = models.RuntimeAssetEndpoints{
		BaseURL:    "https://example.test/models",
		APIBaseURL: "https://example.test/api",
	}
	service, err := edges.newService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
}

type invocationArtifactFileSystemStub struct{}

func (invocationArtifactFileSystemStub) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (invocationArtifactFileSystemStub) Create(string) (io.WriteCloser, error) {
	return nopWriteCloser{}, nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	assetHTTP := &recordingHTTPDoer{name: "asset HTTP"}
	hostHTTP := &recordingHTTPDoer{name: "host HTTP"}
	runtimeHTTP := &recordingHTTPDoer{name: "runtime HTTP"}
	processLauncher := &recordingProcessLauncher{}
	hostClock := &recordingHostClock{}
	runtimeRunner := &recordingCommandRunner{}
	assetMkdirAll := &recordingAssetMkdirAll{}
	assetStat := &recordingAssetStat{}
	assetHome := &recordingAssetHome{}
	assetWriteFile := &recordingAssetWriteFile{}
	assetRename := &recordingAssetRename{}
	assetRemove := &recordingAssetRemove{}
	assetReadFile := &recordingAssetReadFile{}
	assetReadDir := &recordingAssetReadDir{}
	assetCreate := &recordingAssetCreate{}
	assetOpen := &recordingAssetOpen{}
	runtimeInspect := &recordingRuntimeInspect{}
	runtimeTempDir := &recordingRuntimeTempDir{}
	runtimeTempFile := &recordingRuntimeTempFile{}
	processClock := &recordingProcessClock{}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		assetHTTP,
		models.RuntimeAssetEndpoints{},
		assetMkdirAll.mkdirAll,
		assetStat.stat,
		assetHome.home,
		assetWriteFile.write,
		assetRename.rename,
		assetRemove.remove,
		assetReadFile.read,
		assetReadDir.readDir,
		assetCreate.create,
		assetOpen.open,
		processLauncher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect.inspect,
		runtimeTempDir.tempDir,
		runtimeTempFile.create,
		zap.NewNop(),
		processClock.now,
		platformrandom.CryptoSource{},
		nil,
		nil,
		nil,
		modelseffects.LocalRuntimeHooks{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var peer models.Service = service
	if peer == nil {
		t.Fatal("constructed value is not assignable to models.Service")
	}

	assertInertConstruction(t, "asset HTTP", assetHTTP.calls)
	assertInertConstruction(t, "host HTTP", hostHTTP.calls)
	assertInertConstruction(t, "runtime HTTP", runtimeHTTP.calls)
	assertInertConstruction(t, "process launcher", processLauncher.starts)
	assertInertConstruction(t, "host clock", hostClock.nowCalls+hostClock.timerCalls)
	assertInertConstruction(t, "runtime command runner", runtimeRunner.calls)
	assertInertConstruction(t, "asset mkdir", assetMkdirAll.calls)
	assertInertConstruction(t, "asset stat", assetStat.calls)
	assertInertConstruction(t, "asset home", assetHome.calls)
	assertInertConstruction(t, "asset write", assetWriteFile.calls)
	assertInertConstruction(t, "asset rename", assetRename.calls)
	assertInertConstruction(t, "asset remove", assetRemove.calls)
	assertInertConstruction(t, "asset read file", assetReadFile.calls)
	assertInertConstruction(t, "asset read dir", assetReadDir.calls)
	assertInertConstruction(t, "asset create", assetCreate.calls)
	assertInertConstruction(t, "asset open", assetOpen.calls)
	assertInertConstruction(t, "runtime inspect", runtimeInspect.calls)
	assertInertConstruction(t, "runtime temp dir", runtimeTempDir.calls)
	assertInertConstruction(t, "runtime temp file", runtimeTempFile.calls)
	assertInertConstruction(t, "process clock", processClock.calls)

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no lifecycle goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}
}

func assertInertConstruction(t *testing.T, effect string, calls int) {
	t.Helper()
	if calls != 0 {
		t.Fatalf("construction invoked %s %d times, want inert construction", effect, calls)
	}
}

type constructionEdges struct {
	assetPlatform   models.AssetHostPlatform
	assetHTTP       modelseffects.AssetHTTPDoer
	assetEndpoints  models.RuntimeAssetEndpoints
	assetMkdirAll   modelseffects.AssetMakeDirectories
	assetStat       modelseffects.AssetInspectPath
	assetHome       modelseffects.AssetResolveHomeDirectory
	assetWriteFile  modelseffects.AssetWriteFile
	assetRename     modelseffects.AssetRenamePath
	assetRemove     modelseffects.AssetRemovePath
	assetReadFile   modelseffects.AssetReadFile
	assetReadDir    modelseffects.AssetReadDirectory
	assetCreate     modelseffects.AssetCreateFile
	assetOpen       modelseffects.AssetOpenFile
	processLauncher modelseffects.HostProcessLauncher
	hostHTTP        modelseffects.HostHTTPDoer
	hostClock       modelseffects.HostClock
	runtimeRunner   platformprocess.CommandRunner
	runtimeHTTP     modelseffects.RuntimeHTTPDoer
	runtimeInspect  modelseffects.RuntimeInspectFile
	runtimeTempDir  modelseffects.RuntimeTempDirectory
	runtimeTempFile modelseffects.RuntimeCreateTempFile
	now             func() time.Time
	issuerEntropy   platformrandom.Source
}

func validConstructionEdges() constructionEdges {
	return constructionEdges{
		assetPlatform: models.AssetHostPlatform{
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
		},
		assetHTTP:       http.DefaultClient,
		assetMkdirAll:   os.MkdirAll,
		assetStat:       os.Stat,
		assetHome:       os.UserHomeDir,
		assetWriteFile:  os.WriteFile,
		assetRename:     os.Rename,
		assetRemove:     os.Remove,
		assetReadFile:   os.ReadFile,
		assetReadDir:    os.ReadDir,
		assetCreate:     func(path string) (io.WriteCloser, error) { return os.Create(path) },
		assetOpen:       func(path string) (io.ReadCloser, error) { return os.Open(path) },
		processLauncher: inertProcessLauncher{},
		hostHTTP:        http.DefaultClient,
		hostClock:       inertHostClock{},
		runtimeRunner:   inertCommandRunner{},
		runtimeHTTP:     http.DefaultClient,
		runtimeInspect:  os.Stat,
		runtimeTempDir:  os.TempDir,
		runtimeTempFile: func(dir, pattern string) (modelseffects.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		now:           func() time.Time { return time.Unix(123, 456) },
		issuerEntropy: platformrandom.CryptoSource{},
	}
}

func (edges constructionEdges) newService() (models.Service, error) {
	return NewService(
		edges.assetPlatform,
		edges.assetHTTP,
		edges.assetEndpoints,
		edges.assetMkdirAll,
		edges.assetStat,
		edges.assetHome,
		edges.assetWriteFile,
		edges.assetRename,
		edges.assetRemove,
		edges.assetReadFile,
		edges.assetReadDir,
		edges.assetCreate,
		edges.assetOpen,
		edges.processLauncher,
		edges.hostHTTP,
		edges.hostClock,
		edges.runtimeRunner,
		edges.runtimeHTTP,
		edges.runtimeInspect,
		edges.runtimeTempDir,
		edges.runtimeTempFile,
		zap.NewNop(),
		edges.now,
		edges.issuerEntropy,
		nil,
		nil,
		nil,
		modelseffects.LocalRuntimeHooks{},
	)
}

func mustNewWireService(t *testing.T) models.Service {
	t.Helper()
	service, err := validConstructionEdges().newService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestNewServiceServesPublishedModelsPeerBehavior(t *testing.T) {
	t.Parallel()

	var service models.Service = mustNewWireService(t)
	config := models.RuntimeScopeConfig{
		CacheDirectory: t.TempDir(),
		Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{{
				Name:          "voice-local",
				Type:          models.RuntimeWorkerTypeModel,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelLocality: models.RuntimeModelLocalityLocal,
				Operations:    []models.RuntimeOperation{{Name: "TTS"}},
			}},
		},
	}
	opened, err := service.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "OMNIVOICE_Q4_K_M", Operation: "TTS",
	})
	if err != nil {
		t.Fatalf("GetModelReadiness: %v", err)
	}
	if readiness.Readiness.ReadinessState != models.ReadinessStateMissing {
		t.Fatalf("scoped readiness = %s, want MISSING", readiness.Readiness.ReadinessState)
	}

	other := mustNewWireService(t)
	foreign, err := other.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("open foreign scope: %v", err)
	}
	if opened.Scope.String() == foreign.Scope.String() {
		t.Fatalf("separate Wire-built authorities issued the same scope %q", opened.Scope.String())
	}

	_, err = service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: foreign.Scope},
	)
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("ListCatalog foreign scope error = %v, want ErrRuntimeScopeForeign", err)
	}
}

type inertProcessLauncher struct{}

func (inertProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	panic("process launcher called during readiness inspection")
}

type inertHostClock struct{}

func (inertHostClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (inertHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	panic("host timer created during readiness inspection")
}

type inertCommandRunner struct{}

func (inertCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	panic("runtime command called during readiness inspection")
}

type recordingHTTPDoer struct {
	name  string
	calls int
}

func (doer *recordingHTTPDoer) Do(*http.Request) (*http.Response, error) {
	doer.calls++
	panic(doer.name + " client invoked during inert construction")
}

type recordingProcessLauncher struct{ starts int }

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher invoked during inert construction")
}

type recordingHostClock struct {
	nowCalls   int
	timerCalls int
}

func (clock *recordingHostClock) Now() time.Time {
	clock.nowCalls++
	panic("host clock invoked during inert construction")
}

func (clock *recordingHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert construction")
}

type recordingCommandRunner struct{ calls int }

func (runner *recordingCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls++
	panic("runtime command runner invoked during inert construction")
}

type recordingAssetMkdirAll struct{ calls int }

func (effect *recordingAssetMkdirAll) mkdirAll(string, os.FileMode) error {
	effect.calls++
	panic("asset mkdir invoked during inert construction")
}

type recordingAssetStat struct{ calls int }

func (effect *recordingAssetStat) stat(string) (os.FileInfo, error) {
	effect.calls++
	panic("asset stat invoked during inert construction")
}

type recordingAssetHome struct{ calls int }

func (effect *recordingAssetHome) home() (string, error) {
	effect.calls++
	panic("asset home invoked during inert construction")
}

type recordingAssetWriteFile struct{ calls int }

func (effect *recordingAssetWriteFile) write(string, []byte, os.FileMode) error {
	effect.calls++
	panic("asset write invoked during inert construction")
}

type recordingAssetRename struct{ calls int }

func (effect *recordingAssetRename) rename(string, string) error {
	effect.calls++
	panic("asset rename invoked during inert construction")
}

type recordingAssetRemove struct{ calls int }

func (effect *recordingAssetRemove) remove(string) error {
	effect.calls++
	panic("asset remove invoked during inert construction")
}

type recordingAssetReadFile struct{ calls int }

func (effect *recordingAssetReadFile) read(string) ([]byte, error) {
	effect.calls++
	panic("asset read file invoked during inert construction")
}

type recordingAssetReadDir struct{ calls int }

func (effect *recordingAssetReadDir) readDir(string) ([]os.DirEntry, error) {
	effect.calls++
	panic("asset read dir invoked during inert construction")
}

type recordingAssetCreate struct{ calls int }

func (effect *recordingAssetCreate) create(string) (io.WriteCloser, error) {
	effect.calls++
	panic("asset create invoked during inert construction")
}

type recordingAssetOpen struct{ calls int }

func (effect *recordingAssetOpen) open(string) (io.ReadCloser, error) {
	effect.calls++
	panic("asset open invoked during inert construction")
}

type recordingRuntimeInspect struct{ calls int }

func (effect *recordingRuntimeInspect) inspect(string) (os.FileInfo, error) {
	effect.calls++
	panic("runtime inspect invoked during inert construction")
}

type recordingRuntimeTempDir struct{ calls int }

func (effect *recordingRuntimeTempDir) tempDir() string {
	effect.calls++
	panic("runtime temp dir invoked during inert construction")
}

type recordingRuntimeTempFile struct{ calls int }

func (effect *recordingRuntimeTempFile) create(string, string) (modelseffects.RuntimeTempFile, error) {
	effect.calls++
	panic("runtime temp file invoked during inert construction")
}

type recordingProcessClock struct{ calls int }

func (clock *recordingProcessClock) now() time.Time {
	clock.calls++
	panic("process clock invoked during inert construction")
}
