// Package worktree is a transitional compile shim that re-exports worktree
// preparation helpers from the private workstations destination. Peers should
// construct through workers/wire; baseline deletion of this path is owned by
// DEL-WRK.
package worktree

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/worktree"
)

type (
	Service                      = private.Service
	GitCommander                 = private.GitCommander
	PlatformGitCommander         = private.PlatformGitCommander
	PrepareFactoryGitWorktreeResult = private.PrepareFactoryGitWorktreeResult
)

var (
	New                              = private.New
	NewPlatformGitCommander          = private.NewPlatformGitCommander
	PrepareFactoryGitWorktree        = private.PrepareFactoryGitWorktree
	FailedWorkResultFromPreparation  = private.FailedWorkResultFromPreparation
	ShouldPrepareFactoryWorktreeForCodex = private.ShouldPrepareFactoryWorktreeForCodex
	ResolveFactoryWorktreeParent     = private.ResolveFactoryWorktreeParent
	ResolveFactoryWorktreeCheckoutPath = private.ResolveFactoryWorktreeCheckoutPath
)
