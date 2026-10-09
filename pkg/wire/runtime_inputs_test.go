package wire

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type canonicalStdioSessionsStub struct{ factorysessions.Service }

type durableHTTPInspectionStub struct {
	factorysessions.SessionInspectionService
	requestedID string
	err         error
}

func (stub *durableHTTPInspectionStub) QueryArtifacts(_ context.Context, request factorysessions.SessionArtifactQueryRequest) (factorysessions.ListArtifactsResult, error) {
	stub.requestedID = request.SessionID
	return factorysessions.ListArtifactsResult{SessionID: request.SessionID}, stub.err
}

type durableHTTPValidationStub struct {
	factorydefinitions.SubmittedDefinitionValidationOperation
}
type durableHTTPWorkTypeStub struct {
	factorydefinitions.InvocationWorkTypeService
}
type durableHTTPRequestsStub struct {
	factorysessionshttp.RequestPreparation
}

func TestDurableHTTPUsesInjectedInspectionForSelectedSession(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "selected artifacts"
		if missing {
			name = "typed missing session"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inspection := &durableHTTPInspectionStub{}
			if missing {
				inspection.err = factorysessions.ErrDurableSessionNotFound
			}
			// This Sessions fake has no inspection accessor. Only the separately
			// injected owner can answer the selected artifact request.
			handler, err := newDurableExecutionHTTPHandler(canonicalStdioSessionsStub{}, inspection,
				durableHTTPValidationStub{}, durableHTTPWorkTypeStub{}, durableHTTPRequestsStub{}, zap.NewNop(), nil)
			if err != nil {
				t.Fatal(err)
			}
			const sessionID = "dur-sess-selected-inspection"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/artifacts", nil))
			wantStatus := http.StatusOK
			if missing {
				wantStatus = http.StatusNotFound
			}
			if inspection.requestedID != sessionID || response.Code != wantStatus {
				t.Fatalf("selected inspection = %q, status = %d, body = %s", inspection.requestedID, response.Code, response.Body.String())
			}
			if missing && !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
				t.Fatalf("missing session lost typed error: %s", response.Body.String())
			}
		})
	}
}

type testStdioApplication struct{}

func (testStdioApplication) Run(context.Context) error { return nil }

type testStdioRunner struct{ ran *bool }

func (runner testStdioRunner) Run(context.Context) error {
	*runner.ran = true
	return nil
}

type workerSessionsObservationServiceStub struct {
	workersessions.Service
}

type workerSessionsObservationGatewayStub struct {
	factorysessions.Service
	observation workersessions.ObservationService
}

type workerSessionsRuntimeScopeStub struct {
	factorysessions.Service
	effectiveID string
	isDefault   bool
	requestedID string
}

