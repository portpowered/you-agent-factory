package wire

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
)

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}

	result, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = %#v, want non-empty migrated catalog", result)
	}
}

func TestNewServiceComposesCatalogAndExecutionWithSharedCatalogAuthority(t *testing.T) {
	t.Parallel()

	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{
		ID: providers.IDCodex,
	})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q", got.Provider.ID)
	}

	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "shared-catalog-authority",
	})
	if errors.Is(executeErr, providers.ErrUnknownProvider) {
		t.Fatalf(
			"Execute(codex) = %v, want execution bound through shared catalog authority",
			executeErr,
		)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf(
			"Execute(codex) = %#v, want dependency failure from bound adapter without effects",
			executeErr,
		)
	}
}

func TestNewServiceBuildsUsableRoot(t *testing.T) {
	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := root.ListProviders(
		context.Background(),
		providers.ListProvidersRequest{},
	)
	if err != nil || len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = (%#v, %v), want catalog entries", result, err)
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	platformRunner := &inertPlatformCommandRunner{}
	workersRunner := &inertWorkersCommandRunner{}
	cursorTempFiles := &inertTemporaryFileSystem{}
	agyAllocator := &inertPTYAllocator{}
	agyLocator := &inertExecutableLocator{}
	agyInspector := &inertPathInspector{}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := NewService(
		CatalogOption(catalogwire.WithProbeQuery(func(
			_ context.Context,
			descriptor providers.Descriptor,
		) (catalog.ProbeFacts, error) {
			probeCalls++
			return catalog.ProbeFacts{
				Readiness:     descriptor.Readiness,
				Prerequisites: descriptor.Prerequisites,
			}, nil
		})),
		WithCommandRunner(platformRunner),
		WithWorkersCommandRunner(workersRunner),
		WithCursorPlatform(CursorPlatformDependencies{
			OperatingSystem: "windows",
			TemporaryDir:    t.TempDir(),
			TemporaryFiles:  cursorTempFiles,
		}),
		WithAgyPTY(AgyPTYPlatformDependencies{
			Allocator: agyAllocator,
			Locator:   agyLocator,
			Inspector: agyInspector,
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}

	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}
	if platformRunner.calls != 0 {
		t.Fatalf("platform command runner calls = %d, want inert construction", platformRunner.calls)
	}
	if workersRunner.calls != 0 {
		t.Fatalf("workers command runner calls = %d, want inert construction", workersRunner.calls)
	}
	if cursorTempFiles.calls != 0 {
		t.Fatalf("cursor temporary filesystem calls = %d, want inert construction", cursorTempFiles.calls)
	}
	if agyAllocator.calls != 0 || agyLocator.calls != 0 || agyInspector.calls != 0 {
		t.Fatalf(
			"construction invoked Agy PTY platform effects (allocate=%d lookpath=%d stat=%d), want inert construction",
			agyAllocator.calls,
			agyLocator.calls,
			agyInspector.calls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline,
			runtime.NumGoroutine(),
			leaked,
		)
	}

	result, listErr := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if listErr != nil {
		t.Fatalf("ListProviders() = %v", listErr)
	}
	if len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = %#v, want non-empty migrated catalog after inert construction", result)
	}
}

func TestNewServiceAgyExecuteFailsClosedWithoutInjectedPTY(t *testing.T) {
	t.Parallel()

	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDAgy,
		AttemptID: "agy-without-pty-effects",
	})
	if !reflectDeepZeroExecuteResult(result) {
		t.Fatalf("Execute(agy) result = %#v, want zero result on dependency failure", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency ||
		!strings.Contains(failure.Message, "Agy") {
		t.Fatalf(
			"Execute(agy) error = %#v, want Agy dependency-normalized failure without injected PTY effects",
			executeErr,
		)
	}
}

func TestNewServiceInjectsPlatformDependenciesThroughWireOptions(t *testing.T) {
	t.Parallel()

	cursorTempFiles := newRecordingTemporaryFileSystem(`C:\cursor-temp\cursor_prompt_fixture.md`)
	workersRunner := &recordingWorkersCommandRunner{}
	agyAllocator := &recordingPTYAllocator{
		result: agypty.SessionResult{ExitCode: 0, CleanedText: "agy via wire"},
	}
	agyPath := filepath.Join(t.TempDir(), "agy")
	agyLocator := fakeExecutableLocator{string(providers.IDAgy): agyPath}
	agyInspector := fakeExecutableInspector{agyPath: fakeExecutableInfo{directory: false}}

	root, err := NewService(
		WithWorkersCommandRunner(workersRunner),
		WithCursorPlatform(CursorPlatformDependencies{
			OperatingSystem: "windows",
			TemporaryDir:    `C:\cursor-temp`,
			TemporaryFiles:  cursorTempFiles,
		}),
		WithAgyPTY(AgyPTYPlatformDependencies{
			Allocator: agyAllocator,
			Locator:   agyLocator,
			Inspector: agyInspector,
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if cursorTempFiles.created != 0 || workersRunner.calls != 0 || agyAllocator.calls != 0 {
		t.Fatalf(
			"construction invoked platform effects (cursor create=%d runner=%d agy allocate=%d), want inert construction",
			cursorTempFiles.created,
			workersRunner.calls,
			agyAllocator.calls,
		)
	}

	oversizedPrompt := strings.Repeat("x", 8*1024)
	_, cursorErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCursor,
		AttemptID:   "cursor-platform-injection",
		UserMessage: oversizedPrompt,
	})
	if cursorErr == nil {
		t.Fatal("Execute(cursor) error = nil, want failure after oversized prompt materialization")
	}
	if cursorTempFiles.created != 1 {
		t.Fatalf("cursor temporary creates = %d, want injected Cursor platform used on execute", cursorTempFiles.created)
	}
	if cursorTempFiles.file.content != oversizedPrompt {
		t.Fatalf("cursor prompt content = %q, want oversized prompt written through injected platform", cursorTempFiles.file.content)
	}
	if workersRunner.calls != 1 {
		t.Fatalf("workers runner calls = %d, want command dispatch after injected Cursor platform", workersRunner.calls)
	}

	agyResult, agyErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "agy-platform-injection",
		WorkingDirectory: t.TempDir(),
		UserMessage:      "hello through wire",
	})
	if agyErr != nil {
		t.Fatalf("Execute(agy) error = %v, want success with injected PTY platform", agyErr)
	}
	if agyAllocator.calls != 1 {
		t.Fatalf("agy allocator calls = %d, want injected Agy PTY platform used on execute", agyAllocator.calls)
	}
	if agyResult.Content != "agy via wire" {
		t.Fatalf("Execute(agy) content = %q, want injected allocator output", agyResult.Content)
	}
}

