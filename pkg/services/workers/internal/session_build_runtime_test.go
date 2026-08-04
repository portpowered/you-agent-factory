package internal

import (
	"context"
	"errors"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// lifecycleTrackedProvidersService is a providers.Service fake that also
// implements providers.Lifecycle, so tests can prove whether a Close call
// reached a specific per-session-build rebound Providers instance.
type lifecycleTrackedProvidersService struct {
	testProvidersService
	closed *int
}

func (l lifecycleTrackedProvidersService) Close(context.Context) error {
	*l.closed++
	return nil
}

// TestNewSessionBuildRuntimeConstructsIndependentInstance proves
// newSessionBuildRuntime -- the sole surviving construction path for
// per-session-build runner/publisher selection -- builds a genuinely
// independent Service with its own final dependencies rather than mutating
// base's already-constructed dependencies in place.
func TestNewSessionBuildRuntimeConstructsIndependentInstance(t *testing.T) {
	t.Parallel()

	baseProviderRunner := taggedCommandRunner{tag: "base-provider"}
	buildProviderRunner := taggedCommandRunner{tag: "build-provider"}
	baseScriptRunner := taggedCommandRunner{tag: "base-script"}
	buildScriptRunner := taggedCommandRunner{tag: "build-script"}
	basePublisher, baseCalls := recordingProgressPublisher()
	buildPublisher, buildCalls := recordingProgressPublisher()

	base := newTestServiceWithDependencies(t, baseProviderRunner, baseScriptRunner, basePublisher, zap.NewNop())

	built, err := base.newSessionBuildRuntime(buildProviderRunner, buildScriptRunner, buildPublisher)
	if err != nil {
		t.Fatalf("newSessionBuildRuntime() error = %v", err)
	}
	if built == base {
		t.Fatal("newSessionBuildRuntime() returned base itself, want a new independent instance")
	}

	if built.ProviderCommandRunner() != workers.CommandRunner(buildProviderRunner) {
		t.Fatalf("built.ProviderCommandRunner() = %#v, want the supplied build-time instance %#v", built.ProviderCommandRunner(), buildProviderRunner)
	}
	if built.ScriptCommandRunner() != workers.CommandRunner(buildScriptRunner) {
		t.Fatalf("built.ScriptCommandRunner() = %#v, want the supplied build-time instance %#v", built.ScriptCommandRunner(), buildScriptRunner)
	}
	built.progressPublisher(workers.ProgressFragment{})
	if *buildCalls != 1 {
		t.Fatalf("built runtime's progress publisher calls = %d, want 1", *buildCalls)
	}
	if *baseCalls != 0 {
		t.Fatalf("base runtime's progress publisher observed %d calls from the built runtime's activity, want 0", *baseCalls)
	}

	// base's own construction-time dependencies must be untouched: no
	// supported operation replaces an already-constructed runtime's
	// dependencies, including through the session-build seam.
	if base.ProviderCommandRunner() != workers.CommandRunner(baseProviderRunner) {
		t.Fatalf("base.ProviderCommandRunner() = %#v, want unchanged base instance %#v", base.ProviderCommandRunner(), baseProviderRunner)
	}
	if base.ScriptCommandRunner() != workers.CommandRunner(baseScriptRunner) {
		t.Fatalf("base.ScriptCommandRunner() = %#v, want unchanged base instance %#v", base.ScriptCommandRunner(), baseScriptRunner)
	}
}

// TestNewSessionBuildRuntimeNilArgumentsPreserveBaseDependencies proves nil
// runner/publisher arguments to newSessionBuildRuntime preserve base's own
// values on the freshly constructed runtime, rather than requiring every
// caller to repeat them.
func TestNewSessionBuildRuntimeNilArgumentsPreserveBaseDependencies(t *testing.T) {
	t.Parallel()

	baseProviderRunner := taggedCommandRunner{tag: "base-provider"}
	baseScriptRunner := taggedCommandRunner{tag: "base-script"}
	basePublisher, baseCalls := recordingProgressPublisher()

	base := newTestServiceWithDependencies(t, baseProviderRunner, baseScriptRunner, basePublisher, zap.NewNop())

	built, err := base.newSessionBuildRuntime(nil, nil, nil)
	if err != nil {
		t.Fatalf("newSessionBuildRuntime() error = %v", err)
	}

	if built.ProviderCommandRunner() != workers.CommandRunner(baseProviderRunner) {
		t.Fatalf("built.ProviderCommandRunner() = %#v, want base's instance %#v", built.ProviderCommandRunner(), baseProviderRunner)
	}
	if built.ScriptCommandRunner() != workers.CommandRunner(baseScriptRunner) {
		t.Fatalf("built.ScriptCommandRunner() = %#v, want base's instance %#v", built.ScriptCommandRunner(), baseScriptRunner)
	}
	built.progressPublisher(workers.ProgressFragment{})
	if *baseCalls != 1 {
		t.Fatalf("base's progress publisher calls = %d, want 1 (base's publisher should carry over unchanged)", *baseCalls)
	}
}

// TestNewSessionBuildRuntimeSharesProviderLifecycleSink proves a Providers
// service freshly rebound for one session-build closes together with every
// other session-build's rebound Providers service, exactly once, when base
// closes -- the same accumulate-and-close-once semantics the removed
// clone-based rebuiltForBuild seam relied on via its shared providerLifecycles
// pointer, now reached through independent construction instead of mutation.
func TestNewSessionBuildRuntimeSharesProviderLifecycleSink(t *testing.T) {
	t.Parallel()

	base := newTestServiceWithDependencies(t, injectedProviderRunner{}, injectedProviderRunner{}, workers.ProgressPublisher(testProgressPublisher), zap.NewNop())
	base.providerLifecycles = &ownedProviderLifecycles{}

	closedFirst := new(int)
	closedSecond := new(int)
	base.providerRegistryRebinder = func(runner workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		tag := runner.(taggedCommandRunner).tag
		switch tag {
		case "first-build":
			return nil, lifecycleTrackedProvidersService{closed: closedFirst}, nil
		case "second-build":
			return nil, lifecycleTrackedProvidersService{closed: closedSecond}, nil
		default:
			t.Fatalf("unexpected rebind runner tag %q", tag)
			return nil, nil, nil
		}
	}
	// rebindProviderRegistry only invokes the rebinder when there is a
	// current registry to rebind from.
	base.providerRegistry = fakeProviderRegistry{}

	firstBuild, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "first-build"}, nil, nil)
	if err != nil {
		t.Fatalf("newSessionBuildRuntime(first) error = %v", err)
	}
	secondBuild, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "second-build"}, nil, nil)
	if err != nil {
		t.Fatalf("newSessionBuildRuntime(second) error = %v", err)
	}

	if firstBuild.providerLifecycles != base.providerLifecycles {
		t.Fatal("first session-build runtime does not share base's provider lifecycle sink")
	}
	if secondBuild.providerLifecycles != base.providerLifecycles {
		t.Fatal("second session-build runtime does not share base's provider lifecycle sink")
	}

	if err := base.Close(context.Background()); err != nil {
		t.Fatalf("base.Close() error = %v", err)
	}
	if *closedFirst != 1 {
		t.Fatalf("first session-build's rebound Providers service closed %d times, want 1", *closedFirst)
	}
	if *closedSecond != 1 {
		t.Fatalf("second session-build's rebound Providers service closed %d times, want 1", *closedSecond)
	}

	// Closing either the base runtime or a session-build runtime again must
	// not double-close the shared Providers instances.
	if err := firstBuild.Close(context.Background()); err != nil {
		t.Fatalf("firstBuild.Close() error = %v", err)
	}
	if *closedFirst != 1 {
		t.Fatalf("first session-build's rebound Providers service closed %d times after repeat Close, want 1", *closedFirst)
	}
}

