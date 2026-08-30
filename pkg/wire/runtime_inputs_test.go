package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"go.uber.org/zap"
)

type recordingStdioOpening struct {
	request factorysessions.StdioOpeningRequest
	result  factorysessionwire.StdioApplication
}

func (opening *recordingStdioOpening) OpenStdio(
	_ context.Context,
	request factorysessions.StdioOpeningRequest,
) (factorysessionwire.StdioApplication, error) {
	opening.request = request
	return opening.result, nil
}

type testStdioApplication struct{}

func (testStdioApplication) Run(context.Context) error { return nil }

type workerSessionsObservationServiceStub struct {
	workersessions.Service
}

type workerSessionsObservationGatewayStub struct {
	factorysessions.Service
	observation workersessions.ObservationService
}

type metricsSessionProjectionReaderStub struct {
	projection factorysessions.SessionProjection
	err        error
	gotID      string
}

func (stub *metricsSessionProjectionReaderStub) GetFactorySession(
	_ context.Context,
	sessionID string,
) (factorysessions.SessionProjection, error) {
	stub.gotID = sessionID
	return stub.projection, stub.err
}

func TestRuntimeMetricsScopeResolverUsesOnlyCanonicalProjectionIdentity(t *testing.T) {
	reader := &metricsSessionProjectionReaderStub{
		projection: factorysessions.SessionProjection{
			Context: factorysessions.ProjectionContext{
				Session:             &factorysessions.ScopedLiveSessionSummary{IsDefault: true},
				FactorySessionID:    "public-live-id",
				BackendScopeID:      "context-backend-id",
				LogicalSessionKeyID: "context-logical-id",
			},
			Runtime: factorysessions.RuntimeProjection{
				StreamIdentity: &factorysessions.RuntimeStreamIdentity{
					FactorySessionID:    "retained-runtime-id",
					BackendScopeID:      "runtime-backend-id",
					LogicalSessionKeyID: "retained-logical-id",
				},
			},
		},
	}
	resolver := factorysessionwire.NewRuntimeMetricsScopeResolver(reader)
	got, err := resolver.ResolveRuntimeMetricsScope(context.Background(), " public-live-id ")
	if err != nil {
		t.Fatalf("ResolveRuntimeMetricsScope() error = %v", err)
	}
	if reader.gotID != "public-live-id" {
		t.Fatalf("reader selector = %q, want trimmed selector", reader.gotID)
	}
	want := []string{"retained-runtime-id"}
	if got.RequestedFactorySessionID != "public-live-id" {
		t.Fatalf("requested Factory Session ID = %q, want public-live-id", got.RequestedFactorySessionID)
	}
	if !reflect.DeepEqual(got.RetainedFactorySessionIDs, want) {
		t.Fatalf("retained IDs = %#v, want %#v", got.RetainedFactorySessionIDs, want)
	}
}

func TestRuntimeMetricsScopeResolverRetainsOrderedSuccessorLineage(t *testing.T) {
	reader := &metricsSessionProjectionReaderStub{
		projection: factorysessions.SessionProjection{
			Runtime: factorysessions.RuntimeProjection{
				StreamIdentity: &factorysessions.RuntimeStreamIdentity{
					FactorySessionID: "successor-runtime-id",
				},
				RetainedMetricsSessionIDs: []string{
					"successor-runtime-id", "source-runtime-id", "source-runtime-id",
				},
			},
		},
	}
	resolver := factorysessionwire.NewRuntimeMetricsScopeResolver(reader)
	got, err := resolver.ResolveRuntimeMetricsScope(context.Background(), "~default")
	if err != nil {
		t.Fatalf("ResolveRuntimeMetricsScope() error = %v", err)
	}
	want := []string{"successor-runtime-id", "source-runtime-id"}
	if !reflect.DeepEqual(got.RetainedFactorySessionIDs, want) {
		t.Fatalf("retained IDs = %#v, want ordered deduplicated lineage %#v", got.RetainedFactorySessionIDs, want)
	}
}

