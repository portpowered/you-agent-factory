package wire

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
		runtimeTempFile = func(dir, pattern string) (models.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		}
	}

	return modelswire.NewService(
		assetPlatform,
		assetHTTP,
		assetEndpoints,
		assetMkdirAll,
		assetStat,
		assetHome,
		assetWriteFile,
		assetRename,
		assetRemove,
		assetReadFile,
		assetReadDir,
		assetCreate,
		assetOpen,
		launcher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect,
		runtimeTempDir,
		runtimeTempFile,
		zap.NewNop(),
		time.Now,
		platformrandom.CryptoSource{},
		edges.ModelPullMetricsRecorder,
		factorysessionwire.ModelHostDiagnosticLogger(zap.NewNop()),
		factorysessionwire.ModelHostDiagnosticMetrics(edges.InvocationMetricsRecorder),
		workerswire.LocalRuntimeHooks(),
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

func (modelsClock) NewTimer(duration time.Duration) models.HostTimer {
	return modelsTimer{Timer: time.NewTimer(duration)}
}

type modelsTimer struct{ *time.Timer }

func (timer modelsTimer) C() <-chan time.Time { return timer.Timer.C }

type modelsProcessLauncher struct{}

func (modelsProcessLauncher) Start(ctx context.Context, spec models.HostProcessStartSpec) (models.HostManagedProcess, error) {
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
