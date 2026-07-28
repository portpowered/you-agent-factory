// Package mockworker is a transitional compile shim that re-exports mock-worker
// test helpers from the private runners destination. Peers should construct
// through workers/wire; baseline deletion of this path is owned by DEL-WRK.
package mockworker

import (
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

type (
	MockWorkerRunType                 = runnermockworker.MockWorkerRunType
	MockWorkerUnmatchedDispatchPolicy = runnermockworker.MockWorkerUnmatchedDispatchPolicy
	MockWorkersConfig                 = runnermockworker.MockWorkersConfig
	MockWorkerConfig                  = runnermockworker.MockWorkerConfig
	MockWorkInputSelector             = runnermockworker.MockWorkInputSelector
	MockWorkerScriptConfig            = runnermockworker.MockWorkerScriptConfig
	MockWorkerRejectConfig            = runnermockworker.MockWorkerRejectConfig
	MockWorkersConfigFileSystem       = runnermockworker.MockWorkersConfigFileSystem
	MockWorkersConfigLoader           = runnermockworker.MockWorkersConfigLoader
	MockWorkerCommandRunner           = runnermockworker.MockWorkerCommandRunner
)

const (
	MockWorkerRunTypeAccept                      = runnermockworker.MockWorkerRunTypeAccept
	MockWorkerRunTypeScript                      = runnermockworker.MockWorkerRunTypeScript
	MockWorkerRunTypeReject                      = runnermockworker.MockWorkerRunTypeReject
	MockWorkerUnmatchedDispatchPolicyAccept      = runnermockworker.MockWorkerUnmatchedDispatchPolicyAccept
	MockWorkerUnmatchedDispatchPolicyPassthrough = runnermockworker.MockWorkerUnmatchedDispatchPolicyPassthrough
)

var (
	NewEmptyMockWorkersConfig  = runnermockworker.NewEmptyMockWorkersConfig
	NewMockWorkersConfigLoader = runnermockworker.NewMockWorkersConfigLoader
	ParseMockWorkersConfig     = runnermockworker.ParseMockWorkersConfig
)
