package internal

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"go.uber.org/zap"
)

// concurrencyFanout is how many goroutines dispatch concurrently per round in
// TestConcurrentRepeatedExecutionsRetainConstructionInjectedDependencies.
const concurrencyFanout = 20

// concurrencyRepeatRounds is the documented repeat count: the whole
// concurrent fan-out is repeated this many times against the same
// constructed runtime to prove determinism holds under reuse, not just once.
const concurrencyRepeatRounds = 5

// recordingDispatchPublisher is a mutex-guarded ProgressPublisher that groups
// delivered fragments by DispatchID, so a concurrency test can prove no
// fragment crosses from one request's dispatch into another's and none is
// delivered more than the expected number of times.
type recordingDispatchPublisher struct {
	mu        sync.Mutex
	fragments map[string][]workers.ProgressFragment
}

func newRecordingDispatchPublisher() *recordingDispatchPublisher {
	return &recordingDispatchPublisher{fragments: make(map[string][]workers.ProgressFragment)}
}

func (r *recordingDispatchPublisher) publish(fragment workers.ProgressFragment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fragments[fragment.DispatchID] = append(r.fragments[fragment.DispatchID], cloneServiceProgressFragment(fragment))
}

func (r *recordingDispatchPublisher) snapshot() map[string][]workers.ProgressFragment {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][]workers.ProgressFragment, len(r.fragments))
	for id, fragments := range r.fragments {
		out[id] = append([]workers.ProgressFragment(nil), fragments...)
	}
	return out
}

// newTestServiceWithDependenciesAndProviders mirrors newTestServiceWithDependencies
// but additionally accepts a custom providers.Service, so a concurrency test
// can exercise a fully New()-constructed *Service (with real, comparable
// provider/script command runner identities) through the same deterministic
// providers fake the execution-regression tests already rely on.
func newTestServiceWithDependenciesAndProviders(
	t *testing.T,
	providerRunner, scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	providersService providers.Service,
) *Service {
	t.Helper()
	service, err := New(
		inertCurrentRuntimeResolver{},
		testModelsService{},
		providersService,
		providerRunner,
		scriptRunner,
		progressPublisher,
		&workers.MockPTYAllocator{},
		zap.NewNop(),
		false,
		"",
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

// TestConcurrentRepeatedExecutionsRetainConstructionInjectedDependencies
// proves concurrent and repeated Worker execution on one constructed runtime
// always reaches the exact provider runner, script runner, and progress
// publisher supplied at construction: dependency identity never drifts under
// concurrent read pressure, every dispatch's progress is delivered exactly
// once with no cross-request leakage or duplication, and outcomes are
// deterministic across concurrencyRepeatRounds repeats. Synchronization is
// limited to sync.WaitGroup/sync.Mutex/atomic (no sleeps), so the test is
// deterministic and safe to run under the Go race detector.
func TestConcurrentRepeatedExecutionsRetainConstructionInjectedDependencies(t *testing.T) {
	t.Parallel()

	providerRunner := taggedCommandRunner{tag: "concurrent-provider"}
	scriptRunner := taggedCommandRunner{tag: "concurrent-script"}

	fake := newServiceAgentProvidersFake()
	fake.result.Diagnostics.Progress = []providers.ExecuteProgress{
		{Phase: "planning", Detail: "first"},
		{Phase: "responding", Detail: "second"},
	}

	recorder := newRecordingDispatchPublisher()
	service := newTestServiceWithDependenciesAndProviders(
		t, providerRunner, scriptRunner, workers.ProgressPublisher(recorder.publish), fake,
	)
	runtimeCfg := agentInvocationRuntimeConfig()

	wantKinds := []string{
		workers.ProgressFragmentKind,
		workers.ProgressFragmentKind,
		workers.ProgressFragmentKind,
		workers.CompletedFragmentKind,
	}

	for round := 0; round < concurrencyRepeatRounds; round++ {
		var wg sync.WaitGroup
		var identityMismatches atomic.Int32
		for worker := 0; worker < concurrencyFanout; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()

				if service.ProviderCommandRunner() != workers.CommandRunner(providerRunner) {
					identityMismatches.Add(1)
				}
				if service.ScriptCommandRunner() != workers.CommandRunner(scriptRunner) {
					identityMismatches.Add(1)
				}

				executor, err := service.BuildModelInvocationExecutor(runtimeCfg, runtimeCfg.FactoryConfig(), "agent-worker")
				if err != nil {
					t.Errorf("round %d worker %d: BuildModelInvocationExecutor() error = %v", round, worker, err)
					return
				}
				dispatchID := fmt.Sprintf("dispatch-r%d-w%d", round, worker)
				result, err := executor.Execute(context.Background(), agentInvocationExecutionRequest(dispatchID))
				if err != nil {
					t.Errorf("round %d worker %d: Execute() error = %v", round, worker, err)
					return
				}
				if result.Outcome != workers.OutcomeAccepted || result.Output != "fixture output" {
					t.Errorf("round %d worker %d: Execute() result = %#v, want accepted fixture output", round, worker, result)
				}
			}(worker)
		}
		wg.Wait()

		if identityMismatches.Load() != 0 {
			t.Fatalf("round %d: %d concurrent reads observed a substituted or missing provider/script command runner", round, identityMismatches.Load())
		}

		snapshot := recorder.snapshot()
		for worker := 0; worker < concurrencyFanout; worker++ {
			dispatchID := fmt.Sprintf("dispatch-r%d-w%d", round, worker)
			fragments := snapshot[dispatchID]
			if len(fragments) != len(wantKinds) {
				t.Fatalf("round %d worker %d: published fragments = %#v, want %d entries (exactly once, no duplicate delivery)", round, worker, fragments, len(wantKinds))
			}
			for i, kind := range wantKinds {
				if fragments[i].Kind != kind || fragments[i].DispatchID != dispatchID {
					t.Fatalf("round %d worker %d: published[%d] = %#v, want kind %q for %s (no cross-request leakage)", round, worker, i, fragments[i], kind, dispatchID)
				}
			}
		}
	}

	if got, want := fake.calls.Load(), int32(concurrencyRepeatRounds*concurrencyFanout); got != want {
		t.Fatalf("Providers.Execute calls = %d, want exactly %d (every concurrent/repeated request reached the construction-injected runner exactly once)", got, want)
	}

	finalSnapshot := recorder.snapshot()
	if len(finalSnapshot) != concurrencyRepeatRounds*concurrencyFanout {
		t.Fatalf("distinct dispatches observed = %d, want %d (no leaked or merged dispatch identity)", len(finalSnapshot), concurrencyRepeatRounds*concurrencyFanout)
	}
}