func TestRuntimeMetricsScopeResolverRejectsDiscoverableProjectionWithoutRetainedIdentity(t *testing.T) {
	resolver := factorysessionwire.NewRuntimeMetricsScopeResolver(&metricsSessionProjectionReaderStub{
		projection: factorysessions.SessionProjection{
			Context: factorysessions.ProjectionContext{
				FactorySessionID: "public-live-id",
			},
		},
	})
	_, err := resolver.ResolveRuntimeMetricsScope(context.Background(), "public-live-id")
	if err == nil || !strings.Contains(err.Error(), "no retained metrics scope") {
		t.Fatalf("ResolveRuntimeMetricsScope() error = %v, want retained-scope failure", err)
	}
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func (stub *workerSessionsObservationGatewayStub) WorkerSessionsObservationForSession(
	string,
) workersessions.ObservationService {
	return stub.observation
}

func TestWorkerSessionsScopeResolverForwardsObservationCapability(t *testing.T) {
	t.Parallel()

	expected := &workerSessionsObservationServiceStub{}
	resolver := newWorkerSessionsFactorySessionScopeResolver(&workerSessionsObservationGatewayStub{
		observation: expected,
	})
	provider, ok := resolver.(interface {
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	})
	if !ok {
		t.Fatal("Worker Sessions scope resolver does not expose the optional observation capability")
	}
	if got := provider.WorkerSessionsObservationForSession("factory-session-1"); got != expected {
		t.Fatalf("forwarded observation service = %T, want %T", got, expected)
	}
}

func TestStdioApplicationOpenerMapsOnlyInvocationEdgeValues(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("request")
	output := &strings.Builder{}
	opening := &recordingStdioOpening{result: testStdioApplication{}}
	owner := factorysessionwire.NewOpeningPresentationOwner()
	adapter, err := provideStdioApplicationOpener(opening, owner)
	if err != nil {
		t.Fatalf("provideStdioApplicationOpener(): %v", err)
	}
	application, err := adapter.OpenStdio(t.Context(), processcontract.MCPIntent{
		FixtureCatalogPath: "fixtures.json",
		RuntimeBacked:      true,
		ProjectRoot:        "/project",
		HomeDir:            "/home",
		Stdin:              input,
		Stdout:             output,
	})
	if err != nil {
		t.Fatalf("OpenStdio(): %v", err)
	}
	if application == nil {
		t.Fatal("OpenStdio() returned a nil lifecycle-ready application")
	}
	request := opening.request
	presentation, ok := owner.Stdio(request.ScopeID)
	if !ok {
		t.Fatalf("stdio opening scope %q was not registered", request.ScopeID)
	}
	if request.FixtureCatalogPath != "fixtures.json" || !request.RuntimeBacked ||
		request.ProjectRoot != "/project" || request.SystemConfigHome != "/home" ||
		presentation.Input != input || presentation.Output != output {
		t.Fatalf("mapped stdio opening = request:%#v presentation:%#v", request, presentation)
	}
}

func TestStdioApplicationOpenerRequiresOwnerOperation(t *testing.T) {
	t.Parallel()

	adapter, err := provideStdioApplicationOpener(nil, nil)
	if err == nil || adapter != nil {
		t.Fatalf("provideStdioApplicationOpener(nil) = (%v, %v), want nil and error", adapter, err)
	}
}

func TestWorkContentHostPlatformUsesExplicitEdgeOrProcessHost(t *testing.T) {
	t.Parallel()

	if got := provideWorkContentHostPlatform(serviceedges.Edges{WorkContentHostPlatform: "test-os"}); got != "test-os" {
		t.Fatalf("explicit Work content host platform = %q, want test-os", got)
	}
	if got := provideWorkContentHostPlatform(serviceedges.Edges{}); string(got) != runtime.GOOS {
		t.Fatalf("default Work content host platform = %q, want process host %q", got, runtime.GOOS)
	}
}

func TestWorkRequestEffectsUseExplicitEdgesOrProcessDefaults(t *testing.T) {
	t.Parallel()

	generate := work.RequestIDGenerator(func() string { return "edge-id" })
	read := work.SubmittedFileReader(func(string) ([]byte, error) { return []byte("edge"), nil })
	edges := serviceedges.Edges{
		WorkRequestIDGenerator:  generate,
		WorkSubmittedFileReader: read,
	}
	if got := provideWorkRequestIDGenerator(edges)(); got != "edge-id" {
		t.Fatalf("ID generator override = %q, want edge-id", got)
	}
	if got, err := provideWorkSubmittedFileReader(edges)("work.json"); err != nil || string(got) != "edge" {
		t.Fatalf("file reader override = (%q, %v)", got, err)
	}
	if got := provideWorkRequestIDGenerator(serviceedges.Edges{})(); got == "" {
		t.Fatal("default Work Request identity is empty")
	}

	path := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(path, []byte("default"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := provideWorkSubmittedFileReader(serviceedges.Edges{})(path); err != nil || string(got) != "default" {
		t.Fatalf("default file reader = (%q, %v)", got, err)
	}
}

func TestProvideWorkServiceConstructsThroughWorkWireBridge(t *testing.T) {
	t.Parallel()

	staging, err := provideWorkContentStagingService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideWorkContentStagingService() error = %v", err)
	}
	hostPlatform := provideWorkContentHostPlatform(serviceedges.Edges{})
	materializer, err := provideContentMaterializer(hostPlatform, serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideContentMaterializer() error = %v", err)
	}
	readFile := provideWorkSubmittedFileReader(serviceedges.Edges{})
	inspectPath := provideWorkSubmittedFilePathInspector(serviceedges.Edges{})

	service := provideWorkService(nil, readFile, inspectPath, staging, materializer, nil)
	if service == nil {
		t.Fatal("provideWorkService() returned nil service")
	}
	var root work.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to work.Service")
	}
}

func TestFactorySessionRuntimeIdentityUsesExplicitEdgeOrProcessDefault(t *testing.T) {
	t.Parallel()
	override := factorysessions.RuntimeInstanceIDGenerator(func() string { return "runtime-edge" })
	if got := provideFactorySessionRuntimeInstanceIDGenerator(serviceedges.Edges{FactorySessionRuntimeInstanceIDGenerator: override})(); got != "runtime-edge" {
		t.Fatalf("runtime instance identity override = %q", got)
	}
	if got := provideFactorySessionRuntimeInstanceIDGenerator(serviceedges.Edges{})(); got == "" {
		t.Fatal("default runtime instance identity is empty")
	}
}

func TestFactorySessionIdentityUsesExplicitEdgeOrProcessDefault(t *testing.T) {
	t.Parallel()
	override := factorysessions.SessionIDGenerator(func() string { return "session-edge" })
	if got := provideFactorySessionIDGenerator(serviceedges.Edges{FactorySessionIDGenerator: override})(); got != "session-edge" {
		t.Fatalf("Factory Session identity override = %q", got)
	}
	if got := provideFactorySessionIDGenerator(serviceedges.Edges{})(); got == "" {
		t.Fatal("default Factory Session identity is empty")
	}
}

func TestFactorySessionResponseEventIdentityUsesExplicitEdgeOrProcessDefault(t *testing.T) {
	t.Parallel()
	override := factorysessions.ResponseEventIDGenerator(func() string { return "response-event-edge" })
	if got := provideFactorySessionResponseEventIDGenerator(serviceedges.Edges{FactorySessionResponseEventIDGenerator: override})(); got != "response-event-edge" {
		t.Fatalf("response event identity override = %q", got)
	}
	if got := provideFactorySessionResponseEventIDGenerator(serviceedges.Edges{})(); got == "" {
		t.Fatal("default response event identity is empty")
	}
}

func TestFactorySessionCursorPersistenceUsesExplicitEdgesOrPlatformDefaults(t *testing.T) {
	t.Parallel()

	overrideFiles := &cursorPersistenceTestFileSystem{}
	createCalled := false
	createTemporaryFile := factorysessionwire.CursorPersistenceCreateTemporaryFile(func(string, string) (factorysessionwire.CursorPersistenceTemporaryFile, error) {
		createCalled = true
		return nil, os.ErrPermission
	})
	overrides := serviceedges.Edges{
		FactorySessionCursorPersistenceFileSystem: overrideFiles,
		FactorySessionCursorCreateTemporaryFile:   createTemporaryFile,
	}
	if got := provideFactorySessionCursorPersistenceFileSystem(overrides); got != overrideFiles {
		t.Fatalf("filesystem override = %T, want exact edge", got)
	}
	if _, err := provideFactorySessionCursorCreateTemporaryFile(overrides)("ignored", "ignored"); err == nil || !createCalled {
		t.Fatalf("temporary-file override = (%v, %v), want injected edge", err, createCalled)
	}

	files := provideFactorySessionCursorPersistenceFileSystem(serviceedges.Edges{})
	if _, ok := files.(platformfilesystem.Local); !ok {
		t.Fatalf("default filesystem = %T, want policy-free local adapter", files)
	}
	create := provideFactorySessionCursorCreateTemporaryFile(serviceedges.Edges{})
	store, err := provideFactorySessionCursorStoreFactory(files, create)(filepath.Join(t.TempDir(), "cursors"))
	if err != nil {
		t.Fatalf("open default cursor store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
}

type cursorPersistenceTestFileSystem struct {
	platformfilesystem.Local
}

func TestWorkersAgentToolFileSystemUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideWorkersAgentToolFileSystem(serviceedges.Edges{
		WorkersAgentToolFileSystem: override,
	}); got != override {
		t.Fatalf("agent tool filesystem override = %T, want exact edge", got)
	}
	if got := provideWorkersAgentToolFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("agent tool filesystem default = %T, want policy-free local adapter", got)
	}
}

func TestWorkersMockWorkersConfigFileSystemUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	selected := provideWorkersMockWorkersConfigFileSystem(serviceedges.Edges{
		WorkersMockWorkersConfigFileSystem: override,
	})
	if selected != override {
		t.Fatalf("mock workers filesystem override = %T, want exact edge", selected)
	}
	load, err := workers.NewMockWorkersConfigLoader(selected)
	if err != nil {
		t.Fatalf("construct loader from Wire-selected override: %v", err)
	}
	if load == nil {
		t.Fatal("constructed loader = nil")
	}
	if got := provideWorkersMockWorkersConfigFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("mock workers filesystem default = %T, want policy-free local adapter", got)
	}
}

func TestWorkersRetryRandomSourceUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := platformrandom.SourceFunc(func(int64) (int64, error) { return 3, nil })
	if got := provideWorkersRetryRandomSource(serviceedges.Edges{WorkersRetryRandomSource: override}); got == nil {
		t.Fatal("retry random override = nil")
	} else if value, err := got.Int63n(10); err != nil || value != 3 {
		t.Fatalf("retry random override = (%d, %v), want (3, nil)", value, err)
	}
	if got := provideWorkersRetryRandomSource(serviceedges.Edges{}); got != (platformrandom.CryptoSource{}) {
		t.Fatalf("retry random default = %T, want policy-free crypto adapter", got)
	}
}

func TestWorkersWorkstationFileSystemUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideWorkersWorkstationFileSystem(serviceedges.Edges{WorkersWorkstationFileSystem: override}); got != override {
		t.Fatalf("workstation filesystem override = %T, want exact edge", got)
	}
	if got := provideWorkersWorkstationFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("workstation filesystem default = %T, want policy-free local adapter", got)
	}
}

func TestWorkersProviderTemporaryFileSystemUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideWorkersProviderTemporaryFileSystem(serviceedges.Edges{WorkersProviderTemporaryFileSystem: override}); got != override {
		t.Fatalf("provider temporary filesystem override = %T, want exact edge", got)
	}
	if got := provideWorkersProviderTemporaryFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("provider temporary filesystem default = %T, want policy-free local adapter", got)
	}
}

func TestWorkersFactoryDocsFileSystemUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideWorkersFactoryDocsFileSystem(serviceedges.Edges{WorkersFactoryDocsFileSystem: override}); got != override {
		t.Fatalf("Factory docs filesystem override = %T, want exact edge", got)
	}
	if got := provideWorkersFactoryDocsFileSystem(serviceedges.Edges{}); got != (platformfilesystem.Local{}) {
		t.Fatalf("Factory docs filesystem default = %T, want policy-free local adapter", got)
	}
}

func TestFactorySessionDirectoryInspectionUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideFactorySessionDirectoryInspection(serviceedges.Edges{
		FactorySessionDirectoryInspection: override,
	}); got != override {
		t.Fatalf("directory inspection override = %T, want exact edge", got)
	}
	got := provideFactorySessionDirectoryInspection(serviceedges.Edges{})
	if _, ok := got.(platformfilesystem.Local); !ok {
		t.Fatalf("default directory inspection = %T, want policy-free local adapter", got)
	}
}

func TestFactorySessionFileReadersUseExactEdgesOrPlatformDefaults(t *testing.T) {
	t.Parallel()

	called := map[string]bool{}
	edges := serviceedges.Edges{
		FactorySessionContractFixtureReader: func(string) ([]byte, error) { called["fixture"] = true; return nil, nil },
		FactorySessionInvocationInputReader: func(string) ([]byte, error) { called["invocation"] = true; return nil, nil },
		FactorySessionReplayRecordingReader: func(string) ([]byte, error) { called["replay"] = true; return nil, nil },
		FactorySessionInitialWorkReader:     func(string) ([]byte, error) { called["work"] = true; return nil, nil },
	}
	readers := []struct {
		name string
		read func(string) ([]byte, error)
	}{
		{"fixture", provideFactorySessionContractFixtureReader(edges).ReadFile},
		{"invocation", provideFactorySessionInvocationInputReader(edges).ReadFile},
		{"replay", provideFactorySessionReplayRecordingReader(edges).ReadFile},
		{"work", provideFactorySessionInitialWorkReader(edges).ReadFile},
	}
	for _, reader := range readers {
		if _, err := reader.read("ignored"); err != nil || !called[reader.name] {
			t.Fatalf("%s override = (%v, %v)", reader.name, err, called[reader.name])
		}
	}

	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte("injected-default"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	defaults := []func(string) ([]byte, error){
		provideFactorySessionContractFixtureReader(serviceedges.Edges{}).ReadFile,
		provideFactorySessionInvocationInputReader(serviceedges.Edges{}).ReadFile,
		provideFactorySessionReplayRecordingReader(serviceedges.Edges{}).ReadFile,
		provideFactorySessionInitialWorkReader(serviceedges.Edges{}).ReadFile,
	}
	for index, read := range defaults {
		if got, err := read(path); err != nil || string(got) != "injected-default" {
			t.Fatalf("default reader %d = (%q, %v)", index, got, err)
		}
	}
}

func TestFactorySessionHomeDirectoryUsesExplicitEdgeOrProcessDefault(t *testing.T) {
	t.Parallel()
	override := func() (string, error) { return "/edge-home", nil }
	if got, err := provideFactorySessionResolveHomeDirectory(serviceedges.Edges{FactorySessionResolveHomeDirectory: override})(); err != nil || got != "/edge-home" {
		t.Fatalf("home directory override = (%q, %v)", got, err)
	}
	if got, err := provideFactorySessionResolveHomeDirectory(serviceedges.Edges{})(); err != nil || strings.TrimSpace(got) == "" {
		t.Fatalf("default home directory = (%q, %v)", got, err)
	}
}

func TestFactorySessionExecutionOpeningUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	if got := provideFactorySessionExecutionOpeningFileSystem(serviceedges.Edges{
		FactorySessionExecutionOpeningFileSystem: override,
	}); got != override {
		t.Fatalf("execution-opening filesystem override = %T, want exact edge", got)
	}
	got := provideFactorySessionExecutionOpeningFileSystem(serviceedges.Edges{})
	if _, ok := got.(platformfilesystem.Local); !ok {
		t.Fatalf("default execution-opening filesystem = %T, want policy-free local adapter", got)
	}
}

