package codex

import (
	"path/filepath"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

var (
	testFiles           platformfilesystem.Local
	testWalkDirectory   = providersessions.CodexWalkDirectory(filepath.WalkDir)
	testResolveSymlinks = providersessions.CodexResolveSymlinks(filepath.EvalSymlinks)
)
