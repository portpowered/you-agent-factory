package testutil

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

// ProviderErrorSmokeLane declares one provider/model lane in a generated
// provider-error smoke fixture.
type ProviderErrorSmokeLane struct {
	WorkTypeID      string
	WorkerName      string
	WorkstationName string
	Provider        modelprovider.Provider
	Model           string
	PromptBody      string
}

// ProviderErrorSmokePauseIsolationHarness owns a generated two-lane fixture for
// proving that a throttled provider/model lane pauses without blocking an
// unrelated lane.
type ProviderErrorSmokePauseIsolationHarness struct {
	Dir            string
	ThrottledLane  ProviderErrorSmokeLane
	UnaffectedLane ProviderErrorSmokeLane

	providerRunner *ProviderCommandRunner
}

// NewProviderErrorSmokePauseIsolationHarness builds a two-lane smoke fixture
// without requiring committed factory JSON or hand-written lane AGENTS files.
func NewProviderErrorSmokePauseIsolationHarness(
	t *testing.T,
	throttledLane ProviderErrorSmokeLane,
	unaffectedLane ProviderErrorSmokeLane,
) *ProviderErrorSmokePauseIsolationHarness {
	t.Helper()

	normalizeProviderErrorSmokeLane(t, &throttledLane)
	normalizeProviderErrorSmokeLane(t, &unaffectedLane)

	dir := ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			providerErrorSmokeLaneWorkType(throttledLane),
			providerErrorSmokeLaneWorkType(unaffectedLane),
		},
		Workers: []interfaces.FactoryWorkerConfig{
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

	return &ProviderErrorSmokePauseIsolationHarness{
		Dir:            dir,
		ThrottledLane:  throttledLane,
		UnaffectedLane: unaffectedLane,
		providerRunner: providerRunner,
	}
}

// QueueProviderResults appends ordered provider subprocess outcomes to the
// shared script-wrap runner for both pause-isolation lanes.
func (h *ProviderErrorSmokePauseIsolationHarness) QueueProviderResults(results ...platformprocess.CommandResult) {
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
		OnFailure: []interfaces.IOConfig{{
			WorkTypeName: lane.WorkTypeID,
			StateName:    "failed",
		}},
	}
}

func writeProviderErrorSmokeLaneConfig(t *testing.T, dir string, lane ProviderErrorSmokeLane) {
	t.Helper()

	writeProviderErrorSmokeWorkerConfig(t, dir, lane.WorkerName, lane.Provider, lane.Model, lane.PromptBody)
	writeProviderErrorSmokeWorkstationConfig(t, dir, lane.WorkstationName, lane.PromptBody)
}