func TestFactorySessionRuntimePersistenceUsesExplicitEdgeOrPlatformDefault(t *testing.T) {
	t.Parallel()

	override := &cursorPersistenceTestFileSystem{}
	files := provideFactorySessionRuntimePersistenceFileSystem(serviceedges.Edges{
		FactorySessionRuntimePersistenceFileSystem: override,
	})
	if files != override {
		t.Fatalf("runtime persistence filesystem override = %T, want exact edge", files)
	}

	files = provideFactorySessionRuntimePersistenceFileSystem(serviceedges.Edges{})
	if _, ok := files.(runtimePersistenceFileSystem); !ok {
		t.Fatalf("default runtime persistence filesystem = %T, want replay storage adapter", files)
	}
	store, err := provideFactorySessionRuntimePersistenceStoreFactory(files)(t.TempDir())
	if err != nil {
		t.Fatalf("open default runtime persistence store: %v", err)
	}
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.Save(sessionID, []byte(`{"status":"COMPLETED"}`)); err != nil {
		t.Fatalf("save runtime snapshot: %v", err)
	}
	if _, err := store.Load(sessionID); err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestRuntimeOpeningRequestFactoryMapsSelectionsIntoOwnerRequests(t *testing.T) {
	t.Parallel()
	skip := true
	mocks := workers.NewEmptyMockWorkersConfig()
	opening := provideRuntimeOpeningRequestFactory()(runcli.RunConfig{
		Dir: "factory", FactoryConfigPath: "/tmp/factory.json",
		ExecutionBaseDir: "execution", RunnerID: "runner", Worktree: "feature-login",
		HomeDir: "home", WorkFile: "work.json", BindHost: "127.0.0.1",
		Port: 8080, AutoPort: true,
		Continuously: true, Verbose: true, RecordPath: "record.json",
		ReplayPath: "replay.json", ResumePath: "source.recording.json", Workflow: "flow", ModelCacheDir: "models",
		CanonicalSessionID:                "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba",
		WorkerReasoningEffort:             "xhigh",
		InvocationSkipPermissionsOverride: &skip,
	}, mocks)
	request := opening

	if request.FactoryDefinition.Directory != "factory" ||
		request.FactoryDefinition.SourcePath != "/tmp/factory.json" ||
		request.FactoryDefinition.ExecutionBaseDir != "execution" {
		t.Fatalf("Factory Definition request = %#v", request.FactoryDefinition)
	}
	if request.FactoryRuntime.Mode != factorydefinitions.RuntimeModeService || !request.FactoryRuntime.Verbose {
		t.Fatalf("Factory Runtime request = %#v", request.FactoryRuntime)
	}
	if request.FactorySession.SystemConfigHome != "home" ||
		request.FactorySession.Host.Host != "127.0.0.1" ||
		request.FactorySession.Host.Port != 8080 ||
		!request.FactorySession.Host.AutoPort {
		t.Fatalf("Factory Session request = %#v", request.FactorySession)
	}
	if request.FactorySession.PersistencePolicy != factorysessions.PersistencePolicyEnabled {
		t.Fatalf("Factory Session persistence policy = %q, want %q", request.FactorySession.PersistencePolicy, factorysessions.PersistencePolicyEnabled)
	}
	if request.FactorySession.CanonicalSessionID != "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba" {
		t.Fatalf("Factory Session canonical ID = %q, want preallocated UUID", request.FactorySession.CanonicalSessionID)
	}
	if request.Workers.RunnerID != "runner" || request.Workers.Worktree != "feature-login" ||
		request.Workers.WorkerReasoningEffort != "xhigh" ||
		request.Workers.MockWorkers != mocks || request.Workers.InvocationSkipPermissionsOverride != &skip {
		t.Fatalf("Workers request = %#v", request.Workers)
	}
	if request.Recordings.RecordPath != "record.json" || request.Recordings.ReplayPath != "replay.json" ||
		request.Recordings.ResumePath != "source.recording.json" || request.Recordings.WorkflowID != "flow" {
		t.Fatalf("Recordings request = %#v", request.Recordings)
	}
	if request.ModelCacheDirectory != "models" {
		t.Fatalf("Model cache directory = %#v", request.ModelCacheDirectory)
	}
}

func TestRuntimeOpeningRequestFactorySelectsEnabledPersistenceForBatchAndService(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		continuously bool
		mode         factorydefinitions.RuntimeMode
	}{
		{name: "batch", mode: factorydefinitions.RuntimeModeBatch},
		{name: "service", continuously: true, mode: factorydefinitions.RuntimeModeService},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := provideRuntimeOpeningRequestFactory()(runcli.RunConfig{
				Dir:          "factory",
				Continuously: test.continuously,
			}, nil)
			if request.FactoryRuntime.Mode != test.mode {
				t.Fatalf("runtime mode = %q, want %q", request.FactoryRuntime.Mode, test.mode)
			}
			if request.FactorySession.PersistencePolicy != factorysessions.PersistencePolicyEnabled {
				t.Fatalf("persistence policy = %q, want %q", request.FactorySession.PersistencePolicy, factorysessions.PersistencePolicyEnabled)
			}
		})
	}
	assertBatchColdStartOpeningValues(t)
}

