package wire

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

const (
	modelAssetHTTPTimeout   = 45 * time.Second
	modelHostHTTPTimeout    = 2 * time.Second
	modelRuntimeHTTPTimeout = 5 * time.Minute
)

// TODO: this should be decomposed, we should inject these independently.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func provideModelsService(edges serviceedges.Edges) (models.Service, error) {
	assetPlatform := provideModelAssetHostPlatform(edges)
	assetEndpoints := edges.ModelAssetEndpoints

	assetHTTP := edges.ModelAssetHTTPClient
	if assetHTTP == nil {
		assetHTTP = &http.Client{Timeout: modelAssetHTTPTimeout}
	}
	assetMkdirAll := edges.ModelAssetMakeDirectories
	if assetMkdirAll == nil {
		assetMkdirAll = os.MkdirAll
	}
	assetStat := edges.ModelAssetInspectPath
	if assetStat == nil {
		assetStat = os.Stat
	}
	assetHome := edges.ModelAssetResolveHomeDirectory
	if assetHome == nil {
		assetHome = os.UserHomeDir
	}
	assetWriteFile := edges.ModelAssetWriteFile
	if assetWriteFile == nil {
		assetWriteFile = os.WriteFile
	}
	assetRename := edges.ModelAssetRenamePath
	if assetRename == nil {
		assetRename = os.Rename
	}
	assetRemove := edges.ModelAssetRemovePath
	if assetRemove == nil {
		assetRemove = os.Remove
	}
	assetReadFile := edges.ModelAssetReadFile
	if assetReadFile == nil {
		assetReadFile = os.ReadFile
	}
	assetReadDir := edges.ModelAssetReadDirectory
	if assetReadDir == nil {
		assetReadDir = os.ReadDir
	}
	assetCreate := edges.ModelAssetCreateFile
	if assetCreate == nil {
		assetCreate = func(path string) (io.WriteCloser, error) { return os.Create(path) }
	}
	assetOpen := edges.ModelAssetOpenFile
	if assetOpen == nil {
		assetOpen = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	}

	launcher := edges.ModelHostProcessLauncher
	if launcher == nil {
		launcher = modelsProcessLauncher{}
	}
	hostHTTP := edges.ModelHostHTTPClient
	if hostHTTP == nil {
		hostHTTP = &http.Client{Timeout: modelHostHTTPTimeout}
	}
	hostClock := edges.ModelHostClock
	if hostClock == nil {
		hostClock = modelsClock{}
	}
	runtimeRunner := edges.ModelRuntimeCommandRunner
	if runtimeRunner == nil {
		var runnerErr error
		runtimeRunner, runnerErr = providePlatformProcessCommandRunner(edges)
		if runnerErr != nil {
			return nil, runnerErr
		}
	}
	runtimeHTTP := edges.ModelRuntimeHTTPClient
	if runtimeHTTP == nil {
		runtimeHTTP = &http.Client{Timeout: modelRuntimeHTTPTimeout}
	}
	runtimeInspect := edges.ModelRuntimeInspectFile
	if runtimeInspect == nil {
		runtimeInspect = os.Stat
	}
	runtimeTempDir := edges.ModelRuntimeTempDirectory
	if runtimeTempDir == nil {
		runtimeTempDir = os.TempDir
	}
	runtimeTempFile := edges.ModelRuntimeCreateTempFile
	if runtimeTempFile == nil {
		runtimeTempFile = func(dir, pattern string) (interface {
			Close() error
			Name() string
		}, error) {
			return os.CreateTemp(dir, pattern)
		}
	}

	return modelswire.NewService(
		assetPlatform,
		assetHTTP,
		assetEndpoints,
		modelswire.AssetMakeDirectories(assetMkdirAll),
		modelswire.AssetInspectPath(assetStat),
		modelswire.AssetResolveHomeDirectory(assetHome),
		modelswire.AssetWriteFile(assetWriteFile),
		modelswire.AssetRenamePath(assetRename),
		modelswire.AssetRemovePath(assetRemove),
		modelswire.AssetReadFile(assetReadFile),
		modelswire.AssetReadDirectory(assetReadDir),
		modelswire.AssetCreateFile(assetCreate),
		modelswire.AssetOpenFile(assetOpen),
		adaptModelHostProcessLauncher(launcher),
		hostHTTP,
		adaptModelHostClock(hostClock),
		runtimeRunner,
		runtimeHTTP,
		modelswire.RuntimeInspectFile(runtimeInspect),
		modelswire.RuntimeTempDirectory(runtimeTempDir),
		adaptModelRuntimeTempFile(runtimeTempFile),
		zap.NewNop(),
		time.Now,
		platformrandom.CryptoSource{},
		adaptModelsPullMetricsRecorder(edges.ModelPullMetricsRecorder),
		modelswire.HostDiagnosticLogger(factorysessionwire.ModelHostDiagnosticLogger(zap.NewNop())),
		modelswire.HostMetricsRecorder(factorysessionwire.ModelHostDiagnosticMetrics(edges.InvocationMetricsRecorder)),
		modelLocalRuntimeHooks(workerswire.LocalRuntimeHooks()),
	)
}

