package factorydefinitions

// ReplayRuntimeConfig is the Factory Definition lookup reconstructed from a
// detached recording snapshot. Recordings consumes this contract without
// depending on Config's concrete hydrated representation.
type ReplayRuntimeConfig interface {
	RuntimeConfigLookup
	WorkstationByID(string) (*FactoryWorkstationConfig, bool)
}

// ReplayRuntimeConfigDecoder reconstructs runtime definition lookups from a
// recorded Factory Definition snapshot.
type ReplayRuntimeConfigDecoder func(*FactorySnapshot) (ReplayRuntimeConfig, error)

// FactorySnapshotJSONDecoder validates and captures one serialized Factory
// Definition snapshot at an external persistence boundary.
type FactorySnapshotJSONDecoder func([]byte) (*FactorySnapshot, error)

// FactorySnapshotDirectoryLoader loads and captures the effective Factory
// Definition rooted at one directory.
type FactorySnapshotDirectoryLoader func(string) (*FactorySnapshot, error)
