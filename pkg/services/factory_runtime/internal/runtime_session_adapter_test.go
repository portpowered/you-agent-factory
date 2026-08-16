package internal

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func TestRuntimeSessionAdaptersKeepHostedLifecycleBehindNeutralContracts(t *testing.T) {
	current := &adapterRuntimeRecord{directory: "current"}
	replacement := &adapterRuntimeRecord{directory: "replacement"}
	legacyRun := &adapterHostedRun{instance: current, done: make(chan struct{})}
	lifecycle := &adapterLegacyLifecycle{run: legacyRun}

	run, err := adaptRuntimeLifecycle(lifecycle).Start(context.Background(), current)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if run.RuntimeInstance() != current {
		t.Fatalf("RuntimeInstance() = %p, want current record %p", run.RuntimeInstance(), current)
	}
	if err := adaptRuntimeLifecycle(lifecycle).WaitForStart(context.Background(), run); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}
	adaptRuntimeLifecycle(lifecycle).StopSidecars(run)
	if err := adaptRuntimeLifecycle(lifecycle).PublishReplacement(context.Background(), run, replacement); err != nil {
		t.Fatalf("PublishReplacement() error = %v", err)
	}
	if err := adaptRuntimeLifecycle(lifecycle).Stop(run); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if lifecycle.waitCalls != 1 || lifecycle.stopSidecarCalls != 1 || lifecycle.publishCalls != 1 || lifecycle.stopCalls != 1 {
		t.Fatalf("legacy lifecycle calls = %#v, want one call for each operation", lifecycle)
	}

	sidecars := &adapterLegacySidecars{}
	neutralSidecars := adaptRuntimeSidecars(sidecars)
	if err := neutralSidecars.Preseed(context.Background(), current); err != nil {
		t.Fatalf("Preseed() error = %v", err)
	}
	if err := neutralSidecars.Start(context.Background(), run); err != nil {
		t.Fatalf("sidecar Start() error = %v", err)
	}
	neutralSidecars.Stop(run)
	if sidecars.preseedCalls != 1 || sidecars.startCalls != 1 || sidecars.stopCalls != 1 {
		t.Fatalf("legacy sidecar calls = %#v, want one call for each operation", sidecars)
	}

	builder := adaptRuntimeReplacementBuilder(adapterLegacyBuilder{record: replacement})
	built, err := builder.BuildReplacement(context.Background(), "folder", "factory", "session", "execution")
	if err != nil {
		t.Fatalf("BuildReplacement() error = %v", err)
	}
	if built != replacement {
		t.Fatalf("BuildReplacement() = %p, want replacement record %p", built, replacement)
	}
}

type adapterRuntimeRecord struct {
	directory string
}

func (record *adapterRuntimeRecord) RuntimeService() factoryruntime.Service { return nil }
func (record *adapterRuntimeRecord) Directory() string                      { return record.directory }
func (*adapterRuntimeRecord) FolderDirectory() string                       { return "folder" }
func (*adapterRuntimeRecord) BackendScope() string                          { return "scope" }
func (*adapterRuntimeRecord) StartTime() time.Time                          { return time.Time{} }
func (*adapterRuntimeRecord) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return nil
}
func (*adapterRuntimeRecord) CanonicalEvents() []interfaces.FactoryEvent { return nil }
func (*adapterRuntimeRecord) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {
}
func (*adapterRuntimeRecord) StreamGeneration() string   { return "generation" }
func (*adapterRuntimeRecord) RuntimeLogger() *zap.Logger { return zap.NewNop() }
func (*adapterRuntimeRecord) RuntimeMetrics() factoryruntime.MetricsEmitter {
	return nil
}
func (*adapterRuntimeRecord) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return factoryruntime.RuntimeLogDiagnostics{}
}
func (*adapterRuntimeRecord) RecordingLedger() recordings.Ledger { return nil }
func (*adapterRuntimeRecord) CloseArtifacts() error              { return nil }

type adapterHostedRun struct {
	instance factoryruntime.HostedInstance
	done     chan struct{}
	result   error
}

func (run *adapterHostedRun) RuntimeInstance() factoryruntime.HostedInstance { return run.instance }
func (run *adapterHostedRun) Completed() bool {
	select {
	case <-run.done:
		return true
	default:
		return false
	}
}
func (run *adapterHostedRun) Result() error { return run.result }
func (run *adapterHostedRun) Wait() error {
	<-run.done
	return run.result
}
func (run *adapterHostedRun) CancelRun() {
	if !run.Completed() {
		close(run.done)
	}
}
func (run *adapterHostedRun) RunDoneCh() <-chan struct{} { return run.done }

type adapterLegacyLifecycle struct {
	run              *adapterHostedRun
	waitCalls        int
	stopSidecarCalls int
	publishCalls     int
	stopCalls        int
}

func (lifecycle *adapterLegacyLifecycle) Start(context.Context, factoryruntime.HostedInstance) (factoryruntime.HostedHandle, error) {
	return lifecycle.run, nil
}
func (lifecycle *adapterLegacyLifecycle) WaitForStart(context.Context, factoryruntime.HostedHandle) error {
	lifecycle.waitCalls++
	return nil
}
func (lifecycle *adapterLegacyLifecycle) Stop(factoryruntime.HostedHandle) error {
	lifecycle.stopCalls++
	lifecycle.run.CancelRun()
	return nil
}
func (lifecycle *adapterLegacyLifecycle) StopSidecars(factoryruntime.HostedHandle) {
	lifecycle.stopSidecarCalls++
}
func (lifecycle *adapterLegacyLifecycle) PublishReplacement(context.Context, factoryruntime.HostedHandle, factoryruntime.HostedInstance) error {
	lifecycle.publishCalls++
	return nil
}

type adapterLegacySidecars struct {
	preseedCalls int
	startCalls   int
	stopCalls    int
}

func (sidecars *adapterLegacySidecars) Preseed(context.Context, factoryruntime.HostedInstance) error {
	sidecars.preseedCalls++
	return nil
}
func (sidecars *adapterLegacySidecars) Start(context.Context, factoryruntime.HostedHandle) error {
	sidecars.startCalls++
	return nil
}
func (sidecars *adapterLegacySidecars) Stop(factoryruntime.HostedHandle) {
	sidecars.stopCalls++
}

type adapterLegacyBuilder struct {
	record *adapterRuntimeRecord
}

func (builder adapterLegacyBuilder) BuildReplacement(context.Context, string, string, string, string) (factoryruntime.HostedInstance, error) {
	return builder.record, nil
}
