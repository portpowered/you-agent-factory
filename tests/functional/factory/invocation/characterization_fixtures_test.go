package oneshot_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

type localAIFactoryAssetHTTP struct {
	mu    sync.Mutex
	calls int
}

func (client *localAIFactoryAssetHTTP) Do(*http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.calls++
	client.mu.Unlock()
	return nil, errors.New("unexpected Factory LocalAI model network request")
}

func (client *localAIFactoryAssetHTTP) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

type localAIFactoryAssetFileSystem struct{ home string }

func (filesystem localAIFactoryAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (filesystem localAIFactoryAssetFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
func (filesystem localAIFactoryAssetFileSystem) UserHomeDir() (string, error) {
	return filesystem.home, nil
}
func (filesystem localAIFactoryAssetFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (filesystem localAIFactoryAssetFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (filesystem localAIFactoryAssetFileSystem) Remove(path string) error { return os.Remove(path) }
func (filesystem localAIFactoryAssetFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (filesystem localAIFactoryAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (filesystem localAIFactoryAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (filesystem localAIFactoryAssetFileSystem) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

type localAIFactoryRevisionRecorder struct {
	mu       sync.Mutex
	revision string
	sources  []string
}

func (recorder *localAIFactoryRevisionRecorder) Resolve(_ context.Context, source string) (string, error) {
	recorder.mu.Lock()
	recorder.sources = append(recorder.sources, source)
	revision := recorder.revision
	recorder.mu.Unlock()
	return revision, nil
}

func (recorder *localAIFactoryRevisionRecorder) Sources() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]string(nil), recorder.sources...)
}

type localAIFactoryBackendSelectionRecorder struct {
	mu        sync.Mutex
	selection serviceedges.ModelBackendArtifactSelection
	requests  []serviceedges.ModelBackendArtifactSelectionRequest
}

func (recorder *localAIFactoryBackendSelectionRecorder) Resolve(_ context.Context, request serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, request)
	selection := recorder.selection
	recorder.mu.Unlock()
	return selection, nil
}

func (recorder *localAIFactoryBackendSelectionRecorder) Requests() []serviceedges.ModelBackendArtifactSelectionRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]serviceedges.ModelBackendArtifactSelectionRequest(nil), recorder.requests...)
}

type localAIFactoryHostLauncher struct {
	mu              sync.Mutex
	endpoint        string
	specs           []serviceedges.HostProcessStartSpec
	active          int
	starts          int
	stops           int
	waits           int
	waitDone        chan struct{}
	waitRelease     chan struct{}
	releaseWaitOnce sync.Once
}

func (launcher *localAIFactoryHostLauncher) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	waitDone := make(chan struct{})
	waitRelease := make(chan struct{})
	launcher.mu.Lock()
	launcher.specs = append(launcher.specs, cloneLocalAIFactoryHostSpec(spec))
	launcher.starts++
	launcher.active++
	launcher.waitDone = waitDone
	launcher.waitRelease = waitRelease
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &localAIFactoryHostProcess{
		endpoint:    endpoint,
		stopped:     make(chan struct{}),
		waitDone:    waitDone,
		waitRelease: waitRelease,
		launcher:    launcher,
	}, nil
}

func (launcher *localAIFactoryHostLauncher) recordStop() {
	launcher.mu.Lock()
	launcher.stops++
	launcher.active--
	launcher.mu.Unlock()
}

func (launcher *localAIFactoryHostLauncher) recordWait() {
	launcher.mu.Lock()
	launcher.waits++
	launcher.mu.Unlock()
}

func (launcher *localAIFactoryHostLauncher) releaseWait() {
	launcher.mu.Lock()
	waitRelease := launcher.waitRelease
	launcher.mu.Unlock()
	if waitRelease == nil {
		return
	}
	launcher.releaseWaitOnce.Do(func() { close(waitRelease) })
}

func (launcher *localAIFactoryHostLauncher) awaitWait(ctx context.Context) error {
	launcher.mu.Lock()
	waitDone := launcher.waitDone
	launcher.mu.Unlock()
	if waitDone == nil {
		return errors.New("Factory LocalAI host was not started")
	}
	select {
	case <-waitDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (launcher *localAIFactoryHostLauncher) LastSpec() (serviceedges.HostProcessStartSpec, bool) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if len(launcher.specs) == 0 {
		return serviceedges.HostProcessStartSpec{}, false
	}
	return cloneLocalAIFactoryHostSpec(launcher.specs[len(launcher.specs)-1]), true
}

func (launcher *localAIFactoryHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active != 0
}
func (launcher *localAIFactoryHostLauncher) Starts() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}
func (launcher *localAIFactoryHostLauncher) Stops() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}
func (launcher *localAIFactoryHostLauncher) Waits() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.waits
}

