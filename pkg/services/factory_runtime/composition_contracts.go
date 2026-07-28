package factory

import (
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Root contract typed errors for orchestration-neutral Runtime control and
// observation. Peers branch with errors.Is; nested IMP-RUN implementations map
// concrete lifecycle failures onto these sentinels.
var (
	// ErrNotRunning indicates the targeted Factory Runtime instance is not in a
	// running state for the requested control or observation operation.
	ErrNotRunning = errors.New("factory runtime is not running")

	// ErrNotFound indicates the targeted Factory Runtime instance or work scope
	// does not exist for the requested operation.
	ErrNotFound = errors.New("factory runtime target not found")

	// ErrAlreadyStopped indicates terminate/stop was requested against an
	// instance that has already stopped.
	ErrAlreadyStopped = errors.New("factory runtime is already stopped")

	// ErrInvalidLifecycleTransition indicates the requested control operation is
	// not valid from the instance's current lifecycle state.
	ErrInvalidLifecycleTransition = errors.New("factory runtime invalid lifecycle transition")

	// ErrInvalidObservationScope indicates the observation request asked for a
	// scope outside the published orchestration-neutral observation vocabulary.
	ErrInvalidObservationScope = errors.New("factory runtime invalid observation scope")

	// ErrDuplicateDispatchIntent indicates a plan/publish request conflicted with
	// an existing dispatch intent that is not eligible for idempotent re-delivery.
	ErrDuplicateDispatchIntent = dispatchplanning.ErrDuplicateDispatchIntent

	// ErrUnknownDispatchCorrelation indicates accept/retire targeted a correlation
	// that is not present in the Runtime dispatch outbox.
	ErrUnknownDispatchCorrelation = dispatchplanning.ErrUnknownDispatchCorrelation

	// ErrInvalidDispatchResultBoundary indicates the correlated worker result fell
	// outside the published result-boundary vocabulary peers may submit.
	ErrInvalidDispatchResultBoundary = dispatchplanning.ErrInvalidDispatchResultBoundary

	// ErrCheckpointNotFound indicates capture/load/restore targeted a checkpoint
	// identity that is not present in Runtime mutable checkpoint state.
	ErrCheckpointNotFound = errors.New("factory runtime checkpoint not found")

	// ErrCorruptCheckpoint indicates the checkpoint payload or envelope failed
	// integrity or shape checks without exposing strategy codec internals.
	ErrCorruptCheckpoint = errors.New("factory runtime checkpoint is corrupt")

	// ErrIncompatibleCheckpoint indicates the checkpoint schema or opaque payload
	// is incompatible with the Runtime restore surface.
	ErrIncompatibleCheckpoint = errors.New("factory runtime checkpoint is incompatible")

	// ErrCapabilityUnavailable indicates the root contract is published but its
	// canonical runtime implementation belongs to a later implementation cut.
	// Callers must not interpret this error as a successful no-op.
	ErrCapabilityUnavailable = errors.New("factory runtime capability is unavailable")
)

type WorkersRuntimeExecutorsFactory func(
	workers.RuntimeService,
	factorydefinitions.RuntimeConfigLookup,
	*factorydefinitions.FactoryConfig,
	string,
	*workers.Context,
	logging.Logger,
	bool,
	*bool,
	workers.Provider,
	workers.ProgressPublisher,
	workers.ScriptEventRecorder,
	workers.InferenceEventRecorder,
	workers.ModelEventRecorder,
	workers.AgentRunEventRecorder,
	func() time.Time,
) (map[string]workers.WorkerExecutor, error)

type WorkersMockCommandRunnerFactory func(
	*workers.MockWorkersConfig,
	factorydefinitions.RuntimeDefinitionLookup,
	workers.CommandRunner,
) workers.CommandRunner