// TestNewSessionBuildRuntimeReleasesReboundProvidersWhenBaseClosesBeforeAdoption
// proves the deterministic fix for the post-merge finding on this seam: if
// base.Close races ahead of adoption -- closing the shared provider lifecycle
// sink after a Providers instance has been freshly rebound for this build but
// before newSessionBuildRuntime registers it -- the rebound instance is
// released immediately instead of leaked, and no runtime backed by the
// already-closed sink is ever returned. Synchronization is via unbuffered
// channels (no sleeps): the fake rebinder blocks until signalled, letting the
// test deterministically land base.Close() in the exact window between
// rebind and adoption.
func TestNewSessionBuildRuntimeReleasesReboundProvidersWhenBaseClosesBeforeAdoption(t *testing.T) {
	t.Parallel()

	base := newTestServiceWithDependencies(t, injectedProviderRunner{}, injectedProviderRunner{}, workers.ProgressPublisher(testProgressPublisher), zap.NewNop())
	base.providerLifecycles = &ownedProviderLifecycles{}
	base.providerRegistry = fakeProviderRegistry{}

	closed := new(int)
	rebinderEntered := make(chan struct{})
	proceed := make(chan struct{})
	base.providerRegistryRebinder = func(workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		close(rebinderEntered)
		<-proceed
		return nil, lifecycleTrackedProvidersService{closed: closed}, nil
	}

	type buildResult struct {
		runtime *Service
		err     error
	}
	resultCh := make(chan buildResult, 1)
	go func() {
		runtime, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "blocked-build"}, nil, nil)
		resultCh <- buildResult{runtime: runtime, err: err}
	}()

	<-rebinderEntered
	if err := base.Close(context.Background()); err != nil {
		t.Fatalf("base.Close() error = %v", err)
	}
	close(proceed)

	result := <-resultCh
	if result.err == nil {
		t.Fatal("newSessionBuildRuntime() error = nil, want an error because base already closed before adoption")
	}
	if result.runtime != nil {
		t.Fatal("newSessionBuildRuntime() returned a non-nil runtime backed by an already-closed provider lifecycle sink")
	}
	if *closed != 1 {
		t.Fatalf("rebound Providers service closed %d times, want exactly 1 (released instead of leaked)", *closed)
	}
}

