package wire

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"strings"
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