func (stub *workerSessionsRuntimeScopeStub) ResolveFactorySessionRuntimeScope(sessionID string) (string, bool, error) {
	stub.requestedID = sessionID
	return stub.effectiveID, stub.isDefault, nil
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

func TestWorkerSessionsScopeResolverRecognizesExplicitDefaultSessionID(t *testing.T) {
	const defaultID = "550e8400-e29b-41d4-a716-446655440000"
	sessions := &workerSessionsRuntimeScopeStub{effectiveID: defaultID, isDefault: true}
	resolver := newWorkerSessionsFactorySessionScopeResolver(sessions)
	scope, err := resolver.ResolveWorkerSessionScope(context.Background(), defaultID)
	if err != nil || scope.EffectiveID != defaultID || !scope.IsDefault || sessions.requestedID != defaultID {
		t.Fatalf("scope = (%#v, %v), requested = %q; want explicit default UUID", scope, err, sessions.requestedID)
	}
}

func TestStdioHandlerUsesProcessSessionsAndInvocationStreams(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("request")
	output := &strings.Builder{}
	sessions := &canonicalStdioSessionsStub{}
	var selectedRoot string
	var selectedHost string
	var selectedInput, selectedOutput any
	ran := false
	handler, err := provideStdioHandler(
		sessions, nil,
		func(_ context.Context, _ lifecycle.Plan, _ runtimeartifact.Diagnostics, _ <-chan initializer.RuntimeHostBinding) (initializer.LocalRuntimeRunner, error) {
			return testStdioRunner{ran: &ran}, nil
		},
		lifecycle.NewRunner,
		func(_ *mcpserver.Server, in io.Reader, out io.Writer) (mcpstdio.Session, error) {
			selectedInput, selectedOutput = in, out
			return testStdioApplication{}, nil
		},
		func(projectRoot, serverURL string, _ recordings.Service, _ factorysessionwire.RequestPreparation, _ factoryruntime.WorkflowPreviewOperation, bound factorysessions.Service) (*mcpserver.Server, error) {
			selectedRoot = projectRoot
			selectedHost = serverURL
			if bound != sessions {
				t.Fatalf("MCP root = %T, want process root", bound)
			}
			return &mcpserver.Server{}, nil
		},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("provideStdioHandler(): %v", err)
	}
	if handler == nil {
		t.Fatal("provideStdioHandler() returned nil handler")
	}
	if err := handler(t.Context(), processcontract.MCPIntent{
		ProjectRoot: "/project",
		ServerURL:   "http://selected-host:7437",
		Stdin:       input,
		Stdout:      output,
	}); err != nil {
		t.Fatalf("handler(): %v", err)
	}
	if !ran {
		t.Fatal("handler() did not run the lifecycle-ready application")
	}
	if selectedRoot != "/project" || selectedHost != "http://selected-host:7437" || selectedInput != input || selectedOutput != output {
		t.Fatalf("stdio mapping = root:%q input:%T output:%T", selectedRoot, selectedInput, selectedOutput)
	}
}

func TestStdioHandlerRequiresOwnerOperation(t *testing.T) {
	t.Parallel()

	handler, err := provideStdioHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || handler != nil {
		t.Fatalf("provideStdioHandler(nil) = (%v, %v), want nil and error", handler, err)
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

	staging, err := provideWorkContentStagingService(serviceedges.Edges{}, platformclock.Real{})
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

	prep := fixtureWorkPreparation(func(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
		if ctx != t.Context() || input.Request.RequestID != "caller" {
			t.Fatal("provider changed request")
		}
		return work.WorkRequest{RequestID: "completed-preparation"}, nil
	})
	input := workwire.NewInvocationInputAdapter(workwire.NewInvocationInputPolicy(readFile, inspectPath))
	state := workwire.NewStateAccess(workwire.NewRuntimeSessionResolver(nil), nil, fixtureWorkDurability{})
	service := provideWorkService(nil, readFile, inspectPath, staging, materializer, state, prep, input)
	prepared, err := service.PrepareWorkRequest(t.Context(), work.WorkRequestPreparation{Request: work.WorkRequest{RequestID: "caller"}})
	if err != nil || prepared.RequestID != "completed-preparation" {
		t.Fatalf("provided preparation=%#v,error=%v", prepared, err)
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
		{"invocation", provideFactorySessionInvocationInputReader(edges)},
		{"replay", provideFactorySessionReplayRecordingReader(edges)},
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
		provideFactorySessionInvocationInputReader(serviceedges.Edges{}),
		provideFactorySessionReplayRecordingReader(serviceedges.Edges{}),
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

func (files *cursorPersistenceTestFileSystem) ReadFileBounded(path string, limit int64) ([]byte, error) {
	return platformfilesystem.NewRecovery(files.Local, files.Local).ReadFileBounded(path, limit)
}
func (files *cursorPersistenceTestFileSystem) RenameNoReplace(source, destination string) error {
	return platformfilesystem.NewRecovery(files.Local, files.Local).RenameNoReplace(source, destination)
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

// The legacy fallback was characterized before cutover. Providers now receive
// the normalized pair from BuildProcess and must preserve its exact scheduler.
func TestFactoryRuntimeMetricsClockSelectsNormalizedScheduler(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"now-only", "timer-capable", "default", "explicit-scheduler"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
			logical := platformclock.NewDeterministic(base, time.Second)
			edges := serviceedges.Edges{Clock: logical, ProcessScheduler: logical}
			switch name {
			case "now-only":
				edges.Clock = metricsNowOnlyClock{at: base}
				edges.ProcessScheduler = platformclock.Real{}
			case "default":
				edges.Clock = platformclock.Real{}
				edges.ProcessScheduler = platformclock.Real{}
			case "explicit-scheduler":
				edges.Clock = metricsNowOnlyClock{at: base.Add(time.Hour)}
			}
			got := provideFactoryRuntimeMetricsClock(edges)
			if got != edges.ProcessScheduler {
				t.Fatalf("metrics scheduler = %v, want exact selected scheduler %v", got, edges.ProcessScheduler)
			}
			assertSelectedTimeProjections(t, edges)
			if name == "now-only" || name == "default" {
				assertLegacyMetricsWallTimerDelivery(t, got, base)
				return
			}
			assertLogicalMetricsTimerDelivery(t, got, logical, base)
		})
	}
}

func assertSelectedTimeProjections(t *testing.T, edges serviceedges.Edges) {
	t.Helper()
	for name, got := range map[string]platformclock.Source{
		"provider observation": effectiveProviderCommandClock(edges),
		"resolver":             provideFactoryRuntimeClockResolver(edges.Clock)(nil),
	} {
		if got != edges.Clock {
			t.Fatalf("%s clock = %v, want selected source %v", name, got, edges.Clock)
		}
	}
}

func assertLogicalMetricsTimerDelivery(t *testing.T, clock platformclock.TimerSource, logical *platformclock.Deterministic, base time.Time) {
	t.Helper()
	timer := clock.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-timer.C():
		t.Fatal("logical timer fired before tick advance")
	default:
	}
	logical.SetTick(1)
	select {
	case at := <-timer.C():
		if !at.Equal(base.Add(time.Second)) {
			t.Fatalf("logical timer timestamp = %v", at)
		}
	default:
		t.Fatal("logical timer did not fire after tick advance")
	}
}

func assertLegacyMetricsWallTimerDelivery(t *testing.T, clock platformclock.TimerSource, base time.Time) {
	t.Helper()
	// A zero-duration timer proves the explicitly selected wall scheduler delivers
	// without advancing the timestamp-only source; timeout is a failure ceiling.
	timer := clock.NewTimer(0)
	defer timer.Stop()
	select {
	case at := <-timer.C():
		if at.IsZero() || at.Equal(base) {
			t.Fatalf("host timer timestamp = %v", at)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("legacy host timer did not deliver")
	}
}

type metricsNowOnlyClock struct{ at time.Time }

func (clock metricsNowOnlyClock) Now() time.Time { return clock.at }

func TestFactoryRuntimeEffectProvidersDefaultCommandRunnersWhenUnset(t *testing.T) {
	t.Parallel()
	providerRunner, err := provideFactoryRuntimeProviderCommandRunner(selectedTestTimeEdges(serviceedges.Edges{}))
	if err != nil {
		t.Fatalf("provider command runner: %v", err)
	}
	scriptRunner, err := provideFactoryRuntimeScriptCommandRunner(selectedTestTimeEdges(serviceedges.Edges{}))
	if err != nil {
		t.Fatalf("script command runner: %v", err)
	}
	if providerRunner == nil || scriptRunner == nil {
		t.Fatalf("command runners = (%v, %v), want defaults", providerRunner, scriptRunner)
	}
}

type runtimeInputMetricsRecorder struct{}

func (*runtimeInputMetricsRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {}

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
		func() time.Time { return at },
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

func TestRuntimeLogOwnerPreservesSelectedScopedBackend(t *testing.T) {
	t.Parallel()
	owners := newRuntimeObservabilityTestOwners(t)
	core, logs := observer.New(zap.InfoLevel)
	base := zap.New(core).With(zap.String("backend", "selected"))
	for _, sessionID := range []string{"candidate", "peer"} {
		selected := base.With(zap.String("session_id", sessionID), zap.String("invocation_id", sessionID+"-invocation"))
		sink, err := owners.logOwner.Open(selected, factoryruntime.RuntimeLogScopeRequest{
			SessionID: sessionID, RuntimeInstanceID: sessionID + "-runtime", RootDirectory: owners.logRoot,
			Policy: factoryruntime.RuntimeFileLoggingPolicyEnabled,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sink.Close() })
		sink.Logger().Info("scoped-file-log")
		if err := sink.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(sink.Artifact().Path)
		if err != nil {
			t.Fatal(err)
		}
		// Zap With fields already belong to the original backend core. The new
		// file core receives the runtime identity added by the platform opener;
		// backend assertions below protect the selected invocation correlation.
		for _, field := range []string{`"msg":"scoped-file-log"`, `"runtime_instance_id":"` + sessionID + `-runtime"`} {
			if !strings.Contains(string(data), field) {
				t.Fatalf("runtime file missing emitted record identity %s: %s", field, data)
			}
		}
	}
	entries := logs.FilterMessage("scoped-file-log").All()
	if len(entries) != 2 {
		t.Fatalf("selected backend records = %d, want one per opening", len(entries))
	}
	for index, sessionID := range []string{"candidate", "peer"} {
		fields := entries[index].ContextMap()
		if fields["backend"] != "selected" || fields["session_id"] != sessionID || fields["invocation_id"] != sessionID+"-invocation" {
			t.Fatalf("backend log attribution = %#v, want %s", fields, sessionID)
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
	sink, err := owner.Open(zap.NewNop(), factoryruntime.RuntimeLogScopeRequest{
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
		time.Now, func() string { return "unwritable" }, reserver,
	)
	if err != nil {
		t.Fatalf("provideRuntimeLogOwner(): %v", err)
	}
	_, err = owner.Open(zap.NewNop(), factoryruntime.RuntimeLogScopeRequest{
		RuntimeInstanceID: "runtime-unwritable", RootDirectory: unwritable,
		Policy: factoryruntime.RuntimeFileLoggingPolicyEnabled,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime artifact") {
		t.Fatalf("unwritable log destination error = %v, want actionable runtime artifact error", err)
	}
}

func TestWatchReconnectWaitOwnsTimerAndCancellation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"elapsed", "canceled", "already canceled", "zero", "negative"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timer := &watchWaitTimer{ticks: make(chan time.Time, 1)}
			scheduler := &watchWaitScheduler{timer: timer}
			delay := time.Second
			var want error
			switch mode {
			case "elapsed":
				timer.ticks <- time.Unix(0, 0)
			case "canceled":
				scheduler.onCreate = cancel
				want = context.Canceled
			case "already canceled":
				cancel()
				want = context.Canceled
			case "zero":
				delay = 0
			case "negative":
				delay = -time.Second
			}
			wait := provideWatchReconnectWait(scheduler)
			if scheduler.calls != 0 {
				t.Fatal("construction started timer")
			}
			if err := wait(ctx, delay); !errors.Is(err, want) {
				t.Fatalf("wait error=%v want=%v", err, want)
			}
			if delay <= 0 || mode == "already canceled" {
				if scheduler.calls != 0 || timer.stopped {
					t.Fatal("wait created an unnecessary timer")
				}
			} else if scheduler.calls != 1 || scheduler.delay != delay || !timer.stopped {
				t.Fatalf("timer calls=%d delay=%v stopped=%v", scheduler.calls, scheduler.delay, timer.stopped)
			}
		})
	}
}

type watchWaitScheduler struct {
	timer    *watchWaitTimer
	calls    int
	delay    time.Duration
	onCreate func()
}

func TestWatchReconnectPendingWaitsRemainIndependent(t *testing.T) {
	t.Parallel()
	for _, cancelFirst := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelFirst), func(t *testing.T) {
			t.Parallel()
			scheduler := &pendingWatchScheduler{created: make(chan *pendingWatchTimer, 2)}
			wait := provideWatchReconnectWait(scheduler)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			firstResult, secondResult := make(chan error, 1), make(chan error, 1)
			go func() { firstResult <- wait(ctx, time.Second) }()
			first := awaitPendingWatchTimer(t, scheduler)
			go func() { secondResult <- wait(t.Context(), time.Second) }()
			second := awaitPendingWatchTimer(t, scheduler)
			select {
			case err := <-firstResult:
				t.Fatalf("wait completed before delivery: %v", err)
			default:
			}
			var want error
			if cancelFirst {
				cancel()
				want = context.Canceled
			} else {
				first.ticks <- time.Unix(1, 0)
			}
			select {
			case err := <-firstResult:
				if !errors.Is(err, want) || !first.stopped.Load() {
					t.Fatalf("first error=%v stopped=%t want=%v", err, first.stopped.Load(), want)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("pending first wait did not finish")
			}
			select {
			case err := <-secondResult:
				t.Fatalf("peer wait completed without delivery: %v", err)
			default:
			}
			second.ticks <- time.Unix(2, 0)
			select {
			case err := <-secondResult:
				if err != nil || !second.stopped.Load() {
					t.Fatalf("peer error=%v stopped=%t", err, second.stopped.Load())
				}
			case <-time.After(30 * time.Second):
				t.Fatal("pending peer wait did not finish")
			}
		})
	}
}

type pendingWatchScheduler struct{ created chan *pendingWatchTimer }

func (*pendingWatchScheduler) Now() time.Time { return time.Unix(0, 0) }
func (s *pendingWatchScheduler) After(delay time.Duration) <-chan time.Time {
	return s.NewTimer(delay).C()
}
func (s *pendingWatchScheduler) NewTimer(time.Duration) platformclock.Timer {
	timer := &pendingWatchTimer{ticks: make(chan time.Time, 1)}
	s.created <- timer
	return timer
}

type pendingWatchTimer struct {
	ticks   chan time.Time
	stopped atomic.Bool
}

func (t *pendingWatchTimer) C() <-chan time.Time { return t.ticks }
func (t *pendingWatchTimer) Stop() bool          { return !t.stopped.Swap(true) }

func awaitPendingWatchTimer(t *testing.T, s *pendingWatchScheduler) *pendingWatchTimer {
	t.Helper()
	select {
	case timer := <-s.created:
		return timer
	case <-time.After(30 * time.Second):
		t.Fatal("wait did not create timer")
		return nil
	}
}

func (*watchWaitScheduler) Now() time.Time { return time.Unix(0, 0) }
func (scheduler *watchWaitScheduler) After(delay time.Duration) <-chan time.Time {
	return scheduler.NewTimer(delay).C()
}
func (scheduler *watchWaitScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	scheduler.calls++
	scheduler.delay = delay
	if scheduler.onCreate != nil {
		scheduler.onCreate()
	}
	return scheduler.timer
}

type watchWaitTimer struct {
	ticks   chan time.Time
	stopped bool
}

func (timer *watchWaitTimer) C() <-chan time.Time { return timer.ticks }
func (timer *watchWaitTimer) Stop() bool          { timer.stopped = true; return true }

type fixtureWorkDurability struct{}

func (fixtureWorkDurability) CompletedFlushSequence(string) (int64, bool) { return 0, false }

type workDurabilityRecording struct {
	recordings.Service
	read func(string) (recordings.CanonicalEventCursor, bool)
}

func (r workDurabilityRecording) CompletedFlushWatermark(generation string) (recordings.CanonicalEventCursor, bool) {
	return r.read(generation)
}
func TestWorkDurabilityBridgeRequiresCapabilityAndPreservesWatermark(t *testing.T) {
	t.Parallel()
	if got, err := provideWorkDurabilityReader(struct{ recordings.Service }{}); got != nil || err == nil || !strings.Contains(err.Error(), "completed-flush watermark reader is required") {
		t.Fatalf("missing capability = %v, %v", got, err)
	}
	reader, err := provideWorkDurabilityReader(workDurabilityRecording{read: func(generation string) (recordings.CanonicalEventCursor, bool) {
		if generation != "recording" {
			return recordings.CanonicalEventCursor{}, false
		}
		return recordings.CanonicalEventCursor{Sequence: 42}, true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sequence, ok := reader.CompletedFlushSequence("recording"); sequence != 42 || !ok {
		t.Fatalf("completed watermark = %d, %t", sequence, ok)
	}
	if sequence, ok := reader.CompletedFlushSequence("unavailable"); sequence != 0 || ok {
		t.Fatalf("unavailable watermark = %d, %t", sequence, ok)
	}
}

type fixtureWorkPreparation func(context.Context, work.WorkRequestPreparation) (work.WorkRequest, error)

func (f fixtureWorkPreparation) PrepareWorkRequest(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
	return f(ctx, input)
}
