package root_composition_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func assertLocalAIDirectHostReleased(t *testing.T, launcher *localAIHostLauncher) {
	t.Helper()
	waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := launcher.awaitWait(waitContext); err != nil {
		t.Fatalf("wait for LocalAI host Wait completion: %v", err)
	}
	if launcher.Active() || launcher.Starts() != launcher.Stops() || launcher.Stops() != launcher.Waits() {
		t.Fatalf("LocalAI host lifecycle = starts:%d stops:%d waits:%d active:%t, want fully released", launcher.Starts(), launcher.Stops(), launcher.Waits(), launcher.Active())
	}
}

func pathWithinLocalAIRoot(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

type localAIRevisionRecorder struct {
	mu       sync.Mutex
	revision string
	sources  []string
}

func (recorder *localAIRevisionRecorder) Resolve(_ context.Context, source string) (string, error) {
	recorder.mu.Lock()
	recorder.sources = append(recorder.sources, source)
	revision := recorder.revision
	recorder.mu.Unlock()
	return revision, nil
}

func (recorder *localAIRevisionRecorder) Sources() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]string(nil), recorder.sources...)
}

type localAIBackendSelectionRecorder struct {
	mu        sync.Mutex
	selection serviceedges.ModelBackendArtifactSelection
	requests  []serviceedges.ModelBackendArtifactSelectionRequest
}

func (recorder *localAIBackendSelectionRecorder) Resolve(
	_ context.Context,
	request serviceedges.ModelBackendArtifactSelectionRequest,
) (serviceedges.ModelBackendArtifactSelection, error) {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, request)
	selection := recorder.selection
	recorder.mu.Unlock()
	return selection, nil
}

func (recorder *localAIBackendSelectionRecorder) Requests() []serviceedges.ModelBackendArtifactSelectionRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]serviceedges.ModelBackendArtifactSelectionRequest(nil), recorder.requests...)
}

type localAIHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	specs    []serviceedges.HostProcessStartSpec
	active   int
	starts   int
	stops    int
	waits    int
	waitDone <-chan struct{}
}

func (launcher *localAIHostLauncher) Start(
	_ context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	waitDone := make(chan struct{})
	launcher.mu.Lock()
	launcher.specs = append(launcher.specs, cloneLocalAIHostSpec(spec))
	launcher.starts++
	launcher.active++
	launcher.waitDone = waitDone
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &localAIHostProcess{
		endpoint: endpoint,
		stopped:  make(chan struct{}),
		waitDone: waitDone,
		launcher: launcher,
	}, nil
}

func (launcher *localAIHostLauncher) recordStop() {
	launcher.mu.Lock()
	launcher.stops++
	launcher.active--
	launcher.mu.Unlock()
}

func (launcher *localAIHostLauncher) recordWait() {
	launcher.mu.Lock()
	launcher.waits++
	launcher.mu.Unlock()
}

func (launcher *localAIHostLauncher) awaitWait(ctx context.Context) error {
	launcher.mu.Lock()
	waitDone := launcher.waitDone
	launcher.mu.Unlock()
	if waitDone == nil {
		return errors.New("LocalAI host was not started")
	}
	select {
	case <-waitDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (launcher *localAIHostLauncher) LastSpec() (serviceedges.HostProcessStartSpec, bool) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if len(launcher.specs) == 0 {
		return serviceedges.HostProcessStartSpec{}, false
	}
	return cloneLocalAIHostSpec(launcher.specs[len(launcher.specs)-1]), true
}

func (launcher *localAIHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active != 0
}

func (launcher *localAIHostLauncher) Starts() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *localAIHostLauncher) Stops() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

func (launcher *localAIHostLauncher) Waits() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.waits
}

func (launcher *localAIHostLauncher) ModelPath() string {
	spec, _ := launcher.LastSpec()
	return spec.ModelPath
}

type localAIHostProcess struct {
	endpoint string
	stopped  chan struct{}
	waitDone chan struct{}
	launcher *localAIHostLauncher
	once     sync.Once
	waitOnce sync.Once
}

func (process *localAIHostProcess) HealthEndpoint() string { return process.endpoint }

func (process *localAIHostProcess) Wait() error {
	<-process.stopped
	process.waitOnce.Do(func() {
		process.launcher.recordWait()
		close(process.waitDone)
	})
	return nil
}

func (process *localAIHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		process.launcher.recordStop()
	})
	return nil
}

func cloneLocalAIHostSpec(spec serviceedges.HostProcessStartSpec) serviceedges.HostProcessStartSpec {
	spec.Args = append([]string(nil), spec.Args...)
	spec.Env = append([]string(nil), spec.Env...)
	spec.ModelFiles = append([]string(nil), spec.ModelFiles...)
	spec.BackendFiles = append([]string(nil), spec.BackendFiles...)
	return spec
}
