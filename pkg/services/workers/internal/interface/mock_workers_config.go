// Package mockworkers exposes the Workers-owned mock-worker contract to the
// input inventory and schema checks without maintaining a second decoder.
package mockworkers

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type MockWorkerRunType = workers.MockWorkerRunType

const (
	MockWorkerRunTypeAccept = workers.MockWorkerRunTypeAccept
	MockWorkerRunTypeScript = workers.MockWorkerRunTypeScript
	MockWorkerRunTypeReject = workers.MockWorkerRunTypeReject
)

type MockWorkerUnmatchedDispatchPolicy = workers.MockWorkerUnmatchedDispatchPolicy

const (
	MockWorkerUnmatchedDispatchPolicyAccept      = workers.MockWorkerUnmatchedDispatchPolicyAccept
	MockWorkerUnmatchedDispatchPolicyPassthrough = workers.MockWorkerUnmatchedDispatchPolicyPassthrough
)

type MockWorkersConfig = workers.MockWorkersConfig
type MockWorkerConfig = workers.MockWorkerConfig
type MockWorkInputSelector = workers.MockWorkInputSelector
type MockWorkerScriptConfig = workers.MockWorkerScriptConfig
type MockWorkerRejectConfig = workers.MockWorkerRejectConfig
type MockWorkerUsageConfig = workers.MockWorkerUsageConfig
type MockWorkersConfigFileSystem = workers.MockWorkersConfigFileSystem
type MockWorkersConfigLoader = workers.MockWorkersConfigLoader
type MockWorkersConfigDecodeDiagnostics = workers.MockWorkersConfigDecodeDiagnostics
type MockWorkersConfigDiagnosticsLoader = workers.MockWorkersConfigDiagnosticsLoader

func NewMockWorkersConfigLoader(
	fileSystem MockWorkersConfigFileSystem,
) (MockWorkersConfigLoader, error) {
	return workers.NewMockWorkersConfigLoader(fileSystem)
}

func NewMockWorkersConfigDiagnosticsLoader(
	fileSystem MockWorkersConfigFileSystem,
) (MockWorkersConfigDiagnosticsLoader, error) {
	return (workers.MockWorkersConfigCodec{}).NewDiagnosticsLoader(fileSystem)
}

func ParseMockWorkersConfig(data []byte) (*MockWorkersConfig, error) {
	return workers.ParseMockWorkersConfig(data)
}

func ParseMockWorkersConfigWithDiagnostics(
	data []byte,
) (*MockWorkersConfig, MockWorkersConfigDecodeDiagnostics, error) {
	return (workers.MockWorkersConfigCodec{}).ParseWithDiagnostics(data)
}
