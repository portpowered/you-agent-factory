package workers

import mockworkers "github.com/portpowered/infinite-you/pkg/services/workers/internal/interface"

// MockWorkerRunType identifies the deterministic behavior a mock worker entry applies.
type MockWorkerRunType = mockworkers.MockWorkerRunType

const (
	MockWorkerRunTypeAccept MockWorkerRunType = mockworkers.MockWorkerRunTypeAccept
	MockWorkerRunTypeScript MockWorkerRunType = mockworkers.MockWorkerRunTypeScript
	MockWorkerRunTypeReject MockWorkerRunType = mockworkers.MockWorkerRunTypeReject
)

// MockWorkerUnmatchedDispatchPolicy controls how mock-worker mode handles unmatched dispatches.
type MockWorkerUnmatchedDispatchPolicy = mockworkers.MockWorkerUnmatchedDispatchPolicy

const (
	MockWorkerUnmatchedDispatchPolicyAccept      MockWorkerUnmatchedDispatchPolicy = mockworkers.MockWorkerUnmatchedDispatchPolicyAccept
	MockWorkerUnmatchedDispatchPolicyPassthrough MockWorkerUnmatchedDispatchPolicy = mockworkers.MockWorkerUnmatchedDispatchPolicyPassthrough
)

// MockWorkersConfig is the JSON contract for agent-factory mock-worker runs.
type MockWorkersConfig = mockworkers.MockWorkersConfig

// MockWorkerConfig selects a worker dispatch and declares deterministic behavior.
type MockWorkerConfig = mockworkers.MockWorkerConfig

// MockWorkInputSelector narrows a mock worker match by consumed work input.
type MockWorkInputSelector = mockworkers.MockWorkInputSelector

// MockWorkerScriptConfig declares the command a script mock executes.
type MockWorkerScriptConfig = mockworkers.MockWorkerScriptConfig

// MockWorkerRejectConfig declares observable output for a rejected mock result.
type MockWorkerRejectConfig = mockworkers.MockWorkerRejectConfig

// MockWorkersConfigFileSystem is the filesystem effect needed to load mock-worker configuration.
type MockWorkersConfigFileSystem = mockworkers.MockWorkersConfigFileSystem

// MockWorkersConfigLoader reads and validates a customer-selected mock-worker configuration.
type MockWorkersConfigLoader = mockworkers.MockWorkersConfigLoader

var (
	NewEmptyMockWorkersConfig  = mockworkers.NewEmptyMockWorkersConfig
	NewMockWorkersConfigLoader = mockworkers.NewMockWorkersConfigLoader
	ParseMockWorkersConfig     = mockworkers.ParseMockWorkersConfig
)
