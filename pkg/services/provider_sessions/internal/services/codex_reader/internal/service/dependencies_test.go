package service

import (
	"path/filepath"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
)

var (
	testFiles           platformfilesystem.Local
	testWalkDirectory   = providersessionsinternal.CodexWalkDirectory(filepath.WalkDir)
	testResolveSymlinks = providersessionsinternal.CodexResolveSymlinks(filepath.EvalSymlinks)
)
