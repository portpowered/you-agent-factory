package runtimehost

import (
	"context"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

type recordingWorkerSidecarOwner struct {
	calls int
	ctx   context.Context
	group *sync.WaitGroup
	input workersservice.RuntimeSidecarsInput
}

func (o *recordingWorkerSidecarOwner) StartSchedulerSidecarsForRuntime(
	ctx context.Context,
	group *sync.WaitGroup,
	input workersservice.RuntimeSidecarsInput,
) error {
	o.calls++
	o.ctx = ctx
	o.group = group
	o.input = input
	return nil
}

func (*recordingWorkerSidecarOwner) WorkflowIdentityForFactoryDir(string) string { return "" }

func (*recordingWorkerSidecarOwner) SubmitCronTick(
	context.Context,
	interfaces.RuntimeWorkstationLookup,
	string,
	workersservice.WorkRequestSubmitter,
	interfaces.FactoryWorkstationConfig,
	time.Time,
) error {
	return nil
}

func TestHostStartSchedulerSidecarsForRuntimeDelegatesCompleteSessionInput(t *testing.T) {
	owner := &recordingWorkerSidecarOwner{}
	host := &Host{
		workersScheduler: owner,
		policy:           hostCoordinatorPolicy{runtimeMode: interfaces.RuntimeModeService},
	}
	ctx := context.Background()
	group := &sync.WaitGroup{}
	factoryCfg := &interfaces.FactoryConfig{Workstations: []interfaces.FactoryWorkstationConfig{
		{Name: "script", Kind: interfaces.WorkstationKindPoller},
		{Name: "hosted", Kind: interfaces.WorkstationKindPoller},
		{Name: "cron", Kind: interfaces.WorkstationKindCron},
	}}
	runtimeCfg := &sidecarRuntimeConfig{factoryCfg: factoryCfg}
	submitter := workRequestSubmitter(func(context.Context, work.WorkRequest) error { return nil })

	err := host.startSchedulerSidecarsForRuntime(ctx, group, "/factory", factoryCfg, runtimeCfg, submitter)
	if err != nil {
		t.Fatalf("startSchedulerSidecarsForRuntime: %v", err)
	}
	if owner.calls != 1 {
		t.Fatalf("worker sidecar owner calls = %d, want 1", owner.calls)
	}
	if owner.ctx != ctx || owner.group != group {
		t.Fatal("worker sidecar owner did not receive the session context and wait group unchanged")
	}
	if owner.input.FactoryDir != "/factory" || owner.input.FactoryCfg != factoryCfg || owner.input.RuntimeCfg != runtimeCfg || owner.input.Submitter == nil {
		t.Fatalf("worker sidecar input = %#v, want complete active-session input", owner.input)
	}
}

func TestHostStartSchedulerSidecarsForRuntimeSkipsNonServiceRuntime(t *testing.T) {
	owner := &recordingWorkerSidecarOwner{}
	host := &Host{workersScheduler: owner, policy: hostCoordinatorPolicy{runtimeMode: interfaces.RuntimeModeBatch}}
	factoryCfg := &interfaces.FactoryConfig{}
	runtimeCfg := &sidecarRuntimeConfig{factoryCfg: factoryCfg}

	err := host.startSchedulerSidecarsForRuntime(
		context.Background(),
		&sync.WaitGroup{},
		"/factory",
		factoryCfg,
		runtimeCfg,
		func(context.Context, work.WorkRequest) error { return nil },
	)
	if err != nil {
		t.Fatalf("startSchedulerSidecarsForRuntime: %v", err)
	}
	if owner.calls != 0 {
		t.Fatalf("worker sidecar owner calls = %d, want 0", owner.calls)
	}
}

type sidecarRuntimeConfig struct {
	factoryCfg *interfaces.FactoryConfig
}

func (c *sidecarRuntimeConfig) FactoryConfig() *interfaces.FactoryConfig { return c.factoryCfg }
func (*sidecarRuntimeConfig) FactoryDir() string                         { return "/factory" }
func (*sidecarRuntimeConfig) RuntimeBaseDir() string                     { return "/factory" }
func (*sidecarRuntimeConfig) Worker(string) (*workerconfig.Config, bool) { return nil, false }
func (*sidecarRuntimeConfig) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}