func TestNewServiceBindsCodexAndClaudeFromCatalogWithoutEffects(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	root, err := NewService(CatalogOption(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}

	for _, test := range []struct {
		id   providers.ID
		name string
	}{
		{id: providers.IDCodex, name: "Codex"},
		{id: providers.IDClaude, name: "Claude"},
	} {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  test.id,
			AttemptID: "composition-attempt",
		})
		var failure providers.ExecuteFailure
		if !errors.As(executeErr, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency ||
			!strings.Contains(failure.Message, test.name) {
			t.Fatalf(
				"Execute(%q) error = %#v, want matching private adapter",
				test.id,
				executeErr,
			)
		}
	}
	if probeCalls != 2 {
		t.Fatalf("execution probe calls = %d, want one per explicit selection", probeCalls)
	}
}

func TestNewRootRejectsMissingCatalog(t *testing.T) {
	root, err := newRoot(nil, nil, nil, CursorPlatformDependencies{}, AgyPTYPlatformDependencies{})
	if err == nil || root != nil {
		t.Fatalf("newRoot(nil) = (%v, %v), want construction error", root, err)
	}
}

type inertPlatformCommandRunner struct {
	calls int
}

func (r *inertPlatformCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	panic("platform command runner invoked during inert construction")
}

type inertWorkersCommandRunner struct {
	calls int
}

