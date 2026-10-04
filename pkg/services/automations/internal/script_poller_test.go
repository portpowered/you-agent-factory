package internal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

type recordingSubmitter struct {
	mu             sync.Mutex
	calls          int
	submissions    []work.WorkRequest
	submitOverride func(context.Context, work.WorkRequest) error
}

func (r *recordingSubmitter) submit(ctx context.Context, request work.WorkRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.submissions = append(r.submissions, request)
	if r.submitOverride != nil {
		return r.submitOverride(ctx, request)
	}
	return nil
}

func (r *recordingSubmitter) snapshot() (int, []work.WorkRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]work.WorkRequest(nil), r.submissions...)
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

type scriptPollerRuntimeConfigOptions struct {
	poller       interfaces.FactoryWorkstationConfig
	pollerWorker *interfaces.FactoryWorkerConfig
}

func newScriptPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	options scriptPollerRuntimeConfigOptions,
) interfaces.MutableLoadedFactorySource {
	t.Helper()

	poller := options.poller
	if poller.Name == "" {
		poller = newCanonicalScriptPollerWorkstation()
	}
	pollerWorker := options.pollerWorker
	if pollerWorker == nil {
		pollerWorker = newCanonicalScriptPollerWorker()
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.FactoryWorkerConfig{{Name: pollerWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*interfaces.FactoryWorkerConfig{
		pollerWorker.Name: pollerWorker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers:      workerConfigs,
			Workstations: workstationConfigs,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

type pollerRunOutcome struct {
	result        platformprocess.CommandResult
	err           error
	waitForCancel bool
}

type pollerSequenceCommandRunner struct {
	mu       sync.Mutex
	calls    int
	reqs     []platformprocess.CommandRequest
	outcomes []pollerRunOutcome
}

func (r *pollerSequenceCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.reqs = append(r.reqs, req)
	index := r.calls - 1
	var outcome pollerRunOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}
	r.mu.Unlock()

	if outcome.waitForCancel {
		<-ctx.Done()
		return outcome.result, ctx.Err()
	}
	return outcome.result, outcome.err
}

func (r *pollerSequenceCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitForPollerRunnerCalls(t *testing.T, runner *pollerSequenceCommandRunner, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runner.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d poller runner call(s); got %d", want, runner.callCount())
}
