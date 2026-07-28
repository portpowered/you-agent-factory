package root_composition_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingModelEffect = errors.New("recording model effect invoked during BuildProcess")

// TestModelsEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Models without invoking catalog, readiness, host,
// assets, or inference external effects before runtime lifecycle starts.
func TestModelsEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newModelEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalCatalog(); got != 0 {
		t.Fatalf("catalog effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalReadiness(); got != 0 {
		t.Fatalf("readiness effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalHost(); got != 0 {
		t.Fatalf("host effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalAssets(); got != 0 {
		t.Fatalf("assets effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalInference(); got != 0 {
		t.Fatalf("inference effect calls = %d during BuildProcess, want 0", got)
	}
}

type modelEffectRecorder struct {
	assetHTTP           atomic.Int32
	assetMkdirAll       atomic.Int32
	assetStat           atomic.Int32
	assetHome           atomic.Int32
	assetWriteFile      atomic.Int32
	assetRename         atomic.Int32
	assetRemove         atomic.Int32
	assetReadFile       atomic.Int32
	assetReadDir        atomic.Int32
	assetCreate         atomic.Int32
	assetOpen           atomic.Int32
	hostLauncher        atomic.Int32
	hostHTTP            atomic.Int32
	hostClockNow        atomic.Int32
	hostClockTimer      atomic.Int32
	runtimeCommand      atomic.Int32
	runtimeHTTP         atomic.Int32
	runtimeInspect      atomic.Int32
	runtimeTempDir      atomic.Int32
	runtimeTempFile     atomic.Int32
	invocationArtifact  atomic.Int32
	pullMetrics         atomic.Int32
}

func newModelEffectRecorder() *modelEffectRecorder {
	return &modelEffectRecorder{}
}

func (recorder *modelEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient:           &recordingModelHTTPDoer{recorder: recorder, counter: &recorder.assetHTTP},
		ModelAssetEndpoints:            models.RuntimeAssetEndpoints{BaseURL: "https://catalog.example", APIBaseURL: "https://api.catalog.example"},
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "recording-os", Architecture: "recording-arch"},
		ModelAssetMakeDirectories:      recorder.recordAssetMkdirAll,
		ModelAssetInspectPath:          recorder.recordAssetStat,
		ModelAssetResolveHomeDirectory: recorder.recordAssetHome,
		ModelAssetWriteFile:            recorder.recordAssetWriteFile,
		ModelAssetRenamePath:           recorder.recordAssetRename,
		ModelAssetRemovePath:           recorder.recordAssetRemove,
		ModelAssetReadFile:             recorder.recordAssetReadFile,
		ModelAssetReadDirectory:        recorder.recordAssetReadDir,
		ModelAssetCreateFile:           recorder.recordAssetCreate,
		ModelAssetOpenFile:             recorder.recordAssetOpen,
		ModelHostProcessLauncher:       recorder,
		ModelHostHTTPClient:            &recordingModelHTTPDoer{recorder: recorder, counter: &recorder.hostHTTP},
		ModelHostClock:                 recorder,
		ModelRuntimeCommandRunner:      recorder,
		ModelRuntimeHTTPClient:         &recordingModelHTTPDoer{recorder: recorder, counter: &recorder.runtimeHTTP},
		ModelRuntimeInspectFile:        recorder.recordRuntimeInspect,
		ModelRuntimeTempDirectory:      recorder.recordRuntimeTempDir,
		ModelRuntimeCreateTempFile:     recorder.recordRuntimeTempFile,
		ModelInvocationArtifactFileSystem: recorder,
		ModelPullMetricsRecorder:       recorder,
	}
}

func (recorder *modelEffectRecorder) totalCatalog() int32 {
	return recorder.assetHTTP.Load()
}

func (recorder *modelEffectRecorder) totalReadiness() int32 {
	return recorder.hostHTTP.Load() + recorder.hostClockNow.Load() + recorder.hostClockTimer.Load()
}

func (recorder *modelEffectRecorder) totalHost() int32 {
	return recorder.hostLauncher.Load() + recorder.hostHTTP.Load()
}

func (recorder *modelEffectRecorder) totalAssets() int32 {
	return recorder.assetHTTP.Load() +
		recorder.assetMkdirAll.Load() +
		recorder.assetStat.Load() +
		recorder.assetHome.Load() +
		recorder.assetWriteFile.Load() +
		recorder.assetRename.Load() +
		recorder.assetRemove.Load() +
		recorder.assetReadFile.Load() +
		recorder.assetReadDir.Load() +
		recorder.assetCreate.Load() +
		recorder.assetOpen.Load()
}

func (recorder *modelEffectRecorder) totalInference() int32 {
	return recorder.runtimeCommand.Load() +
		recorder.runtimeHTTP.Load() +
		recorder.runtimeInspect.Load() +
		recorder.runtimeTempDir.Load() +
		recorder.runtimeTempFile.Load() +
		recorder.invocationArtifact.Load() +
		recorder.pullMetrics.Load()
}

type recordingModelHTTPDoer struct {
	recorder *modelEffectRecorder
	counter  *atomic.Int32
}

func (client *recordingModelHTTPDoer) Do(*http.Request) (*http.Response, error) {
	client.counter.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetMkdirAll(string, os.FileMode) error {
	recorder.assetMkdirAll.Add(1)
	return errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetStat(string) (os.FileInfo, error) {
	recorder.assetStat.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetHome() (string, error) {
	recorder.assetHome.Add(1)
	return "", errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetWriteFile(string, []byte, os.FileMode) error {
	recorder.assetWriteFile.Add(1)
	return errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetRename(string, string) error {
	recorder.assetRename.Add(1)
	return errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetRemove(string) error {
	recorder.assetRemove.Add(1)
	return errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetReadFile(string) ([]byte, error) {
	recorder.assetReadFile.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetReadDir(string) ([]os.DirEntry, error) {
	recorder.assetReadDir.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetCreate(string) (io.WriteCloser, error) {
	recorder.assetCreate.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordAssetOpen(string) (io.ReadCloser, error) {
	recorder.assetOpen.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) Start(context.Context, models.HostProcessStartSpec) (models.HostManagedProcess, error) {
	recorder.hostLauncher.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) Now() time.Time {
	recorder.hostClockNow.Add(1)
	return time.Unix(0, 0).UTC()
}

func (recorder *modelEffectRecorder) NewTimer(time.Duration) models.HostTimer {
	recorder.hostClockTimer.Add(1)
	return recordingModelHostTimer{}
}

type recordingModelHostTimer struct{}

func (recordingModelHostTimer) C() <-chan time.Time { return nil }
func (recordingModelHostTimer) Stop() bool          { return false }

func (recorder *modelEffectRecorder) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	recorder.runtimeCommand.Add(1)
	return platformprocess.CommandResult{}, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordRuntimeInspect(string) (os.FileInfo, error) {
	recorder.runtimeInspect.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) recordRuntimeTempDir() string {
	recorder.runtimeTempDir.Add(1)
	return ""
}

func (recorder *modelEffectRecorder) recordRuntimeTempFile(string, string) (models.RuntimeTempFile, error) {
	recorder.runtimeTempFile.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) Open(string) (io.ReadCloser, error) {
	recorder.invocationArtifact.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) Create(string) (io.WriteCloser, error) {
	recorder.invocationArtifact.Add(1)
	return nil, errRecordingModelEffect
}

func (recorder *modelEffectRecorder) RecordModelPullMetric(models.PullMetric) {
	recorder.pullMetrics.Add(1)
}
