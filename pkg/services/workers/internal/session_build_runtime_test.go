package internal

import (
	"context"
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
