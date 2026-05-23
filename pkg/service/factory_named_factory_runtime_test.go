package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
)

func TestFactoryService_WaitToComplete_ReturnsClosedChannelWithoutRuntime(t *testing.T) {
	svc := &FactoryService{}

	select {
	case <-svc.WaitToComplete():
	default:
		t.Fatal("expected WaitToComplete without runtime to return a closed channel")
	}
}

func TestFactoryService_WaitToComplete_DelegatesToActiveRuntime(t *testing.T) {
	waitCh := make(chan struct{})
	svc := &FactoryService{
		factory: &aggregateSnapshotFactory{
			waitToComplete: waitCh,
		},
	}

	if got := svc.WaitToComplete(); got != waitCh {
		t.Fatalf("WaitToComplete channel = %p, want %p", got, waitCh)
	}
	close(waitCh)
}

func TestFactoryService_Pause_RequiresActiveRuntimeAndWrapsPauseErrors(t *testing.T) {
	svc := &FactoryService{}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("Pause without runtime error = %v, want runtime unavailable", err)
	}

	svc.factory = &aggregateSnapshotFactory{pauseErr: fmt.Errorf("pause failed")}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "pause factory: pause failed") {
		t.Fatalf("Pause wrapped error = %v, want wrapped pause failure", err)
	}

	svc.factory = &aggregateSnapshotFactory{}
	if err := svc.Pause(context.Background()); err != nil {
		t.Fatalf("Pause success error = %v", err)
	}
}

func TestFactoryService_CurrentRuntimeBundleAndDirComparisonHelpers(t *testing.T) {
	if bundle := (*FactoryService)(nil).currentRuntimeBundle(); bundle != nil {
		t.Fatalf("nil service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc := &FactoryService{}
	if bundle := svc.currentRuntimeBundle(); bundle != nil {
		t.Fatalf("empty service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc.cfg = &FactoryServiceConfig{Dir: "C:/factory"}
	svc.factory = &aggregateSnapshotFactory{}
	svc.runtimeCfg = &config.LoadedFactoryConfig{}
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected populated currentRuntimeBundle")
	}
	if bundle.dir != svc.cfg.Dir || bundle.factory != svc.factory || bundle.runtimeCfg != svc.runtimeCfg {
		t.Fatalf("currentRuntimeBundle = %#v, want service fields copied through", bundle)
	}

	if sameFactoryDir("", svc.cfg.Dir) {
		t.Fatal("sameFactoryDir should reject blank paths")
	}
	if !sameFactoryDir("C:/factory/./named", "C:/factory/named") {
		t.Fatal("sameFactoryDir should normalize equivalent paths")
	}
}

func TestLiveRuntimeHandle_CompletionHelpers(t *testing.T) {
	if !(*liveRuntimeHandle)(nil).completed() {
		t.Fatal("nil liveRuntimeHandle should report completed")
	}
	if err := (*liveRuntimeHandle)(nil).wait(); err != nil {
		t.Fatalf("nil liveRuntimeHandle wait error = %v, want nil", err)
	}

	handle := &liveRuntimeHandle{
		runDone: make(chan struct{}),
	}
	if handle.completed() {
		t.Fatal("open runDone should report incomplete")
	}
	handle.setRunResult(fmt.Errorf("run failed"))
	if !handle.completed() {
		t.Fatal("closed runDone should report completed")
	}
	if err := handle.wait(); err == nil || err.Error() != "run failed" {
		t.Fatalf("wait error = %v, want run failed", err)
	}
}