// assertBatchColdStartOpeningValues
// records the exact request values that distinguish the no-listener batch
// path from the explicitly hosted path. The later optimization story may
// change when unused work happens, but it must not change this owner request
// boundary.
func assertBatchColdStartOpeningValues(t *testing.T) {
	t.Helper()
	opening := provideRuntimeOpeningRequestFactory()
	mocks := workers.NewEmptyMockWorkersConfig()
	base := runcli.RunConfig{
		Dir:               "factory",
		FactoryConfigPath: "factory/factory.json",
		ExecutionBaseDir:  "execution",
		HomeDir:           "isolated-home",
		WorkFile:          "one-work.json",
		BindHost:          "127.0.0.1",
	}

	batch := opening(base, mocks)
	if batch.FactoryRuntime.Mode != factorydefinitions.RuntimeModeBatch {
		t.Fatalf("batch runtime mode = %q, want %q", batch.FactoryRuntime.Mode, factorydefinitions.RuntimeModeBatch)
	}
	if batch.FactorySession.Host.Directory != base.Dir ||
		batch.FactorySession.Host.WorkFile != base.WorkFile ||
		!batch.FactorySession.Host.MockWorkers {
		t.Fatalf("batch host values = %+v, want directory/work file/mock workers from the batch request", batch.FactorySession.Host)
	}
	if batch.FactorySession.Host.Host != "127.0.0.1" || batch.FactorySession.Host.Port != 0 ||
		batch.FactorySession.Host.AutoPort || batch.FactorySession.Host.Pprof {
		t.Fatalf("batch host values = %+v, want no listener request", batch.FactorySession.Host)
	}

	serverConfig := base
	serverConfig.Port = 8123
	serverConfig.AutoPort = true
	serverConfig.Pprof = true
	server := opening(serverConfig, mocks)
	if server.FactoryRuntime.Mode != factorydefinitions.RuntimeModeBatch {
		t.Fatalf("hosted batch runtime mode = %q, want %q", server.FactoryRuntime.Mode, factorydefinitions.RuntimeModeBatch)
	}
	if got := server.FactorySession.Host; got.Host != "127.0.0.1" || got.Port != 8123 || !got.AutoPort || !got.Pprof {
		t.Fatalf("hosted batch host values = %+v, want explicit API host request", got)
	}

	t.Logf("batch host: directory=%q work=%q mock_workers=%t host=%q port=%d auto_port=%t pprof=%t; hosted host: %+v",
		batch.FactorySession.Host.Directory, batch.FactorySession.Host.WorkFile,
		batch.FactorySession.Host.MockWorkers, batch.FactorySession.Host.Host,
		batch.FactorySession.Host.Port, batch.FactorySession.Host.AutoPort,
		batch.FactorySession.Host.Pprof, server.FactorySession.Host)
}

func TestRuntimeInputResolverCopiesRequestWithoutSelectingEffects(t *testing.T) {
	t.Parallel()
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "factory"},
	}
	resolved, err := provideRuntimeInputResolver()(t.Context(), request)
	if err != nil {
		t.Fatalf("resolve inputs: %v", err)
	}
	if resolved.Request == request || resolved.Request.FactoryDefinition.Directory != "factory" {
		t.Fatalf("resolved request = %#v; want detached value", resolved.Request)
	}
	request.FactoryDefinition.Directory = "mutated"
	if resolved.Request.FactoryDefinition.Directory != "factory" {
		t.Fatal("resolved request retained caller mutation")
	}
}

func TestFactoryRuntimeEffectProvidersSelectExactProcessEdges(t *testing.T) {
	t.Parallel()
	clock := platformclock.Real{}
	metrics := &runtimeInputMetricsRecorder{}
	providerRunner := &processCommandRunner{}
	scriptRunner := &processCommandRunner{}
	edges := serviceedges.Edges{
		Clock:                     clock,
		InvocationMetricsRecorder: metrics,
		ProviderCommandRunner:     providerRunner,
		ScriptCommandRunner:       scriptRunner,
	}
	if got := provideFactoryRuntimeClock(edges); got != clock {
		t.Fatalf("clock = %v, want exact edge", got)
	}
	if got := provideFactorySessionInvocationMetricsRecorder(edges); got != metrics {
		t.Fatalf("metrics recorder = %v, want exact edge", got)
	}
	gotProvider, err := provideFactoryRuntimeProviderCommandRunner(edges)
	if err != nil {
		t.Fatalf("provider command runner: %v", err)
	}
	gotScript, err := provideFactoryRuntimeScriptCommandRunner(edges)
	if err != nil {
		t.Fatalf("script command runner: %v", err)
	}
	if gotProvider != providerRunner {
		t.Fatalf("provider command runner = %v, want edge runner %v", gotProvider, providerRunner)
	}
	if gotScript != scriptRunner {
		t.Fatalf("script command runner = %v, want edge runner %v", gotScript, scriptRunner)
	}
}

