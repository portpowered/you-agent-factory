package testutil

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/workers"
)

const defaultProviderErrorSmokeWorktreeTemplate = "{{ (index .Inputs 0).Name }}"

// ProviderErrorSmokeLane declares one provider/model lane in a generated
// provider-error smoke fixture.
type ProviderErrorSmokeLane struct {
	WorkTypeID      string
	WorkerName      string
	WorkstationName string
	Provider        workers.ModelProvider
	Model           string
	PromptBody      string
}

type providerErrorSmokePauseIsolationHarnessConfig struct {
	serviceOptions []ServiceTestHarnessOption
}

// ProviderErrorSmokePauseIsolationHarnessOption customizes the generated
// two-lane pause-isolation smoke fixture.
type ProviderErrorSmokePauseIsolationHarnessOption func(*providerErrorSmokePauseIsolationHarnessConfig)

// WithProviderErrorSmokePauseIsolationServiceOptions forwards service harness
// options to the generated pause-isolation fixture.
func WithProviderErrorSmokePauseIsolationServiceOptions(
	opts ...ServiceTestHarnessOption,
) ProviderErrorSmokePauseIsolationHarnessOption {
	return func(cfg *providerErrorSmokePauseIsolationHarnessConfig) {
		cfg.serviceOptions = append(cfg.serviceOptions, opts...)
	}
}

// ProviderErrorSmokePauseIsolationHarness owns a generated two-lane fixture for
// proving that a throttled provider/model lane pauses without blocking an
// unrelated lane.
type ProviderErrorSmokePauseIsolationHarness struct {
	Dir            string
	ThrottledLane  ProviderErrorSmokeLane
	UnaffectedLane ProviderErrorSmokeLane

	providerRunner *ProviderCommandRunner
	serviceOptions []ServiceTestHarnessOption
}

// NewProviderErrorSmokePauseIsolationHarness builds a two-lane smoke fixture
// without requiring committed factory JSON or hand-written lane AGENTS files.
func NewProviderErrorSmokePauseIsolationHarness(
	t *testing.T,
	throttledLane ProviderErrorSmokeLane,
	unaffectedLane ProviderErrorSmokeLane,
	opts ...ProviderErrorSmokePauseIsolationHarnessOption,
) *ProviderErrorSmokePauseIsolationHarness {
	t.Helper()

	cfg := &providerErrorSmokePauseIsolationHarnessConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	normalizeProviderErrorSmokeLane(t, &throttledLane)
	normalizeProviderErrorSmokeLane(t, &unaffectedLane)

	dir := ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			providerErrorSmokeLaneWorkType(throttledLane),
			providerErrorSmokeLaneWorkType(unaffectedLane),
		},
		Workers: []interfaces.WorkerConfig{
			{Name: throttledLane.WorkerName},
			{Name: unaffectedLane.WorkerName},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			providerErrorSmokeLaneWorkstation(throttledLane),
			providerErrorSmokeLaneWorkstation(unaffectedLane),
		},
	})
	writeProviderErrorSmokeLaneConfig(t, dir, throttledLane)
	writeProviderErrorSmokeLaneConfig(t, dir, unaffectedLane)

	providerRunner := NewProviderCommandRunner()
	serviceOptions := append([]ServiceTestHarnessOption{
		WithProviderCommandRunner(providerRunner),
	}, cfg.serviceOptions...)

	return &ProviderErrorSmokePauseIsolationHarness{
		Dir:            dir,
		ThrottledLane:  throttledLane,
		UnaffectedLane: unaffectedLane,
		providerRunner: providerRunner,
		serviceOptions: serviceOptions,
	}
}

// BuildServiceHarness constructs the ServiceTestHarness for the generated
// pause-isolation fixture. Call this after seeding any startup work.
func (h *ProviderErrorSmokePauseIsolationHarness) BuildServiceHarness(t *testing.T) *ServiceTestHarness {
	t.Helper()
	return NewServiceTestHarness(t, h.Dir, h.serviceOptions...)
}

// BuildRunningServiceHarness constructs the generated pause-isolation service
// harness, starts the real async run loop, and registers cleanup for the test.
func (h *ProviderErrorSmokePauseIsolationHarness) BuildRunningServiceHarness(
	t *testing.T,
	timeout time.Duration,
) *ServiceTestHarness {
	t.Helper()
	return buildRunningProviderErrorSmokeServiceHarness(t, h.BuildServiceHarness(t), timeout)
}

// QueueProviderResults appends ordered provider subprocess outcomes to the
// shared script-wrap runner for both pause-isolation lanes.
func (h *ProviderErrorSmokePauseIsolationHarness) QueueProviderResults(results ...workers.CommandResult) {
	h.providerRunner.Queue(results...)
}

// ProviderRunner exposes the recorded provider subprocess seam for assertions.
func (h *ProviderErrorSmokePauseIsolationHarness) ProviderRunner() *ProviderCommandRunner {
	return h.providerRunner
}

// SeedWork writes a stable named submission into the generated fixture so
// startup preseed preserves deterministic work identity.
func (h *ProviderErrorSmokePauseIsolationHarness) SeedWork(t *testing.T, work ProviderErrorSmokeWork) {
	t.Helper()
	WriteSeedRequest(t, h.Dir, submitRequestFromProviderErrorSmokeWork(work))
}

// WaitForThrottleRequeue waits until the throttled lane requeues to init after
// exhausting bounded provider retries.
func (h *ProviderErrorSmokePauseIsolationHarness) WaitForThrottleRequeue(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()
	return WaitForProviderErrorThrottleRequeue(t, serviceHarness, work, timeout)
}

func normalizeProviderErrorSmokeLane(t *testing.T, lane *ProviderErrorSmokeLane) {
	t.Helper()

	if lane.WorkTypeID == "" {
		t.Fatal("normalizeProviderErrorSmokeLane: WorkTypeID is required")
	}
	if lane.WorkerName == "" {
		t.Fatal("normalizeProviderErrorSmokeLane: WorkerName is required")
	}
	if lane.WorkstationName == "" {
		t.Fatal("normalizeProviderErrorSmokeLane: WorkstationName is required")
	}
	if lane.Provider == "" {
		t.Fatal("normalizeProviderErrorSmokeLane: Provider is required")
	}
	if lane.Model == "" {
		t.Fatal("normalizeProviderErrorSmokeLane: Model is required")
	}
	if lane.PromptBody == "" {
		lane.PromptBody = "Process the " + lane.WorkTypeID + " task.\n"
	}
}

func providerErrorSmokeLaneWorkType(lane ProviderErrorSmokeLane) interfaces.WorkTypeConfig {
	return interfaces.WorkTypeConfig{
		Name: lane.WorkTypeID,
		States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		},
	}
}

func providerErrorSmokeLaneWorkstation(lane ProviderErrorSmokeLane) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           lane.WorkstationName,
		WorkerTypeName: lane.WorkerName,
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: lane.WorkTypeID,
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: lane.WorkTypeID,
			StateName:    "complete",
		}},
		OnFailure: &interfaces.IOConfig{
			WorkTypeName: lane.WorkTypeID,
			StateName:    "failed",
		},
		Worktree: defaultProviderErrorSmokeWorktreeTemplate,
	}
}

func writeProviderErrorSmokeLaneConfig(t *testing.T, dir string, lane ProviderErrorSmokeLane) {
	t.Helper()

	writeProviderErrorSmokeWorkerConfig(t, dir, lane.WorkerName, lane.Provider, lane.Model, lane.PromptBody)
	writeProviderErrorSmokeWorkstationConfig(t, dir, lane.WorkstationName, lane.PromptBody)
}
