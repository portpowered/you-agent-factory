package mockworker

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type (
	MockWorkerRunType                 = workers.MockWorkerRunType
	MockWorkerUnmatchedDispatchPolicy = workers.MockWorkerUnmatchedDispatchPolicy
	MockWorkersConfig                 = workers.MockWorkersConfig
	MockWorkerConfig                  = workers.MockWorkerConfig
	MockWorkInputSelector             = workers.MockWorkInputSelector
	MockWorkerScriptConfig            = workers.MockWorkerScriptConfig
	MockWorkerRejectConfig            = workers.MockWorkerRejectConfig
	MockWorkersConfigFileSystem       = workers.MockWorkersConfigFileSystem
	MockWorkersConfigLoader           = workers.MockWorkersConfigLoader
)

const (
	MockWorkerRunTypeAccept                      = workers.MockWorkerRunTypeAccept
	MockWorkerRunTypeScript                      = workers.MockWorkerRunTypeScript
	MockWorkerRunTypeReject                      = workers.MockWorkerRunTypeReject
	MockWorkerUnmatchedDispatchPolicyAccept      = workers.MockWorkerUnmatchedDispatchPolicyAccept
	MockWorkerUnmatchedDispatchPolicyPassthrough = workers.MockWorkerUnmatchedDispatchPolicyPassthrough
)

var (
	NewEmptyMockWorkersConfig  = workers.NewEmptyMockWorkersConfig
	NewMockWorkersConfigLoader = workers.NewMockWorkersConfigLoader
	ParseMockWorkersConfig     = workers.ParseMockWorkersConfig
)