func TestFactoryRuntimeEffectProvidersDefaultCommandRunnersWhenUnset(t *testing.T) {
	t.Parallel()
	providerRunner, err := provideFactoryRuntimeProviderCommandRunner(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provider command runner: %v", err)
	}
	scriptRunner, err := provideFactoryRuntimeScriptCommandRunner(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("script command runner: %v", err)
	}
	if providerRunner == nil || scriptRunner == nil {
		t.Fatalf("command runners = (%v, %v), want defaults", providerRunner, scriptRunner)
	}
}

type runtimeInputMetricsRecorder struct{}

func (*runtimeInputMetricsRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {}

func TestRuntimeInputResolverRejectsMissingRequiredInputs(t *testing.T) {
	t.Parallel()
	request := &factorysessions.RuntimeOpeningRequest{}
	tests := []struct {
		name    string
		ctx     context.Context
		request *factorysessions.RuntimeOpeningRequest
		want    string
	}{
		{name: "nil context", request: request, want: "context is required"},
		{name: "nil request", ctx: t.Context(), want: "runtime opening request is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := provideRuntimeInputResolver()(test.ctx, test.request)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

type runtimeObservabilityTestOwners struct {
	logOwner     factoryruntime.RuntimeLogOwner
	metricsOwner factoryruntime.RuntimeMetricsOwner
	logRoot      string
	metricsRoot  string
}

func newRuntimeObservabilityTestOwners(t *testing.T) runtimeObservabilityTestOwners {
	t.Helper()
	at := time.Date(2026, time.August, 10, 15, 4, 2, 0, time.UTC)
	root := t.TempDir()
	owners := runtimeObservabilityTestOwners{
		logRoot:     filepath.Join(root, "logs"),
		metricsRoot: filepath.Join(root, "metrics"),
	}
	reserver, err := provideRuntimeArtifactPathReserver()
	if err != nil {
		t.Fatalf("provideRuntimeArtifactPathReserver(): %v", err)
	}
	var logCollision atomic.Int32
	owners.logOwner, err = provideRuntimeLogOwner(
		zap.NewNop(), func() time.Time { return at },
		func() string { return "log-" + strconv.Itoa(int(logCollision.Add(1))) }, reserver,
	)
	if err != nil {
		t.Fatalf("provideRuntimeLogOwner(): %v", err)
	}
	var metricCollision atomic.Int32
	metricsCoordination, err := provideRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("provideRuntimeMetricsCoordination(): %v", err)
	}
	metricsFileSystem := provideRuntimeMetricsRetentionFileSystem()
	owners.metricsOwner, err = provideRuntimeMetricsOwner(
		zap.NewNop(),
		func() time.Time { return at },
		func() string { return "metric-" + strconv.Itoa(int(metricCollision.Add(1))) }, reserver,
		metricsFileSystem,
		metricsCoordination,
	)
	if err != nil {
		t.Fatalf("provideRuntimeMetricsOwner(): %v", err)
	}
	assertRuntimeObservabilityConstructionIsInert(t, owners)
	return owners
}

func assertRuntimeObservabilityConstructionIsInert(t *testing.T, owners runtimeObservabilityTestOwners) {
	t.Helper()
	for name, root := range map[string]string{"log": owners.logRoot, "metrics": owners.metricsRoot} {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("%s root after owner construction stat error = %v, want not exist", name, err)
		}
	}
}

func TestRuntimeLogOwnerKeepsPrivateScopesIsolated(t *testing.T) {
	t.Parallel()
	owners := newRuntimeObservabilityTestOwners(t)
	first := openRuntimeLogTestScope(t, owners.logOwner, owners.logRoot, "session-first")
	second := openRuntimeLogTestScope(t, owners.logOwner, owners.logRoot, "session-second")
	if first.Artifact().Path == second.Artifact().Path {
		t.Fatalf("log scope paths collide: %q", first.Artifact().Path)
	}
	first.Logger().Info("first session log")
	second.Logger().Info("second session log")
	if err := first.Close(); err != nil {
		t.Fatalf("close first log scope: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first log scope twice: %v", err)
	}
	second.Logger().Info("second session remains open")
	defer second.Close()
	assertRuntimeLogRecords(t, first, second)
}

func openRuntimeLogTestScope(t *testing.T, owner factoryruntime.RuntimeLogOwner, root, sessionID string) factoryruntime.RuntimeLogSink {
	t.Helper()
	sink, err := owner.Open(factoryruntime.RuntimeLogScopeRequest{
		SessionID: sessionID, RuntimeInstanceID: "runtime-shared",
		FolderPath: "/folder", FactoryDirectory: "/factory", RootDirectory: root,
		Policy: factoryruntime.RuntimeFileLoggingPolicyEnabled,
	})
	if err != nil {
		t.Fatalf("open log scope %q: %v", sessionID, err)
	}
	return sink
}

func assertRuntimeLogRecords(t *testing.T, first, second factoryruntime.RuntimeLogSink) {
	t.Helper()
	firstBytes, err := os.ReadFile(first.Artifact().Path)
	if err != nil {
		t.Fatalf("read first log scope: %v", err)
	}
	secondBytes, err := os.ReadFile(second.Artifact().Path)
	if err != nil {
		t.Fatalf("read second log scope: %v", err)
	}
	if !strings.Contains(string(firstBytes), "first session log") || strings.Contains(string(firstBytes), "second session log") {
		t.Fatalf("first log scope leaked another session: %s", firstBytes)
	}
	if !strings.Contains(string(secondBytes), "second session remains open") {
		t.Fatalf("second log scope did not remain writable: %s", secondBytes)
	}
}

func TestRuntimeMetricsOwnerKeepsPrivateScopesIsolated(t *testing.T) {
	t.Parallel()
	owners := newRuntimeObservabilityTestOwners(t)
	first := openRuntimeMetricsTestScope(t, owners.metricsOwner, owners.metricsRoot, "session-first")
	second := openRuntimeMetricsTestScope(t, owners.metricsOwner, owners.metricsRoot, "session-second")
	if first.Path() == second.Path() {
		t.Fatalf("metrics scope paths collide: %q", first.Path())
	}
	if err := first.Counter(t.Context(), "first.metric", 1, factoryruntime.Fields{}); err != nil {
		t.Fatalf("write first metrics scope: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first metrics scope: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first metrics scope twice: %v", err)
	}
	if err := second.Counter(t.Context(), "second.metric", 1, factoryruntime.Fields{}); err != nil {
		t.Fatalf("write second metrics scope after first close: %v", err)
	}
	defer second.Close()
	assertRuntimeMetricsRecords(t, first, second)
}

func openRuntimeMetricsTestScope(t *testing.T, owner factoryruntime.RuntimeMetricsOwner, root, sessionID string) factoryruntime.RuntimeMetricsSink {
	t.Helper()
	sink, err := owner.Open(factoryruntime.RuntimeMetricsScopeRequest{
		Scope: factoryruntime.RuntimeMetricsScope{
			SessionID: sessionID, RuntimeInstanceID: "runtime-shared",
			FolderPath: "/folder", FactoryDir: "/factory",
		},
		RootDirectory: root, Policy: factoryruntime.RuntimeMetricsPolicyEnabled,
	})
	if err != nil {
		t.Fatalf("open metrics scope %q: %v", sessionID, err)
	}
	return sink
}

func assertRuntimeMetricsRecords(t *testing.T, first, second factoryruntime.RuntimeMetricsSink) {
	t.Helper()
	firstBytes, err := os.ReadFile(first.Path())
	if err != nil {
		t.Fatalf("read first metrics scope: %v", err)
	}
	secondBytes, err := os.ReadFile(second.Path())
	if err != nil {
		t.Fatalf("read second metrics scope: %v", err)
	}
	if !strings.Contains(string(firstBytes), "session-first") || strings.Contains(string(firstBytes), "second.metric") {
		t.Fatalf("first metrics scope leaked another session: %s", firstBytes)
	}
	if !strings.Contains(string(secondBytes), "session-second") {
		t.Fatalf("second metrics scope did not record after first close: %s", secondBytes)
	}
}

func TestRuntimeObservabilityOwnerRejectsUnwritableDestination(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	unwritable := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(unwritable, []byte("file"), 0o600); err != nil {
		t.Fatalf("create destination sentinel: %v", err)
	}
	reserver, err := provideRuntimeArtifactPathReserver()
	if err != nil {
		t.Fatalf("provideRuntimeArtifactPathReserver(): %v", err)
	}
	owner, err := provideRuntimeLogOwner(
		zap.NewNop(), time.Now, func() string { return "unwritable" }, reserver,
	)
	if err != nil {
		t.Fatalf("provideRuntimeLogOwner(): %v", err)
	}
	_, err = owner.Open(factoryruntime.RuntimeLogScopeRequest{
		RuntimeInstanceID: "runtime-unwritable", RootDirectory: unwritable,
		Policy: factoryruntime.RuntimeFileLoggingPolicyEnabled,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime artifact") {
		t.Fatalf("unwritable log destination error = %v, want actionable runtime artifact error", err)
	}
}