type localAIFactoryHostProcess struct {
	endpoint    string
	stopped     chan struct{}
	waitDone    chan struct{}
	waitRelease chan struct{}
	launcher    *localAIFactoryHostLauncher
	once        sync.Once
	waitOnce    sync.Once
}

func (process *localAIFactoryHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *localAIFactoryHostProcess) Wait() error {
	<-process.stopped
	<-process.waitRelease
	process.waitOnce.Do(func() {
		process.launcher.recordWait()
		close(process.waitDone)
	})
	return nil
}
func (process *localAIFactoryHostProcess) Stop(context.Context) error {
	process.once.Do(func() { close(process.stopped); process.launcher.recordStop() })
	return nil
}

func cloneLocalAIFactoryHostSpec(spec serviceedges.HostProcessStartSpec) serviceedges.HostProcessStartSpec {
	spec.Args = append([]string(nil), spec.Args...)
	spec.Env = append([]string(nil), spec.Env...)
	spec.ModelFiles = append([]string(nil), spec.ModelFiles...)
	spec.BackendFiles = append([]string(nil), spec.BackendFiles...)
	return spec
}

type localAIFactoryHostProtocolRecorder struct {
	mu       sync.Mutex
	requests []serviceedges.ModelHostProtocolNegotiationRequest
}

func (recorder *localAIFactoryHostProtocolRecorder) Negotiate(_ context.Context, _ string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	recorder.mu.Lock()
	request.ModelFiles = append([]string(nil), request.ModelFiles...)
	recorder.requests = append(recorder.requests, request)
	recorder.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: true}, nil
}

func (recorder *localAIFactoryHostProtocolRecorder) Requests() []serviceedges.ModelHostProtocolNegotiationRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	requests := make([]serviceedges.ModelHostProtocolNegotiationRequest, len(recorder.requests))
	for index, request := range recorder.requests {
		request.ModelFiles = append([]string(nil), request.ModelFiles...)
		requests[index] = request
	}
	return requests
}

type localAIFactoryHostCompatibilityRecorder struct {
	mu       sync.Mutex
	requests []serviceedges.ModelHostCompatibilityRequest
}

func (recorder *localAIFactoryHostCompatibilityRecorder) Check(_ context.Context, request serviceedges.ModelHostCompatibilityRequest) error {
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, request)
	recorder.mu.Unlock()
	return nil
}

func (recorder *localAIFactoryHostCompatibilityRecorder) Requests() []serviceedges.ModelHostCompatibilityRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]serviceedges.ModelHostCompatibilityRequest(nil), recorder.requests...)
}

type localAIFactoryInvocationProtocolRecorder struct {
	mu       sync.Mutex
	response string
	failure  error
	request  modelprovider.InvocationProtocolRequest
	calls    int
}

func (recorder *localAIFactoryInvocationProtocolRecorder) Predict(ctx context.Context, request modelprovider.InvocationProtocolRequest) (modelprovider.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return modelprovider.InvocationProtocolResponse{}, err
	}
	recorder.mu.Lock()
	recorder.request = cloneLocalAIFactoryInvocationProtocolRequest(request)
	recorder.calls++
	response, failure := recorder.response, recorder.failure
	recorder.mu.Unlock()
	if failure != nil {
		return modelprovider.InvocationProtocolResponse{}, failure
	}
	return modelprovider.InvocationProtocolResponse{Text: response}, nil
}

func (recorder *localAIFactoryInvocationProtocolRecorder) Calls() int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.calls
}
func (recorder *localAIFactoryInvocationProtocolRecorder) Request() modelprovider.InvocationProtocolRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return cloneLocalAIFactoryInvocationProtocolRequest(recorder.request)
}

func cloneLocalAIFactoryInvocationProtocolRequest(request modelprovider.InvocationProtocolRequest) modelprovider.InvocationProtocolRequest {
	request.Inputs = append([]modelprovider.InvocationProtocolInput(nil), request.Inputs...)
	request.Parameters = append([]modelprovider.OperationParameter(nil), request.Parameters...)
	return request
}
