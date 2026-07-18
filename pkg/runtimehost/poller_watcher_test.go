package runtimehost

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
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

func TestNewLiveSessionStateBuildsOptionalRuntimeHandle(t *testing.T) {
	t.Parallel()

	spec := &runtimebuild.SessionBuildSpec{}
	empty := NewLiveSessionState(nil, spec)
	if empty == nil || empty.bundle != nil || empty.handle != nil || empty.spec != spec {
		t.Fatalf("empty live session state = %#v, want detached spec without handle", empty)
	}
	bundle := &factoryRuntimeBundle{}
	bound := NewLiveSessionState(bundle, spec)
	if bound.bundle != bundle || bound.handle == nil || bound.handle.Bundle != bundle || bound.spec != spec {
		t.Fatalf("bound live session state = %#v, want runtime handle", bound)
	}
	policy := CoordinatorPolicyFromConfig(nil)
	if policy.FactoryDir() != "" || policy.MockWorkersConfig() != nil {
		t.Fatalf("nil coordinator policy = %#v, want zero value", policy)
	}
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

func TestApplicationRuntimeWaitForRuntimeReportsAPIServerExit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiErr    error
		wantError string
	}{
		{name: "failure", apiErr: errors.New("bind: address already in use"), wantError: "API server failed: bind: address already in use"},
		{name: "unexpected clean exit", wantError: "API server stopped unexpectedly"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			apiExit := make(chan error, 1)
			apiExit <- test.apiErr
			close(apiExit)
			host := &Host{apiServerExit: apiExit}
			host.setRunState(context.Background(), "test-session", &liveRuntimeHandle{RunDone: make(chan struct{})})
			runtime := &ApplicationRuntime{host: host}

			err := runtime.waitForRuntime(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("waitForRuntime() error = %v, want %q", err, test.wantError)
			}
			if test.apiErr != nil && !errors.Is(err, test.apiErr) {
				t.Fatalf("waitForRuntime() error = %v, want wrapped API error", err)
			}
		})
	}
}
