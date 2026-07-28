package workers

import "github.com/portpowered/infinite-you/pkg/services/workers/agypty"

// PTYAllocator opens a platform PTY for one supervised child process.
// Factory Sessions runtime-opening and execution-opening call sites name this
// Workers root contract instead of importing workers/agypty.
type PTYAllocator = agypty.PTYAllocator

// MockPTYAllocator is the hermetic PTY allocator seam Sessions boundary tests
// inject without importing workers/agypty.
type MockPTYAllocator = agypty.MockAllocator

// ProviderRegistry names the validated provider manifest-to-integration catalog
// join Sessions conductor invocation factories receive at runtime. The
// concrete registry is constructed within Workers; Sessions names only this
// root contract instead of importing workers/provider/registry.
type ProviderRegistry interface {
	UsesNativeRunner(identity string) bool
	RunnerMetadata(identity string) (RunnerMetadata, error)
	ResolveRunnerSelection(
		workstationRunner string,
		factoryRunner string,
		workerModelProvider string,
	) (ResolvedRunnerSelection, error)
}
