package wire_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	contentmaterializationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/wire"
)

func TestDefaultContentMaterializationHTTPTimeoutMatchesNestedWire(t *testing.T) {
	t.Parallel()

	if workwire.DefaultContentMaterializationHTTPTimeout != contentmaterializationwire.DefaultHTTPTimeout {
		t.Fatalf(
			"DefaultContentMaterializationHTTPTimeout = %v, want %v",
			workwire.DefaultContentMaterializationHTTPTimeout,
			contentmaterializationwire.DefaultHTTPTimeout,
		)
	}
}

func TestContentMaterializationRedirectPolicyDelegatesToNestedWire(t *testing.T) {
	t.Parallel()

	policy := workwire.ContentMaterializationRedirectPolicy(2, false)
	if policy == nil {
		t.Fatal("ContentMaterializationRedirectPolicy() = nil")
	}
	if err := policy(&http.Request{URL: mustParseURL(t, "http://example.com/a")}, []*http.Request{
		{URL: mustParseURL(t, "http://example.com/b")},
		{URL: mustParseURL(t, "http://example.com/c")},
	}); err == nil {
		t.Fatal("ContentMaterializationRedirectPolicy() error = nil, want redirect limit")
	}
}

func TestNewContentStagingServiceConstructsPublishedRole(t *testing.T) {
	t.Parallel()

	staging, err := workwire.NewContentStagingService(
		&stubStagingFileSystem{root: t.TempDir()},
		stubStagingRandom{},
		&stubStagingClock{now: time.Unix(0, 0)},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("NewContentStagingService() error = %v", err)
	}
	var role work.ContentStagingService = staging
	if role == nil {
		t.Fatal("constructed value is not assignable to work.ContentStagingService")
	}
}

