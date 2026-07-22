package wire

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"go.uber.org/zap"
)

type recordingStdioOpening struct {
	request factorysessions.StdioOpeningRequest
	result  factorysessions.StdioApplication
}

func (opening *recordingStdioOpening) OpenStdio(
	_ context.Context,
	request factorysessions.StdioOpeningRequest,
) (factorysessions.StdioApplication, error) {
	opening.request = request
	return opening.result, nil
}

type testStdioApplication struct{}

func (testStdioApplication) Run(context.Context) error { return nil }

func TestStdioApplicationOpenerMapsOnlyInvocationEdgeValues(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("request")
	output := &strings.Builder{}
	opening := &recordingStdioOpening{result: testStdioApplication{}}
	adapter, err := provideStdioApplicationOpener(opening)
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
	if application != opening.result {
		t.Fatal("OpenStdio() did not return the exact lifecycle-ready application")
	}
	request := opening.request
	if request.FixtureCatalogPath != "fixtures.json" || !request.RuntimeBacked ||
		request.ProjectRoot != "/project" || request.SystemConfigHome != "/home" ||
		request.Input != input || request.Output != output {
		t.Fatalf("mapped stdio opening request = %#v", request)
	}
}

func TestStdioApplicationOpenerRequiresOwnerOperation(t *testing.T) {
	t.Parallel()

	adapter, err := provideStdioApplicationOpener(nil)
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
	createTemporaryFile := factorysessions.CursorPersistenceCreateTemporaryFile(func(string, string) (factorysessions.CursorPersistenceTemporaryFile, error) {
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
	if _, ok := files.(platformfilesystem.Local); !ok {
		t.Fatalf("default runtime persistence filesystem = %T, want policy-free local adapter", files)
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

func TestRuntimeOpeningRequestFactoryMapsSelectionsIntoOwnerRequests(t *testing.T) {
	t.Parallel()
	skip := true
	mocks := workers.NewEmptyMockWorkersConfig()
	opening := provideRuntimeOpeningRequestFactory()(runcli.RunConfig{
		Dir: "factory", ExecutionBaseDir: "execution", RunnerID: "runner",
		HomeDir: "home", WorkFile: "work.json", Port: 8080, AutoPort: true,
		Continuously: true, Verbose: true, RecordPath: "record.json",
		ReplayPath: "replay.json", Workflow: "flow", ModelCacheDir: "models",
		InvocationSkipPermissionsOverride: &skip,
	}, mocks, func(factorysessions.RuntimeHostBinding) {})
	request := opening.Runtime

	if request.FactoryDefinition.Directory != "factory" || request.FactoryDefinition.ExecutionBaseDir != "execution" {
		t.Fatalf("Factory Definition request = %#v", request.FactoryDefinition)
	}
	if request.FactoryRuntime.Mode != factorydefinitions.RuntimeModeService || !request.FactoryRuntime.Verbose {
		t.Fatalf("Factory Runtime request = %#v", request.FactoryRuntime)
	}
	if request.FactorySession.SystemConfigHome != "home" || request.FactorySession.Host.Port != 8080 || !request.FactorySession.Host.AutoPort {
		t.Fatalf("Factory Session request = %#v", request.FactorySession)
	}
	if request.Workers.RunnerID != "runner" || request.Workers.MockWorkers != mocks || request.Workers.InvocationSkipPermissionsOverride != &skip {
		t.Fatalf("Workers request = %#v", request.Workers)
	}
	if request.Recordings.RecordPath != "record.json" || request.Recordings.ReplayPath != "replay.json" || request.Recordings.WorkflowID != "flow" {
		t.Fatalf("Recordings request = %#v", request.Recordings)
	}
	if request.Models.CacheDirectory != "models" || opening.Ports.InvocationMetricsRecorder != nil || opening.Ports.RuntimeHostObserver == nil {
		t.Fatalf("Models/ports = %#v / %#v", request.Models, opening.Ports)
	}
}

func TestRuntimeInputResolverMergesEdgesAndDetachesAggregate(t *testing.T) {
	t.Parallel()
	defaultClock := platformclock.Real{}
	defaultMetrics := &runtimeInputMetricsRecorder{}
	invocationMetrics := &runtimeInputMetricsRecorder{}
	invocationObserver := factorysessions.RuntimeHostObserver(func(factorysessions.RuntimeHostBinding) {})
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "factory"},
	}
	resolver := provideRuntimeInputResolver(
		serviceedges.Edges{Clock: defaultClock, InvocationMetricsRecorder: defaultMetrics}, provideFactoryRuntimeClockResolver(),
	)
	resolved, err := resolver(t.Context(), request, factorysessions.ApplicationOpeningPorts{
		InvocationMetricsRecorder: invocationMetrics,
		RuntimeHostObserver:       invocationObserver,
	}, zap.NewNop())
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
	if resolved.Edges.Clock == nil || resolved.Logger == nil {
		t.Fatalf("resolved edges/logger = %#v / %#v", resolved.Edges, resolved.Logger)
	}
	if resolved.Edges.InvocationMetricsRecorder != invocationMetrics || resolved.Edges.RuntimeHostObserver == nil {
		t.Fatalf("resolved invocation ports = %#v, want exact invocation replacements", resolved.Edges)
	}
}

type runtimeInputMetricsRecorder struct{}

func (*runtimeInputMetricsRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {}

func TestRuntimeInputResolverRejectsMissingRequiredInputs(t *testing.T) {
	t.Parallel()
	request := &factorysessions.RuntimeOpeningRequest{}
	validEdges := serviceedges.Edges{Clock: platformclock.Real{}}
	tests := []struct {
		name    string
		ctx     context.Context
		request *factorysessions.RuntimeOpeningRequest
		edges   serviceedges.Edges
		logger  *zap.Logger
		want    string
	}{
		{name: "nil context", request: request, edges: validEdges, logger: zap.NewNop(), want: "context is required"},
		{name: "nil request", ctx: t.Context(), edges: validEdges, logger: zap.NewNop(), want: "runtime opening request is required"},
		{name: "nil logger", ctx: t.Context(), request: request, edges: validEdges, want: "runtime logger is required"},
		{name: "nil clock", ctx: t.Context(), request: request, logger: zap.NewNop(), want: "runtime clock edge is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := provideRuntimeInputResolver(test.edges, func(clock factoryruntime.Clock) factoryruntime.Clock {
				return clock
			})(test.ctx, test.request, factorysessions.ApplicationOpeningPorts{}, test.logger)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
