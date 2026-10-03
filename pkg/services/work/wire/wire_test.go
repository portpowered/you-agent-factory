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
	contentmaterializationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/wire"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
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

func TestNewRuntimeServiceConstructsPublishedRootFromWiredCollaborators(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs(t)
	staging, err := workwire.NewContentStagingService(
		inputs.filesystem,
		inputs.random,
		inputs.clock,
		0,
	)
	if err != nil {
		t.Fatalf("NewContentStagingService() error = %v", err)
	}
	materializer, err := workwire.NewContentMaterializationService(
		inputs.hostPlatform,
		inputs.httpDoer,
		inputs.inspectPath,
		inputs.createTempFile,
		inputs.removePath,
		inputs.writeFile,
		inputs.openFile,
	)
	if err != nil {
		t.Fatalf("NewContentMaterializationService() error = %v", err)
	}

	service := testRuntimeService(
		inputs.runtimes,
		os.ReadFile,
		nil,
		staging,
		materializer,
	)
	if service == nil {
		t.Fatal("NewRuntimeService() returned nil service")
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
	if err.Error() != "Work state access snapshot reader is required" {
		t.Fatalf("ListWork() error = %v, want recordings adapter required", err)
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

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	staging, err := workwire.NewContentStagingService(in.filesystem, in.random, in.clock, in.stagingTTL)
	if err != nil {
		return nil, err
	}
	materializer, err := workwire.NewContentMaterializationService(in.hostPlatform, in.httpDoer, in.inspectPath, in.createTempFile, in.removePath, in.writeFile, in.openFile)
	if err != nil {
		return nil, err
	}
	return testRuntimeService(in.runtimes, os.ReadFile, os.Stat, staging, materializer), nil
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

func testRuntimeService(runtimes work.RuntimeResolver, readFile work.SubmittedFileReader, inspectPath work.SubmittedFilePathInspector, staging work.ContentStagingService, materializer work.ContentMaterializer) work.Service {
	if staging == nil {
		staging = admissionOnlyContentStaging{}
	}
	if materializer == nil {
		materializer = admissionOnlyContentMaterializer{}
	}
	content := workwire.NewContentPreparation(workwire.NewContentPolicy())
	prep := workwire.NewRequestPreparationService(workwire.NewRequestPolicy(workwire.NewRequestContentBridge(content)))
	input := workwire.NewInvocationInputAdapter(workwire.NewInvocationInputPolicy(readFile, inspectPath))
	state := workwire.NewStateAccess(workwire.NewRuntimeSessionResolver(runtimes), nil, fixtureDurability{})
	return workwire.NewRuntimeService(runtimes, readFile, inspectPath, staging, materializer, state, prep, input)
}

type fixtureDurability struct{}

func (fixtureDurability) CompletedFlushSequence(string) (int64, bool) { return 0, false }

// admissionOnlyContentStaging keeps unsupported content operations explicit.
type admissionOnlyContentStaging struct{}

func (admissionOnlyContentStaging) StageContent(context.Context, work.StageContentRequest) (work.StageContentResult, error) {
	return work.StageContentResult{}, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) PrepareContent(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
	return nil, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) ResolveContent(context.Context, string) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) CleanupContent(context.Context, string) error {
	return errors.New("Work content staging is required")
}

// admissionOnlyContentMaterializer keeps unsupported materialization explicit.
type admissionOnlyContentMaterializer struct{}

func (admissionOnlyContentMaterializer) MaterializeContentURL(context.Context, string) (string, work.ContentCleanup, error) {
	return "", nil, errors.New("Work content materializer is required")
}