func TestNewContentMaterializationServiceConstructsPublishedRole(t *testing.T) {
	t.Parallel()

	materializer, err := workwire.NewContentMaterializationService(
		work.ContentHostPlatform(runtime.GOOS),
		validMaterializationHTTPClient(),
		os.Stat,
		validCreateTempFile,
		os.Remove,
		os.WriteFile,
		validOpenFile,
	)
	if err != nil {
		t.Fatalf("NewContentMaterializationService() error = %v", err)
	}
	var role work.ContentMaterializer = materializer
	if role == nil {
		t.Fatal("constructed value is not assignable to work.ContentMaterializer")
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	valid := validNewServiceInputs(t)
	tests := []struct {
		name   string
		mutate func(*newServiceInputs)
		wantErr string
	}{
		{
			name:    "runtime resolver",
			mutate:  func(in *newServiceInputs) { in.runtimes = nil },
			wantErr: "construct Work: runtime resolver is required",
		},
		{
			name:    "content staging filesystem",
			mutate:  func(in *newServiceInputs) { in.filesystem = nil },
			wantErr: "construct Work: content staging filesystem is required",
		},
		{
			name:    "content staging random",
			mutate:  func(in *newServiceInputs) { in.random = nil },
			wantErr: "construct Work: content staging random is required",
		},
		{
			name:    "content staging clock",
			mutate:  func(in *newServiceInputs) { in.clock = nil },
			wantErr: "construct Work: content staging clock is required",
		},
		{
			name:    "content host platform",
			mutate:  func(in *newServiceInputs) { in.hostPlatform = "" },
			wantErr: "construct Work: content host platform is required",
		},
		{
			name:    "HTTP doer",
			mutate:  func(in *newServiceInputs) { in.httpDoer = nil },
			wantErr: "construct Work: HTTP doer is required",
		},
		{
			name:    "inspect path",
			mutate:  func(in *newServiceInputs) { in.inspectPath = nil },
			wantErr: "construct Work: inspect path is required",
		},
		{
			name:    "create temporary file",
			mutate:  func(in *newServiceInputs) { in.createTempFile = nil },
			wantErr: "construct Work: create temporary file is required",
		},
		{
			name:    "remove path",
			mutate:  func(in *newServiceInputs) { in.removePath = nil },
			wantErr: "construct Work: remove path is required",
		},
		{
			name:    "write file",
			mutate:  func(in *newServiceInputs) { in.writeFile = nil },
			wantErr: "construct Work: write file is required",
		},
		{
			name:    "open file",
			mutate:  func(in *newServiceInputs) { in.openFile = nil },
			wantErr: "construct Work: open file is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := valid
			test.mutate(&inputs)
			service, err := inputs.callNewService()
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if err.Error() != test.wantErr {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.wantErr)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServicePropagatesContentStagingConstructionErrors(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs(t)
	inputs.random = failingStagingRandom{}
	service, err := inputs.callNewService()
	if err == nil {
		t.Fatal("NewService() error = nil, want content staging construction failure")
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func TestNewServicePropagatesContentMaterializationConstructionErrors(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs(t)
	inputs.hostPlatform = "   "
	service, err := inputs.callNewService()
	if err == nil {
		t.Fatal("NewService() error = nil, want content host platform rejection")
	}
	if err.Error() != "construct Work: content host platform is required" {
		t.Fatalf("NewService() error = %q, want wire-level host platform rejection", err.Error())
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs(t).callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root work.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to work.Service")
	}
}

func TestNewServiceServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := validNewServiceInputs(t).callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root work.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}

	_, err = root.SubmitWorkRequestForSession(ctx, "session-unresolved", work.WorkRequest{})
	if err == nil {
		t.Fatal("SubmitWorkRequestForSession() error = nil, want unresolved runtime failure")
	}
	if err.Error() != "Work state access session adapter is unavailable" {
		t.Fatalf("SubmitWorkRequestForSession() error = %v, want unavailable session adapter", err)
	}

	_, err = root.ListWork(ctx, "session-unresolved", work.ListOptions{})
	if err == nil {
		t.Fatal("ListWork() error = nil, want unresolved runtime failure")
	}
	if err.Error() != "Work state access session adapter is unavailable" {
		t.Fatalf("ListWork() error = %v, want unavailable session adapter", err)
	}

	_, err = root.MoveWorkForSession(ctx, "session-unresolved", "work-1", "done", "req-1")
	if err == nil {
		t.Fatal("MoveWorkForSession() error = nil, want unresolved runtime failure")
	}
	if err.Error() != "Work state access session adapter is unavailable" {
		t.Fatalf("MoveWorkForSession() error = %v, want unavailable session adapter", err)
	}

	_, err = root.StageContent(ctx, work.StageContentRequest{
		ItemType:  "invalid",
		FileName:  "file.txt",
		MediaType: "text/plain",
		Content:   []byte("data"),
	})
	var stagingErr *work.ContentStagingError
	if !errors.As(err, &stagingErr) {
		t.Fatalf("StageContent() error = %v, want *ContentStagingError", err)
	}

	_, cleanup, err := root.MaterializeContentURL(ctx, "http://127.0.0.1/secret")
	if cleanup != nil {
		cleanup()
	}
	if !errors.Is(err, work.ErrUnsafeContentURL) {
		t.Fatalf("MaterializeContentURL() error = %v, want ErrUnsafeContentURL", err)
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	var (
		runtimeCalls    int
		filesystemCalls int
		clockCalls      int
		httpCalls       int
		inspectCalls    int
		createTempCalls int
		removeCalls     int
		writeCalls      int
		openCalls       int
		randomCalls     int
	)
	inputs := validNewServiceInputs(t)
	inputs.runtimes = inertRuntimeResolver{onResolve: func() {
		runtimeCalls++
		panic("runtime resolved during inert construction")
	}}
	inputs.filesystem = inertStagingFileSystem{onCall: func() {
		filesystemCalls++
		panic("content staging filesystem invoked during inert construction")
	}}
	inputs.random = inertStagingRandom{
		onRead: func() { randomCalls++ },
	}
	inputs.clock = inertStagingClock{onNow: func() {
		clockCalls++
		panic("content staging clock invoked during inert construction")
	}}
	inputs.httpDoer = inertHTTPDoer{onDo: func() {
		httpCalls++
		panic("HTTP doer invoked during inert construction")
	}}
	inputs.inspectPath = func(string) (fs.FileInfo, error) {
		inspectCalls++
		panic("inspect path invoked during inert construction")
	}
	inputs.createTempFile = func(string, string) (work.ContentTemporaryFile, error) {
		createTempCalls++
		panic("create temporary file invoked during inert construction")
	}
	inputs.removePath = func(string) error {
		removeCalls++
		panic("remove path invoked during inert construction")
	}
	inputs.writeFile = func(string, []byte, fs.FileMode) error {
		writeCalls++
		panic("write file invoked during inert construction")
	}
	inputs.openFile = func(string) (io.WriteCloser, error) {
		openCalls++
		panic("open file invoked during inert construction")
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := inputs.callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root work.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to work.Service")
	}
	if runtimeCalls != 0 {
		t.Fatalf("construction resolved runtimes %d times, want inert construction", runtimeCalls)
	}
	if filesystemCalls != 0 {
		t.Fatalf("construction invoked content staging filesystem %d times, want inert construction", filesystemCalls)
	}
	if clockCalls != 0 {
		t.Fatalf("construction read content staging clock %d times, want inert construction", clockCalls)
	}
	if httpCalls != 0 {
		t.Fatalf("construction invoked HTTP doer %d times, want inert construction", httpCalls)
	}
	if inspectCalls != 0 || createTempCalls != 0 || removeCalls != 0 || writeCalls != 0 || openCalls != 0 {
		t.Fatalf(
			"construction invoked materialization effects (inspect=%d create=%d remove=%d write=%d open=%d), want inert construction",
			inspectCalls, createTempCalls, removeCalls, writeCalls, openCalls,
		)
	}
	if randomCalls != 1 {
		t.Fatalf("construction read content staging randomness %d times, want exactly one signing-secret read", randomCalls)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}
}

type newServiceInputs struct {
	runtimes       work.RuntimeResolver
	filesystem     work.ContentStagingFileSystem
	random         work.ContentStagingRandom
	clock          work.ContentStagingClock
	stagingTTL     time.Duration
	hostPlatform   work.ContentHostPlatform
	httpDoer       work.ContentHTTPDoer
	inspectPath    work.ContentInspectPath
	createTempFile work.ContentCreateTemporaryFile
	removePath     work.ContentRemovePath
	writeFile      work.ContentWriteFile
	openFile       work.ContentOpenFile
}

func validNewServiceInputs(t *testing.T) newServiceInputs {
	t.Helper()
	return newServiceInputs{
		runtimes:       stubRuntimeResolver{},
		filesystem:     &stubStagingFileSystem{root: t.TempDir()},
		random:         stubStagingRandom{},
		clock:          &stubStagingClock{now: time.Unix(0, 0)},
		stagingTTL:     time.Minute,
		hostPlatform:   work.ContentHostPlatform(runtime.GOOS),
		httpDoer:       validMaterializationHTTPClient(),
		inspectPath:    os.Stat,
		createTempFile: validCreateTempFile,
		removePath:     os.Remove,
		writeFile:      os.WriteFile,
		openFile:       validOpenFile,
	}
}

func (in newServiceInputs) callNewService() (work.Service, error) {
	return workwire.NewService(
		in.runtimes,
		in.filesystem,
		in.random,
		in.clock,
		in.stagingTTL,
		in.hostPlatform,
		in.httpDoer,
		in.inspectPath,
		in.createTempFile,
		in.removePath,
		in.writeFile,
		in.openFile,
	)
}

type stubRuntimeResolver struct{}

func (stubRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return nil, nil
}

type stubStagingFileSystem struct {
	root string
}

func (f *stubStagingFileSystem) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(f.root, pattern)
}

func (f *stubStagingFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (f *stubStagingFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (f *stubStagingFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

type stubStagingRandom struct{}

func (stubStagingRandom) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = 0x11
	}
	return len(buffer), nil
}

type failingStagingRandom struct{}

func (failingStagingRandom) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

type stubStagingClock struct {
	now time.Time
}

func (c *stubStagingClock) Now() time.Time { return c.now }

func validMaterializationHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       workwire.DefaultContentMaterializationHTTPTimeout,
		CheckRedirect: workwire.ContentMaterializationRedirectPolicy(0, false),
	}
}

func validCreateTempFile(dir, pattern string) (work.ContentTemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func validOpenFile(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed
}

type inertRuntimeResolver struct {
	onResolve func()
}

func (r inertRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	if r.onResolve != nil {
		r.onResolve()
	}
	return nil, nil
}

type inertStagingFileSystem struct {
	onCall func()
}

func (f inertStagingFileSystem) MkdirTemp(string, string) (string, error) {
	if f.onCall != nil {
		f.onCall()
	}
	return "", nil
}

func (f inertStagingFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	if f.onCall != nil {
		f.onCall()
	}
	return nil
}

func (f inertStagingFileSystem) Stat(string) (fs.FileInfo, error) {
	if f.onCall != nil {
		f.onCall()
	}
	return nil, nil
}

func (f inertStagingFileSystem) RemoveAll(string) error {
	if f.onCall != nil {
		f.onCall()
	}
	return nil
}

type inertStagingRandom struct {
	onRead func()
}

func (r inertStagingRandom) Read(buffer []byte) (int, error) {
	if r.onRead != nil {
		r.onRead()
	}
	for i := range buffer {
		buffer[i] = 0x11
	}
	return len(buffer), nil
}

type inertStagingClock struct {
	onNow func()
}

func (c inertStagingClock) Now() time.Time {
	if c.onNow != nil {
		c.onNow()
	}
	return time.Unix(0, 0)
}

type inertHTTPDoer struct {
	onDo func()
}

func (d inertHTTPDoer) Do(*http.Request) (*http.Response, error) {
	if d.onDo != nil {
		d.onDo()
	}
	return nil, nil
}
