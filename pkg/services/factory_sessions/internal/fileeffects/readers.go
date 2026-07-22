// Package fileeffects owns the exact host-file read capabilities consumed by
// Factory Session implementations. It is dependency-free so implementation
// subpackages and the process edge aggregator can share the same contracts.
package fileeffects

// ContractFixtureReader loads the explicitly selected deterministic Factory
// Session fixture catalog.
type ContractFixtureReader func(string) ([]byte, error)

func (read ContractFixtureReader) ReadFile(path string) ([]byte, error) { return read(path) }

// InvocationInputReader loads customer-provided files referenced by invocation
// arguments during interpolation validation.
type InvocationInputReader func(string) ([]byte, error)

func (read InvocationInputReader) ReadFile(path string) ([]byte, error) { return read(path) }

// ReplayRecordingReader inspects a customer-selected portable Factory Session
// recording before runtime construction.
type ReplayRecordingReader func(string) ([]byte, error)

func (read ReplayRecordingReader) ReadFile(path string) ([]byte, error) { return read(path) }

// InitialWorkReader loads the customer-selected initial Work request for a
// Factory Session runtime.
type InitialWorkReader func(string) ([]byte, error)

func (read InitialWorkReader) ReadFile(path string) ([]byte, error) { return read(path) }
