package providers

import (
	"io/fs"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

// CodexPromptFileSystem supplies the exact file effects for an attempt-owned
// instruction profile. CreateTemp must create exclusively with private file
// permissions, inheriting no broader access than the selected Codex home.
type CodexPromptFileSystem interface {
	platformfilesystem.TemporaryFileSystem
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	EvalSymlinks(string) (string, error)
}
