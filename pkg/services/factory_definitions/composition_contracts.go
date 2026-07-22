package factorydefinitions

// RuntimeOpeningRequest contains only Factory Definition source-selection
// values used while opening a Factory Session runtime.
type RuntimeOpeningRequest struct {
	Directory        string
	ExecutionBaseDir string
}

// InitialFactorySnapshotFactory captures the portable Factory Definition
// snapshot recorded when a runtime is created.
type InitialFactorySnapshotFactory func(
	LoadedFactorySource,
) (*FactorySnapshot, error)