func providePlatformProcessCommandRunner(edges serviceedges.Edges) (platformprocess.CommandRunner, error) {
	clock := edges.PlatformProcessClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	newCommand := edges.PlatformProcessCommandFactory
	if newCommand == nil {
		newCommand = exec.Command
	}
	runner, err := platformprocess.NewExecCommandRunner(newCommand, clock, nil)
	if err != nil {
		return nil, err
	}
	return runner, nil
}

func provideModelAssetHostPlatform(edges serviceedges.Edges) models.AssetHostPlatform {
	platform := edges.ModelAssetHostPlatform
	if strings.TrimSpace(platform.OperatingSystem) == "" {
		platform.OperatingSystem = runtime.GOOS
	}
	if strings.TrimSpace(platform.Architecture) == "" {
		platform.Architecture = runtime.GOARCH
	}
	return platform
}

type modelsClock struct{}

func (modelsClock) Now() time.Time { return time.Now() }

func (modelsClock) NewTimer(duration time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	return modelsTimer{Timer: time.NewTimer(duration)}
}

type modelsTimer struct{ *time.Timer }

func (timer modelsTimer) C() <-chan time.Time { return timer.Timer.C }

type modelsProcessLauncher struct{}

func (modelsProcessLauncher) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return nil, fmt.Errorf("supervised process command is required")
	}
	endpoint := strings.TrimSpace(spec.HealthEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("supervised process health endpoint is required")
	}
	cmd := exec.Command(command, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append([]string(nil), spec.Env...)
	}
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return &modelsManagedProcess{cmd: cmd, healthEndpoint: endpoint, done: done}, nil
}

type modelHostProcessLauncherAdapter struct {
	next interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	}
}

func adaptModelHostProcessLauncher(next interface {
	Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}, error)
}) modelswire.HostProcessLauncher {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostProcessLauncherAdapter{next: next}
}

func (adapter modelHostProcessLauncherAdapter) Start(
	ctx context.Context,
	spec modelswire.HostProcessStartSpec,
) (modelswire.HostManagedProcess, error) {
	process, err := adapter.next.Start(ctx, serviceedges.HostProcessStartSpec{
		Command:        spec.Command,
		Args:           spec.Args,
		Env:            spec.Env,
		WorkDir:        spec.WorkDir,
		HealthEndpoint: spec.HealthEndpoint,
	})
	if err != nil || process == nil {
		return modelswire.HostManagedProcess(process), err
	}
	return modelswire.HostManagedProcess(process), nil
}

type modelHostClockAdapter struct {
	next interface {
		Now() time.Time
		NewTimer(time.Duration) interface {
			C() <-chan time.Time
			Stop() bool
		}
	}
}

func adaptModelHostClock(next interface {
	Now() time.Time
	NewTimer(time.Duration) interface {
		C() <-chan time.Time
		Stop() bool
	}
}) modelswire.HostClock {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostClockAdapter{next: next}
}

func (adapter modelHostClockAdapter) Now() time.Time { return adapter.next.Now() }

func (adapter modelHostClockAdapter) NewTimer(duration time.Duration) modelswire.HostTimer {
	return modelswire.HostTimer(adapter.next.NewTimer(duration))
}

func adaptModelRuntimeTempFile(next serviceedges.RuntimeCreateTempFile) modelswire.RuntimeCreateTempFile {
	if next == nil {
		return nil
	}
	return func(dir, pattern string) (modelswire.RuntimeTempFile, error) {
		file, err := next(dir, pattern)
		return modelswire.RuntimeTempFile(file), err
	}
}

type modelsPullMetricsAdapter struct {
	next interface {
		RecordModelPullMetric(serviceedges.PullMetric)
	}
}

func adaptModelsPullMetricsRecorder(next interface {
	RecordModelPullMetric(serviceedges.PullMetric)
}) modelswire.PullMetricsRecorder {
	if next == nil {
		return nil
	}
	return modelsPullMetricsAdapter{next: next}
}

// isNilModelEdgeDependency preserves Models Wire's typed-nil validation when
// an edge-owned interface is wrapped by a canonical adapter. Without this
// check, a nil pointer would become a non-nil adapter value and fail later at
// invocation rather than during inert construction.
func isNilModelEdgeDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (adapter modelsPullMetricsAdapter) RecordModelPullMetric(metric modelswire.PullMetric) {
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.next.RecordModelPullMetric(serviceedges.PullMetric{
		Name:   metric.Name,
		Labels: labels,
	})
}

func modelLocalRuntimeHooks(hooks workers.LocalRuntimeHooks) modelswire.LocalRuntimeHooks {
	return modelswire.LocalRuntimeHooks{
		MarkResourceWaitStarted:  hooks.MarkResourceWaitStarted,
		MarkResourceWaitFinished: hooks.MarkResourceWaitFinished,
		MarkLoadRequested:        hooks.MarkLoadRequested,
		MarkLoadFinished:         hooks.MarkLoadFinished,
		MarkLoadReused:           hooks.MarkLoadReused,
	}
}

type modelsManagedProcess struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	healthEndpoint string
	done           chan error
	stopped        bool
}

func (p *modelsManagedProcess) HealthEndpoint() string { return p.healthEndpoint }

func (p *modelsManagedProcess) Wait() error {
	if p == nil || p.done == nil {
		return nil
	}
	return <-p.done
}

func (p *modelsManagedProcess) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.stopped = true
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