// TestNewSessionBuildRuntimeReleasesReboundProvidersWhenConstructionFailsAfterRebind
// proves the other half of the post-merge finding: a construction failure
// that happens after a Providers instance was already rebound (here, forced
// by clearing base's required Factory docs loader) still releases that
// rebound instance instead of leaking it.
func TestNewSessionBuildRuntimeReleasesReboundProvidersWhenConstructionFailsAfterRebind(t *testing.T) {
	t.Parallel()

	base := newTestServiceWithDependencies(t, injectedProviderRunner{}, injectedProviderRunner{}, workers.ProgressPublisher(testProgressPublisher), zap.NewNop())
	base.providerLifecycles = &ownedProviderLifecycles{}
	base.providerRegistry = fakeProviderRegistry{}
	base.factoryDocs = nil

	closed := new(int)
	base.providerRegistryRebinder = func(workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		return nil, lifecycleTrackedProvidersService{closed: closed}, nil
	}

	runtime, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "build"}, nil, nil)
	if err == nil {
		t.Fatal("newSessionBuildRuntime() error = nil, want a construction error (nil Factory docs loader)")
	}
	if runtime != nil {
		t.Fatal("newSessionBuildRuntime() returned a non-nil runtime after a construction failure")
	}
	if *closed != 1 {
		t.Fatalf("rebound Providers service closed %d times after a construction failure, want exactly 1 (released instead of leaked)", *closed)
	}
}

// failingCloseProvidersService is a providers.Service fake that also
// implements providers.Lifecycle, whose Close always fails with a sentinel
// error. Tests use it to prove a fallback release failure is preserved and
// observable on the caller's returned error, rather than silently discarded.
type failingCloseProvidersService struct {
	testProvidersService
}

var errSentinelLifecycleCloseFailure = errors.New("sentinel: lifecycle close failed")

func (failingCloseProvidersService) Close(context.Context) error {
	return errSentinelLifecycleCloseFailure
}

