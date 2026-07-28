package factory

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

// HostedLedger is the Recordings capability required by runtime hosting.
// Hosting observes canonical history and records replacement boundaries without
// depending on the concrete recordings event-history implementation.
type HostedLedger interface {
	recordings.Ledger
	RecordFactoryChange(int, interfaces.FactoryChangeEventPayload, time.Time)
}

// HostedInstance is the public capability view of one built Factory Runtime.
// Consumers operate the runtime through this contract rather than inspecting
// the host's construction bundle.
type HostedInstance interface {
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

// HostedHandle is the public lifecycle view of a running Factory Runtime.
// Sidecar coordination and artifact ownership remain with the runtime host.
type HostedHandle interface {
	RuntimeInstance() HostedInstance
	Completed() bool
	Result() error
	Wait() error
	CancelRun()
	RunDoneCh() <-chan struct{}
}

// Lifecycle starts, observes, and stops hosted runtime instances. The
// initializer selects when lifecycle operations occur; implementations own
// how runtime loops, sidecars, and artifacts are managed.
type Lifecycle interface {
	Start(context.Context, HostedInstance) (HostedHandle, error)
	WaitForStart(context.Context, HostedHandle) error
	Stop(HostedHandle) error
	StopSidecars(HostedHandle)
	PublishReplacement(context.Context, HostedHandle, HostedInstance) error
}

// Sidecars owns runtime-scoped listeners, metrics observers, and automation
// without exposing the concrete host implementation.
type Sidecars interface {
	Preseed(context.Context, HostedInstance) error
	Start(context.Context, HostedHandle) error
	Stop(HostedHandle)
}

// ReplacementBuilder constructs a replacement runtime without exposing the
// concrete runtime-build service to Factory Sessions.
type ReplacementBuilder interface {
	BuildReplacement(context.Context, string, string, string, string) (HostedInstance, error)
}

// RuntimeModeOrDefault normalizes an omitted process mode to batch execution.
func RuntimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}

