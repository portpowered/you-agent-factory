package wire

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

var (
	_ modelswire.AssetHTTPDoer                = serviceedges.Edges{}.ModelAssetHTTPClient
	_ modelswire.HostHTTPDoer                 = serviceedges.Edges{}.ModelHostHTTPClient
	_ modelswire.RuntimeHTTPDoer              = serviceedges.Edges{}.ModelRuntimeHTTPClient
	_ modelswire.InvocationArtifactFileSystem = serviceedges.Edges{}.ModelInvocationArtifactFileSystem
	_ modelswire.HostProcessLauncher          = modelHostProcessLauncherAdapter{}
	_ modelswire.HostClock                    = modelHostClockAdapter{}
	_ modelswire.RuntimeCreateTempFile        = adaptModelRuntimeTempFile(nil)
	_ modelswire.PullMetricsRecorder          = modelsPullMetricsAdapter{}
)

func TestModelsServiceIsConstructedOnceAndOpensRuntimeScopeOnSameRoot(t *testing.T) {
	t.Parallel()

	root, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService: %v", err)
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{}); err == nil {
		t.Fatal("unbound Models service unexpectedly accepted a catalog operation")
	}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: t.TempDir(),
			Runtime:        models.RuntimeConfig{},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntimeScope returned a zero scope")
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{
		Scope: opened.Scope,
	}); err != nil {
		t.Fatalf("same process-scoped Models root rejected its opened scope: %v", err)
	}
	closed, err := root.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if !closed.Closed || closed.Scope != opened.Scope {
		t.Fatalf("CloseRuntimeScope result = %#v, want issued scope closed", closed)
	}
}

type workingDirectoryOverride struct{}

func (*workingDirectoryOverride) Getwd() (string, error) { return "override", nil }

type artifactWriteCloser struct{ bytes.Buffer }

func (*artifactWriteCloser) Close() error { return nil }

type invocationArtifactFileSystemOverride struct {
	opened  string
	created string
	output  *artifactWriteCloser
}

type portableFileSystemOverride struct {
	platformfilesystem.Local
	walked bool
}

func (f *portableFileSystemOverride) WalkDir(string, fs.WalkDirFunc) error {
	f.walked = true
	return nil
}

func TestFactoryDefinitionPortableFileSystemPreservesOverrideAndSelectsDefault(t *testing.T) {
	t.Parallel()

	selectedDefault := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{})
	if _, ok := selectedDefault.(platformfilesystem.Local); !ok {
		t.Fatalf("default portable filesystem = %T, want platform local adapter", selectedDefault)
	}
	if err := selectedDefault.WalkDir(t.TempDir(), func(string, fs.DirEntry, error) error { return nil }); err != nil {
		t.Fatalf("default portable directory walker: %v", err)
	}

	override := &portableFileSystemOverride{}
	selected := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{
		FactoryDefinitionPortableFileSystem: override,
	})
	if selected != override {
		t.Fatal("portable filesystem override was not selected")
	}
	if err := selected.WalkDir("unused", nil); err != nil {
		t.Fatalf("portable directory walker override: %v", err)
	}
	if !override.walked {
		t.Fatal("portable directory walker override was not selected")
	}
}

func (s *invocationArtifactFileSystemOverride) Open(path string) (io.ReadCloser, error) {
	s.opened = path
	return io.NopCloser(bytes.NewBufferString("audio")), nil
}

func (s *invocationArtifactFileSystemOverride) Create(path string) (io.WriteCloser, error) {
	s.created = path
	s.output = &artifactWriteCloser{}
	return s.output, nil
}

func TestModelInvocationEdgesPreserveOverridesAndSelectPlatformDefaults(t *testing.T) {
	t.Parallel()

	if _, ok := provideFactorySessionsWorkingDirectory(serviceedges.Edges{}).(platformfilesystem.Local); !ok {
		t.Fatalf("default working-directory edge = %T, want platform filesystem adapter", provideFactorySessionsWorkingDirectory(serviceedges.Edges{}))
	}
	workingOverride := &workingDirectoryOverride{}
	if got := provideFactorySessionsWorkingDirectory(serviceedges.Edges{FactorySessionsWorkingDirectory: workingOverride}); got != workingOverride {
		t.Fatalf("working-directory override = %#v, want original override", got)
	}

	filesystemOverride := &invocationArtifactFileSystemOverride{}
	exporter, err := provideModelInvocationArtifactExporter(serviceedges.Edges{
		ModelInvocationArtifactFileSystem: filesystemOverride,
	})
	if err != nil {
		t.Fatalf("provideModelInvocationArtifactExporter: %v", err)
	}
	if err := exporter.ExportInvocationArtifact("runtime.wav", "customer.wav"); err != nil {
		t.Fatalf("ExportInvocationArtifact: %v", err)
	}
	if filesystemOverride.opened != "runtime.wav" || filesystemOverride.created != "customer.wav" || filesystemOverride.output.String() != "audio" {
		t.Fatalf("artifact override observed (%q, %q, %q)", filesystemOverride.opened, filesystemOverride.created, filesystemOverride.output.String())
	}
	if got := provideModelInvocationTimeout(); got != factorysessions.DefaultModelInvocationTimeout {
		t.Fatalf("model invocation timeout = %v, want %v", got, factorysessions.DefaultModelInvocationTimeout)
	}
}

func TestModelAssetHostPlatformPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	if got := provideModelAssetHostPlatform(serviceedges.Edges{}); got != (models.AssetHostPlatform{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
	}) {
		t.Fatalf("default model asset host platform = %#v, want current process platform", got)
	}

	override := models.AssetHostPlatform{OperatingSystem: "customer-os", Architecture: "customer-arch"}
	if got := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: override}); got != override {
		t.Fatalf("model asset host platform override = %#v, want %#v", got, override)
	}
}