// TestNewSessionBuildRuntimePreservesFallbackReleaseErrorAfterConstructionFailure
// proves the post-merge blocking finding is fixed: when a rebound Providers
// instance's fallback Close (triggered because construction failed after the
// rebind) itself fails, that failure is joined onto the returned error
// instead of being discarded with `_ = lifecycle.Close(...)`.
func TestNewSessionBuildRuntimePreservesFallbackReleaseErrorAfterConstructionFailure(t *testing.T) {
	t.Parallel()

	base := newTestServiceWithDependencies(t, injectedProviderRunner{}, injectedProviderRunner{}, workers.ProgressPublisher(testProgressPublisher), zap.NewNop())
	base.providerLifecycles = &ownedProviderLifecycles{}
	base.providerRegistry = fakeProviderRegistry{}
	base.factoryDocs = nil

	base.providerRegistryRebinder = func(workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		return nil, failingCloseProvidersService{}, nil
	}

	runtime, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "build"}, nil, nil)
	if err == nil {
		t.Fatal("newSessionBuildRuntime() error = nil, want a construction error (nil Factory docs loader) joined with the fallback release failure")
	}
	if runtime != nil {
		t.Fatal("newSessionBuildRuntime() returned a non-nil runtime after a construction failure")
	}
	if !errors.Is(err, errSentinelLifecycleCloseFailure) {
		t.Fatalf("newSessionBuildRuntime() error = %v, want it to preserve the fallback release's sentinel error via errors.Is", err)
	}
}

// TestNewSessionBuildRuntimePreservesFallbackReleaseErrorWhenBaseClosesBeforeAdoption
// proves the other caller of releaseUnadoptedProviders -- the close-before-
// adoption race -- also preserves a fallback release failure instead of
// discarding it.
func TestNewSessionBuildRuntimePreservesFallbackReleaseErrorWhenBaseClosesBeforeAdoption(t *testing.T) {
	t.Parallel()

	base := newTestServiceWithDependencies(t, injectedProviderRunner{}, injectedProviderRunner{}, workers.ProgressPublisher(testProgressPublisher), zap.NewNop())
	base.providerLifecycles = &ownedProviderLifecycles{}
	base.providerRegistry = fakeProviderRegistry{}

	rebinderEntered := make(chan struct{})
	proceed := make(chan struct{})
	base.providerRegistryRebinder = func(workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		close(rebinderEntered)
		<-proceed
		return nil, failingCloseProvidersService{}, nil
	}

	type buildResult struct {
		runtime *Service
		err     error
	}
	resultCh := make(chan buildResult, 1)
	go func() {
		runtime, err := base.newSessionBuildRuntime(taggedCommandRunner{tag: "blocked-build"}, nil, nil)
		resultCh <- buildResult{runtime: runtime, err: err}
	}()

	<-rebinderEntered
	if err := base.Close(context.Background()); err != nil {
		t.Fatalf("base.Close() error = %v", err)
	}
	close(proceed)

	result := <-resultCh
	if result.err == nil {
		t.Fatal("newSessionBuildRuntime() error = nil, want an error because base already closed before adoption")
	}
	if result.runtime != nil {
		t.Fatal("newSessionBuildRuntime() returned a non-nil runtime backed by an already-closed provider lifecycle sink")
	}
	if !errors.Is(result.err, errSentinelLifecycleCloseFailure) {
		t.Fatalf("newSessionBuildRuntime() error = %v, want it to preserve the fallback release's sentinel error via errors.Is", result.err)
	}
}

// fakeProviderRegistry is a minimal, non-nil workers.ProviderRegistry
// implementation with no registered runners, used only to make
// rebindProviderRegistry treat base as already having a current registry to
// rebind from.
type fakeProviderRegistry struct{}

func (fakeProviderRegistry) UsesNativeRunner(string) bool { return false }
func (fakeProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}
func (fakeProviderRegistry) RunnerIdentities() []string { return nil }
func (fakeProviderRegistry) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	return workers.RunnerMetadata{}, nil
}
func (fakeProviderRegistry) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
}
func (fakeProviderRegistry) ResolveRunnerSelection(string, string, string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolvedRunnerSelection{}, nil
}
