package factory

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

// RuntimeRecord is the neutral opening record exchanged between Factory
// Runtime and Factory Sessions. It describes the resources selected for one
// activation without publishing the concrete host or its run-loop state.
//
// This is an internal composition edge. Public Runtime callers retain the
// opaque RuntimeBinding and use Service for control, observation, Work, and
// result operations.
type RuntimeRecord interface {
	RuntimeService() Service
	Directory() string
	FolderDirectory() string
	BackendScope() string
	StartTime() time.Time
	LoadedRuntimeConfig() LoadedConfig
	CanonicalEvents() []interfaces.FactoryEvent
	AddEventTypeRecorder(func(interfaces.FactoryEventType))
	StreamGeneration() string
	RuntimeLogger() *zap.Logger
	RuntimeMetrics() MetricsEmitter
	RuntimeDiagnostics() RuntimeLogDiagnostics
	RecordingLedger() recordings.Ledger
	CloseArtifacts() error
}

// RuntimeRun is the neutral lifecycle view of one activated Runtime. The
// concrete run-loop handle remains private to Factory Runtime.
type RuntimeRun interface {
	RuntimeInstance() RuntimeRecord
	Completed() bool
	Result() error
	Wait() error
	CancelRun()
	RunDoneCh() <-chan struct{}
}

// RuntimeLifecycle owns start, readiness, stop, and replacement publication
// for one Runtime without exposing hosted implementation types to peers.
type RuntimeLifecycle interface {
	Start(context.Context, RuntimeRecord) (RuntimeRun, error)
	WaitForStart(context.Context, RuntimeRun) error
	Stop(RuntimeRun) error
	StopSidecars(RuntimeRun)
	PublishReplacement(context.Context, RuntimeRun, RuntimeRecord) error
}

// RuntimeSidecars owns the runtime-scoped listener and automation phase. Its
// implementation is selected by Factory Runtime and remains behind this
// neutral edge for Factory Sessions startup orchestration.
type RuntimeSidecars interface {
	Preseed(context.Context, RuntimeRecord) error
	Start(context.Context, RuntimeRun) error
	Stop(RuntimeRun)
}

// RuntimeReplacementBuilder constructs a replacement Runtime record without
// exposing the concrete runtime-build service to Factory Sessions.
type RuntimeReplacementBuilder interface {
	BuildReplacement(context.Context, string, string, string, string) (RuntimeRecord, error)
}
