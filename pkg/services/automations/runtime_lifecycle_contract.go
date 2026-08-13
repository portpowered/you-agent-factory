package automations

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RuntimeLifecycle is the Automations-owned lifecycle capability for one
// activated Factory Runtime. The capability is deliberately separate from
// Service so ordinary reconciliation and status consumers cannot mutate the
// runtime registry.
type RuntimeLifecycle interface {
	ActivateRuntime(context.Context, RuntimeActivationRequest) (RuntimeActivationResult, error)
	DeactivateRuntime(context.Context, RuntimeDeactivationRequest) (RuntimeDeactivationResult, error)
}

// RuntimeStarter is an internal composition capability used by Factory
// Runtime to preserve the existing preseed-then-start ordering. Activation
// allocates detached source state; StartRuntime begins its effects.
type RuntimeStarter interface {
	StartRuntime(context.Context, string) error
}

// RuntimeActivationRequest carries the detached definition and the exact
// runtime-scoped effects needed by Automations to activate its sources.
type RuntimeActivationRequest struct {
	RuntimeID        string
	FactorySessionID string
	Snapshot         factorydefinitions.RuntimeSnapshot
	Inputs           RuntimeActivationInputs
}

// RuntimeActivationInputs contains replaceable effects for one runtime. The
// values are not retained as canonical Factory state; they only let the root
// connect source observations to the owning Work and input boundaries.
type RuntimeActivationInputs struct {
	Submitter       WorkRequestSubmitter
	StartSchedulers bool
	Filesystem      RuntimeFilesystemInputs
}

// RuntimeFilesystemInputs carries the detached watcher inputs selected by
// Factory Runtime. A nil Filesystem input set simply means this runtime has no
// filesystem watcher to activate.
type RuntimeFilesystemInputs struct {
	Files             FilesystemInputReader
	WalkDirectory     FilesystemDirectoryWalker
	WorkRequestIDs    work.RequestIDGenerator
	KnownWorkTypes    []string
	ValidStatesByType map[string]map[string]bool
}

// RuntimeActivationResult reports the accepted runtime identity and whether
// the request was an idempotent duplicate.
type RuntimeActivationResult struct {
	RuntimeID  string
	State      RuntimeLifecycleState
	Idempotent bool
}

// RuntimeDeactivationRequest identifies one runtime to stop and release.
type RuntimeDeactivationRequest struct {
	RuntimeID string
}

// RuntimeDeactivationResult reports the released runtime identity and whether
// it was already absent.
type RuntimeDeactivationResult struct {
	RuntimeID  string
	State      RuntimeLifecycleState
	Idempotent bool
}

// RuntimeLifecycleState is the detached lifecycle state of a runtime's source
// ownership. It intentionally does not expose scheduler implementation state.
type RuntimeLifecycleState string

const (
	RuntimeLifecycleActivated RuntimeLifecycleState = "activated"
	RuntimeLifecycleStopped   RuntimeLifecycleState = "stopped"
)
