// Package wire is the Workers service composition boundary.
//
// Wire performs construction only, returns the singular workers.Service root
// interface, and starts no lifecycle components. Parent-private runtime_assembly,
// workstations, and runners (agent/script/inference) owner wiring stays inside
// the owner service assembly path; peers depend on Service rather than owner
// internals or construction ports. Hosted runner ownership is not constructed.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"

	worktree "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/worktree"
)

var (
	NewWorktree             = worktree.New
	NewPlatformGitCommander = worktree.NewPlatformGitCommander
)

var NewFactoryDocsLoader = workstationswire.NewFactoryDocsLoader

var NewExecutor = invocation.NewExecutor
var NewLibraryHarnessAdapter = agentrun.NewLibraryHarnessAdapter
