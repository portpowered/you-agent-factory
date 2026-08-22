package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

// RuntimeOpeningRequest contains Factory Definition source-selection values
// and optional normalized invocation values used while opening a one-shot
// Factory Session runtime.
type RuntimeOpeningRequest struct {
	Directory        string
	SourcePath       string
	ExecutionBaseDir string
	// InvocationArguments carries already-normalized values for one-shot
	// invocation opening. Nil keeps long-lived openings on their authored
	// definition; a non-nil empty set marks a one-shot compatibility opening.
	InvocationArguments *work.InvocationArguments
}

// InitialFactorySnapshotFactory captures the portable Factory Definition
// snapshot recorded when a runtime is created.
type InitialFactorySnapshotFactory func(
	LoadedFactorySource,
) (*FactorySnapshot, error)