func TestModelsCompositionAdaptsEdgePortsAtTheWireBoundary(t *testing.T) {
	t.Parallel()

	process := &modelEdgeManagedProcess{healthEndpoint: "http://model-host/health"}
	var gotSpec serviceedges.HostProcessStartSpec
	launcher := adaptModelHostProcessLauncher(&modelEdgeProcessLauncher{
		process: process,
		gotSpec: &gotSpec,
	})
	gotProcess, err := launcher.Start(context.Background(), modelswire.HostProcessStartSpec{
		Command: "model-host", Args: []string{"serve"}, Env: []string{"MODEL=seal"},
		WorkDir: "runtime", HealthEndpoint: process.healthEndpoint,
	})
	if err != nil {
		t.Fatalf("adapted process launcher: %v", err)
	}
	if gotSpec.Command != "model-host" || len(gotSpec.Args) != 1 || gotSpec.Args[0] != "serve" ||
		len(gotSpec.Env) != 1 || gotSpec.Env[0] != "MODEL=seal" || gotSpec.WorkDir != "runtime" ||
		gotSpec.HealthEndpoint != process.healthEndpoint {
		t.Fatalf("adapted process spec = %#v, want exact edge projection", gotSpec)
	}
	if gotProcess.HealthEndpoint() != process.healthEndpoint {
		t.Fatalf("adapted process health endpoint = %q, want %q", gotProcess.HealthEndpoint(), process.healthEndpoint)
	}
	if err := gotProcess.Stop(context.Background()); err != nil {
		t.Fatalf("adapted managed process Stop: %v", err)
	}
	if !process.stopped {
		t.Fatal("adapted managed process did not preserve the edge process")
	}

	timer := &modelEdgeTimer{}
	clock := adaptModelHostClock(modelEdgeClock{timer: timer})
	if got := clock.Now(); !got.Equal(modelEdgeClockTime) {
		t.Fatalf("adapted host clock Now = %v, want %v", got, modelEdgeClockTime)
	}
	if got := clock.NewTimer(time.Second); got != timer {
		t.Fatal("adapted host clock did not preserve the edge timer")
	}

	tempFile := &modelEdgeTempFile{name: "runtime.tmp"}
	createTempFile := adaptModelRuntimeTempFile(func(string, string) (interface {
		Close() error
		Name() string
	}, error) {
		return tempFile, nil
	})
	gotTempFile, err := createTempFile("runtime", "model-*")
	if err != nil {
		t.Fatalf("adapted runtime temp file: %v", err)
	}
	if gotTempFile.Name() != tempFile.name {
		t.Fatalf("adapted temp file name = %q, want %q", gotTempFile.Name(), tempFile.name)
	}

	labels := map[string]string{"model": "seal"}
	recorder := &modelEdgePullMetricsRecorder{}
	adaptedRecorder := adaptModelsPullMetricsRecorder(recorder)
	adaptedRecorder.RecordModelPullMetric(modelswire.PullMetric{Name: "model.pull", Labels: labels})
	labels["model"] = "mutated-after-record"
	if recorder.metric.Name != "model.pull" || recorder.metric.Labels["model"] != "seal" {
		t.Fatalf("adapted pull metric = %#v, want copied edge metric", recorder.metric)
	}
}

func TestModelsCompositionRejectsTypedNilHostEdges(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		edges serviceedges.Edges
		want  string
	}{
		{
			name: "process launcher",
			edges: serviceedges.Edges{
				ModelHostProcessLauncher: (*modelEdgeProcessLauncher)(nil),
			},
			want: "model host process launcher is required",
		},
		{
			name: "clock",
			edges: serviceedges.Edges{
				ModelHostClock: (*modelEdgeClock)(nil),
			},
			want: "model host clock is required",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := provideModelsService(testCase.edges)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("provideModelsService() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

var modelEdgeClockTime = time.Unix(1_725_000_000, 0)

type modelEdgeManagedProcess struct {
	healthEndpoint string
	stopped        bool
}

func (process *modelEdgeManagedProcess) HealthEndpoint() string { return process.healthEndpoint }
func (*modelEdgeManagedProcess) Wait() error                    { return nil }
func (process *modelEdgeManagedProcess) Stop(context.Context) error {
	process.stopped = true
	return nil
}

type modelEdgeProcessLauncher struct {
	process *modelEdgeManagedProcess
	gotSpec *serviceedges.HostProcessStartSpec
}

func (launcher *modelEdgeProcessLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	*launcher.gotSpec = spec
	return launcher.process, nil
}

type modelEdgeTimer struct{}

func (*modelEdgeTimer) C() <-chan time.Time { return nil }
func (*modelEdgeTimer) Stop() bool          { return true }

type modelEdgeClock struct {
	timer interface {
		C() <-chan time.Time
		Stop() bool
	}
}

func (modelEdgeClock) Now() time.Time { return modelEdgeClockTime }
func (clock modelEdgeClock) NewTimer(time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	return clock.timer
}

type modelEdgeTempFile struct {
	name string
}

func (file *modelEdgeTempFile) Close() error { return nil }
func (file *modelEdgeTempFile) Name() string { return file.name }

type modelEdgePullMetricsRecorder struct {
	metric serviceedges.PullMetric
}

func (recorder *modelEdgePullMetricsRecorder) RecordModelPullMetric(metric serviceedges.PullMetric) {
	recorder.metric = metric
}
