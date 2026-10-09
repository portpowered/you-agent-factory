package service

import (
	"context"
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// OpeningSessionIdentity reads only the effective identity of the requested session.
type OpeningSessionIdentity interface {
	ResolveOpeningSessionIdentity(context.Context, string) (string, error)
}

// RuntimeOpeningBinding owns final live observation and lifecycle binding.
// Construction is inert; each call receives only detached facts and acquired handles.
type RuntimeOpeningBinding struct {
	gateway          OpeningSessionIdentity
	recordings       recordings.Service
	providerOverride providers.Service
	commandRunner    platformprocess.CommandRunner
}

func NewRuntimeOpeningBinding(gateway OpeningSessionIdentity, recordingsService recordings.Service,
	providerOverride ProviderOverrideService, commandRunner ProviderCommandRunner) *RuntimeOpeningBinding {
	return &RuntimeOpeningBinding{gateway: gateway, recordings: recordingsService,
		providerOverride: providerOverride, commandRunner: commandRunner}
}

type RuntimeOpeningBindingRequest struct {
	Facts       roles.SessionOpeningFacts
	RecordPath  string
	MockWorkers *workers.MockWorkersConfig
}

// Final binding consumes observations from the acquired record, without its
// configuration, replacement builder, or lifecycle capabilities.
type openingRuntimeObservations = interface {
	RuntimeService() factoryruntime.Service
	StreamGeneration() string
	RuntimeLogger() *zap.Logger
	RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics
	RecordingLedger() recordings.Ledger
}

func (operation *RuntimeOpeningBinding) Bind(ctx context.Context, request RuntimeOpeningBindingRequest,
	state *runtimebinding.SessionState, selectedClock factoryruntime.Clock, startup openingRuntimeObservations,
	rootRuntime factoryruntime.Service, processRuntime roles.ProcessRuntime,
	execution durableexecution.Service,
	publishCurrentBoard func(context.Context) error, cleanup interface {
		Add(func() error)
		Close() error
	},
) error {
	if state == nil {
		return fmt.Errorf("construct runtime scope: completed session state is unavailable")
	}
	if recording, ok := startup.(recordings.RuntimeRecordingStartup); ok {
		recording.DeferRecordingPublication()
	}
	if rootRuntime == nil {
		return fmt.Errorf("construct runtime scope: session runtime does not implement Factory Runtime root Service")
	}
	admission, _ := rootRuntime.(factoryruntime.ResourceCapacityLeaseAdmission)
	binder, ok := execution.(workerScopeBinder)
	if !ok {
		return fmt.Errorf("bind worker scope for Factory Session %q: live child scope binder is required", strings.TrimSpace(request.Facts.FactorySessionID))
	}
	release, err := binder.BindWorkerScope(request.Facts.FactorySessionID, admission,
		request.Facts.RuntimeID, startup.StreamGeneration(), operation.providerOverride,
		request.MockWorkers, operation.commandRunner, runtimeProgressPublisher(startup), runtimeWorkerAttemptStarter(startup))
	if err != nil {
		return fmt.Errorf("bind worker scope for Factory Session %q: %w", strings.TrimSpace(request.Facts.FactorySessionID), err)
	}
	releaseLiveChange := bindLiveChangeScope(request.Facts.FactorySessionID, execution, rootRuntime)
	releaseDurability := bindDispatchDurability(request.Facts.FactorySessionID, execution, startup.RecordingLedger(), startup.StreamGeneration())
	cleanup.Add(func() error {
		release()
		releaseDurability()
		releaseLiveChange()
		return nil
	})
	operation.bindState(ctx, state, request.Facts, rootRuntime, startup, processRuntime)
	state.Clock = selectedClock
	state.OrderlyStop = operation.orderlyStop(request.Facts.RuntimeID, request.RecordPath, publishCurrentBoard)
	return nil
}

//nolint:contextcheck // Preserve detached callers' nil-context compatibility.
func (operation *RuntimeOpeningBinding) bindState(ctx context.Context, state *runtimebinding.SessionState,
	facts roles.SessionOpeningFacts, rootRuntime factoryruntime.Service, startup openingRuntimeObservations,
	processRuntime roles.ProcessRuntime,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	effectiveID := operation.resolveOpenedFactorySessionID(ctx, strings.TrimSpace(facts.FactorySessionID))
	state.Process = processRuntime
	state.ModelInvocation = modelinvocation.RuntimeModelInvocation{
		FactorySessionID: effectiveID, Scope: facts.ModelsScope,
		RuntimeID: facts.RuntimeID, GenerationID: startup.StreamGeneration(),
		FactoryDirectory: facts.Directory, WorkingDirectory: facts.Directory,
	}
	state.ModelsScope = facts.ModelsScope
	state.SetWorkerSessions(openedWorkerSessionsObservation(rootRuntime, startup, effectiveID))
	state.Logger = startup.RuntimeLogger()
	state.Diagnostics = startup.RuntimeDiagnostics()
}

func (operation *RuntimeOpeningBinding) orderlyStop(runtimeID, recordPath string,
	publishCurrentBoard func(context.Context) error) func(context.Context) error {
	return operation.afterFlush(operation.recordingFlush(runtimeID, recordPath), publishCurrentBoard)
}

func (*RuntimeOpeningBinding) afterFlush(flush func(context.Context) error,
	publishCurrentBoard func(context.Context) error) func(context.Context) error {
	if flush == nil {
		return nil
	}
	return func(ctx context.Context) error {
		if err := flush(ctx); err != nil {
			return err
		}
		return publishCurrentBoard(ctx)
	}
}

func (operation *RuntimeOpeningBinding) resolveOpenedFactorySessionID(
	ctx context.Context,
	factorySessionID string,
) string {
	effectiveID := factorySessionID
	if operation.gateway == nil || factorySessionID == "" {
		return effectiveID
	}
	resolvedID, err := operation.gateway.ResolveOpeningSessionIdentity(ctx, factorySessionID)
	if err != nil {
		return effectiveID
	}
	if resolved := strings.TrimSpace(resolvedID); resolved != "" {
		return resolved
	}
	return effectiveID
}

// recordingFlush adapts the already-composed Recordings root to the
// initializer lifecycle boundary. A missing record path means that this
// runtime has no live recording to flush, so the orderly-stop phase remains a
// no-op.
func (operation *RuntimeOpeningBinding) recordingFlush(
	recordingID string,
	recordPath string,
) func(context.Context) error {
	if operation.recordings == nil || strings.TrimSpace(recordingID) == "" || strings.TrimSpace(recordPath) == "" {
		return nil
	}
	return func(ctx context.Context) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if _, err := operation.recordings.FlushRecording(recordings.FlushRecordingRequest{
			RecordingID: recordings.RecordingID(recordingID),
		}); err != nil {
			return fmt.Errorf("flush live recording during orderly shutdown: %w", err)
		}
		return nil
	}
}

// PublishRuntime binds the acquired scoped owner after Runtime Root publishes its
// opaque activation capability. It never selects a current or replacement owner.
func (*RuntimeOpeningBinding) PublishRuntime(bindRuntime func(string, factoryruntime.RuntimeBinding) error,
	sessionID string, binding factoryruntime.RuntimeBinding,
) error {
	if strings.TrimSpace(sessionID) == "" || bindRuntime == nil {
		return nil
	}
	return bindRuntime(sessionID, binding)
}
