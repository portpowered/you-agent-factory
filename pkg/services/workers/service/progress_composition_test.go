package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"go.uber.org/zap"
)

var testRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

func TestNewRequiresCompositionSelectedWorkerEffects(t *testing.T) {
	base := func(provider, script workers.CommandRunner, allocator agypty.PTYAllocator, logger *zap.Logger, now func() time.Time) error {
		_, err := New(
			inertCurrentRuntimeResolver{}, testModelsService{}, provider, script, allocator,
			logger, false, "", nil, nil, now, os.Environ, os.Getwd, nil, nil, nil, nil,
			testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
			testRetryRandom,
			platformfilesystem.Local{},
			platformfilesystem.Local{},
		)
		return err
	}
	validRunner := injectedProviderRunner{}
	validAllocator := &agypty.MockAllocator{}
	for _, testCase := range []struct {
		name string
		err  error
		want string
	}{
		{name: "provider runner", err: base(nil, validRunner, validAllocator, zap.NewNop(), time.Now), want: "provider command runner is required"},
		{name: "script runner", err: base(validRunner, nil, validAllocator, zap.NewNop(), time.Now), want: "script command runner is required"},
		{name: "PTY allocator", err: base(validRunner, validRunner, nil, zap.NewNop(), time.Now), want: "Agy PTY allocator is required"},
		{name: "logger", err: base(validRunner, validRunner, validAllocator, nil, time.Now), want: "logger is required"},
		{name: "clock", err: base(validRunner, validRunner, validAllocator, zap.NewNop(), nil), want: "clock is required"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.err == nil || !strings.Contains(testCase.err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", testCase.err, testCase.want)
			}
		})
	}
	_, err := New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "worktree preparer is required") {
		t.Fatalf("missing worktree preparer error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, nil,
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "agent-run harness is required") {
		t.Fatalf("missing agent-run harness error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		nil,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "provider retry random source is required") {
		t.Fatalf("missing retry random source error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		nil,
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "workstation filesystem is required") {
		t.Fatalf("missing workstation filesystem error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "provider temporary filesystem is required") {
		t.Fatalf("missing provider temporary filesystem error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, validRunner, validRunner, validAllocator,
		zap.NewNop(), false, "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, nil, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "executable path inspector is required") {
		t.Fatalf("missing executable path inspector error = %v", err)
	}
}

func TestNewInvocationRequiresCompositionSelectedWorkerEffects(t *testing.T) {
	clock := workerprocess.ClockFunc(time.Now)
	if _, err := NewInvocation(nil, clock, &agypty.MockAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command runner is required") {
		t.Fatalf("missing command runner error = %v", err)
	}
	if _, err := NewInvocation(injectedProviderRunner{}, clock, nil, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("missing allocator error = %v", err)
	}
	if _, err := NewInvocation(injectedProviderRunner{}, nil, &agypty.MockAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command clock is required") {
		t.Fatalf("missing command clock error = %v", err)
	}
	if _, err := NewInvocation(injectedProviderRunner{}, clock, &agypty.MockAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, nil, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "executable path inspector is required") {
		t.Fatalf("missing executable path inspector error = %v", err)
	}
	if _, err := NewInvocation(injectedProviderRunner{}, clock, &agypty.MockAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "temporary filesystem is required") {
		t.Fatalf("missing temporary filesystem error = %v", err)
	}
}

func testWorkerService(t *testing.T, providerRunner workers.CommandRunner) *Service {
	t.Helper()
	if providerRunner == nil {
		providerRunner = injectedProviderRunner{}
	}
	service, err := New(
		inertCurrentRuntimeResolver{},
		testModelsService{},
		providerRunner,
		injectedProviderRunner{},
		&agypty.MockAllocator{},
		zap.NewNop(),
		false,
		"",
		nil,
		nil,
		time.Now,
		os.Environ,
		os.Getwd,
		nil,
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testResolveSymlinks,
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("construct Worker service: %v", err)
	}
	return service
}

type inertCurrentRuntimeResolver struct{}

func (inertCurrentRuntimeResolver) CurrentRuntime() *factorysessions.LiveRuntime {
	return nil
}

type testModelsService struct{}

func (s testModelsService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return s, nil
}

type injectedProviderRunner struct{}

type testFactoryWorktreePreparer struct{}

func (testFactoryWorktreePreparer) Prepare(context.Context, string, string) (workers.FactoryWorktreePreparation, error) {
	return workers.FactoryWorktreePreparation{}, nil
}

func testFactoryDocsLoader(string) (map[string]string, error) { return map[string]string{}, nil }

func testResolveSymlinks(path string) (string, error) { return path, nil }

func (injectedProviderRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (testModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{}, nil
}
func (testModelsService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, nil
}
func (testModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (testModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, nil
}

func (testModelsService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, nil
}

func TestWithProgressInstallsDefaultRunner(t *testing.T) {
	t.Parallel()

	got, err := testWorkerService(t, nil).WithProgressPublisher(
		nil,
		func(workers.ProgressFragment) {},
		true,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("WithProgress() error = %v", err)
	}
	if !got.ProviderCommandInjected() {
		t.Fatal("WithProgress() did not install the progress-publishing provider runner")
	}
}

func TestWithProgressPreservesInjectedProviderRunner(t *testing.T) {
	t.Parallel()

	runner := injectedProviderRunner{}
	service := testWorkerService(t, runner)
	got, err := service.WithProgressPublisher(
		nil,
		func(workers.ProgressFragment) {},
		true,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("WithProgress() error = %v", err)
	}
	if got != service {
		t.Fatal("WithProgress() replaced the composition-selected provider runner")
	}
}