func (r *inertWorkersCommandRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	r.calls++
	panic("workers command runner invoked during inert construction")
}

type inertTemporaryFileSystem struct {
	calls int
}

func (f *inertTemporaryFileSystem) CreateTemp(string, string) (platformfilesystem.TemporaryFile, error) {
	f.calls++
	panic("cursor temporary file creation during inert construction")
}

func (f *inertTemporaryFileSystem) Remove(string) error {
	f.calls++
	panic("cursor temporary file remove during inert construction")
}

type inertPTYAllocator struct {
	calls int
}

func (a *inertPTYAllocator) Allocate(
	_ context.Context,
	_ agypty.ProcessLaunch,
	_ agypty.SessionConfig,
) (agypty.PTYSession, error) {
	a.calls++
	panic("agy PTY allocation during inert construction")
}

type inertExecutableLocator struct {
	calls int
}

func (l *inertExecutableLocator) LookPath(string) (string, error) {
	l.calls++
	panic("agy executable lookup during inert construction")
}

type inertPathInspector struct {
	calls int
}

func (i *inertPathInspector) Stat(string) (fs.FileInfo, error) {
	i.calls++
	panic("agy path inspect during inert construction")
}

func reflectDeepZeroExecuteResult(result providers.ExecuteResult) bool {
	return result == providers.ExecuteResult{}
}

type recordingWorkersCommandRunner struct {
	calls int
}

func (r *recordingWorkersCommandRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	r.calls++
	return workers.CommandResult{}, nil
}

type recordingTemporaryFileSystem struct {
	mu        sync.Mutex
	file      *recordingTemporaryFile
	created   int
	directory string
	pattern   string
	removes   int
}

func newRecordingTemporaryFileSystem(path string) *recordingTemporaryFileSystem {
	return &recordingTemporaryFileSystem{
		file: &recordingTemporaryFile{path: path},
	}
}

func (f *recordingTemporaryFileSystem) CreateTemp(directory, pattern string) (platformfilesystem.TemporaryFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	f.directory = directory
	f.pattern = pattern
	return f.file, nil
}

func (f *recordingTemporaryFileSystem) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	return nil
}

type recordingTemporaryFile struct {
	mu      sync.Mutex
	path    string
	content string
	closes  int
}

func (f *recordingTemporaryFile) Name() string { return f.path }

func (f *recordingTemporaryFile) WriteString(value string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = value
	return len(value), nil
}

func (f *recordingTemporaryFile) Close() error {
	f.mu.Lock()
	f.closes++
	f.mu.Unlock()
	return nil
}

type recordingPTYAllocator struct {
	calls  int
	result agypty.SessionResult
}

func (a *recordingPTYAllocator) Allocate(
	_ context.Context,
	_ agypty.ProcessLaunch,
	_ agypty.SessionConfig,
) (agypty.PTYSession, error) {
	a.calls++
	return &recordingPTYSession{result: a.result}, nil
}

type recordingPTYSession struct {
	result agypty.SessionResult
}

func (s *recordingPTYSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *recordingPTYSession) Close() error { return nil }

type fakeExecutableLocator map[string]string

func (l fakeExecutableLocator) LookPath(name string) (string, error) {
	if path, ok := l[name]; ok {
		return path, nil
	}
	return "", fs.ErrNotExist
}

type fakeExecutableInspector map[string]fakeExecutableInfo

func (i fakeExecutableInspector) Stat(path string) (fs.FileInfo, error) {
	if info, ok := i[path]; ok {
		return info, nil
	}
	return nil, fs.ErrNotExist
}

type fakeExecutableInfo struct {
	directory bool
}

func (i fakeExecutableInfo) Name() string       { return "agy" }
func (i fakeExecutableInfo) Size() int64        { return 0 }
func (i fakeExecutableInfo) Mode() fs.FileMode  { return 0o755 }
func (i fakeExecutableInfo) ModTime() time.Time { return time.Time{} }
func (i fakeExecutableInfo) IsDir() bool        { return i.directory }
func (i fakeExecutableInfo) Sys() any           { return nil }
