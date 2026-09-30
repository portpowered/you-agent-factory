package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

// RuntimeSelection contains Factory Definition source and invocation values
// selected for one Factory Session.
type RuntimeSelection struct {
	Directory        string
	SourcePath       string
	ExecutionBaseDir string
	// InvocationArguments carries already-normalized one-shot values. Nil
	// keeps a live session on its authored definition; a non-nil empty set
	// selects the one-shot invocation path.
	InvocationArguments *work.InvocationArguments
}

// InitialFactorySnapshotFactory captures the portable Factory Definition
// snapshot recorded when a runtime is created.
type InitialFactorySnapshotFactory func(
	LoadedFactorySource,
) (*FactorySnapshot, error)
