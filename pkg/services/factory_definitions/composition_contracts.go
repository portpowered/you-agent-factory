package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

// RuntimeOpeningRequest contains only Factory Definition source-selection
// values used while opening a Factory Session runtime.
type RuntimeOpeningRequest struct {
	Directory        string
	SourcePath       string
	ExecutionBaseDir string
	// InvocationArguments carries already-normalized values for one-shot
	// invocation opening. Nil keeps long-lived and compatibility openings on
	// their authored definition until a later invocation supplies input.
	InvocationArguments *work.InvocationArguments
}

// InitialFactorySnapshotFactory captures the portable Factory Definition
// snapshot recorded when a runtime is created.
type InitialFactorySnapshotFactory func(
	LoadedFactorySource,
) (*FactorySnapshot, error)
